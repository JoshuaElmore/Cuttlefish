use sha2::{Sha256, Digest};
use postgres::{Client, NoTls};
use serde::Deserialize;
use std::fs;
use std::error::Error;

#[derive(Deserialize)]
pub struct DbConfig {
    pub host: String,
    pub user: String,
    pub password: String,
    pub dbname: String,
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

pub fn get_db_client(config: &DbConfig) -> Result<Client, postgres::Error> {
    let conn_str = format!(
        "host={} user={} password={} dbname={}",
        config.host, config.user, config.password, config.dbname
    );
    Client::connect(&conn_str, NoTls)
}

pub fn compute_hash(path: &str) -> Vec<u8> {
    let mut hasher = Sha256::new();
    hasher.update(path.as_bytes());
    hasher.finalize().to_vec()
}
