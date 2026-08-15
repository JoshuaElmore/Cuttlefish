# Cuttlefish

A high-performance filesystem intelligence platform. Indexes billions of inodes into PostgreSQL, computes recursive directory statistics, and exposes everything through a REST API with a React browser UI — and through an MCP server, so AI assistants can query the index directly.

![Cuttlefish file browser showing directory stats panel](docs/screenshot.png)

---

## Components

| Component | Language | Role |
|---|---|---|
| `fs_indexer` | Rust | Walks the filesystem in parallel, writes raw metadata to PostgreSQL |
| `fs_aggregator` | Rust | Computes recursive directory and ownership statistics |
| `fs_api` | Go | REST API and MCP server over the database; serves the compiled UI |
| `fs_ui` | TypeScript / React | Browser frontend |
| `fs_common` | Rust (lib) | Shared config, DB connection, and path hashing used by the Rust binaries |

---

## Installation (RPM)

Pre-built RPMs for Rocky Linux / RHEL / AlmaLinux are attached to each [GitHub Release](../../releases). A single package contains all binaries, the web UI, and all systemd units.

### 1. Install the package

**Rocky Linux / RHEL 9:**
```bash
sudo rpm -i cuttlefish-<version>-1.el9.x86_64.rpm
```

**Rocky Linux / RHEL 10:**
```bash
sudo rpm -i cuttlefish-<version>-1.el10.x86_64.rpm
```

### 2. Configure

```bash
sudo cp /etc/cuttlefish/fs_config.yml.example /etc/cuttlefish/fs_config.yml
sudo $EDITOR /etc/cuttlefish/fs_config.yml
```

At minimum set your PostgreSQL credentials and generate a session secret:

```bash
openssl rand -hex 32   # paste result into session_secret
```

### 3. Enable the API server

```bash
sudo systemctl enable --now cuttlefish-api
# → http://localhost:8080
```

### 4. Schedule nightly indexing

```bash
sudo systemctl enable --now cuttlefish-index.timer
```

This fires `fs_indexer` nightly at 2am. On success it automatically chains to `fs_aggregator`. Check the next scheduled run and last result:

```bash
systemctl status cuttlefish-index.timer
journalctl -u cuttlefish-index.service
journalctl -u cuttlefish-aggregate.service
```

To re-aggregate without re-indexing (e.g. after a config change):

```bash
sudo systemctl start cuttlefish-aggregate.service
```

---

## Systemd units

| Unit | Type | Description |
|---|---|---|
| `cuttlefish-api.service` | Service | REST API server — enable and run permanently |
| `cuttlefish-index.service` | Oneshot | Runs `fs_indexer` as root (scan root/threads from `fs_config.yml`); chains to aggregate on success |
| `cuttlefish-index.timer` | Timer | Triggers the indexer nightly at 2am (`Persistent=true`) |
| `cuttlefish-aggregate.service` | Oneshot | Runs `fs_aggregator` as the `cuttlefish` user |

---

## Configuration

All components read `fs_config.yml` from their working directory (`/etc/cuttlefish` when installed via RPM). The file is **gitignored** — use `fs_config_template.yml` as the starting point.

```yaml
database:
  host: localhost
  user: postgres
  password: "..."
  dbname: fs_index
  sslmode: require

indexer:
  root_path: /            # required; scan root for fs_indexer
  threads: 8              # optional, defaults to 8

auth:
  session_secret: "..."   # openssl rand -hex 32 — must be ≥ 32 chars

  local:                  # single admin account
    username: admin
    password: "..."

  # oidc:                 # uncomment to use SSO instead of local auth
  #   issuer: "https://accounts.google.com"
  #   client_id: "..."
  #   client_secret: "..."
  #   redirect_url: "http://yourhost/auth/callback"

mcp:                      # optional; these are the defaults
  enabled: true
  path: "/mcp"
  # listen_addr: ":8081"  # serve MCP on its own port instead of the main one
  # token: "..."          # long-lived bearer credential for headless clients
```

The API server rejects a missing, default, or short `session_secret` at startup,
and likewise an `mcp.path` that isn't absolute or shadows an existing route, or
an `mcp.token` under 32 characters.

---

## Building from source

### Prerequisites

- Rust (stable)
- Go 1.26+
- Node.js 22+
- PostgreSQL client libraries

### Build

```bash
# 0640, not a plain cp: the config holds the DB password and session secret, and
# every component refuses to start if it is readable by other users on the host.
install -m 0640 fs_config_template.yml fs_config.yml
$EDITOR fs_config.yml
make all
```

### Run

```bash
cd /path/containing/fs_config.yml
sudo fs_indexer/target/release/fs_indexer         # index filesystem (root_path/threads from fs_config.yml; root required to scan)
fs_aggregator/target/release/fs_aggregator        # compute stats
fs_api/fs_api                                     # start API server → :8080
```

### Build targets

```bash
make all              # build everything
make build-indexer    # Rust release build of fs_indexer
make build-aggregator # Rust release build of fs_aggregator
make build-ui         # Vite production build of fs_ui → copies into fs_api/ui/
make build-api        # Go build of fs_api
make swagger-api      # regenerate Swagger docs then build API
make test-api         # go test (MCP tests; DB-backed ones need CUTTLEFISH_TEST_DSN)
make run-indexer      # build + scan /
make run-aggregator   # build + aggregate
make run-api          # build everything + start server
make clean            # remove all build artifacts and node_modules
```

---

## Database schema

Tables are created automatically on first run.

| Table | Written by | Description |
|---|---|---|
| `filesystem_index` | `fs_indexer` | One row per filesystem entry; primary key is SHA-256(path) |
| `identity_map` | `fs_indexer` | UID/GID → username/groupname |
| `scan_sessions` | `fs_indexer` | Per-run timestamps; used for stale-entry cleanup |
| `dir_stats` | `fs_aggregator` | Recursive size, file count, and time ranges per directory |
| `user_stats` | `fs_aggregator` | Total size and file count per UID/GID |

---

## API

Swagger UI is available at `http://localhost:8080/swagger/` once the server is running.

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/list?path=` | List directory children |
| `GET` | `/api/file/stats?path=` | Single file metadata |
| `GET` | `/api/dir/stats?path=` | Single directory metadata + aggregates |
| `GET` | `/api/user/list` | User storage totals (sortable, paginated) |
| `GET` | `/api/group/list` | Group storage totals (sortable, paginated) |
| `GET` | `/api/search` | Advanced search (POST body of AND/OR rules) |
| `GET` | `/api/scans` | Indexer/aggregator run history |
| `POST` | `/mcp` | MCP endpoint (Streamable HTTP) |
| `POST` | `/auth/login` | Local login |
| `GET` | `/auth/oidc/start` | Begin SSO login flow |
| `POST` | `/auth/logout` | Clear session |

---

## MCP server

`fs_api` speaks the [Model Context Protocol](https://modelcontextprotocol.io) at
`/mcp`, exposing the index to AI clients as read-only tools: `get_entry_stats`,
`list_directory`, `search_files`, `list_user_stats`, `list_group_stats` and
`list_scan_sessions`, plus a `cuttlefish://index/status` resource reporting how
fresh the snapshot is. Only metadata is served — file contents are never indexed.

It is on by default and guarded by the same session auth as `/api/*`. A headless
client authenticates with the session token as a bearer credential:

```bash
TOKEN=$(curl -s -c - -X POST http://localhost:8080/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"..."}' | awk '/session/{print $7}')
```

```json
{
  "mcpServers": {
    "cuttlefish": {
      "type": "http",
      "url": "http://localhost:8080/mcp",
      "headers": { "Authorization": "Bearer <token>" }
    }
  }
}
```

Configure it under `mcp:` in `fs_config.yml` — `enabled`, `path`, `listen_addr`
for a dedicated port, and an optional long-lived `token` for clients that cannot
log in. See `fs_api/CLAUDE.md` for the full reference.
