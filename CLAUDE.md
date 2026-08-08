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

### `filesystem_index` — written by `fs_indexer`

| Column | Type | Notes |
|---|---|---|
| `path` | TEXT | Full absolute path |
| `path_hash` | BYTEA PK | SHA-256(path); used as PK and to link parent↔child |
| `parent_hash` | BYTEA | SHA-256(parent path); `NULL` for `/` |
| `size_bytes` | BIGINT | |
| `file_type` | INTEGER | 1=file, 2=directory, 3=symlink, 0=other |
| `permissions` | TEXT | Octal string, e.g. `"755"` |
| `uid` / `gid` | INTEGER | Numeric owner/group |
| `atime/mtime/ctime` | BIGINT | Unix epoch seconds |
| `metadata` | TEXT | Reserved, currently empty |
| `last_seen_session` | TEXT | UUID of the indexer run that last touched this row |

### `identity_map` — written by `fs_indexer`

Maps numeric UID/GID to username/groupname. Populated after each walk via the OS `users` crate.

| Column | Notes |
|---|---|
| `id` + `id_type` | Composite PK; `id_type` is `'uid'` or `'gid'` |
| `name` | Resolved username or groupname |

### `scan_sessions` — written by `fs_indexer` and `fs_aggregator`

One row per indexer or aggregator run, via the shared helpers in `fs_common` (`ensure_scan_sessions_table`, `record_scan_start`, `report_scan_progress`, `end_scan_session`). `started_at`/`ended_at` are also used to correlate `last_seen_session` for stale-entry cleanup (indexer runs only).

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
cp fs_config_template.yml fs_config.yml
```

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

Note: `fs_common` (used by the Rust binaries) reads `database` and `indexer`; the `auth` section is only used by `fs_api`. `indexer` is read only by `fs_indexer` — `fs_aggregator` ignores it.

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

**Path hashing** — `compute_hash(path)` in `fs_common` produces a SHA-256 digest of the path string. This is used as the primary key in `filesystem_index` and `dir_stats`, and as the parent-child link (`parent_hash`). Children of a directory are found by querying `WHERE parent_hash = $1` — no joins needed.

**Stale entry cleanup** — each indexer run mints a UUID session ID. Every upserted row carries `last_seen_session`. After the walk completes, rows with a different session ID are deleted. This keeps the index current across incremental re-scans.

**Aggregator bottom-up DP** — `fs_aggregator` fetches all entries `ORDER BY length(path) DESC` so children always appear before their parents. A single linear pass accumulates values into a `HashMap<path, DirNode>`, propagating each entry's contribution up to its parent. No recursive SQL queries, no second pass.

**Producer/consumer pipeline in indexer** — the filesystem walker runs in a spawned thread and sends `FileRecord`s through an unbounded `crossbeam-channel`. The main thread owns the DB connection and drains the channel in batches of 1 000 rows per transaction, keeping memory bounded while maximising write throughput.
