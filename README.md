# FS Indexer & Aggregator

A high-performance suite for indexing massive filesystems and analyzing directory sizes using Rust and PostgreSQL.

## Components

### 1. `fs_indexer`
The core crawler that scans the filesystem and populates the database.
- **Parallel Crawling**: Uses multi-threaded walking to saturate I/O.
- **Non-Intrusive**: Uses `lstat` and `statx` logic to avoid reading file contents.
- **UPSERT Logic**: Uses absolute paths as primary keys to allow for efficient re-indexing without duplicates.
- **Extended Metadata**: Supports the `--all-metadata` flag to capture xattrs (SELinux, etc.) into a JSONB/Text column.

### 2. `fs_aggregator`
A post-processing tool that computes rolled-up statistics for directories.
- **Recursive Summation**: Calculates total size and latest timestamps for every directory.
- **Pre-computed Stats**: Stores results in a `dir_stats` table for instant querying of folder sizes.

## Setup & Installation

### Prerequisites
- Rust (latest stable)
- PostgreSQL (Running in Docker or locally)

### Database Setup
If using the provided Docker setup:
```bash
docker run --name fs-postgres -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=fs_index -p 5432:5432 -d postgres:latest
```

### Building
```bash
# Build Indexer
cd fs_indexer && cargo build --release

# Build Aggregator
cd ../fs_aggregator && cargo build --release
```

## Usage

### Step 1: Index the Filesystem
```bash
./fs_indexer/target/release/fs_indexer /path/to/scan [threads] [--all-metadata]
```

### Step 2: Generate Directory Statistics
```bash
./fs_aggregator/target/release/fs_aggregator
```

## Database Schema
- `filesystem_index`: Individual file/folder metadata.
- `dir_stats`: Aggregated directory sizes and timestamps.
