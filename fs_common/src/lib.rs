use sha2::{Sha256, Digest};
use postgres::{Client, GenericClient, NoTls};
use postgres_native_tls::MakeTlsConnector;
use native_tls::TlsConnector;
use serde::Deserialize;
use std::fs;
use std::error::Error;
use uuid::Uuid;

#[derive(Deserialize)]
pub struct DbConfig {
    pub host: String,
    pub user: String,
    pub password: String,
    pub dbname: String,
    #[serde(default = "default_sslmode")]
    pub sslmode: String,
}

fn default_sslmode() -> String {
    "require".to_string()
}

#[derive(Deserialize)]
pub struct IndexerConfig {
    pub root_path: String,
    #[serde(default = "default_threads")]
    pub threads: usize,
}

fn default_threads() -> usize {
    8
}

#[derive(Deserialize)]
pub struct Config {
    pub database: DbConfig,
    #[serde(default)]
    pub indexer: Option<IndexerConfig>,
}

/// Rejects a config file that is readable or writable by anyone outside its
/// owner and group, or writable by the group. The file carries the database
/// password (and, for fs_api, the session-signing secret), so a world-readable
/// copy hands any local user on the indexed host both the ability to read the
/// index directly and to forge API sessions.
///
/// Group *read* is permitted: the intended deployment is root:cuttlefish 0640,
/// so the unprivileged API/aggregator user can read a root-owned file.
fn check_config_permissions(path: &str) -> Result<(), Box<dyn Error>> {
    use std::os::unix::fs::PermissionsExt;

    let mode = fs::metadata(path)?.permissions().mode() & 0o777;
    if mode & 0o027 != 0 {
        return Err(format!(
            "config file {} has permissions {:04o}; it contains the database password and \
             must not be group-writable or accessible to other users. Fix with: \
             sudo chown root:cuttlefish {} && sudo chmod 0640 {}",
            path, mode, path, path
        )
        .into());
    }
    Ok(())
}

pub fn load_config(path: &str) -> Result<Config, Box<dyn Error>> {
    check_config_permissions(path)?;
    let content = fs::read_to_string(path)?;
    let config: Config = serde_yaml::from_str(&content)?;
    Ok(config)
}

/// Wraps v in single quotes and escapes ' and \ per the libpq keyword=value format.
fn libpq_escape(v: &str) -> String {
    let mut out = String::with_capacity(v.len() + 2);
    out.push('\'');
    for c in v.chars() {
        if c == '\'' || c == '\\' {
            out.push('\\');
        }
        out.push(c);
    }
    out.push('\'');
    out
}

pub fn get_db_client(config: &DbConfig) -> Result<Client, Box<dyn Error>> {
    let conn_str = format!(
        "host={} user={} password={} dbname={}",
        libpq_escape(&config.host),
        libpq_escape(&config.user),
        libpq_escape(&config.password),
        libpq_escape(&config.dbname)
    );

    match config.sslmode.as_str() {
        "disable" => Ok(Client::connect(&conn_str, NoTls)?),
        "require" => {
            // Encrypted but certificate not verified (matches PostgreSQL sslmode=require).
            let connector = TlsConnector::builder()
                .danger_accept_invalid_certs(true)
                .build()?;
            Ok(Client::connect(&conn_str, MakeTlsConnector::new(connector))?)
        }
        _ => {
            // verify-ca, verify-full, or any unrecognised value → full TLS verification.
            let connector = TlsConnector::new()?;
            Ok(Client::connect(&conn_str, MakeTlsConnector::new(connector))?)
        }
    }
}

/// Byte width of the `path_hash` / `parent_hash` keys.
///
/// A SHA-256 digest truncated to 128 bits. The two largest indexes in the
/// database — the `filesystem_index` primary key and `idx_fsindex_parent_hash`
/// — are almost entirely key bytes, and how much of them stays resident in
/// shared_buffers is what governs the cost of the indexer's random-probe
/// upsert; halving the key halves both. At 10^8 entries the birthday bound
/// puts the chance of any collision at ~10^-23.
///
/// Do not shorten this further. At 8 bytes the same 10^8 entries carry a
/// ~0.03% chance of a collision, and a collision here silently merges two
/// filesystem entries into one row — the exact failure an audit tool must not
/// have. Changing the value at all invalidates an existing index (see
/// `stored_path_hash_len`).
pub const PATH_HASH_LEN: usize = 16;

pub fn compute_hash(path: &str) -> Vec<u8> {
    let mut hasher = Sha256::new();
    hasher.update(path.as_bytes());
    hasher.finalize()[..PATH_HASH_LEN].to_vec()
}

/// Byte width of the hashes already stored in `filesystem_index`, or `None` if
/// the table does not exist yet or is empty.
///
/// An index written with a different `PATH_HASH_LEN` is unreadable to this
/// binary rather than merely stale: every lookup and every parent→child link
/// is keyed by a hash recomputed from the path, so no query would ever match a
/// row of the other width. Callers use this to detect that case instead of
/// silently returning "not found" for the entire filesystem.
pub fn stored_path_hash_len<C: GenericClient>(client: &mut C) -> Result<Option<i32>, Box<dyn Error>> {
    let table: Option<String> = client
        .query_one("SELECT to_regclass('filesystem_index')::text", &[])?
        .get(0);
    if table.is_none() {
        return Ok(None);
    }
    let rows = client.query("SELECT octet_length(path_hash) FROM filesystem_index LIMIT 1", &[])?;
    Ok(rows.first().map(|r| r.get(0)))
}

/// Switches `table` to UNLOGGED if it is currently a permanent table, returning
/// whether anything changed.
///
/// An unlogged table and its indexes are kept out of the WAL entirely, which
/// also removes the full-page writes that dominate a bulk load's write volume
/// after each checkpoint. The trade is that PostgreSQL truncates unlogged
/// tables during crash recovery and never replicates them to a standby — which
/// is acceptable only for tables that a rerun of fs_indexer (and then
/// fs_aggregator) rebuilds from the filesystem itself. Never apply this to
/// `scan_sessions`: run history is the one thing here that cannot be
/// regenerated.
///
/// The current persistence is checked first because `ALTER TABLE ... SET
/// UNLOGGED` rewrites the whole table and all of its indexes; callers should
/// still prefer to call this while the table is empty.
pub fn ensure_unlogged<C: GenericClient>(client: &mut C, table: &str) -> Result<bool, Box<dyn Error>> {
    // The table name is interpolated into DDL, where it cannot be a bind
    // parameter. Every caller passes a literal, so this only has to be enough
    // to guarantee that stays true.
    if table.is_empty() || !table.chars().all(|c| c.is_ascii_alphanumeric() || c == '_') {
        return Err(format!("refusing to build DDL for table name {:?}", table).into());
    }

    let rows = client.query(
        "SELECT relpersistence::text FROM pg_class WHERE oid = to_regclass($1)",
        &[&table],
    )?;

    // 'p' = permanent, 'u' = already unlogged, 't' = temporary, none = no such
    // table. Only the first needs (or tolerates) the ALTER.
    let persistence: Option<String> = rows.first().map(|r| r.get(0));
    if persistence.as_deref() != Some("p") {
        return Ok(false);
    }

    client.batch_execute(&format!("ALTER TABLE {} SET UNLOGGED", table))?;
    Ok(true)
}

/// Generates a fresh scan session id. A thin wrapper so callers that don't
/// otherwise need the `uuid` crate (fs_aggregator) don't have to depend on it
/// directly just to start a session.
pub fn new_session_id() -> String {
    Uuid::new_v4().to_string()
}

/// Ensures the scan_sessions table exists, including columns added after the
/// original single-row-per-indexer-run design (scan_type, status,
/// files_scanned). Shared by fs_indexer and fs_aggregator so both scan types
/// are tracked the same way, including on databases created by an older binary.
///
/// Storing files_scanned directly on scan_sessions (rather than deriving it by
/// joining/counting filesystem_index) is a deliberate denormalization: it's the
/// only place fs_aggregator's row count can live, and it keeps GET /api/scans
/// cheap regardless of index size.
pub fn ensure_scan_sessions_table(client: &mut Client) -> Result<(), Box<dyn Error>> {
    client.batch_execute(
        "CREATE TABLE IF NOT EXISTS scan_sessions (
            session_id TEXT PRIMARY KEY,
            scan_type TEXT NOT NULL DEFAULT 'indexer',
            status TEXT NOT NULL DEFAULT 'running',
            files_scanned BIGINT NOT NULL DEFAULT 0,
            started_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            ended_at TIMESTAMP
        );
        ALTER TABLE scan_sessions ADD COLUMN IF NOT EXISTS scan_type TEXT NOT NULL DEFAULT 'indexer';
        ALTER TABLE scan_sessions ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'running';
        ALTER TABLE scan_sessions ADD COLUMN IF NOT EXISTS files_scanned BIGINT NOT NULL DEFAULT 0;"
    )?;
    Ok(())
}

/// Records the start of a scan_sessions row. `scan_type` is "indexer" or
/// "aggregator"; status starts at "running" (the column default).
pub fn record_scan_start(client: &mut Client, session_id: &str, scan_type: &str) -> Result<(), Box<dyn Error>> {
    client.execute(
        "INSERT INTO scan_sessions (session_id, scan_type) VALUES ($1, $2)",
        &[&session_id, &scan_type],
    )?;
    Ok(())
}

/// Updates the live file count on a still-running session, without ending it.
/// Callers should treat failures here as non-fatal — a missed progress tick
/// shouldn't abort the scan itself.
pub fn report_scan_progress(client: &mut Client, session_id: &str, files_scanned: i64) -> Result<(), Box<dyn Error>> {
    client.execute(
        "UPDATE scan_sessions SET files_scanned = $2 WHERE session_id = $1",
        &[&session_id, &files_scanned],
    )?;
    Ok(())
}

/// Marks a session finished. `status` is "success" or "failed".
pub fn end_scan_session(client: &mut Client, session_id: &str, status: &str, files_scanned: i64) -> Result<(), Box<dyn Error>> {
    client.execute(
        "UPDATE scan_sessions SET status = $2, files_scanned = $3, ended_at = CURRENT_TIMESTAMP WHERE session_id = $1",
        &[&session_id, &status, &files_scanned],
    )?;
    Ok(())
}

/// Reaps sessions of `scan_type` left in status='running' by a previous run
/// that crashed or was killed before it could call `end_scan_session` (e.g.
/// SIGKILL, OOM, power loss — panics on the walker thread are already caught
/// and closed out normally, but nothing runs on a hard kill of the process).
/// fs_indexer and fs_aggregator are one-shot batch processes, not meant to
/// run two-at-a-time against the same database, so any row of this
/// scan_type still 'running' when a new run starts must be orphaned. Call
/// after `ensure_scan_sessions_table` and before `record_scan_start`, so the
/// new session's own row is never touched.
pub fn reap_stale_scan_sessions(client: &mut Client, scan_type: &str) -> Result<u64, Box<dyn Error>> {
    let reaped = client.execute(
        "UPDATE scan_sessions SET status = 'failed', ended_at = CURRENT_TIMESTAMP
         WHERE scan_type = $1 AND status = 'running'",
        &[&scan_type],
    )?;
    Ok(reaped)
}

#[cfg(test)]
mod tests {
    use super::*;

    /// compute_hash is a cross-language contract: fs_api recomputes the same
    /// hashes in Go (`pathHash` in handlers.go) to look rows up by primary key,
    /// so the two implementations disagreeing makes every path lookup miss
    /// while both sides still look individually correct. These vectors are the
    /// Go implementation's output; if a change here breaks them, fs_api must
    /// change in the same commit.
    #[test]
    fn compute_hash_matches_the_go_implementation() {
        let vectors = [
            ("/", "8a5edab282632443219e051e4ade2d1d"),
            ("/home", "2cc974af6afc822c42a4e914df05c697"),
            ("/home/joshua", "9904e206bdc5bf9c1de25c8a343ed277"),
            ("/proc/kcore", "9ccf97e31dea525b8f36feb3738f19a7"),
            ("/tmp/a b/ünïcode.txt", "ade6187ad939c7f9bd85cfea54aad1fa"),
        ];
        for (path, expected) in vectors {
            let got: String = compute_hash(path).iter().map(|b| format!("{:02x}", b)).collect();
            assert_eq!(got, expected, "hash mismatch for {:?}", path);
        }
    }

    #[test]
    fn compute_hash_is_a_truncated_sha256_of_the_declared_width() {
        let full = {
            let mut h = Sha256::new();
            h.update(b"/home/joshua");
            h.finalize().to_vec()
        };
        assert_eq!(compute_hash("/home/joshua").len(), PATH_HASH_LEN);
        assert_eq!(compute_hash("/home/joshua"), full[..PATH_HASH_LEN]);
    }

    /// 8 bytes carries a ~0.03% collision chance across 10^8 entries, and a
    /// collision silently merges two filesystem entries into one row.
    #[test]
    fn path_hash_is_wide_enough_to_be_collision_free_in_practice() {
        assert!(PATH_HASH_LEN >= 16, "PATH_HASH_LEN must not drop below 128 bits");
        assert!(PATH_HASH_LEN <= 32, "PATH_HASH_LEN cannot exceed the SHA-256 digest");
    }
}
