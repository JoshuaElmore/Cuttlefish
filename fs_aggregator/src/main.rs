use postgres::{Client, NoTls};
use std::error::Error;
use std::collections::HashMap;
use std::path::Path;
use sha2::{Sha256, Digest};

struct DirNode {
    size: i64,
    mtime: i64,
    atime: i64,
    count: i32,
}

fn calculate_hash(path: &str) -> Vec<u8> {
    let mut hasher = Sha256::new();
    hasher.update(path.as_bytes());
    hasher.finalize().to_vec()
}

fn main() -> Result<(), Box<dyn Error>> {
    let mut client = Client::connect("host=localhost user=postgres password=postgres dbname=fs_index", NoTls)?;

    println!("Creating aggregation table...");
    client.batch_execute("
        CREATE TABLE IF NOT EXISTS dir_stats (
            path_hash BYTEA PRIMARY KEY, 
            path TEXT,
            total_size_bytes BIGINT, 
            last_modified BIGINT, 
            last_accessed BIGINT, 
            file_count INT
        );
    ")?;

    println!("Fetching all files and directories from index...");
    
    // We fetch all data sorted by path length DESCENDING.
    let rows = client.query(
        "SELECT path, size_bytes, mtime, atime, file_type FROM filesystem_index ORDER BY length(path) DESC", 
        &[]
    )?;

    // We store nodes by their path string, but we will write them to the DB by their hash
    let mut aggregates: HashMap<String, DirNode> = HashMap::new();

    println!("Processing {} entries bottom-up...", rows.len());

    for row in rows {
        let path_str: String = row.get("path");
        let size: i64 = row.get("size_bytes");
        let mtime: i64 = row.get("mtime");
        let atime: i64 = row.get("atime");
        let file_type: i32 = row.get("file_type");

        let mut current_size = size;
        let mut current_mtime = mtime;
        let mut current_atime = atime;
        let mut current_count = 1;

        if file_type == 2 {
            if let Some(node) = aggregates.get(&path_str) {
                current_size += node.size;
                current_mtime = current_mtime.max(node.mtime);
                current_atime = current_atime.max(node.atime);
                current_count += node.count;
            }
        }

        let path_obj = Path::new(&path_str);
        if let Some(parent_path) = path_obj.parent() {
            let parent_str = parent_path.to_string_lossy().into_owned();
            if !parent_str.is_empty() && parent_str != "/" {
                let node = aggregates.entry(parent_str).or_insert(DirNode {
                    size: 0,
                    mtime: 0,
                    atime: 0,
                    count: 0,
                });

                node.size += current_size;
                node.mtime = node.mtime.max(current_mtime);
                node.atime = node.atime.max(current_atime);
                node.count += current_count;
            }
        }
    }

    println!("Writing aggregates to database...");
    let mut transaction = client.transaction()?;
    let stmt = transaction.prepare("
        INSERT INTO dir_stats (path_hash, path, total_size_bytes, last_modified, last_accessed, file_count) 
        VALUES ($1, $2, $3, $4, $5, $6) 
        ON CONFLICT (path_hash) 
        DO UPDATE SET 
            path = EXCLUDED.path,
            total_size_bytes = EXCLUDED.total_size_bytes, 
            last_modified = EXCLUDED.last_modified, 
            last_accessed = EXCLUDED.last_accessed, 
            file_count = EXCLUDED.file_count
    ")?;

    for (path, node) in aggregates {
        let path_hash = calculate_hash(&path);
        transaction.execute(&stmt, &[
            &path_hash, 
            &path, 
            &node.size, 
            &node.mtime, 
            &node.atime, 
            &node.count
        ])?;
    }

    transaction.commit()?;
    println!("Aggregation complete. Path hashes used for dir_stats.");
    Ok(())
}
