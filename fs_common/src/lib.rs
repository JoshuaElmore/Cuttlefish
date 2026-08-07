use sha2::{Sha256, Digest};
use postgres::{Client, NoTls};
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
pub struct Config {
    pub database: DbConfig,
}

pub fn load_config(path: &str) -> Result<Config, Box<dyn Error>> {
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

pub fn compute_hash(path: &str) -> Vec<u8> {
    let mut hasher = Sha256::new();
    hasher.update(path.as_bytes());
    hasher.finalize().to_vec()
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
