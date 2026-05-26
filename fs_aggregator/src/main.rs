use std::error::Error;
use std::collections::HashMap;
use std::path::Path;

/// Aggregated statistics for a directory.
struct DirNode {
    size: i64,
    mtime_first: i64,
    mtime_last: i64,
    atime_first: i64,
    atime_last: i64,
    ctime_first: i64,
    ctime_last: i64,
    count: i32,
}

fn main() -> Result<(), Box<dyn Error>> {
    // Load configuration from TOML file via shared library.
    let config = fs_common::load_config("fs_config.toml")?;
    let mut client = fs_common::get_db_client(&config.database)?;

    println!("Creating aggregation table...");
    client.batch_execute("\
        CREATE TABLE IF NOT EXISTS dir_stats (
            path_hash BYTEA PRIMARY KEY, 
            path TEXT,
            total_size_bytes BIGINT, 
            mtime_first BIGINT, 
            mtime_last BIGINT, 
            atime_first BIGINT, 
            atime_last BIGINT, 
            ctime_first BIGINT, 
            ctime_last BIGINT, 
            file_count INT
        );
    ").map_err(|e| { eprintln!("Failed to create dir_stats table: {}", e); e })?;

    println!("Fetching all files and directories from index...");
    
    // Crucial: fetch entries sorted by path length DESCENDING.
    // This enables the bottom-up DP approach: we process children before parents.
    let rows = client.query(
        "SELECT path, size_bytes, mtime, atime, ctime, file_type FROM filesystem_index ORDER BY length(path) DESC", 
        &[]
    ).map_err(|e| { eprintln!("Failed to fetch filesystem index: {}", e); e })?;

    // Temporary storage for computed aggregates.
    let mut aggregates: HashMap<String, DirNode> = HashMap::new();

    println!("Processing {} entries bottom-up...", rows.len());

    for row in rows {
        let path_str: String = row.get("path");
        let size: i64 = row.get("size_bytes");
        let mtime: i64 = row.get("mtime");
        let atime: i64 = row.get("atime");
        let ctime: i64 = row.get("ctime");
        let file_type: i32 = row.get("file_type");

        let mut current_size = size;
        let mut current_mtime_first = mtime;
        let mut current_mtime_last = mtime;
        let mut current_atime_first = atime;
        let mut current_atime_last = atime;
        let mut current_ctime_first = ctime;
        let mut current_ctime_last = ctime;
        let mut current_count = 1;

        // If this is a directory, it may already have accumulated values from its children.
        if file_type == 2 {
            if let Some(node) = aggregates.get(&path_str) {
                current_size += node.size;
                current_mtime_first = current_mtime_first.min(node.mtime_first);
                current_mtime_last = current_mtime_last.max(node.mtime_last);
                current_atime_first = current_atime_first.min(node.atime_first);
                current_atime_last = current_atime_last.max(node.atime_last);
                current_ctime_first = current_ctime_first.min(node.ctime_first);
                current_ctime_last = current_ctime_last.max(node.ctime_last);
                current_count += node.count;
            }
        }

        // Propagate these values up to the parent directory.
        let path_obj = Path::new(&path_str);
        if let Some(parent_path) = path_obj.parent() {
            let parent_str = parent_path.to_string_lossy().into_owned();
            if !parent_str.is_empty() && parent_str != "/" {
                let node = aggregates.entry(parent_str).or_insert(DirNode {
                    size: 0,
                    mtime_first: i64::MAX,
                    mtime_last: 0,
                    atime_first: i64::MAX,
                    atime_last: 0,
                    ctime_first: i64::MAX,
                    ctime_last: 0,
                    count: 0,
                });

                node.size += current_size;
                node.mtime_first = node.mtime_first.min(current_mtime_first);
                node.mtime_last = node.mtime_last.max(current_mtime_last);
                node.atime_first = node.atime_first.min(current_atime_first);
                node.atime_last = node.atime_last.max(current_atime_last);
                node.ctime_first = node.ctime_first.min(current_ctime_first);
                node.ctime_last = node.ctime_last.max(current_ctime_last);
                node.count += current_count;
            }
        }
    }

    println!("Writing aggregates to database...");
    let mut transaction = client.transaction()?;
    let stmt = transaction.prepare("\
        INSERT INTO dir_stats (path_hash, path, total_size_bytes, mtime_first, mtime_last, atime_first, atime_last, ctime_first, ctime_last, file_count) 
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) 
        ON CONFLICT (path_hash) 
        DO UPDATE SET 
            path = EXCLUDED.path,
            total_size_bytes = EXCLUDED.total_size_bytes, 
            mtime_first = EXCLUDED.mtime_first, 
            mtime_last = EXCLUDED.mtime_last, 
            atime_first = EXCLUDED.atime_first, 
            atime_last = EXCLUDED.atime_last, 
            ctime_first = EXCLUDED.ctime_first, 
            ctime_last = EXCLUDED.ctime_last, 
            file_count = EXCLUDED.file_count
    ").map_err(|e| { eprintln!("Failed to prepare aggregation statement: {}", e); e })?;

    for (path, node) in aggregates {
        let path_hash = fs_common::compute_hash(&path);
        if let Err(e) = transaction.execute(&stmt, &[
            &path_hash, 
            &path, 
            &node.size, 
            &node.mtime_first, 
            &node.mtime_last, 
            &node.atime_first, 
            &node.atime_last, 
            &node.ctime_first, 
            &node.ctime_last, 
            &node.count
        ]) {
            eprintln!("Failed to insert stats for path {}: {}", path, e);
        }
    }

    transaction.commit()?;
    println!("Aggregation complete. Path hashes used for dir_stats.");
    Ok(())
}
