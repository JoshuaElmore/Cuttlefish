# FS Indexer Suite

A professional-grade filesystem metadata indexing and analysis suite.

## Architecture

- **`fs_indexer` (Rust)**: High-performance parallel crawler. Uses `statx` and `sha2` to index billions of inodes into PostgreSQL.
- **`fs_aggregator` (Rust)**: Bottom-up Dynamic Programming engine. Pre-calculates directory sizes and timestamps.
- **`fs_api` (Go)**: Stateless REST API and static file server. Serves the frontend and provides metadata access.
- **`fs_ui` (React/TS)**: Modern interactive explorer for browsing the indexed filesystem.

## Tech Stack

- **Languages**: Rust, Go, TypeScript.
- **Database**: PostgreSQL (indexed via path hashes).
- **Frontend**: React + TypeScript.

## Quick Start

Use the provided Makefile to build the entire pipeline:

```bash
# Build all components
make all

# Start the API and WebUI
make run-api
```

The dashboard will be available at `http://localhost:8080`.
