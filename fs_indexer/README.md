# FS Indexer

A high-performance, multi-threaded filesystem metadata indexer written in Rust.

## Features
- **Parallel Crawling**: Uses the `ignore` crate to saturate I/O and scan billions of inodes efficiently.
- **Non-Intrusive**: Uses `lstat` (via `statx` logic) to retrieve metadata without triggering file reads or following symlinks.
- **Writer Isolation**: Implements a producer-consumer pattern with a dedicated CSV writer thread to prevent lock contention.
- **Configurable**: Set the number of threads to match your hardware or network capabilities.

## Installation

### Prerequisites
- Rust (latest stable)
- Cargo

### Build
```bash
cargo build --release
```

## Usage
Run the indexer by providing a path to scan and an optional thread count:

```bash
./target/release/fs_indexer <path_to_scan> [num_threads]
```

### Example
```bash
./target/release/fs_indexer /home/joshua 16
```

## Output
The program generates a `filesystem_index.csv` in the current working directory with the following columns:
- `path`: Absolute path to the file/directory
- `inode`: Inode number
- `size_bytes`: Size in bytes
- `permissions`: Octal mode (includes file type)
- `uid`: User ID
- `gid`: Group ID
- `atime`: Last access time (Unix epoch)
- `mtime`: Last modification time (Unix epoch)
- `ctime`: Last status change time (Unix epoch)
