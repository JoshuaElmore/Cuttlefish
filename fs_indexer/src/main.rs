use std::env;
use std::error::Error;
use std::os::unix::fs::MetadataExt;
use std::thread;
use ignore::WalkBuilder;
use postgres::{Client, NoTls};
use crossbeam_channel::unbounded;
use serde_json::{Value, Map};

struct FileRecord {
    path: String,
    size: u64,
    file_type: i32,
    permissions: String,
    uid: u32,
    gid: u32,
    atime: i64,
    mtime: i64,
    ctime: i64,
    extra_metadata: String,
}

fn get_file_type_int(mode: u32) -> i32 {
    match mode & 0o170000 {
        0o100000 => 1, // Regular File
        0o040000 => 2, // Directory
        0o120000 => 3, // Symlink
        0o014000 => 4, // FIFO
        0o012000 => 5, // Char Device
        0o016000 => 6, // Block Device
        0o010000 => 7, // Socket
        _ => 0,        // Unknown
    }
}

fn collect_xattrs(path: &std::path::Path) -> String {
    let mut map = Map::new();
    if let Ok(attrs) = xattr::list(path) {
        for attr in attrs {
            if let Ok(Some(val)) = xattr::get(path, &attr) {
                let val_str = if let Ok(s) = String::from_utf8(val.clone()) {
                    s
                } else {
                    format!("0x{:x?}", val)
                };
                map.insert(attr.to_string_lossy().into_owned(), Value::String(val_str));
            }
        }
    }
    serde_json::to_string(&Value::Object(map)).unwrap_or_else(|_| "{}".to_string())
}

fn main() -> Result<(), Box<dyn Error>> {
    let args: Vec<String> = env::args().collect();
    if args.len() < 2 {
        eprintln!("Usage: {} <path_to_scan> [threads] [--all-metadata]", args[0]);
        std::process::exit(1);
    }

    let scan_path_raw = &args[1];
    let scan_path = std::fs::canonicalize(scan_path_raw)?;
    
    let mut pull_all_metadata = false;
    let mut thread_count_arg = None;

    for arg in args.iter().skip(2) {
        if arg == "--all-metadata" {
            pull_all_metadata = true;
        } else if let Ok(n) = arg.parse::<usize>() {
            thread_count_arg = Some(n);
        }
    }

    let num_threads = thread_count_arg.unwrap_or_else(num_cpus::get);

    println!("Scanning absolute path: {:?} with {} threads...", scan_path, num_threads);
    if pull_all_metadata {
        println!("Extended metadata collection enabled.");
    }

    let (tx, rx) = unbounded::<FileRecord>();

    // DB Writer Thread
    let writer_handle = thread::spawn(move || -> Result<(), Box<dyn Error + Send + Sync>> {
        let mut client = Client::connect("host=localhost user=postgres password=postgres dbname=fs_index", NoTls)?;
        
        let mut count = 0;
        // Since we removed inode, the absolute path is now the unique identifier
        let upsert_sql = "
            INSERT INTO filesystem_index (path, size_bytes, file_type, permissions, uid, gid, atime, mtime, ctime, metadata) 
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) 
            ON CONFLICT (path) 
            DO UPDATE SET 
                size_bytes = EXCLUDED.size_bytes,
                file_type = EXCLUDED.file_type,
                permissions = EXCLUDED.permissions,
                uid = EXCLUDED.uid,
                gid = EXCLUDED.gid,
                atime = EXCLUDED.atime,
                mtime = EXCLUDED.mtime,
                ctime = EXCLUDED.ctime,
                metadata = EXCLUDED.metadata
        ";
        let stmt = client.prepare(upsert_sql)?;

        let mut transaction = client.transaction()?;

        for record in rx {
            transaction.execute(&stmt, &[
                &record.path, 
                &(record.size as i64),
                &record.file_type, 
                &record.permissions, 
                &(record.uid as i32), 
                &(record.gid as i32),
                &record.atime, 
                &record.mtime, 
                &record.ctime,
                &record.extra_metadata
            ])?;
            count += 1;
            
            if count % 1000 == 0 {
                transaction.commit()?;
                transaction = client.transaction()?;
            }
        }
        transaction.commit()?;
        Ok(())
    });

    let walker = WalkBuilder::new(&scan_path)
        .threads(num_threads)
        .standard_filters(false)
        .build_parallel();

    walker.run(|| {
        let tx = tx.clone();
        let pull_all = pull_all_metadata;
        Box::new(move |entry| {
            if let Ok(entry) = entry {
                if let Ok(meta) = entry.metadata() {
                    let mode = meta.mode();
                    
                    let extra_metadata = if pull_all {
                        collect_xattrs(entry.path())
                    } else {
                        "{}".to_string()
                    };

                    let record = FileRecord {
                        path: entry.path().to_string_lossy().into_owned(),
                        size: meta.len(),
                        file_type: get_file_type_int(mode),
                        permissions: format!("{:o}", mode & 0o777),
                        uid: meta.uid(),
                        gid: meta.gid(),
                        atime: meta.atime(),
                        mtime: meta.mtime(),
                        ctime: meta.ctime(),
                        extra_metadata,
                    };
                    let _ = tx.send(record);
                }
            }
            ignore::WalkState::Continue
        })
    });

    drop(tx);
    writer_handle.join().expect("Writer thread panicked").expect("Writer error");

    println!("Indexing complete. Absolute paths used as unique IDs.");
    Ok(())
}

mod num_cpus {
    pub fn get() -> usize {
        std::thread::available_parallelism().map(|n| n.get()).unwrap_or(4)
    }
}
