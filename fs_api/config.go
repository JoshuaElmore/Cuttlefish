package main

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Database DatabaseConfig `yaml:"database"`
	Auth     AuthConfig     `yaml:"auth"`
	Server   ServerConfig   `yaml:"server"`
	UI       UIConfig       `yaml:"ui"`
}

type UIConfig struct {
	Title string `yaml:"title"` // browser tab title; defaults to defaultUITitle
}

const defaultUITitle = "Cuttlefish"

type ServerConfig struct {
	ListenAddr string `yaml:"listen_addr"`
	TLSCert    string `yaml:"tls_cert"`
	TLSKey     string `yaml:"tls_key"`
	// AllowInsecureHTTP opts into the plaintext listener for TLS-terminating
	// reverse-proxy and local-dev deployments; without it (and without TLS
	// material) the server refuses to start.
	AllowInsecureHTTP bool `yaml:"allow_insecure_http"`
}

type DatabaseConfig struct {
	Host     string `yaml:"host"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	DBName   string `yaml:"dbname"`
	SSLMode  string `yaml:"sslmode"`
}

type AuthConfig struct {
	SessionSecret string          `yaml:"session_secret"`
	Local         LocalAuthConfig `yaml:"local"`
	OIDC          OIDCConfig      `yaml:"oidc"`
}

type LocalAuthConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type OIDCConfig struct {
	Issuer       string `yaml:"issuer"`
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	RedirectURL  string `yaml:"redirect_url"`
	// Optional allowlists; when both are empty any verified identity is
	// accepted, as before.
	AllowedUsers   []string `yaml:"allowed_users"`
	AllowedDomains []string `yaml:"allowed_domains"`

	// Role mapping from the IdP's groups claim. Admin and viewer groups live
	// here rather than in the DB: the admin group has to exist before anyone
	// can reach the admin page to edit the DB.
	GroupsClaim string `yaml:"groups_claim"` // defaults to "groups"
	// UsernameClaim names the claim used as the session identity, e.g. "upn"
	// or "preferred_username"; unset (or absent from a token) means email, then sub.
	UsernameClaim string   `yaml:"username_claim"`
	ExtraScopes   []string `yaml:"extra_scopes"` // e.g. "groups", "offline_access"
	AdminGroups   []string `yaml:"admin_groups"`
	ViewerGroups  []string `yaml:"viewer_groups"`
	// RecheckInterval bounds how long an IdP-side change (user disabled,
	// removed from a group) takes to reach a live session. Defaults to 5m.
	RecheckInterval time.Duration `yaml:"recheck_interval"`
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

// checkConfigPermissions rejects a config file that is readable or writable by
// anyone outside its owner and group, or writable by the group. The file holds
// the database password — which can write a sessions row, i.e. mint a login —
// and the OIDC client secret.
//
// Group read is permitted: the intended deployment is root:cuttlefish 0640, so
// the unprivileged service user can read a root-owned file.
func checkConfigPermissions(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	if mode := fi.Mode().Perm(); mode&0o027 != 0 {
		return fmt.Errorf("config file %s has permissions %04o; it contains the database "+
			"password and the OIDC client secret, and must not be group-writable or accessible to other "+
			"users. Fix with: sudo chown root:cuttlefish %s && sudo chmod 0640 %s, or supply every "+
			"password/session_secret/client_secret as a ${ENV_VAR} reference instead",
			path, mode, path, path)
	}
	return nil
}

var (
	envRef     = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)
	envRefOnly = regexp.MustCompile(`^\$\{[A-Za-z_][A-Za-z0-9_]*\}$`)
	// Keys whose values are secrets wherever they appear; mirrored in fs_common.
	secretKeys = map[string]bool{"password": true, "session_secret": true, "client_secret": true}
)

// expandEnv replaces each ${VAR} in the scalar values under n with that
// environment variable, failing on an unset one so a typo can't become an
// empty password. It reports whether any secret key holds a literal value
// rather than a bare ${VAR}, i.e. whether the file itself is sensitive.
// Expansion is done on parsed values, not the raw text, so an env value
// containing YAML syntax can't inject or reshape keys.
func expandEnv(n *yaml.Node, secret bool) (literalSecret bool, err error) {
	switch n.Kind {
	case yaml.DocumentNode, yaml.SequenceNode:
		for _, c := range n.Content {
			lit, err := expandEnv(c, secret)
			if err != nil {
				return false, err
			}
			literalSecret = literalSecret || lit
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			lit, err := expandEnv(n.Content[i+1], secretKeys[n.Content[i].Value])
			if err != nil {
				return false, err
			}
			literalSecret = literalSecret || lit
		}
	case yaml.ScalarNode:
		literalSecret = secret && n.Value != "" && !envRefOnly.MatchString(n.Value)
		var missing string
		n.Value = envRef.ReplaceAllStringFunc(n.Value, func(m string) string {
			name := m[2 : len(m)-1]
			v, ok := os.LookupEnv(name)
			if !ok && missing == "" {
				missing = name
			}
			return v
		})
		if missing != "" {
			return false, fmt.Errorf("line %d references environment variable %s, which is not set", n.Line, missing)
		}
	}
	return literalSecret, nil
}

func loadConfig(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("cannot read config file %s: %v", path, err)
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		log.Fatalf("cannot parse config file: %v", err)
	}
	literalSecret, err := expandEnv(&root, false)
	if err != nil {
		log.Fatalf("config file %s: %v", path, err)
	}
	// With every secret supplied as ${VAR} the file holds nothing sensitive
	// (e.g. a Kubernetes ConfigMap), so its permissions don't matter.
	if literalSecret {
		if err := checkConfigPermissions(path); err != nil {
			log.Fatal(err)
		}
	}
	if err := root.Decode(&config); err != nil {
		log.Fatalf("cannot parse config file: %v", err)
	}
	if config.Auth.SessionSecret == "" {
		log.Fatal("auth.session_secret must be set in fs_config.yml")
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
		if config.Auth.Local.Username == "admin" {
			log.Fatal("auth.local.username must be changed from the default example value")
		}
		if config.Auth.Local.Password == "changeme" {
			log.Fatal("auth.local.password must be changed from the default example value")
		}
	}
	if config.Auth.Mode() == "oidc" {
		if len(config.Auth.OIDC.AdminGroups) == 0 {
			log.Fatal("auth.oidc.admin_groups must list at least one group, or nobody can manage access grants")
		}
		if config.Auth.OIDC.GroupsClaim == "" {
			config.Auth.OIDC.GroupsClaim = "groups"
		}
		if config.Auth.OIDC.RecheckInterval <= 0 {
			config.Auth.OIDC.RecheckInterval = 5 * time.Minute
		}
	}
	if strings.TrimSpace(config.UI.Title) == "" {
		config.UI.Title = defaultUITitle
	}
}
