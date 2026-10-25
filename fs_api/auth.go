package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
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
// revoked: deleting the row (logout, admin revoke, failed IdP re-check) blocks
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
		CREATE INDEX IF NOT EXISTS sessions_username ON sessions (username);
		CREATE TABLE IF NOT EXISTS access_grants (
			id          SERIAL PRIMARY KEY,
			group_name  TEXT NOT NULL,
			path_prefix TEXT NOT NULL,
			created_by  TEXT NOT NULL,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
			UNIQUE (group_name, path_prefix)
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
		return revoke("no longer allowed", nil)
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
	user = email
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
}

// resolveAccess maps groups to a role via config, and for restricted users
// loads their grants fresh on every request so admin edits apply immediately.
func resolveAccess(user string, groups []string) (*access, error) {
	a := &access{User: user, Role: roleRestricted, Prefixes: []string{}}
	switch {
	case config.Auth.Mode() == "local":
		a.Role = roleAdmin // the single local account has no groups to map
	case slices.ContainsFunc(groups, func(g string) bool { return slices.Contains(config.Auth.OIDC.AdminGroups, g) }):
		a.Role = roleAdmin
	case slices.ContainsFunc(groups, func(g string) bool { return slices.Contains(config.Auth.OIDC.ViewerGroups, g) }):
		a.Role = roleViewer
	default:
		rows, err := db.Query(`SELECT DISTINCT path_prefix FROM access_grants WHERE group_name = ANY($1) ORDER BY 1`, pq.Array(groups))
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var p string
			if err := rows.Scan(&p); err != nil {
				return nil, err
			}
			a.Prefixes = append(a.Prefixes, p)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return a, nil
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

// fullAccessMiddleware guards endpoints that summarise the whole index, which
// would leak other users' usage to someone limited to a few prefixes.
func fullAccessMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if accessFrom(r).Role == roleRestricted {
			respondError(w, http.StatusForbidden, "not available to your role")
			return
		}
		next(w, r)
	}
}

// --- Auth endpoint handlers ---

// GET /auth/mode
func handleAuthMode(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"mode": config.Auth.Mode()})
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
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
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
