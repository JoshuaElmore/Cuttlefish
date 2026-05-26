# FS Aggregator

The `fs_aggregator` computes hierarchical statistics (size, file counts, time ranges) for the indexed filesystem.

## Algorithm: Bottom-Up DP
The aggregator employs a Dynamic Programming approach to avoid redundant tree traversals:
1. **Sorted Fetch:** Fetches all entries from the index sorted by path length in descending order.
2. **Bottom-Up Propagation:** Processes leaf nodes first, propagating their size and metadata up to their parent directories.
3. **Single Pass:** This ensures that by the time a directory is processed, all its children's aggregates are already computed.

## Database Schema
- `dir_stats`: Stores aggregated metrics per directory, keyed by `path_hash`.

## Current Limitations
- **Memory Usage:** Currently loads the entire index into memory for processing. For datasets exceeding $10^7$ inodes, a streaming cursor or DB-side aggregation is required.

## Usage
```bash
./fs_aggregator
```
