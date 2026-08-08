use std::borrow::Cow;
use std::error::Error;
use std::os::unix::ffi::OsStrExt;
use std::os::unix::fs::MetadataExt;
use std::thread;
use std::time::{Duration, Instant};
use ignore::WalkBuilder;
use postgres::Client;
use postgres::binary_copy::BinaryCopyInWriter;
use postgres::types::Type;
use crossbeam_channel::bounded;
use uuid::Uuid;
use users::{get_user_by_uid, get_group_by_gid};

/// Channel capacity. Bounds producer backlog so a fast parallel walker can't
/// outrun the DB writer and blow up memory on large filesystems (backpressure).
const CHANNEL_CAPACITY: usize = 100_000;

/// Number of records bulk-loaded per flush. Larger batches amortise the
/// COPY + upsert round-trip; small enough that the in-flight batch is cheap.
const BATCH_SIZE: usize = 50_000;

/// Metadata record for a single filesystem entry.
struct FileRecord {
    /// UTF-8 rendering of the path, for display and for the trigram search
    /// index. Lossy when the real name is not valid UTF-8 — `path_raw` carries
    /// the truth in that case.
    path: String,
    /// The exact bytes the kernel returned, stored only when they differ from
    /// `path` (i.e. the name is not valid UTF-8). NULL for the overwhelming
    /// majority of rows, so this costs almost nothing at scale while keeping
    /// non-UTF-8 names recoverable and addressable.
    path_raw: Option<Vec<u8>>,
    path_hash: Vec<u8>,
    parent_hash: Option<Vec<u8>>,
    size_bytes: i64,
    file_type: i32,
    permissions: String,
    uid: u32,
    gid: u32,
    atime: i64,
    mtime: i64,
    ctime: i64,
    metadata: String,
}

/// Determines the parent path of a given path.
/// Returns None for the root directory.
///
/// Operates on raw bytes rather than `&str` because the parent link has to be
/// derived from the same bytes that `compute_hash_bytes` keys on. Splitting a
/// UTF-8 rendering instead would give a child of a non-UTF-8-named directory a
/// `parent_hash` that matches nothing, and `/api/list` would report that
/// directory as empty. `/` is ASCII and cannot appear inside a UTF-8
/// continuation byte, so scanning for it bytewise is exact for any encoding.
fn get_parent_path(path: &[u8]) -> Option<&[u8]> {
    if path == b"/" {
        return None;
    }
    let mut end = path.len();
    while end > 0 && path[end - 1] == b'/' {
        end -= 1;
    }
    let trimmed = &path[..end];
    let idx = trimmed.iter().rposition(|&c| c == b'/')?;
    let parent = &trimmed[..idx];
    Some(if parent.is_empty() { b"/" } else { parent })
}

fn main() -> Result<(), Box<dyn Error>> {
    let config = fs_common::load_config("fs_config.yml")?;
    let indexer_config = config.indexer.as_ref()
        .ok_or("fs_config.yml is missing the `indexer` section (root_path is required)")?;
    let root_path = indexer_config.root_path.clone();
    let threads = indexer_config.threads;

    // Unique ID for this scan session (scan_sessions bookkeeping only).
    let session_id = Uuid::new_v4().to_string();
    println!("Starting scan session: {}", session_id);

    // Bounded channel: when full, the producer blocks instead of buffering the
    // entire filesystem in RAM.
    let (tx, rx) = bounded(CHANNEL_CAPACITY);

    let root_path_clone = root_path.clone();

    // Producer thread: walks the filesystem and sends metadata to the consumer.
    // Returns the number of entries it had to skip (unreadable dirs/files) and
    // the number whose names were not valid UTF-8, so the caller can report
    // both. A panic here is detected via join() below and blocks the
    // stale-entry cleanup, so a crashed walk can never wipe the index.
    let producer = thread::spawn(move || -> (u64, u64) {
        let mut skipped: u64 = 0;
        let mut lossy: u64 = 0;
        // Every filter the walker offers is disabled explicitly. The `ignore`
        // crate's defaults honour .ignore/.gitignore/.git/info/exclude files and
        // git's global excludes — which means any unprivileged user could hide a
        // subtree from this root-privileged audit scan just by dropping an
        // `.ignore` file in it (and hidden entries would then be *deleted* from
        // the index by the stale-entry cleanup below, since they never reach
        // seen_hashes). An audit tool must index what is on disk, not what the
        // audited user consents to.
        let walker = WalkBuilder::new(root_path_clone)
            .threads(threads)
            .hidden(false)        // index dotfiles
            .ignore(false)        // ignore .ignore / .rgignore files
            .git_ignore(false)    // ignore .gitignore files
            .git_global(false)    // ignore git's core.excludesFile
            .git_exclude(false)   // ignore .git/info/exclude
            .parents(false)       // don't apply ignore files above the scan root
            .follow_links(false)  // already the default; a symlink must never redirect a root walk
            .build();
        for result in walker {
            let entry = match result {
                Ok(e) => e,
                Err(e) => {
                    eprintln!("Walk error: {}", e);
                    skipped += 1;
                    continue;
                }
            };

            let meta = match entry.metadata() {
                Ok(m) => m,
                Err(_) => {
                    eprintln!("Failed to read metadata for entry: {}", entry.path().display());
                    skipped += 1;
                    continue;
                }
            };

            // Key off the raw bytes, never the UTF-8 rendering: two names that
            // differ only in invalid bytes render identically, and hashing that
            // rendering merges them into one row and loses a file. See
            // fs_common::compute_hash_bytes.
            let raw = entry.path().as_os_str().as_bytes();
            let path_hash = fs_common::compute_hash_bytes(raw);
            let parent_hash = get_parent_path(raw).map(fs_common::compute_hash_bytes);

            // from_utf8_lossy only allocates when it had to substitute, so the
            // Cow tells us whether this name survived the rendering intact.
            let rendered = String::from_utf8_lossy(raw);
            let path_raw = match &rendered {
                Cow::Borrowed(_) => None,
                Cow::Owned(_) => {
                    lossy += 1;
                    Some(raw.to_vec())
                }
            };
            let path = rendered.into_owned();

            let mode = meta.mode();
            let file_type = match mode & 0o170000 {
                0o100000 => 1, // Regular file
                0o040000 => 2, // Directory
                0o120000 => 3, // Symbolic link
                _ => 0,
            };

            if tx.send(FileRecord {
                path,
                path_raw,
                path_hash,
                parent_hash,
                size_bytes: meta.len() as i64,
                file_type,
                permissions: format!("{:o}", mode & 0o777),
                uid: meta.uid(),
                gid: meta.gid(),
                atime: meta.atime(),
                mtime: meta.mtime(),
                ctime: meta.ctime(),
                metadata: "".to_string(),
            }).is_err() {
                eprintln!("Consumer gone; stopping walk.");
                break;
            }
        }
        (skipped, lossy)
    });

    // Database setup
    let mut client = fs_common::get_db_client(&config.database)?;

    // The index can always be rebuilt by rerunning the scan, so batch commits
    // don't need to wait for the WAL to reach disk.
    client.batch_execute("SET synchronous_commit = off")?;

    // Initialize schema. UNLOGGED: this table is derived data that a rerun of
    // this binary rebuilds from the filesystem, so there is nothing in it worth
    // the WAL traffic — see fs_common::ensure_unlogged for the full trade-off,
    // and the ALTER below for databases created before this was the default.
    client.execute(
        "CREATE UNLOGGED TABLE IF NOT EXISTS filesystem_index (
            path TEXT,
            path_raw BYTEA,
            path_hash BYTEA PRIMARY KEY,
            parent_hash BYTEA,
            size_bytes BIGINT,
            file_type INTEGER,
            permissions TEXT,
            uid INTEGER,
            gid INTEGER,
            atime BIGINT,
            mtime BIGINT,
            ctime BIGINT,
            metadata TEXT
        )",
        &[]
    ).map_err(|e| { eprintln!("Schema creation failed: {}", e); e })?;

    fs_common::ensure_scan_sessions_table(&mut client)
        .map_err(|e| { eprintln!("Session table creation failed: {}", e); e })?;

    match fs_common::reap_stale_scan_sessions(&mut client, "indexer") {
        Ok(0) => {}
        Ok(n) => eprintln!("Reaped {} stale indexer session(s) left running by a prior crash.", n),
        Err(e) => eprintln!("Failed to reap stale indexer sessions (continuing): {}", e),
    }

    // Migrate databases created by older binaries. Stale-entry cleanup no longer
    // stamps a session ID on every row (see seen_hashes below), so the column is
    // dead weight — and its index forced non-HOT updates of the entire table on
    // every rescan while never being usable for the old `!=` cleanup predicate.
    // The path_raw ADD COLUMN has to land before the staging table is created
    // below, since that table is declared LIKE filesystem_index.
    client.batch_execute(
        "DROP INDEX IF EXISTS idx_fsindex_last_seen;
         ALTER TABLE filesystem_index DROP COLUMN IF EXISTS last_seen_session;
         ALTER TABLE filesystem_index ADD COLUMN IF NOT EXISTS path_raw BYTEA;"
    ).map_err(|e| { eprintln!("Schema migration failed: {}", e); e })?;

    // Migrate an index built with a different path-hash width. Clearing it up
    // front rather than letting the scan replace the rows one batch at a time:
    // hashes of the old width are already unreachable — every lookup and every
    // parent→child link is keyed by a hash recomputed from the path — so no
    // usable data is being discarded, and the stale-entry cleanup at the end of
    // this run would delete every one of those rows anyway, after the table had
    // spent the whole scan carrying two full generations of them.
    //
    // This is the one path that drops the index without a completed scan behind
    // it. That is safe precisely because a mismatched index is already dead to
    // this binary: if the scan below fails, the "index left intact" guarantee
    // would have been preserving rows no query can reach.
    match fs_common::stored_path_hash_len(&mut client)? {
        Some(n) if n as usize != fs_common::PATH_HASH_LEN => {
            eprintln!(
                "filesystem_index holds {}-byte path hashes but this binary writes {}-byte \
                 hashes; clearing the index so this scan rebuilds it.",
                n, fs_common::PATH_HASH_LEN
            );
            client.batch_execute("TRUNCATE filesystem_index")
                .map_err(|e| { eprintln!("Path-hash width migration failed: {}", e); e })?;
        }
        _ => {}
    }

    // Convert a table created before UNLOGGED became the default. Deliberately
    // after the truncate above, so the rewrite that ALTER TABLE performs has
    // nothing left to copy in the case where both migrations apply.
    match fs_common::ensure_unlogged(&mut client, "filesystem_index") {
        Ok(true) => println!("Converted filesystem_index to UNLOGGED."),
        Ok(false) => {}
        Err(e) => { eprintln!("Failed to convert filesystem_index to UNLOGGED: {}", e); return Err(e); }
    }

    client.execute(
        "CREATE TABLE IF NOT EXISTS identity_map (
            id INTEGER,
            id_type TEXT CHECK (id_type IN ('uid', 'gid')),
            name TEXT NOT NULL,
            PRIMARY KEY (id, id_type)
        )",
        &[]
    ).map_err(|e| { eprintln!("Identity map creation failed: {}", e); e })?;

    // Session-scoped staging table for bulk upserts (LIKE picks up every column
    // of filesystem_index), plus the set of every path_hash seen this scan —
    // the source of truth for stale-entry cleanup.
    client.batch_execute(
        "CREATE TEMP TABLE staging_index (LIKE filesystem_index INCLUDING DEFAULTS);
         CREATE TEMP TABLE seen_hashes (path_hash BYTEA);"
    ).map_err(|e| { eprintln!("Staging table creation failed: {}", e); e })?;

    println!("Recording session start in DB...");
    fs_common::record_scan_start(&mut client, &session_id, "indexer")?;
    println!("Session {} recorded in database.", session_id);

    // Batch consumption
    let mut batch = Vec::with_capacity(BATCH_SIZE);
    let mut total_processed = 0u64;
    let mut insert_failed = false;
    let start_time = Instant::now();
    let mut last_report = Instant::now();

    while let Ok(record) = rx.recv() {
        batch.push(record);
        total_processed += 1;

        if batch.len() >= BATCH_SIZE {
            if let Err(e) = insert_batch(&mut client, &batch) {
                eprintln!("Batch insert failed: {}", e);
                insert_failed = true;
            }
            batch.clear();
        }

        if last_report.elapsed() >= Duration::from_secs(30) {
            let elapsed = start_time.elapsed().as_secs_f64();
            let fps = total_processed as f64 / elapsed;
            println!("Progress: {} files indexed | Speed: {:.2} files/sec", total_processed, fps);
            if let Err(e) = fs_common::report_scan_progress(&mut client, &session_id, total_processed as i64) {
                eprintln!("Progress update failed (non-fatal): {}", e);
            }
            last_report = Instant::now();
        }
    }
    if !batch.is_empty() {
        if let Err(e) = insert_batch(&mut client, &batch) {
            eprintln!("Final batch insert failed: {}", e);
            insert_failed = true;
        }
    }

    // Join the walker so we know whether it finished or panicked. This is the
    // gate that protects the destructive cleanup below.
    let walk_aborted = match producer.join() {
        Ok((skipped, lossy)) => {
            if skipped > 0 {
                println!("Walk completed with {} skipped/unreadable entries.", skipped);
            }
            if lossy > 0 {
                println!(
                    "{} entries have names that are not valid UTF-8; their exact bytes are in \
                     filesystem_index.path_raw and the `path` column shows a lossy rendering.",
                    lossy
                );
            }
            false
        }
        Err(_) => {
            eprintln!("Producer thread panicked; the walk did not complete.");
            true
        }
    };

    // Only prune stale entries if we are confident the index fully reflects the
    // current filesystem. Otherwise a partial scan would delete live entries
    // (a failed batch means its hashes never reached seen_hashes).
    if walk_aborted || insert_failed {
        eprintln!(
            "Scan did not complete cleanly (walk_aborted={}, insert_failed={}); \
             skipping stale-entry cleanup to protect the index.",
            walk_aborted, insert_failed
        );
        fs_common::end_scan_session(&mut client, &session_id, "failed", total_processed as i64)?;
        return Err("indexer scan incomplete; index left intact".into());
    }

    println!("Scan complete. Session {} closed.", session_id);

    println!("Updating identity mappings...");
    if let Err(e) = resolve_identities(&mut client) {
        eprintln!("Identity resolution failed: {}", e);
        fs_common::end_scan_session(&mut client, &session_id, "failed", total_processed as i64)?;
        return Err(e);
    }

    println!("Cleaning up files that no longer exist...");
    let deleted = match cleanup_stale_entries(&mut client) {
        Ok(n) => n,
        Err(e) => {
            eprintln!("Cleanup failed: {}", e);
            fs_common::end_scan_session(&mut client, &session_id, "failed", total_processed as i64)?;
            return Err(e);
        }
    };
    println!("Removed {} stale entries from the index.", deleted);

    if let Err(e) = create_query_indexes(&mut client) {
        eprintln!("Index creation failed: {}", e);
        fs_common::end_scan_session(&mut client, &session_id, "failed", total_processed as i64)?;
        return Err(e);
    }

    fs_common::end_scan_session(&mut client, &session_id, "success", total_processed as i64)?;

    Ok(())
}

/// Deletes rows whose path_hash was not seen during this scan. Anti-joining
/// against the seen_hashes temp table replaces the old per-row session-ID
/// stamp, which rewrote every row (and its index entries) on every rescan just
/// to mark it as still present.
fn cleanup_stale_entries(client: &mut Client) -> Result<u64, Box<dyn Error>> {
    // Give the planner real row counts for the anti-join.
    client.batch_execute("ANALYZE seen_hashes")?;
    let deleted = client.execute(
        "DELETE FROM filesystem_index WHERE NOT EXISTS (
             SELECT 1 FROM seen_hashes s WHERE s.path_hash = filesystem_index.path_hash)",
        &[],
    )?;
    Ok(deleted)
}

/// Ensures the secondary indexes the API's queries depend on:
/// parent_hash for /api/list, the rest for /api/search filters. Runs after the
/// scan (IF NOT EXISTS makes reruns free) so the initial bulk load doesn't pay
/// index maintenance on every inserted row; on later incremental scans only
/// changed rows touch these indexes.
fn create_query_indexes(client: &mut Client) -> Result<(), Box<dyn Error>> {
    println!("Ensuring query indexes exist (the first run may take a while)...");
    client.batch_execute(
        "CREATE INDEX IF NOT EXISTS idx_fsindex_parent_hash ON filesystem_index (parent_hash);
         CREATE INDEX IF NOT EXISTS idx_fsindex_uid ON filesystem_index (uid);
         CREATE INDEX IF NOT EXISTS idx_fsindex_gid ON filesystem_index (gid);
         CREATE INDEX IF NOT EXISTS idx_fsindex_size ON filesystem_index (size_bytes);
         CREATE INDEX IF NOT EXISTS idx_fsindex_mtime ON filesystem_index (mtime);"
    )?;

    // Trigram index serving the search API's contains/starts_with/regex path
    // filters. Requires the pg_trgm extension, which needs elevated DB
    // privileges to install — degrade to sequential-scan search if unavailable.
    if let Err(e) = client.batch_execute(
        "CREATE EXTENSION IF NOT EXISTS pg_trgm;
         CREATE INDEX IF NOT EXISTS idx_fsindex_path_trgm ON filesystem_index USING gin (path gin_trgm_ops);"
    ) {
        eprintln!(
            "pg_trgm path index not created ({}); substring/regex search will use sequential scans.",
            e
        );
    }
    Ok(())
}

/// Resolves unique UIDs and GIDs to usernames/groupnames and stores them in the identity_map.
fn resolve_identities(client: &mut Client) -> Result<(), Box<dyn Error>> {
    println!("Resolving unique UID/GID mappings...");

    let uids = client.query("SELECT DISTINCT uid FROM filesystem_index", &[])?;
    let mut user_count = 0;
    for row in uids {
        let uid: i32 = row.get(0);
        if let Some(user) = get_user_by_uid(uid as u32) {
            let name = user.name().to_string_lossy().into_owned();
            client.execute(
                "INSERT INTO identity_map (id, id_type, name) VALUES ($1, 'uid', $2) ON CONFLICT (id, id_type) DO NOTHING",
                &[&uid, &name],
            ).map_err(|e| { eprintln!("User resolution failed for UID {}: {}", uid, e); e })?;
            user_count += 1;
        }
    }
    println!("Resolved {} unique users.", user_count);

    let gids = client.query("SELECT DISTINCT gid FROM filesystem_index", &[])?;
    let mut group_count = 0;
    for row in gids {
        let gid: i32 = row.get(0);
        if let Some(group) = get_group_by_gid(gid as u32) {
            let name = group.name().to_string_lossy().into_owned();
            client.execute(
                "INSERT INTO identity_map (id, id_type, name) VALUES ($1, 'gid', $2) ON CONFLICT (id, id_type) DO NOTHING",
                &[&gid, &name],
            ).map_err(|e| { eprintln!("Group resolution failed for GID {}: {}", gid, e); e })?;
            group_count += 1;
        }
    }
    println!("Resolved {} unique groups.", group_count);

    Ok(())
}

/// Bulk-loads a batch of FileRecords: binary COPY into the staging table, then a
/// single set-based upsert into filesystem_index. This replaces per-row INSERTs
/// (one round-trip per row) with one COPY + one upsert per batch.
fn insert_batch(client: &mut Client, batch: &[FileRecord]) -> Result<(), Box<dyn Error>> {
    let mut transaction = client.transaction()?;

    {
        let sink = transaction.copy_in(
            "COPY staging_index (path, path_raw, path_hash, parent_hash, size_bytes, file_type, permissions, uid, gid, atime, mtime, ctime, metadata) FROM STDIN WITH (FORMAT binary)"
        )?;
        let mut writer = BinaryCopyInWriter::new(sink, &[
            Type::TEXT, Type::BYTEA, Type::BYTEA, Type::BYTEA, Type::INT8, Type::INT4,
            Type::TEXT, Type::INT4, Type::INT4, Type::INT8, Type::INT8,
            Type::INT8, Type::TEXT,
        ]);

        for r in batch {
            writer.write(&[
                &r.path,
                &r.path_raw,
                &r.path_hash,
                &r.parent_hash,
                &r.size_bytes,
                &r.file_type,
                &r.permissions,
                &(r.uid as i32),
                &(r.gid as i32),
                &r.atime,
                &r.mtime,
                &r.ctime,
                &r.metadata,
            ])?;
        }
        writer.finish()?;
    }

    // Record every hash seen this scan; stale-entry cleanup anti-joins against
    // this set at the end of the run.
    transaction.execute("INSERT INTO seen_hashes SELECT path_hash FROM staging_index", &[])?;

    // DISTINCT ON guards against a path appearing twice in one batch, which would
    // otherwise make ON CONFLICT error ("cannot affect row a second time").
    // The WHERE clause skips rows whose metadata is unchanged, so a rescan of a
    // mostly-unchanged filesystem produces almost no heap or index writes.
    transaction.execute(
        // `path` is deliberately absent from the DO UPDATE SET: it is a pure
        // function of the path the hash was taken over, so for a given
        // path_hash it cannot change. `path_raw` is a function of the same
        // thing and equally immutable in steady state, but it still has to be
        // updated (and compared) so that a database upgraded from a schema
        // without the column backfills it — the ALTER adds it as NULL, and
        // every pre-existing row would otherwise keep a NULL path_raw forever
        // while its name is genuinely not UTF-8. Once backfilled the
        // comparison is NULL-vs-NULL for almost every row and costs nothing.
        "INSERT INTO filesystem_index (path, path_raw, path_hash, parent_hash, size_bytes, file_type, permissions, uid, gid, atime, mtime, ctime, metadata) \
         SELECT DISTINCT ON (path_hash) path, path_raw, path_hash, parent_hash, size_bytes, file_type, permissions, uid, gid, atime, mtime, ctime, metadata \
         FROM staging_index ORDER BY path_hash \
         ON CONFLICT (path_hash) DO UPDATE SET \
            size_bytes = EXCLUDED.size_bytes, file_type = EXCLUDED.file_type, \
            permissions = EXCLUDED.permissions, uid = EXCLUDED.uid, gid = EXCLUDED.gid, \
            atime = EXCLUDED.atime, mtime = EXCLUDED.mtime, ctime = EXCLUDED.ctime, \
            parent_hash = EXCLUDED.parent_hash, path_raw = EXCLUDED.path_raw \
         WHERE (filesystem_index.size_bytes, filesystem_index.file_type, filesystem_index.permissions, \
                filesystem_index.uid, filesystem_index.gid, filesystem_index.atime, \
                filesystem_index.mtime, filesystem_index.ctime, filesystem_index.parent_hash, \
                filesystem_index.path_raw) \
               IS DISTINCT FROM \
               (EXCLUDED.size_bytes, EXCLUDED.file_type, EXCLUDED.permissions, \
                EXCLUDED.uid, EXCLUDED.gid, EXCLUDED.atime, EXCLUDED.mtime, \
                EXCLUDED.ctime, EXCLUDED.parent_hash, EXCLUDED.path_raw)",
        &[],
    )?;

    transaction.execute("TRUNCATE staging_index", &[])?;
    transaction.commit()?;
    Ok(())
}
