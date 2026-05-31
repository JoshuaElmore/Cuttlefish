use sha2::{Sha256, Digest};
use postgres::{Client, NoTls};
use postgres_native_tls::MakeTlsConnector;
use native_tls::TlsConnector;
use serde::Deserialize;
use std::fs;
use std::error::Error;

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
    let config: Config = toml::from_str(&content)?;
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
