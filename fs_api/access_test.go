package main

import (
	"reflect"
	"testing"
)

func TestAccessAllows(t *testing.T) {
	restricted := func(prefixes ...string) *access {
		return &access{Role: roleRestricted, Prefixes: prefixes}
	}
	cases := []struct {
		name string
		a    *access
		path string
		want bool
	}{
		{"admin anything", &access{Role: roleAdmin}, "/etc/shadow", true},
		{"viewer anything", &access{Role: roleViewer}, "/root", true},
		{"prefix itself", restricted("/home/joshua"), "/home/joshua", true},
		{"below prefix", restricted("/home/joshua"), "/home/joshua/x", true},
		{"sibling sharing name prefix", restricted("/home/joshua"), "/home/joshuax", false},
		{"ancestor", restricted("/home/joshua"), "/home", false},
		{"root", restricted("/home/joshua"), "/", false},
		{"dotdot escape", restricted("/home/joshua"), "/home/joshua/../alice", false},
		{"trailing slash", restricted("/home/joshua"), "/home/joshua/", false},
		{"relative", restricted("/home/joshua"), "home/joshua", false},
		{"double slash", restricted("/home/joshua"), "//home/joshua", false},
		{"root prefix", restricted("/"), "/anything", true},
		{"no prefixes", restricted(), "/home/joshua", false},
	}
	for _, c := range cases {
		if got := c.a.allows(c.path); got != c.want {
			t.Errorf("%s: allows(%q) = %v, want %v", c.name, c.path, got, c.want)
		}
	}
}

func TestScopeClause(t *testing.T) {
	cases := []struct {
		name     string
		prefixes []string
		want     string
		wantArgs []interface{}
	}{
		{"empty", nil, "FALSE", []interface{}{"a", "b"}},
		{"root", []string{"/home", "/"}, "TRUE", nil}, // args beyond the first two are irrelevant once TRUE
		{"two prefixes", []string{"/home/joshua", "/srv/a_b%"},
			"(f.path = $3 OR f.path LIKE $4 OR f.path = $5 OR f.path LIKE $6)",
			[]interface{}{"a", "b", "/home/joshua", "/home/joshua/%", "/srv/a_b%", `/srv/a\_b\%/%`}},
	}
	for _, c := range cases {
		args := []interface{}{"a", "b"}
		if got := scopeClause(c.prefixes, &args); got != c.want {
			t.Errorf("%s: clause = %q, want %q", c.name, got, c.want)
		}
		if c.wantArgs != nil && !reflect.DeepEqual(args, c.wantArgs) {
			t.Errorf("%s: args = %#v, want %#v", c.name, args, c.wantArgs)
		}
	}
}

type claimCase struct {
	name       string
	claims     map[string]any
	user, mail string
	groups     []string
}

func TestIdentityFromClaims(t *testing.T) {
	saved := config
	t.Cleanup(func() { config = saved })
	config.Auth.OIDC.GroupsClaim = "groups"

	cases := []claimCase{
		{"array with non-string", map[string]any{"email": "a@x.com", "sub": "s1", "groups": []any{"g1", 42, "g2"}},
			"a@x.com", "a@x.com", []string{"g1", "g2"}},
		{"single string", map[string]any{"email": "a@x.com", "groups": "g1"}, "a@x.com", "a@x.com", []string{"g1"}},
		{"missing claim", map[string]any{"email": "a@x.com"}, "a@x.com", "a@x.com", []string{}},
		{"sub fallback", map[string]any{"sub": "s1"}, "s1", "", []string{}},
	}
	check := func(c claimCase) {
		user, mail, groups := identityFromClaims(c.claims)
		if user != c.user || mail != c.mail {
			t.Errorf("%s: user/email = %q/%q, want %q/%q", c.name, user, mail, c.user, c.mail)
		}
		if groups == nil || !reflect.DeepEqual(groups, c.groups) {
			t.Errorf("%s: groups = %#v, want %#v", c.name, groups, c.groups)
		}
	}
	for _, c := range cases {
		check(c)
	}

	config.Auth.OIDC.UsernameClaim = "upn"
	check(claimCase{"username claim", map[string]any{"upn": "Joshua@corp.com", "email": "j@x.com"}, "Joshua@corp.com", "j@x.com", []string{}})
	check(claimCase{"username claim absent", map[string]any{"email": "j@x.com"}, "j@x.com", "j@x.com", []string{}})
}

func TestGrantUser(t *testing.T) {
	cases := []struct {
		identity, want string
		ok             bool
	}{
		{"joshua@domain.com", "joshua", true},
		{"Joshua@Domain.com", "joshua", true},
		{"joshua", "joshua", true},
		{"j.doe@x.com", "j.doe", true},
		{"a/b@x.com", "", false}, // would escape /home/{user}
		{"..@x.com", "", false},
		{".@x.com", "", false},
		{"@x.com", "", false},
		{"", "", false},
		{"a\x00b@x.com", "", false},
	}
	for _, c := range cases {
		got, ok := grantUser(c.identity)
		if got != c.want || ok != c.ok {
			t.Errorf("grantUser(%q) = %q, %v; want %q, %v", c.identity, got, ok, c.want, c.ok)
		}
	}
}

// Only the branches that decide the role from config; the restricted branch reads access_grants.
func TestResolveAccessWithoutDB(t *testing.T) {
	saved := config
	t.Cleanup(func() { config = saved })

	oidc := OIDCConfig{Issuer: "https://idp.example", AdminGroups: []string{"admins"}, ViewerGroups: []string{"auditors"}}
	cases := []struct {
		name   string
		oidc   OIDCConfig
		groups []string
		want   string
	}{
		{"local mode", OIDCConfig{}, []string{}, roleAdmin},
		{"admin group", oidc, []string{"staff", "admins"}, roleAdmin},
		{"admin beats viewer", oidc, []string{"auditors", "admins"}, roleAdmin},
		{"viewer group", oidc, []string{"auditors"}, roleViewer},
	}
	for _, c := range cases {
		config.Auth.OIDC = c.oidc
		a, err := resolveAccess("u", c.groups)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if a.Role != c.want || a.User != "u" || a.Prefixes == nil {
			t.Errorf("%s: got %+v, want role %q", c.name, a, c.want)
		}
	}
}
