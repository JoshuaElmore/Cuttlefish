use postgres::{Client, NoTls};
use std::error::Error;

fn main() -> Result<(), Box<dyn Error>> {
    let mut client = Client::connect("host=localhost user=postgres password=postgres dbname=fs_index", NoTls)?;

    println!("Creating aggregation table...");
    
    client.batch_execute("
        CREATE TABLE IF NOT EXISTS dir_stats (
            path TEXT PRIMARY KEY,
            total_size_bytes BIGINT,
            last_modified BIGINT,
            last_accessed BIGINT,
            file_count INT
        );
    ")?;

    println!("Calculating aggregates...");

    // Get all directories
    let dirs = client.query("SELECT path FROM filesystem_index WHERE file_type = 2", &[])?;

    let mut total_processed = 0;
    for dir_row in dirs {
        let dir_path: String = dir_row.get(0);
        let search_path = format!("{}/%", dir_path);
        
        // Use COALESCE to ensure we get 0 instead of NULL if a directory is empty
        let stats = client.query_one(
            "SELECT 
                COALESCE(sum(size_bytes), 0) as total_size, 
                COALESCE(max(mtime), 0) as max_mtime, 
                COALESCE(max(atime), 0) as max_atime, 
                count(*) as f_count 
             FROM filesystem_index 
             WHERE path LIKE $1", 
            &[&search_path]
        )?;

        // Postgres SUM returns numeric/decimal for big sums, 
        // we cast to i64 in the query or use a flexible type.
        // Actually, size_bytes is BIGINT, so sum is numeric.
        // Using get::<_, rust_decimal::Decimal> or just casting in SQL.
        
        // Let's refine the query to cast explicitly to BIGINT
        let stats = client.query_one(
            "SELECT 
                COALESCE(sum(size_bytes)::BIGINT, 0) as total_size, 
                COALESCE(max(mtime), 0) as max_mtime, 
                COALESCE(max(atime), 0) as max_atime, 
                count(*)::INT as f_count 
             FROM filesystem_index 
             WHERE path LIKE $1", 
            &[&search_path]
        )?;

        let total_size: i64 = stats.get("total_size");
        let max_mtime: i64 = stats.get("max_mtime");
        let max_atime: i64 = stats.get("max_atime");
        let f_count: i32 = stats.get("f_count");

        client.execute(
            "INSERT INTO dir_stats (path, total_size_bytes, last_modified, last_accessed, file_count) 
             VALUES ($1, $2, $3, $4, $5) 
             ON CONFLICT (path) 
             DO UPDATE SET 
                total_size_bytes = EXCLUDED.total_size_bytes, 
                last_modified = EXCLUDED.last_modified, 
                last_accessed = EXCLUDED.last_accessed, 
                file_count = EXCLUDED.file_count",
            &[&dir_path, &total_size, &max_mtime, &max_atime, &f_count]
        )?;

        total_processed += 1;
        if total_processed % 100 == 0 {
            println!("Processed {} directories...", total_processed);
        }
    }

    println!("Aggregation complete. Results stored in `dir_stats` table.");
    Ok(())
}
