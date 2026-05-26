# File System Indexer & Aggregator

A high-performance system for indexing billions of inodes and calculating bottom-up directory statistics.

## Components

### 1. `fs_indexer`
The indexer performs a recursive scan of a specified directory tree and stores metadata for every file and directory into a PostgreSQL database.

**Key Features:**
- **Parallel Scanning**: Uses `ignore` (Ripgrep's engine) for fast, multi-threaded filesystem traversal.
- **Path Hashing**: Uses SHA-256 hashes of paths as primary keys to ensure high-performance lookups and avoid string-comparison bottlenecks at scale.
- **Identity Resolution**: Maps UID/GID to actual system usernames and group names.
- **Session Tracking**: Tracks scan sessions to identify and prune stale entries (files that were deleted since the last scan).

**Usage:**
```bash
# Build the indexer
make build-indexer

# Run the indexer: <root_path> [threads]
./fs_indexer/target/release/fs_indexer /home/joshua 16
```

---

### 2. `fs_aggregator`
The aggregator calculates recursive directory statistics (total size, file counts, and time ranges) using a bottom-up Dynamic Programming approach.

**Key Features:**
- **Bottom-Up Aggregation**: Processes entries sorted by path length in descending order. This ensures that a directory's statistics are only calculated after all its children have been processed.
- **Efficient Metadata Storage**: Writes aggregated results into the `dir_stats` table, which can be queried instantly by the API.
- **Comprehensive Stats**: Tracks `mtime`, `atime`, and `ctime` ranges (first/last) for every directory in the hierarchy.

**Usage:**
```bash
# Build the aggregator
make build-aggregator

# Run the aggregator
./fs_aggregator/target/release/fs_aggregator
```

## Configuration

Both tools share a common configuration file located at `fs_config.toml` in the project root:

```toml
[database]
host = "localhost"
user = "postgres"
password = "postgres"
dbname = "fs_index"
```

## Database Schema

- `filesystem_index`: The primary metadata store (paths, hashes, sizes, times).
- `dir_stats`: Aggregated statistics for directories.
- `identity_map`: Cached mapping of UIDs/GIDs to names.
- `scan_sessions`: History of indexing runs.

## Workflow

1. **Index**: Run `fs_indexer` to populate the database with current filesystem state.
2. **Aggregate**: Run `fs_aggregator` to compute recursive directory totals.
3. **Query**: Use the `fs_api` (Go) to retrieve the data via REST endpoints.
