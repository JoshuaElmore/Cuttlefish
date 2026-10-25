package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/lib/pq"
	"golang.org/x/oauth2"
)

var (
	oidcProvider *oidc.Provider
	oauth2Cfg    *oauth2.Config
)

// --- Login rate limiter ---

const (
	loginMaxAttempts  = 10
	loginLockDuration = 15 * time.Minute
)

type ipRecord struct {
	failures    int
	lockedUntil time.Time
}

var (
	loginMu      sync.Mutex
	loginRecords = make(map[string]*ipRecord)
)

func loginAllowed(ip string) bool {
	loginMu.Lock()
	defer loginMu.Unlock()
	rec := loginRecords[ip]
	return rec == nil || time.Now().After(rec.lockedUntil)
}

func loginFailed(ip string) {
	loginMu.Lock()
	defer loginMu.Unlock()
	rec := loginRecords[ip]
	if rec == nil {
		rec = &ipRecord{}
		loginRecords[ip] = rec
	}
	rec.failures++
	if rec.failures >= loginMaxAttempts {
		rec.lockedUntil = time.Now().Add(loginLockDuration)
		rec.failures = 0
	}
}

func loginSucceeded(ip string) {
	loginMu.Lock()
	defer loginMu.Unlock()
	delete(loginRecords, ip)
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func initOIDC(ctx context.Context) error {
	var err error
	oidcProvider, err = oidc.NewProvider(ctx, config.Auth.OIDC.Issuer)
	if err != nil {
		return err
	}
	oauth2Cfg = &oauth2.Config{
		ClientID:     config.Auth.OIDC.ClientID,
		ClientSecret: config.Auth.OIDC.ClientSecret,
		RedirectURL:  config.Auth.OIDC.RedirectURL,
		Endpoint:     oidcProvider.Endpoint(),
		Scopes:       append([]string{oidc.ScopeOpenID, "profile", "email"}, config.Auth.OIDC.ExtraScopes...),
	}
	return nil
}

// --- Sessions (server-side rows, opaque cookie) ---
//
// A session is a row in `sessions`, not a self-contained token, so it can be
// revoked: deleting the row (logout, failed IdP re-check) blocks
// the very next request. The cookie holds 32 random bytes; the table holds only
// their SHA-256, so reading the table never yields a usable cookie.

const sessionLifetime = 24 * time.Hour

func ensureAuthTables() error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			id_hash       BYTEA PRIMARY KEY,
			username      TEXT NOT NULL,
			groups        TEXT[] NOT NULL DEFAULT '{}',
			refresh_token TEXT,
			created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
			checked_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
			expires_at    TIMESTAMPTZ NOT NULL
		);
		CREATE TABLE IF NOT EXISTS access_grants (
			id          SERIAL PRIMARY KEY,
			group_name  TEXT NOT NULL,
			path_prefix TEXT NOT NULL,
			created_by  TEXT NOT NULL,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		-- group_type: 'idp' matches the IdP groups claim, 'unix' the user's Unix
		-- groups from identity_membership. Pre-existing grants are all IdP ones,
		-- and an IdP group may share a name with a Unix group, hence the wider key.
		ALTER TABLE access_grants ADD COLUMN IF NOT EXISTS group_type TEXT NOT NULL DEFAULT 'idp'
			CHECK (group_type IN ('idp', 'unix'));
		ALTER TABLE access_grants DROP CONSTRAINT IF EXISTS access_grants_group_name_path_prefix_key;
		CREATE UNIQUE INDEX IF NOT EXISTS access_grants_type_group_prefix
			ON access_grants (group_type, group_name, path_prefix);
		CREATE TABLE IF NOT EXISTS identity_rules (
			id           SERIAL PRIMARY KEY,
			match_suffix TEXT NOT NULL UNIQUE,
			replace_with TEXT NOT NULL DEFAULT '',
			created_by   TEXT NOT NULL,
			created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		-- Owned by fs_indexer (same DDL there); created here too because access
		-- resolution and the Users tab read them before the first scan runs.
		CREATE TABLE IF NOT EXISTS identity_map (
			id      INTEGER,
			id_type TEXT CHECK (id_type IN ('uid', 'gid')),
			name    TEXT NOT NULL,
			PRIMARY KEY (id, id_type)
		);
		CREATE TABLE IF NOT EXISTS identity_membership (
			uid INTEGER,
			gid INTEGER,
			PRIMARY KEY (uid, gid)
		);
	`)
	return err
}

func hashSessionToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

func issueSession(w http.ResponseWriter, r *http.Request, user string, groups []string, refreshToken string) error {
	expires := time.Now().Add(sessionLifetime)
	if config.Auth.Mode() == "oidc" && refreshToken == "" {
		// Without a refresh token the IdP can't be re-asked about this user, so
		// the session must not outlive one re-check interval.
		log.Printf("OIDC: no refresh token issued for %q; session limited to %v (add offline_access to auth.oidc.extra_scopes)",
			user, config.Auth.OIDC.RecheckInterval)
		expires = time.Now().Add(config.Auth.OIDC.RecheckInterval)
	}
	token, err := randomHex(32)
	if err != nil {
		return err
	}
	if _, err := db.Exec(`DELETE FROM sessions WHERE expires_at < now()`); err != nil {
		log.Printf("session cleanup failed: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO sessions (id_hash, username, groups, refresh_token, expires_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5)`,
		hashSessionToken(token), user, pq.Array(groups), refreshToken, expires); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   effectiveScheme(r) == "https",
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(time.Until(expires).Seconds()),
	})
	return nil
}

// loadSession returns the live session behind the request's cookie, re-checking
// it against the IdP when it is due. ok is false when there is no session or it
// was just revoked.
func loadSession(r *http.Request) (user string, groups []string, ok bool) {
	cookie, err := r.Cookie("session")
	if err != nil {
		return "", nil, false
	}
	idHash := hashSessionToken(cookie.Value)
	err = db.QueryRow(`SELECT username, groups FROM sessions WHERE id_hash = $1 AND expires_at > now()`,
		idHash).Scan(&user, pq.Array(&groups))
	if err != nil {
		if err != sql.ErrNoRows {
			log.Printf("session lookup failed: %v", err)
		}
		return "", nil, false
	}
	if config.Auth.Mode() != "oidc" {
		return user, groups, true
	}
	return recheckSession(r.Context(), idHash, user, groups)
}

// recheckSession asks the IdP, via the refresh token, whether the user is still
// valid and what their groups are now, at most once per recheck_interval. The
// UPDATE … RETURNING claims the re-check, so concurrent requests don't spend a
// rotating refresh token twice; the losers carry on with the stored groups.
func recheckSession(ctx context.Context, idHash []byte, user string, groups []string) (string, []string, bool) {
	var refreshToken sql.NullString
	err := db.QueryRow(`
		UPDATE sessions SET checked_at = now()
		WHERE id_hash = $1 AND checked_at < now() - make_interval(secs => $2)
		RETURNING refresh_token`,
		idHash, config.Auth.OIDC.RecheckInterval.Seconds()).Scan(&refreshToken)
	if err == sql.ErrNoRows {
		return user, groups, true // not due
	}
	if err != nil {
		log.Printf("session recheck claim failed: %v", err)
		return "", nil, false
	}
	if !refreshToken.Valid {
		return user, groups, true // already capped to one interval by issueSession
	}

	revoke := func(reason string, err error) (string, []string, bool) {
		log.Printf("OIDC: revoking session of %q: %s: %v", user, reason, err)
		if _, err := db.Exec(`DELETE FROM sessions WHERE id_hash = $1`, idHash); err != nil {
			log.Printf("session delete failed: %v", err)
		}
		return "", nil, false
	}

	// Any refresh failure revokes, including the IdP being unreachable: fail
	// closed, since an unanswered re-check is indistinguishable from a revoked user.
	tok, err := oauth2Cfg.TokenSource(ctx, &oauth2.Token{RefreshToken: refreshToken.String}).Token()
	if err != nil {
		return revoke("refresh failed", err)
	}
	claims, err := tokenClaims(ctx, tok)
	if err != nil {
		return revoke("claims unavailable", err)
	}
	newUser, email, newGroups := identityFromClaims(claims)
	if newUser != user {
		return revoke("identity changed", fmt.Errorf("now %q", newUser))
	}
	if !identityAllowed(newUser, email) {
		return revoke("no longer allowed", errors.New("fails auth.oidc.allowed_users/allowed_domains"))
	}
	newRefresh := tok.RefreshToken
	if newRefresh == "" {
		newRefresh = refreshToken.String // IdP doesn't rotate
	}
	if _, err := db.Exec(`UPDATE sessions SET groups = $2, refresh_token = $3 WHERE id_hash = $1`,
		idHash, pq.Array(newGroups), newRefresh); err != nil {
		log.Printf("session update failed: %v", err)
	}
	return user, newGroups, true
}

// tokenClaims returns the verified ID-token claims from a token response,
// falling back to the userinfo endpoint when a refresh response carries no
// id_token (allowed by the spec, and common).
func tokenClaims(ctx context.Context, tok *oauth2.Token) (map[string]any, error) {
	claims := map[string]any{}
	if raw, ok := tok.Extra("id_token").(string); ok && raw != "" {
		idToken, err := oidcProvider.Verifier(&oidc.Config{ClientID: config.Auth.OIDC.ClientID}).Verify(ctx, raw)
		if err != nil {
			return nil, err
		}
		return claims, idToken.Claims(&claims)
	}
	info, err := oidcProvider.UserInfo(ctx, oauth2.StaticTokenSource(tok))
	if err != nil {
		return nil, err
	}
	return claims, info.Claims(&claims)
}

// identityFromClaims extracts the session username (email, else sub) and the
// configured groups claim, which IdPs send as an array or, for one group, a string.
func identityFromClaims(c map[string]any) (user, email string, groups []string) {
	groups = []string{} // never nil: pq.Array(nil) is NULL, and sessions.groups is NOT NULL
	email, _ = c["email"].(string)
	if config.Auth.OIDC.UsernameClaim != "" {
		user, _ = c[config.Auth.OIDC.UsernameClaim].(string)
	}
	if user == "" {
		user = email
	}
	if user == "" {
		user, _ = c["sub"].(string)
	}
	switch v := c[config.Auth.OIDC.GroupsClaim].(type) {
	case []any:
		for _, g := range v {
			if s, ok := g.(string); ok {
				groups = append(groups, s)
			}
		}
	case string:
		groups = []string{v}
	}
	return user, email, groups
}

// identityAllowed applies the optional allowlists: auth.oidc.allowed_users
// matches the identity itself, auth.oidc.allowed_domains the email domain. When
// both are empty any verified identity is accepted.
func identityAllowed(user, email string) bool {
	if len(config.Auth.OIDC.AllowedUsers) > 0 {
		if !slices.Contains(config.Auth.OIDC.AllowedUsers, user) {
			log.Printf("OIDC: rejecting %q: not in auth.oidc.allowed_users", user)
			return false
		}
	} else if len(config.Auth.OIDC.AllowedDomains) > 0 {
		domain := ""
		if i := strings.LastIndexByte(email, '@'); i >= 0 {
			domain = email[i+1:]
		}
		if domain == "" || !slices.Contains(config.Auth.OIDC.AllowedDomains, domain) {
			log.Printf("OIDC: rejecting %q: email domain not in auth.oidc.allowed_domains", user)
			return false
		}
	}
	return true
}

// --- Roles and path access ---

const (
	roleAdmin      = "admin"      // everything, plus /api/admin/*
	roleViewer     = "viewer"     // read everything
	roleRestricted = "restricted" // only paths under Prefixes
)

// access is what a request may see. It doubles as the /auth/me response.
type access struct {
	User     string   `json:"user"`
	Role     string   `json:"role"`
	Prefixes []string `json:"prefixes"`
	// UnixUser is User mapped through identity_rules: the name looked up in
	// identity_map and substituted for {user}. Set for restricted users only.
	UnixUser string `json:"unix_user,omitempty"`
}

// grantEveryone is the access_grants group_name that matches every user.
const grantEveryone = "*"

// identityRule rewrites an IdP identity's suffix into a Unix username's:
// "@corp.com" → "" strips a domain, "@corp.com" → "@ad.corp.com" swaps one,
// and "" → "@ad.corp.com" appends to names that match nothing longer.
type identityRule struct {
	MatchSuffix string `json:"match_suffix"`
	ReplaceWith string `json:"replace_with"`
}

func loadIdentityRules() ([]identityRule, error) {
	rows, err := db.Query(`SELECT match_suffix, replace_with FROM identity_rules`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rules []identityRule
	for rows.Next() {
		var r identityRule
		if err := rows.Scan(&r.MatchSuffix, &r.ReplaceWith); err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// mapIdentity turns an IdP identity into the Unix username it corresponds to.
// Matching is case-insensitive and the longest matching suffix wins, so rule
// order never matters. With no matching rule the @domain is dropped
// (joshua@corp.com → joshua).
func mapIdentity(identity string, rules []identityRule) string {
	id := strings.ToLower(identity)
	best, bestSuffix := -1, ""
	for i, r := range rules {
		// Compared lowered on both sides, and sliced by the lowered suffix's own
		// length: lowercasing can change a string's byte length.
		if s := strings.ToLower(r.MatchSuffix); strings.HasSuffix(id, s) && (best < 0 || len(s) > len(bestSuffix)) {
			best, bestSuffix = i, s
		}
	}
	if best < 0 {
		name, _, _ := strings.Cut(id, "@")
		return name
	}
	return id[:len(id)-len(bestSuffix)] + strings.ToLower(rules[best].ReplaceWith)
}

// grantUser checks that a mapped Unix name can be substituted for {user} in a
// grant's path. ok is false when it can't safely be one path segment — a "/"
// or a dot-name would let an identity climb out of /home/{user} — and the
// grant is then skipped rather than widened.
func grantUser(name string) (string, bool) {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\x00") {
		return "", false
	}
	return name, true
}

// resolveAccess maps groups to a role via config, and for restricted users
// loads their grants fresh on every request so admin edits apply immediately.
// Grants for group "*" apply to everyone; "{user}" in a grant's path expands
// to the identity mapped through identity_rules.
func resolveAccess(user string, groups []string) (*access, error) {
	a := &access{User: user, Role: roleFor(groups), Prefixes: []string{}}
	if a.Role != roleRestricted {
		return a, nil
	}
	rules, err := loadIdentityRules()
	if err != nil {
		return nil, err
	}
	a.UnixUser = mapIdentity(user, rules)
	grants, err := matchingGrants(groups, a.UnixUser)
	if err != nil {
		return nil, err
	}
	for _, g := range grants {
		if g.Path == "" {
			log.Printf("skipping grant %q for %q: identity is not a safe path segment", g.PathPrefix, user)
			continue
		}
		a.Prefixes = append(a.Prefixes, g.Path)
	}
	slices.Sort(a.Prefixes)
	a.Prefixes = slices.Compact(a.Prefixes)
	return a, nil
}

// roleFor maps a session's IdP groups to its role.
func roleFor(groups []string) string {
	switch {
	case config.Auth.Mode() == "local":
		return roleAdmin // the single local account has no groups to map
	case slices.ContainsFunc(groups, func(g string) bool { return slices.Contains(config.Auth.OIDC.AdminGroups, g) }):
		return roleAdmin
	case slices.ContainsFunc(groups, func(g string) bool { return slices.Contains(config.Auth.OIDC.ViewerGroups, g) }):
		return roleViewer
	}
	return roleRestricted
}

// MatchedGrant is an access grant that applies to a user, with {user} filled in.
type MatchedGrant struct {
	GroupType  string `json:"group_type"`
	GroupName  string `json:"group_name"`
	PathPrefix string `json:"path_prefix"` // as stored, possibly with {user}
	Path       string `json:"path"`        // what the user gets; "" if {user} can't be filled safely
}

// matchingGrants returns the grants whose IdP group is in groups (or is "*"),
// or whose Unix group the indexer recorded for the scanned user named
// unixUser; a user who owns no indexed files has no Unix groups.
func matchingGrants(groups []string, unixUser string) ([]MatchedGrant, error) {
	rows, err := db.Query(`
		SELECT group_type, group_name, path_prefix FROM access_grants
		WHERE (group_type = 'idp' AND (group_name = ANY($1) OR group_name = $2))
		   OR (group_type = 'unix' AND group_name IN (
		        SELECT g.name FROM identity_membership m
		        JOIN identity_map u ON u.id = m.uid AND u.id_type = 'uid'
		        JOIN identity_map g ON g.id = m.gid AND g.id_type = 'gid'
		        WHERE u.name = $3))
		ORDER BY path_prefix, group_type, group_name`,
		pq.Array(groups), grantEveryone, unixUser)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	grants := []MatchedGrant{}
	for rows.Next() {
		var g MatchedGrant
		if err := rows.Scan(&g.GroupType, &g.GroupName, &g.PathPrefix); err != nil {
			return nil, err
		}
		g.Path = g.PathPrefix
		if strings.Contains(g.Path, "{user}") {
			name, ok := grantUser(unixUser)
			g.Path = ""
			if ok {
				g.Path = strings.ReplaceAll(g.PathPrefix, "{user}", name)
			}
		}
		grants = append(grants, g)
	}
	return grants, rows.Err()
}

// allows reports whether p is inside one of a restricted user's prefixes.
// Non-canonical paths are refused outright, so "/home/me/../you" can't pass the
// prefix test; the indexer only ever stores canonical paths anyway.
func (a *access) allows(p string) bool {
	if a.Role != roleRestricted {
		return true
	}
	if !strings.HasPrefix(p, "/") || path.Clean(p) != p {
		return false
	}
	for _, pre := range a.Prefixes {
		if pre == "/" || p == pre || strings.HasPrefix(p, pre+"/") {
			return true
		}
	}
	return false
}

type accessKey struct{}

// accessFrom returns the caller's access; only valid behind authMiddleware.
func accessFrom(r *http.Request) *access {
	return r.Context().Value(accessKey{}).(*access)
}

// --- Middleware ---

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, groups, ok := loadSession(r)
		if !ok {
			respondError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		a, err := resolveAccess(user, groups)
		if err != nil {
			log.Printf("access lookup failed: %v", err)
			respondError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), accessKey{}, a)))
	}
}

func adminMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if accessFrom(r).Role != roleAdmin {
			respondError(w, http.StatusForbidden, "admin role required")
			return
		}
		next(w, r)
	}
}

// --- Auth endpoint handlers ---

// GET /auth/mode
func handleAuthMode(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"mode":          config.Auth.Mode(),
		"auto_redirect": config.Auth.Mode() == "oidc" && config.Auth.OIDC.AutoRedirect,
	})
}

// GET /auth/me  (behind authMiddleware)
func handleAuthMe(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, accessFrom(r))
}

// POST /auth/login  (local mode)
func handleLocalLogin(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !loginAllowed(ip) {
		w.Header().Set("Retry-After", "900")
		respondError(w, http.StatusTooManyRequests, "too many failed attempts, try again later")
		return
	}

	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	wantUser := []byte(config.Auth.Local.Username)
	wantPass := []byte(config.Auth.Local.Password)
	gotUser := []byte(body.Username)
	gotPass := []byte(body.Password)

	// constant-time comparison to avoid timing attacks
	userOK := subtle.ConstantTimeCompare(wantUser, gotUser) == 1
	passOK := subtle.ConstantTimeCompare(wantPass, gotPass) == 1
	if !userOK || !passOK {
		loginFailed(ip)
		respondError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	loginSucceeded(ip)
	if err := issueSession(w, r, body.Username, []string{}, ""); err != nil {
		respondError(w, http.StatusInternalServerError, "session error")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"user": body.Username})
}

// GET /auth/oidc/start  (OIDC mode — redirects to provider)
func handleOIDCStart(w http.ResponseWriter, r *http.Request) {
	state, err := randomHex(16)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to generate state")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "oidc_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   effectiveScheme(r) == "https",
		MaxAge:   300,
	})
	http.Redirect(w, r, oauth2Cfg.AuthCodeURL(state), http.StatusFound)
}

// GET /auth/callback  (OIDC mode — provider redirects here)
func handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	stateCookie, err := r.Cookie("oidc_state")
	if err != nil || stateCookie.Value != r.URL.Query().Get("state") {
		respondError(w, http.StatusBadRequest, "invalid state")
		return
	}
	// clear state cookie
	http.SetCookie(w, &http.Cookie{Name: "oidc_state", Value: "", MaxAge: -1, Path: "/"})

	token, err := oauth2Cfg.Exchange(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		respondError(w, http.StatusInternalServerError, "token exchange failed")
		return
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		respondError(w, http.StatusInternalServerError, "no id_token in response")
		return
	}

	verifier := oidcProvider.Verifier(&oidc.Config{ClientID: config.Auth.OIDC.ClientID})
	idToken, err := verifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "id_token verification failed")
		return
	}

	claims := map[string]any{}
	if err := idToken.Claims(&claims); err != nil {
		respondError(w, http.StatusInternalServerError, "invalid id_token claims")
		return
	}
	user, email, groups := identityFromClaims(claims)
	if !identityAllowed(user, email) {
		respondError(w, http.StatusForbidden, "user not allowed")
		return
	}

	if err := issueSession(w, r, user, groups, token.RefreshToken); err != nil {
		log.Printf("issue session: %v", err)
		respondError(w, http.StatusInternalServerError, "session error")
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

// POST /auth/logout
func handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("session"); err == nil {
		if _, err := db.Exec(`DELETE FROM sessions WHERE id_hash = $1`, hashSessionToken(cookie.Value)); err != nil {
			log.Printf("logout: session delete failed: %v", err)
		}
	}
	http.SetCookie(w, &http.Cookie{Name: "session", Value: "", MaxAge: -1, Path: "/"})
	respondJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func tlsEnabled() bool {
	return config.Server.TLSCert != "" && config.Server.TLSKey != ""
}

// effectiveScheme reports the scheme the browser sees. Behind a TLS-terminating
// reverse proxy this process serves plaintext, so r.TLS is nil and the proxy's
// X-Forwarded-Proto is the only signal — honoured only when allow_insecure_http
// says that proxy is deliberate; with real TLS on this listener the header is
// attacker-adjacent and ignored.
func effectiveScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if config.Server.AllowInsecureHTTP {
		if fwd := r.Header.Get("X-Forwarded-Proto"); fwd != "" {
			scheme, _, _ := strings.Cut(fwd, ",")
			return strings.ToLower(strings.TrimSpace(scheme))
		}
	}
	return "http"
}
