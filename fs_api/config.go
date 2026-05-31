package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Database DatabaseConfig `toml:"database"`
	Auth     AuthConfig     `toml:"auth"`
	Server   ServerConfig   `toml:"server"`
}

type ServerConfig struct {
	ListenAddr string `toml:"listen_addr"`
	TLSCert    string `toml:"tls_cert"`
	TLSKey     string `toml:"tls_key"`
}

type DatabaseConfig struct {
	Host     string `toml:"host"`
	User     string `toml:"user"`
	Password string `toml:"password"`
	DBName   string `toml:"dbname"`
	SSLMode  string `toml:"sslmode"`
}

type AuthConfig struct {
	SessionSecret string          `toml:"session_secret"`
	Local         LocalAuthConfig `toml:"local"`
	OIDC          OIDCConfig      `toml:"oidc"`
}

type LocalAuthConfig struct {
	Username string `toml:"username"`
	Password string `toml:"password"`
}

type OIDCConfig struct {
	Issuer       string `toml:"issuer"`
	ClientID     string `toml:"client_id"`
	ClientSecret string `toml:"client_secret"`
	RedirectURL  string `toml:"redirect_url"`
}

func (a *AuthConfig) Mode() string {
	if a.OIDC.Issuer != "" {
		return "oidc"
	}
	return "local"
}

// libpqEscape wraps v in single quotes and escapes ' and \ per the libpq
// keyword=value connection-string format, preventing injection of extra parameters.
func libpqEscape(v string) string {
	var b strings.Builder
	b.Grow(len(v) + 2)
	b.WriteByte('\'')
	for i := 0; i < len(v); i++ {
		c := v[i]
		if c == '\'' || c == '\\' {
			b.WriteByte('\\')
		}
		b.WriteByte(c)
	}
	b.WriteByte('\'')
	return b.String()
}

func (d *DatabaseConfig) ConnStr() string {
	return fmt.Sprintf("host=%s user=%s password=%s dbname=%s sslmode=%s",
		libpqEscape(d.Host), libpqEscape(d.User), libpqEscape(d.Password),
		libpqEscape(d.DBName), libpqEscape(d.SSLMode))
}

var config Config

func loadConfig(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("cannot read config file %s: %v", path, err)
	}
	if _, err := toml.Decode(string(data), &config); err != nil {
		log.Fatalf("cannot parse config file: %v", err)
	}
	if config.Auth.SessionSecret == "" {
		log.Fatal("auth.session_secret must be set in fs_config.toml")
	}
	if config.Auth.SessionSecret == "change-me-to-a-random-secret" {
		log.Fatal("auth.session_secret must be changed from the default example value (generate one with: openssl rand -hex 32)")
	}
	if len(config.Auth.SessionSecret) < 32 {
		log.Fatal("auth.session_secret must be at least 32 characters long")
	}
	if config.Auth.Mode() == "local" {
		if config.Auth.Local.Username == "" || config.Auth.Local.Password == "" {
			log.Fatal("auth.local.username and auth.local.password must be set when OIDC is not configured")
		}
	}
}
