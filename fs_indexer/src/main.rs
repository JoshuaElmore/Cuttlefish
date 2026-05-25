use std::env;
use std::error::Error;
use std::os::unix::fs::MetadataExt;
use std::thread;
use ignore::WalkBuilder;
use postgres::{Client, NoTls};
use crossbeam_channel::unbounded;
use sha2::{Sha256, Digest};

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

    let (tx, rx) = unbounded();

    // Walker thread
    let root_path_clone = root_path.clone();
    thread::spawn(move || {
        // .build() returns a Walk, .flatten() turns it into an iterator of DirEntry
        for entry in WalkBuilder::new(root_path_clone).threads(threads).build().flatten() {
            // The flatten() iterator yields DirEntry directly, NOT Result<DirEntry, Error>
            let entry = entry; 
            if let Ok(meta) = entry.metadata() {
                let path = entry.path().to_string_lossy().into_owned();
                let path_hash = compute_hash(&path);
                let parent_path = get_parent_path(&path);
                let parent_hash = parent_path.as_ref().map(|p| compute_hash(p));

                let mode = meta.mode();
                let file_type = match mode & 0o170000 {
                    0o100000 => 1, // Regular
                    0o040000 => 2, // Directory
                    0o120000 => 3, // Symlink
                    _ => 0,
                };

                tx.send(FileRecord {
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
                    metadata: "{}".to_string(),
                }).unwrap();
            }
        }
    });

    // DB Writer thread
    let mut client = Client::connect("host=localhost user=postgres password=postgres dbname=fs_index", NoTls)?;
    let mut batch = Vec::with_capacity(1000);
    
    while let Ok(record) = rx.recv() {
        batch.push(record);
        if batch.len() >= 1000 {
            insert_batch(&mut client, &batch)?;
            batch.clear();
        }
    }
    if !batch.is_empty() {
        insert_batch(&mut client, &batch)?;
    }

    Ok(())
}

fn insert_batch(client: &mut Client, batch: &[FileRecord]) -> Result<(), Box<dyn Error>> {
    let mut transaction = client.transaction()?;
    let stmt = transaction.prepare(
        "INSERT INTO filesystem_index (path, path_hash, parent_hash, size_bytes, file_type, permissions, uid, gid, atime, mtime, ctime, metadata) 
         VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) 
         ON CONFLICT (path_hash) DO UPDATE SET 
         size_bytes = EXCLUDED.size_bytes, file_type = EXCLUDED.file_type, 
         permissions = EXCLUDED.permissions, mtime = EXCLUDED.mtime, parent_hash = EXCLUDED.parent_hash"
    )?;

    for r in batch {
        transaction.execute(&stmt, &[
            &r.path, &r.path_hash, &r.parent_hash, &r.size_bytes, &r.file_type, 
            &r.permissions, &(r.uid as i32), &(r.gid as i32), &r.atime, &r.mtime, &r.ctime, &r.metadata
        ])?;
    }
    transaction.commit()?;
    Ok(())
}
