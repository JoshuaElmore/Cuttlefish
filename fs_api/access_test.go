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
		name string
		ok   bool
	}{
		{"joshua", true},
		{"j.doe", true},
		{"joshua@ad.corp.com", true}, // SSSD fully-qualified names are one segment
		{"a/b", false},               // would escape /home/{user}
		{"..", false},
		{".", false},
		{"", false},
		{"a\x00b", false},
	}
	for _, c := range cases {
		if _, ok := grantUser(c.name); ok != c.ok {
			t.Errorf("grantUser(%q) ok = %v, want %v", c.name, ok, c.ok)
		}
	}
}

func TestMapIdentity(t *testing.T) {
	rules := []identityRule{
		{"@corp.com", ""},
		{"@eu.corp.com", "@ad.eu.corp.com"},
		{"@Partner.COM", "_partner"},
	}
	cases := []struct {
		rules    []identityRule
		identity string
		want     string
	}{
		{nil, "Joshua@Domain.com", "joshua"}, // no rules: drop @domain
		{nil, "joshua", "joshua"},
		{rules, "joshua@corp.com", "joshua"},
		{rules, "anna@eu.corp.com", "anna@ad.eu.corp.com"},                 // longest suffix wins over @corp.com
		{rules, "Bob@partner.com", "bob_partner"},                          // case-insensitive both sides
		{rules, "carl@other.org", "carl"},                                  // no match: default strip
		{[]identityRule{{"", "@ad.corp.com"}}, "dave", "dave@ad.corp.com"}, // "" appends
		{[]identityRule{{"", "@ad.corp.com"}, {"@corp.com", "@ad.corp.com"}}, "erin@corp.com", "erin@ad.corp.com"},
		{[]identityRule{{"@\u0130", "@x"}}, "i@i", "i@x"}, // İ lowers to a shorter i: must not mis-slice or panic
	}
	for _, c := range cases {
		if got := mapIdentity(c.identity, c.rules); got != c.want {
			t.Errorf("mapIdentity(%q) = %q, want %q", c.identity, got, c.want)
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
