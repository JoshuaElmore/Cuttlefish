# Cuttlefish

Filesystem intelligence platform. Indexes a host's filesystem into PostgreSQL, computes directory and ownership aggregates, and exposes the data through a REST API with a React browser UI.

## Components

| Component | Language | Role |
|---|---|---|
| `fs_common` | Rust (lib) | Shared config loading, DB connection, SHA-256 path hashing, scan_sessions bookkeeping |
| `fs_indexer` | Rust (bin) | Walks the filesystem, writes raw metadata to PostgreSQL |
| `fs_aggregator` | Rust (bin) | Reads the index, computes rolled-up stats |
| `fs_api` | Go | REST API over the DB; also serves the compiled UI as static files |
| `fs_ui` | TypeScript/React | Browser frontend (Vite 8) |

See the component-level CLAUDE.md files for implementation details:
- `fs_api/CLAUDE.md`
- `fs_ui/CLAUDE.md`

---

## Data flow

```
Filesystem
    │
    ▼
fs_indexer  ──────────────────────────────► filesystem_index
                                            identity_map
                                            scan_sessions
                                                │
                                                ▼
                                         fs_aggregator ──► dir_stats
                                                           user_stats
                                                               │
                                                               ▼
                                                           fs_api ◄── fs_ui (browser)
```

**Typical execution order: indexer → aggregator → api.** The indexer and aggregator are one-shot batch processes; the API server runs continuously.

---

## Database schema

All components share a single PostgreSQL database (`fs_index` by default). Tables are created by the binaries on first run via `CREATE TABLE IF NOT EXISTS`.

**Durability classes.** `filesystem_index`, `dir_stats` and `user_stats` are `UNLOGGED`: they are derived data that a rerun of the indexer (then the aggregator) rebuilds from the filesystem itself, so they are kept out of the WAL entirely — which also removes the full-page writes that dominate a bulk load's write volume after each checkpoint. Measured on 1M synthetic rows, a logged load wrote 230 MB of WAL and the unlogged equivalent wrote none. The costs are real and non-negotiable: **PostgreSQL truncates unlogged tables during crash recovery, and never replicates them to a standby.** An unclean shutdown therefore means rerunning `fs_indexer` and `fs_aggregator`, and these tables cannot be the basis of a physical-replication read replica.

`scan_sessions` and `identity_map` stay permanent. Run history is the one thing here that no rescan can regenerate, so it must survive a crash — never apply `ensure_unlogged` to it.

Existing databases are converted in place: `fs_common::ensure_unlogged` checks `pg_class.relpersistence` first and only issues the `ALTER TABLE ... SET UNLOGGED` when the table is still permanent, because that statement rewrites the table and all of its indexes. Both callers arrange to run it while the table is empty.

### `filesystem_index` — written by `fs_indexer`

| Column | Type | Notes |
|---|---|---|
| `path` | TEXT | Full absolute path, as UTF-8. Lossy when the real name isn't valid UTF-8 — see `path_raw` |
| `path_raw` | BYTEA | The exact bytes the kernel returned. `NULL` whenever they're identical to `path`, which is almost always |
| `path_hash` | BYTEA PK | SHA-256 of the **raw path bytes**, truncated to 16 bytes; used as PK and to link parent↔child |
| `parent_hash` | BYTEA | Same hash of the parent path; `NULL` for `/` |
| `size_bytes` | BIGINT | |
| `file_type` | INTEGER | 1=file, 2=directory, 3=symlink, 0=other |
| `permissions` | TEXT | Octal string, e.g. `"755"` |
| `uid` / `gid` | INTEGER | Numeric owner/group |
| `atime/mtime/ctime` | BIGINT | Unix epoch seconds |
| `metadata` | TEXT | Reserved, currently empty |

Secondary indexes (created by `fs_indexer` after a successful scan): `parent_hash` (drives `/api/list`), `uid`, `gid`, `size_bytes`, `mtime` (search filters), and a `pg_trgm` GIN index on `path` for substring/regex search (skipped with a warning if the extension can't be installed).

### `identity_map` — written by `fs_indexer`

Maps numeric UID/GID to username/groupname. Populated after each walk via the OS `users` crate.

| Column | Notes |
|---|---|
| `id` + `id_type` | Composite PK; `id_type` is `'uid'` or `'gid'` |
| `name` | Resolved username or groupname |

### `scan_sessions` — written by `fs_indexer` and `fs_aggregator`

One row per indexer or aggregator run, via the shared helpers in `fs_common` (`ensure_scan_sessions_table`, `record_scan_start`, `report_scan_progress`, `end_scan_session`).

| Column | Notes |
|---|---|
| `session_id` | TEXT PK; a fresh UUID per run |
| `scan_type` | `'indexer'` or `'aggregator'` |
| `status` | `'running'` → `'success'` or `'failed'` |
| `files_scanned` | Live progress counter, written directly by the running process (indexer: files indexed so far; aggregator: `filesystem_index` rows processed). Deliberately denormalized — derived by joining/counting `filesystem_index` doesn't work for aggregator runs and doesn't scale for indexer runs on a large index. |
| `started_at` / `ended_at` | `ended_at` is `NULL` while `status = 'running'` |

### `dir_stats` — written by `fs_aggregator`

Per-directory recursive aggregates. Keyed by `path_hash` to match `filesystem_index`.

| Column | Notes |
|---|---|
| `path_hash` | BYTEA PK |
| `path` | TEXT |
| `total_size_bytes` | Recursive sum of all descendant file sizes |
| `file_count` | Recursive count of all descendant entries |
| `mtime_first/last` | Min/max mtime across all descendants |
| `atime_first/last` | Min/max atime |
| `ctime_first/last` | Min/max ctime |

### `user_stats` — written by `fs_aggregator`

Per-UID/GID totals across the entire index. Composite PK `(id_type, id_value)`.

---

## Configuration

All components read `fs_config.yml` from the **project root** at startup. The file is gitignored — copy `fs_config_template.yml` and fill in real values:

```bash
install -m 0640 fs_config_template.yml fs_config.yml
```

Every component validates the config file's permissions before reading it and **exits** if it is group-writable or accessible to other users (`mode & 0027 != 0`). The file carries the database password and `auth.session_secret`, and anyone who can read the latter can forge a session cookie for `fs_api`. Group *read* is allowed so the intended `root:cuttlefish 0640` deployment works. Git does not track file modes, so a fresh clone needs the `install -m 0640` above rather than a plain `cp`.

```yaml
database:
  host: localhost
  user: postgres
  password: "..."
  dbname: fs_index
  sslmode: require        # never use "disable" in production

indexer:
  root_path: /            # required; scan root for fs_indexer
  threads: 8              # optional, defaults to 8

ui:
  title: "Cuttlefish"     # browser tab title; fs_api only, defaults to "Cuttlefish"

auth:
  # Generate with: openssl rand -hex 32
  # Must be ≥32 chars and must not be the template default value.
  session_secret: "..."

  local:                  # used when auth.oidc is absent
    username: "..."
    password: "..."

  # oidc:                 # when set, local auth is ignored
  #   issuer: "https://accounts.google.com"
  #   client_id: "..."
  #   client_secret: "..."
  #   redirect_url: "http://host/auth/callback"
```

`fs_api` validates the session secret at startup: it fatally rejects an empty value, the template default string, or a value shorter than 32 characters.

Note: `fs_common` (used by the Rust binaries) reads `database` and `indexer`; the `auth`, `server` and `ui` sections are only used by `fs_api`. `indexer` is read only by `fs_indexer` — `fs_aggregator` ignores it.

**Page title** — `ui.title` is substituted into `index.html` by `fs_api` at startup (`renderIndexHTML` in `main.go`), not baked in at UI build time, so retitling an instance needs a server restart but no `npm` rebuild. Both `/` and the SPA fallback route through `serveIndex`; the static file server must not handle `index.html` itself or it would serve the untouched on-disk copy. The value is HTML-escaped on the way in.

---

## Build & run

```bash
make all              # build everything: swagger → indexer → aggregator → ui → api
make build-indexer    # Rust release build of fs_indexer
make build-aggregator # Rust release build of fs_aggregator
make build-ui         # npm install + Vite build, copies output into fs_api/ui/
make build-api        # Go build of fs_api
make swagger-api      # regenerate Swagger docs then build api

make run-indexer      # build + run fs_indexer (scans /)
make run-aggregator   # build + run fs_aggregator
make run-api          # swagger + build-ui + build-api + start server

make clean            # remove all build artifacts and node_modules
```

Run binaries directly from the **project root** (so they find `fs_config.yml`):

```bash
sudo fs_indexer/target/release/fs_indexer
fs_aggregator/target/release/fs_aggregator
fs_api/fs_api
```

`fs_indexer` takes no CLI arguments — its scan root and thread count come from the `indexer` section of `fs_config.yml` (see Configuration above). The indexer requires read access to the scanned path (typically `sudo` for `/`).

---

## Key design decisions

**Path hashing** — `compute_hash(path)` in `fs_common` produces a SHA-256 digest of the path string, **truncated to 16 bytes** (`fs_common::PATH_HASH_LEN`). This is used as the primary key in `filesystem_index` and `dir_stats`, and as the parent-child link (`parent_hash`). Children of a directory are found by querying `WHERE parent_hash = $1` — no joins needed.

The truncation is a size decision. The primary key and `idx_fsindex_parent_hash` are the two largest indexes in the database and are almost entirely key bytes, and how much of them stays resident in shared_buffers governs the cost of the indexer's random-probe upsert. Measured over 1M representative paths, halving the key cut index size 29% and total relation size 25%. At 10^8 entries the birthday bound puts the chance of any collision at ~10^-23. **Do not shorten it further:** at 8 bytes the same 10^8 entries carry a ~0.03% collision chance, and a collision here silently merges two filesystem entries into one row.

**Hash the raw bytes, never a rendering.** A Unix filename is an arbitrary byte string, not text. `compute_hash_bytes` keys on the bytes the kernel returned, because hashing a UTF-8 rendering is lossy: `from_utf8_lossy` maps every invalid byte to U+FFFD, so `file_\xFF` and `file_\xFE` collapse to one string, one hash, and one row — a real file vanishing from the index with no error and a successful exit. That is both the worst failure mode an audit tool can have and an evasion vector, since a user can hide a file by giving it a name whose lossy form collides with a sibling's. It is the same class of problem as honouring `.ignore` files, and is refused for the same reason.

`path` keeps the (possibly lossy) rendering for display and for the trigram search index; `path_raw` carries the exact bytes and is `NULL` unless they differ. `fs_indexer` reports a count of non-UTF-8 names at the end of each scan rather than passing over them silently. `fs_aggregator` folds ancestors over `COALESCE(path_raw, convert_to(path,'UTF8'))` so a directory with a non-UTF-8 name gets a `dir_stats` row keyed the same way — previously its `to_str()` returning `None` also aborted the fold for every ancestor above it. `fs_api` returns `path_raw` (base64) on `FileInfo` when present; a client addresses such an entry by percent-encoding those bytes into `?path=`, since Go strings are byte sequences and `pathHash` already hashes whatever bytes arrive.

Because `compute_hash(s) == compute_hash_bytes(s.as_bytes())`, **no reindex is needed** — only the previously broken rows change hash. An upgraded database self-heals on the next scan: the stale-entry cleanup removes the old lossy-hashed rows, the correct ones are inserted, and `path_raw` backfills (which is why it appears in the upsert's `DO UPDATE SET` and change-guard, unlike `path`).

The width is a cross-language contract — `fs_api` recomputes the same hashes in Go (`pathHash`/`pathHashLen` in `handlers.go`) to look rows up by primary key. The two implementations disagreeing makes every lookup miss while each side still looks correct on its own, so `fs_common` pins the Go implementation's output in a unit test; a change to either must change both in the same commit.

Changing `PATH_HASH_LEN` invalidates an existing index rather than merely dating it: no query can reach a row of the other width. Both Rust binaries detect this via `fs_common::stored_path_hash_len`. `fs_indexer` truncates the index up front and rebuilds it during the scan — the one path that drops the index without a completed scan behind it, which is safe precisely because those rows are already unreachable. `fs_aggregator` refuses to run, since it would otherwise publish a full table of aggregates that join to nothing. `fs_api` logs a startup warning instead of silently answering 404 for a fully indexed filesystem.

**The walker honours no ignore files** — `fs_indexer` explicitly disables every filter the `ignore` crate offers (`hidden`, `ignore`, `git_ignore`, `git_global`, `git_exclude`, `parents`). The crate's defaults respect `.ignore`, `.gitignore`, `.git/info/exclude` and git's global excludes, which on a root-privileged audit scan would let any unprivileged user hide a subtree from the index with a one-line `.ignore` file — and, because hidden entries never reach `seen_hashes`, have their existing rows deleted by the stale-entry cleanup. Do not re-enable these: an audit tool must index what is on disk, not what the audited user consents to.

**Stale entry cleanup** — each batch records its `path_hash`es into a session-scoped `seen_hashes` temp table. After the walk completes cleanly, rows whose hash was never seen are deleted via an anti-join (`WHERE NOT EXISTS`). The upsert itself is change-guarded (`ON CONFLICT ... DO UPDATE ... WHERE ... IS DISTINCT FROM ...`), so a rescan of a mostly-unchanged filesystem produces almost no heap or index writes — unlike the earlier design, which stamped a session ID onto every row on every scan.

**Aggregator ancestor folding** — `fs_aggregator` streams the index unordered (no `ORDER BY`, no server-side sort) and folds each entry's size/count/times directly into every ancestor directory in a `HashMap<path, DirNode>` (O(path depth) updates per entry). This yields the same recursive totals as a children-before-parents DP without forcing PostgreSQL to sort the whole table by `length(path)` first. No recursive SQL queries, no second pass.

**Producer/consumer pipeline in indexer** — the filesystem walker runs in a spawned thread and sends `FileRecord`s through an unbounded `crossbeam-channel`. The main thread owns the DB connection and drains the channel in batches of 1 000 rows per transaction, keeping memory bounded while maximising write throughput.
