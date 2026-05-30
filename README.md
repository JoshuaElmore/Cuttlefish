# Cuttlefish

A high-performance filesystem intelligence platform. Indexes billions of inodes into PostgreSQL, computes recursive directory statistics, and exposes everything through a REST API with a React browser UI.

![Cuttlefish file browser showing directory stats panel](docs/screenshot.png)

---

## Components

| Component | Language | Role |
|---|---|---|
| `fs_indexer` | Rust | Walks the filesystem in parallel, writes raw metadata to PostgreSQL |
| `fs_aggregator` | Rust | Computes recursive directory and ownership statistics |
| `fs_api` | Go | REST API over the database; serves the compiled UI |
| `fs_ui` | TypeScript / React | Browser frontend |
| `fs_common` | Rust (lib) | Shared config, DB connection, and path hashing used by the Rust binaries |

---

## Quickstart

### 1. Configure

Copy the config template and fill in your PostgreSQL credentials and a strong session secret:

```bash
cp fs_config_template.toml fs_config.toml
$EDITOR fs_config.toml
```

Generate a session secret:

```bash
openssl rand -hex 32
```

### 2. Build everything

```bash
make all
```

This runs: Swagger generation → indexer → aggregator → UI → API.

### 3. Index your filesystem

```bash
cd /path/to/your/config   # or the project root if running locally
sudo fs_indexer/target/release/fs_indexer /  16
```

Arguments: `<root path> [thread count]` — defaults to 8 threads.

### 4. Aggregate statistics

```bash
fs_aggregator/target/release/fs_aggregator
```

### 5. Start the API server

```bash
fs_api/fs_api
# → http://localhost:8080
```

---

## Installation (RPM)

Pre-built RPMs for RHEL 9 / Rocky Linux 9 / AlmaLinux 9 are attached to each [GitHub Release](../../releases).

```bash
# API server
sudo rpm -i cuttlefish-api-<version>.x86_64.rpm

# Indexer and aggregator (optional, run on the machine being scanned)
sudo rpm -i cuttlefish-indexer-<version>.x86_64.rpm
sudo rpm -i cuttlefish-aggregator-<version>.x86_64.rpm
```

After installing `cuttlefish-api`:

```bash
# Create config from the installed template
sudo cp /etc/cuttlefish/fs_config.toml.example /etc/cuttlefish/fs_config.toml
sudo $EDITOR /etc/cuttlefish/fs_config.toml

# Enable and start
sudo systemctl enable --now cuttlefish-api
```

The service runs as the `cuttlefish` system user and listens on `:8080`.

---

## Configuration

All components read `fs_config.toml` from their working directory. The file is **gitignored** — use `fs_config_template.toml` as the starting point.

```toml
[database]
host     = "localhost"
user     = "postgres"
password = "..."
dbname   = "fs_index"
sslmode  = "require"

[auth]
session_secret = "..."   # openssl rand -hex 32 — must be ≥ 32 chars

[auth.local]             # single admin account
username = "admin"
password = "..."

# [auth.oidc]            # uncomment to use SSO instead of local auth
# issuer       = "https://accounts.google.com"
# client_id    = "..."
# client_secret = "..."
# redirect_url  = "http://yourhost/auth/callback"
```

The API server rejects a missing, default, or short `session_secret` at startup.

---

## Build targets

```bash
make all              # build everything
make build-indexer    # Rust release build of fs_indexer
make build-aggregator # Rust release build of fs_aggregator
make build-ui         # Vite production build of fs_ui → copies into fs_api/ui/
make build-api        # Go build of fs_api
make swagger-api      # regenerate Swagger docs then build API
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

## Typical workflow

```
sudo fs_indexer /  →  fs_aggregator  →  fs_api (runs continuously)
```

The indexer and aggregator are one-shot batch processes. Re-run them on a schedule (e.g. nightly cron) to keep the index current; the aggregator overwrites stale stats idempotently.

---

## API

Swagger UI is available at `http://localhost:8080/swagger/` once the server is running.

Key endpoints:

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/list?path=` | List directory children |
| `GET` | `/api/file/stats?path=` | Single file metadata |
| `GET` | `/api/dir/stats?path=` | Single directory metadata + aggregates |
| `GET` | `/api/user/list` | User storage totals (sortable, paginated) |
| `GET` | `/api/group/list` | Group storage totals (sortable, paginated) |
| `POST` | `/auth/login` | Local login |
| `GET` | `/auth/oidc/start` | Begin SSO login flow |
| `POST` | `/auth/logout` | Clear session |
