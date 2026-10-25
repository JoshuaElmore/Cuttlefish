package main

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func expandYAML(t *testing.T, src string) (Config, bool, error) {
	t.Helper()
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(src), &root); err != nil {
		t.Fatal(err)
	}
	lit, err := expandEnv(&root, false)
	var c Config
	if err == nil {
		if err := root.Decode(&c); err != nil {
			t.Fatal(err)
		}
	}
	return c, lit, err
}

func TestExpandEnv(t *testing.T) {
	t.Setenv("CF_DB_PASS", "p@ss: #not-a-comment\nfoo: bar")
	t.Setenv("CF_HOST", "db")
	c, lit, err := expandYAML(t, `
database:
  host: ${CF_HOST}.svc
  password: ${CF_DB_PASS}
auth:
  oidc:
    admin_groups: ["${CF_HOST}-admins"]
`)
	if err != nil {
		t.Fatal(err)
	}
	if lit {
		t.Error("secret given as ${VAR} counted as literal")
	}
	if c.Database.Host != "db.svc" || c.Database.Password != "p@ss: #not-a-comment\nfoo: bar" {
		t.Errorf("got host %q password %q", c.Database.Host, c.Database.Password)
	}
	if len(c.Auth.OIDC.AdminGroups) != 1 || c.Auth.OIDC.AdminGroups[0] != "db-admins" {
		t.Errorf("sequence not expanded: %v", c.Auth.OIDC.AdminGroups)
	}
}

func TestExpandEnvLiteralSecret(t *testing.T) {
	t.Setenv("CF_X", "x")
	for _, src := range []string{
		"database:\n  password: hunter2",
		"auth:\n  oidc:\n    client_secret: pre${CF_X}",
		"auth:\n  session_secret: abc",
	} {
		if _, lit, err := expandYAML(t, src); err != nil || !lit {
			t.Errorf("%q: literal=%v err=%v, want literal secret", src, lit, err)
		}
	}
	if _, lit, _ := expandYAML(t, "database:\n  host: localhost"); lit {
		t.Error("non-secret key counted as secret")
	}
}

func TestExpandEnvUnset(t *testing.T) {
	if _, _, err := expandYAML(t, "database:\n  password: ${CF_SURELY_UNSET_VAR}"); err == nil {
		t.Error("unset variable did not fail")
	}
}
