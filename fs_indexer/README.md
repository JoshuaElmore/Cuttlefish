# FS Indexer

The `fs_indexer` is a high-performance filesystem metadata crawler designed to index massive directory structures into a PostgreSQL database.

## Architecture
- **Producer:** Uses `ignore::WalkBuilder` for multi-threaded, recursive filesystem traversal. All configured scan roots go into one parallel walker, so they are traversed concurrently by a single shared thread pool with work stealing between them.
- **Consumer:** Collects metadata and performs batched inserts into PostgreSQL to maximize throughput.
- **Identity Mapping:** Defers UID/GID resolution to a post-processing step to avoid LDAP/NIS bottlenecks during the crawl.
- **Session Tracking:** Uses UUID-based session IDs for run bookkeeping. Stale entries (files deleted since the last scan) are pruned by anti-joining the hashes seen this run, restricted to the subtrees this run actually scanned.

## Database Schema
- `filesystem_index`: Primary storage for file/directory metadata.
- `scan_sessions`: Tracks start/end times of indexing runs.
- `identity_map`: Maps numeric IDs to human-readable names.

## Usage

Takes no arguments; the scan roots and thread count come from the `indexer`
section of `fs_config.yml` in the working directory.

```bash
sudo ./fs_indexer
```

```yaml
indexer:
  root_paths:             # or root_path: / for a single root
    - /home
    - /srv/data
  threads: 8              # walker pool shared across all roots
```

Roots must not overlap — the indexer validates this before touching the
database and exits with an error naming both offending roots.

## Performance Considerations
- **Batching:** Rows are bulk-loaded via binary `COPY` into a staging table and upserted in batches of 50,000.
- **Indexing:** Uses `path_hash` (BYTEA) as the primary key for fast lookups.
