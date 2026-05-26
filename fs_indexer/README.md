# FS Indexer

The `fs_indexer` is a high-performance filesystem metadata crawler designed to index massive directory structures into a PostgreSQL database.

## Architecture
- **Producer:** Uses `ignore::WalkBuilder` for multi-threaded, recursive filesystem traversal.
- **Consumer:** Collects metadata and performs batched inserts into PostgreSQL to maximize throughput.
- **Identity Mapping:** Defers UID/GID resolution to a post-processing step to avoid LDAP/NIS bottlenecks during the crawl.
- **Session Tracking:** Uses UUID-based session IDs to identify and prune stale entries (files deleted since the last scan).

## Database Schema
- `filesystem_index`: Primary storage for file/directory metadata.
- `scan_sessions`: Tracks start/end times of indexing runs.
- `identity_map`: Maps numeric IDs to human-readable names.

## Usage
```bash
./fs_indexer <root_path> [threads]
```

## Performance Considerations
- **Batching:** Inserts are performed in groups of 1,000.
- **Indexing:** Uses `path_hash` (BYTEA) as the primary key for fast lookups.
