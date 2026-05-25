use std::env;
use std::error::Error;
use std::os::unix::fs::MetadataExt;
use std::thread;
use std::time::{Duration, Instant};
use ignore::WalkBuilder;
use postgres::{Client, NoTls};
use crossbeam_channel::unbounded;
use sha2::{Sha256, Digest};
use uuid::Uuid;
use users::{get_user_by_uid, get_group_by_gid};

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

fn compute_hash(path: &str) -> Vec<u8> {
    let mut hasher = Sha256::new();
    hasher.update(path.as_bytes());
    hasher.finalize().to_vec()
}

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

    let session_id = Uuid::new_v4().to_string();
    println!("Starting scan session: {}", session_id);

    let (tx, rx) = unbounded();

    let root_path_clone = root_path.clone();
    let sid_for_thread = session_id.clone();
    thread::spawn(move || {
        for entry in WalkBuilder::new(root_path_clone).threads(threads).build().flatten() {
            if let Ok(meta) = entry.metadata() {
                let path = entry.path().to_string_lossy().into_owned();
                let path_hash = compute_hash(&path);
                let parent_path = get_parent_path(&path);
                let parent_hash = parent_path.as_ref().map(|p| compute_hash(p));

                let mode = meta.mode();
                let file_type = match mode & 0o170000 {
                    0o100000 => 1,
                    0o040000 => 2,
                    0o120000 => 3,
                    _ => 0,
                };

                if let Err(e) = tx.send(FileRecord {
                    path,
                    path_hash,
                    parent_hash,
                    size_bytes: meta.len() as i64,
                    file_type,
                    permissions: format!("{:o}", mode & 0o777),
                    uid: meta.uid(),
                    gid: meta.gid(),
                    atime: meta.atime() as i64,
                    mtime: meta.mtime() as i64,
                    ctime: meta.ctime() as i64,
                    metadata: "".to_string(),
                    session_id: sid_for_thread.clone(),
                }) {
                    eprintln!("Worker thread failed to send record: {}", e);
                    break;
                }
            }
        }
    });

    let mut client = Client::connect("host=localhost user=postgres password=postgres dbname=fs_index", NoTls)?;
    
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
    )?;

    client.execute(
        "CREATE TABLE IF NOT EXISTS scan_sessions (
            session_id TEXT PRIMARY KEY, 
            started_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP, 
            ended_at TIMESTAMP
        )", 
        &[]
    )?;

    client.execute("ALTER TABLE filesystem_index ADD COLUMN IF NOT EXISTS last_seen_session TEXT", &[])?;

    client.execute(
        "CREATE TABLE IF NOT EXISTS identity_map (
            id INTEGER, 
            id_type TEXT CHECK (id_type IN ('uid', 'gid')), 
            name TEXT NOT NULL,
            PRIMARY KEY (id, id_type)
        )", 
        &[]
    )?;
    
    println!("Recording session start in DB...");
    client.execute("INSERT INTO scan_sessions (session_id) VALUES ($1)", &[&session_id])?;
    println!("Session {} recorded in database.", session_id);
    
    let mut batch = Vec::with_capacity(1000);
    let mut total_processed = 0u64;
    let start_time = Instant::now();
    let mut last_report = Instant::now();

    while let Ok(record) = rx.recv() {
        batch.push(record);
        total_processed += 1;

        if batch.len() >= 1000 {
            insert_batch(&mut client, &batch)?;
            batch.clear();
        }

        if last_report.elapsed() >= Duration::from_secs(30) {
            let elapsed = start_time.elapsed().as_secs_f64();
            let fps = total_processed as f64 / elapsed;
            println!("Progress: {} files indexed | Speed: {:.2} files/sec", total_processed, fps);
            last_report = Instant::now();
        }
    }
    if !batch.is_empty() {
        insert_batch(&mut client, &batch)?;
    }

    client.execute("UPDATE scan_sessions SET ended_at = CURRENT_TIMESTAMP WHERE session_id = $1", &[&session_id])?;
    println!("Scan complete. Session {} closed.", session_id);
    
    println!("Updating identity mappings...");
    resolve_identities(&mut client)?;

    println!("Cleaning up files that no longer exist...");
    let deleted = client.execute(
        "DELETE FROM filesystem_index WHERE last_seen_session != $1", 
        &[&session_id]
    )?;
    println!("Removed {} stale entries from the index.", deleted);

    Ok(())
}

fn resolve_identities(client: &mut Client) -> Result<(), Box<dyn Error>> {
    println!("Resolving unique UID/GID mappings...");
    
    // Resolve Users
    let uids = client.query("SELECT DISTINCT uid FROM filesystem_index", &[])?;
    let mut user_count = 0;
    for row in uids {
        let uid: i32 = row.get(0);
        if let Some(user) = get_user_by_uid(uid as u32) {
            let name = user.name().to_string_lossy().into_owned();
            client.execute(
                "INSERT INTO identity_map (id, id_type, name) VALUES ($1, 'uid', $2) ON CONFLICT (id, id_type) DO NOTHING",
                &[&uid, &name],
            )?;
            user_count += 1;
        }
    }
    println!("Resolved {} unique users.", user_count);

    // Resolve Groups
    let gids = client.query("SELECT DISTINCT gid FROM filesystem_index", &[])?;
    let mut group_count = 0;
    for row in gids {
        let gid: i32 = row.get(0);
        if let Some(group) = get_group_by_gid(gid as u32) {
            let name = group.name().to_string_lossy().into_owned();
            client.execute(
                "INSERT INTO identity_map (id, id_type, name) VALUES ($1, 'gid', $2) ON CONFLICT (id, id_type) DO NOTHING",
                &[&gid, &name],
            )?;
            group_count += 1;
        }
    }
    println!("Resolved {} unique groups.", group_count);

    Ok(())
}

fn insert_batch(client: &mut Client, batch: &[FileRecord]) -> Result<(), Box<dyn Error>> {
    let mut transaction = client.transaction()?;
    
    let stmt = transaction.prepare(
        "INSERT INTO filesystem_index (path, path_hash, parent_hash, size_bytes, file_type, permissions, uid, gid, atime, mtime, ctime, metadata, last_seen_session) 
         VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13) 
         ON CONFLICT (path_hash) DO UPDATE SET 
         size_bytes = EXCLUDED.size_bytes, file_type = EXCLUDED.file_type, 
         permissions = EXCLUDED.permissions, mtime = EXCLUDED.mtime, 
         parent_hash = EXCLUDED.parent_hash, last_seen_session = EXCLUDED.last_seen_session"
    )?;

    for r in batch {
        transaction.execute(&stmt, &[
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
            &r.session_id
        ])?;
    }
    transaction.commit()?;
    Ok(())
}
