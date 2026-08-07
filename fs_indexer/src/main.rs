use std::env;
use std::error::Error;
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
const CHANNEL_CAPACITY: usize = 50_000;

/// Number of records bulk-loaded per flush. Larger batches amortise the
/// COPY + upsert round-trip; small enough that the in-flight batch is cheap.
const BATCH_SIZE: usize = 5_000;

/// Metadata record for a single filesystem entry.
struct FileRecord {
    path: String,
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
    session_id: String,
}

/// Determines the parent path of a given path.
/// Returns None for the root directory.
fn get_parent_path(path: &str) -> Option<String> {
    if path == "/" {
        return None;
    }
    let trimmed = path.trim_end_matches('/');
    if let Some(idx) = trimmed.rfind('/') {
        let parent = &trimmed[..idx];
        return if parent.is_empty() { Some("/".to_string()) } else { Some(parent.to_string()) };
    }
    None
}

fn main() -> Result<(), Box<dyn Error>> {
    let args: Vec<String> = env::args().collect();
    if args.len() < 2 {
        eprintln!("Usage: fs_indexer <path> [threads]");
        std::process::exit(1);
    }

    let root_path = args[1].clone();
    let threads = if args.len() > 2 {
        args[2].parse::<usize>().unwrap_or(8)
    } else {
        8
    };

    // Unique ID for this scan session to track stale entries.
    let session_id = Uuid::new_v4().to_string();
    println!("Starting scan session: {}", session_id);

    // Bounded channel: when full, the producer blocks instead of buffering the
    // entire filesystem in RAM.
    let (tx, rx) = bounded(CHANNEL_CAPACITY);

    let root_path_clone = root_path.clone();
    let sid_for_thread = session_id.clone();

    // Producer thread: walks the filesystem and sends metadata to the consumer.
    // Returns the number of entries it had to skip (unreadable dirs/files) so the
    // caller can report them. A panic here is detected via join() below and blocks
    // the stale-entry cleanup, so a crashed walk can never wipe the index.
    let producer = thread::spawn(move || -> u64 {
        let mut skipped: u64 = 0;
        for result in WalkBuilder::new(root_path_clone).threads(threads).hidden(false).build() {
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

            let path = entry.path().to_string_lossy().into_owned();
            let path_hash = fs_common::compute_hash(&path);
            let parent_path = get_parent_path(&path);
            let parent_hash = parent_path.as_ref().map(|p| fs_common::compute_hash(p));

            let mode = meta.mode();
            let file_type = match mode & 0o170000 {
                0o100000 => 1, // Regular file
                0o040000 => 2, // Directory
                0o120000 => 3, // Symbolic link
                _ => 0,
            };

            if tx.send(FileRecord {
                path,
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
                session_id: sid_for_thread.clone(),
            }).is_err() {
                eprintln!("Consumer gone; stopping walk.");
                break;
            }
        }
        skipped
    });

    // Database setup
    let config = fs_common::load_config("fs_config.yml")?;
    let mut client = fs_common::get_db_client(&config.database)?;

    // Initialize schema
    client.execute(
        "CREATE TABLE IF NOT EXISTS filesystem_index (
            path TEXT,
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
            metadata TEXT,
            last_seen_session TEXT
        )",
        &[]
    ).map_err(|e| { eprintln!("Schema creation failed: {}", e); e })?;

    fs_common::ensure_scan_sessions_table(&mut client)
        .map_err(|e| { eprintln!("Session table creation failed: {}", e); e })?;

    client.execute("ALTER TABLE filesystem_index ADD COLUMN IF NOT EXISTS last_seen_session TEXT", &[])?;

    // Speeds up the stale-entry cleanup scan at the end of large runs.
    client.execute("CREATE INDEX IF NOT EXISTS idx_fsindex_last_seen ON filesystem_index (last_seen_session)", &[])?;

    client.execute(
        "CREATE TABLE IF NOT EXISTS identity_map (
            id INTEGER,
            id_type TEXT CHECK (id_type IN ('uid', 'gid')),
            name TEXT NOT NULL,
            PRIMARY KEY (id, id_type)
        )",
        &[]
    ).map_err(|e| { eprintln!("Identity map creation failed: {}", e); e })?;

    // Session-scoped staging table for bulk upserts. LIKE picks up every column
    // of filesystem_index (including last_seen_session added just above).
    client.batch_execute(
        "CREATE TEMP TABLE staging_index (LIKE filesystem_index INCLUDING DEFAULTS); TRUNCATE staging_index;"
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
        Ok(skipped) => {
            if skipped > 0 {
                println!("Walk completed with {} skipped/unreadable entries.", skipped);
            }
            false
        }
        Err(_) => {
            eprintln!("Producer thread panicked; the walk did not complete.");
            true
        }
    };

    // Only prune stale entries if we are confident the index fully reflects the
    // current filesystem. Otherwise a partial scan would delete live entries.
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
    let deleted = match client.execute(
        "DELETE FROM filesystem_index WHERE last_seen_session != $1",
        &[&session_id]
    ) {
        Ok(n) => n,
        Err(e) => {
            eprintln!("Cleanup failed: {}", e);
            fs_common::end_scan_session(&mut client, &session_id, "failed", total_processed as i64)?;
            return Err(e.into());
        }
    };
    println!("Removed {} stale entries from the index.", deleted);

    fs_common::end_scan_session(&mut client, &session_id, "success", total_processed as i64)?;

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
            "COPY staging_index (path, path_hash, parent_hash, size_bytes, file_type, permissions, uid, gid, atime, mtime, ctime, metadata, last_seen_session) FROM STDIN WITH (FORMAT binary)"
        )?;
        let mut writer = BinaryCopyInWriter::new(sink, &[
            Type::TEXT, Type::BYTEA, Type::BYTEA, Type::INT8, Type::INT4,
            Type::TEXT, Type::INT4, Type::INT4, Type::INT8, Type::INT8,
            Type::INT8, Type::TEXT, Type::TEXT,
        ]);

        for r in batch {
            writer.write(&[
                &r.path,
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
                &r.session_id,
            ])?;
        }
        writer.finish()?;
    }

    // DISTINCT ON guards against a path appearing twice in one batch, which would
    // otherwise make ON CONFLICT error ("cannot affect row a second time").
    transaction.execute(
        "INSERT INTO filesystem_index (path, path_hash, parent_hash, size_bytes, file_type, permissions, uid, gid, atime, mtime, ctime, metadata, last_seen_session) \
         SELECT DISTINCT ON (path_hash) path, path_hash, parent_hash, size_bytes, file_type, permissions, uid, gid, atime, mtime, ctime, metadata, last_seen_session \
         FROM staging_index ORDER BY path_hash \
         ON CONFLICT (path_hash) DO UPDATE SET \
            size_bytes = EXCLUDED.size_bytes, file_type = EXCLUDED.file_type, \
            permissions = EXCLUDED.permissions, mtime = EXCLUDED.mtime, \
            parent_hash = EXCLUDED.parent_hash, last_seen_session = EXCLUDED.last_seen_session",
        &[],
    )?;

    transaction.execute("TRUNCATE staging_index", &[])?;
    transaction.commit()?;
    Ok(())
}
