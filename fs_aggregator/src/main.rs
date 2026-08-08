use std::error::Error;
use std::collections::HashMap;
use std::path::Path;

use fallible_iterator::FallibleIterator;
use postgres::binary_copy::BinaryCopyInWriter;
use postgres::types::{ToSql, Type};
use postgres::Client;

/// Aggregated statistics for a directory.
struct DirNode {
    size: i64,
    mtime_first: i64,
    mtime_last: i64,
    atime_first: i64,
    atime_last: i64,
    ctime_first: i64,
    ctime_last: i64,
    count: i64,
}

fn main() -> Result<(), Box<dyn Error>> {
    // Load configuration from TOML file via shared library.
    let config = fs_common::load_config("fs_config.yml")?;
    let mut client = fs_common::get_db_client(&config.database)?;

    fs_common::ensure_scan_sessions_table(&mut client)?;
    let session_id = fs_common::new_session_id();
    fs_common::record_scan_start(&mut client, &session_id, "aggregator")?;
    println!("Starting aggregator session: {}", session_id);

    match run_aggregation(&mut client) {
        Ok(processed) => {
            fs_common::end_scan_session(&mut client, &session_id, "success", processed as i64)?;
            Ok(())
        }
        Err(e) => {
            eprintln!("Aggregation failed: {}", e);
            // Best-effort: don't let a failure recording the failure mask the original error.
            let _ = fs_common::end_scan_session(&mut client, &session_id, "failed", 0);
            Err(e)
        }
    }
}

/// Runs the full aggregation pass and returns the number of filesystem_index
/// entries it processed. Split out from main() so the scan_sessions bookkeeping
/// above can record success/failure around a single call.
fn run_aggregation(client: &mut Client) -> Result<u64, Box<dyn Error>> {
    println!("Removing /proc/kcore from index...");
    // Delete by path_hash so this hits the primary key instead of scanning the
    // unindexed path column.
    let kcore_hash = fs_common::compute_hash("/proc/kcore");
    client.execute("DELETE FROM filesystem_index WHERE path_hash = $1", &[&kcore_hash]).map_err(|e| {
        eprintln!("Failed to remove /proc/kcore: {}", e);
        e
    })?;

    println!("Creating aggregation tables...");
    client.batch_execute(" \
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
            file_count BIGINT
        );
        CREATE TABLE IF NOT EXISTS user_stats (
            id_type TEXT, -- 'uid' or 'gid'
            id_value INT,
            total_size_bytes BIGINT,
            file_count BIGINT,
            PRIMARY KEY (id_type, id_value)
        );
    ").map_err(|e| { eprintln!("Failed to create stats tables: {}", e); e })?;

    // Everything from here runs in a single transaction: we read a consistent
    // snapshot of the index, wipe the old aggregates, and bulk-load the new ones.
    // If anything fails the transaction rolls back, leaving the previous stats
    // intact rather than half-rebuilt.
    let mut transaction = client.transaction()?;

    // Temporary storage for computed aggregates.
    let mut aggregates: HashMap<String, DirNode> = HashMap::new();
    let mut user_aggregates: HashMap<(String, i32), (i64, i64)> = HashMap::new();

    println!("Streaming files and directories from index...");

    // Stream the index in whatever order Postgres produces it — no ORDER BY.
    // Each entry folds its own stats directly into every ancestor directory
    // (O(path depth) map updates per row). This yields the same recursive
    // totals as a children-before-parents DP, without forcing the server to
    // sort the entire table by length(path) first — at hundreds of millions of
    // rows that's a huge disk-spilling external sort before the first row can
    // stream.
    //
    // query_raw streams rows lazily through a portal instead of buffering the
    // entire index in memory, so peak memory is bounded by the number of
    // directories (the `aggregates` map), not the number of files.
    let no_params: [&(dyn ToSql + Sync); 0] = [];
    let mut processed: u64 = 0;
    {
        let mut rows = transaction.query_raw(
            "SELECT path, size_bytes, mtime, atime, ctime, uid, gid FROM filesystem_index WHERE file_type IN (1, 2)",
            no_params,
        ).map_err(|e| { eprintln!("Failed to query filesystem index: {}", e); e })?;

        while let Some(row) = rows.next().map_err(|e| { eprintln!("Failed to fetch row: {}", e); e })? {
            let path_str: String = row.get("path");
            let size: i64 = row.get("size_bytes");
            let mtime: i64 = row.get("mtime");
            let atime: i64 = row.get("atime");
            let ctime: i64 = row.get("ctime");
            let uid: i32 = row.get("uid");
            let gid: i32 = row.get("gid");

            // Track user/group stats.
            let u_stat = user_aggregates.entry(("uid".to_string(), uid)).or_insert((0, 0));
            u_stat.0 += size;
            u_stat.1 += 1;

            let g_stat = user_aggregates.entry(("gid".to_string(), gid)).or_insert((0, 0));
            g_stat.0 += size;
            g_stat.1 += 1;

            // Fold this entry's stats into every ancestor directory. A
            // directory's aggregates cover its strict descendants, so the entry
            // contributes to each ancestor but not to itself; "/" gets no
            // dir_stats row, matching the previous behaviour.
            for ancestor in Path::new(&path_str).ancestors().skip(1) {
                let anc = match ancestor.to_str() {
                    Some(s) if !s.is_empty() && s != "/" => s,
                    _ => break,
                };
                match aggregates.get_mut(anc) {
                    Some(node) => {
                        node.size += size;
                        node.count += 1;
                        node.mtime_first = node.mtime_first.min(mtime);
                        node.mtime_last = node.mtime_last.max(mtime);
                        node.atime_first = node.atime_first.min(atime);
                        node.atime_last = node.atime_last.max(atime);
                        node.ctime_first = node.ctime_first.min(ctime);
                        node.ctime_last = node.ctime_last.max(ctime);
                    }
                    None => {
                        aggregates.insert(anc.to_string(), DirNode {
                            size,
                            mtime_first: mtime,
                            mtime_last: mtime,
                            atime_first: atime,
                            atime_last: atime,
                            ctime_first: ctime,
                            ctime_last: ctime,
                            count: 1,
                        });
                    }
                }
            }

            processed += 1;
        }
    }

    println!("Processed {} entries; rebuilding {} directory and {} user/group rows...",
        processed, aggregates.len(), user_aggregates.len());

    // Wipe the previous aggregates so directories/users that no longer exist are
    // dropped rather than left behind as stale rows. TRUNCATE here is part of the
    // transaction, so readers keep seeing the old stats until we commit.
    // The ALTER statements migrate older databases whose file_count columns were
    // created as INT; on an already-truncated table this is effectively free.
    transaction.batch_execute(" \
        TRUNCATE TABLE dir_stats, user_stats; \
        ALTER TABLE dir_stats ALTER COLUMN file_count TYPE BIGINT; \
        ALTER TABLE user_stats ALTER COLUMN file_count TYPE BIGINT; \
    ").map_err(|e| { eprintln!("Failed to reset stats tables: {}", e); e })?;

    println!("Bulk-loading directory stats...");
    {
        let sink = transaction.copy_in(
            "COPY dir_stats (path_hash, path, total_size_bytes, mtime_first, mtime_last, atime_first, atime_last, ctime_first, ctime_last, file_count) FROM STDIN WITH (FORMAT binary)"
        )?;
        let mut writer = BinaryCopyInWriter::new(sink, &[
            Type::BYTEA, Type::TEXT, Type::INT8, Type::INT8, Type::INT8,
            Type::INT8, Type::INT8, Type::INT8, Type::INT8, Type::INT8,
        ]);
        for (path, node) in &aggregates {
            let path_hash = fs_common::compute_hash(path);
            writer.write(&[
                &path_hash,
                path,
                &node.size,
                &node.mtime_first,
                &node.mtime_last,
                &node.atime_first,
                &node.atime_last,
                &node.ctime_first,
                &node.ctime_last,
                &node.count,
            ])?;
        }
        writer.finish()?;
    }

    println!("Bulk-loading user/group stats...");
    {
        let sink = transaction.copy_in(
            "COPY user_stats (id_type, id_value, total_size_bytes, file_count) FROM STDIN WITH (FORMAT binary)"
        )?;
        let mut writer = BinaryCopyInWriter::new(sink, &[
            Type::TEXT, Type::INT4, Type::INT8, Type::INT8,
        ]);
        for ((id_type, id_val), (size, count)) in &user_aggregates {
            writer.write(&[id_type, id_val, size, count])?;
        }
        writer.finish()?;
    }

    transaction.commit()?;
    println!("Aggregation complete.");

    Ok(processed)
}
