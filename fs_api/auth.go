package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
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
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}
	return nil
}

// --- Session (JWT in HTTP-only cookie) ---

type sessionClaims struct {
	jwt.RegisteredClaims
	User string `json:"usr"`
}

func issueSession(w http.ResponseWriter, user string) error {
	claims := sessionClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		User: user,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(config.Auth.SessionSecret))
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    signed,
		Path:     "/",
		HttpOnly: true,
		Secure:   tlsEnabled(),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
	})
	return nil
}

// parseSessionToken validates a signed session token and returns the user it
// names. It is separate from sessionUser because the MCP endpoint accepts the
// same token as an Authorization: Bearer header — an MCP client has no cookie
// jar — and the two must agree on exactly what makes a session valid.
func parseSessionToken(raw string) (string, bool) {
	token, err := jwt.ParseWithClaims(raw, &sessionClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(config.Auth.SessionSecret), nil
	})
	if err != nil || !token.Valid {
		return "", false
	}
	claims, ok := token.Claims.(*sessionClaims)
	if !ok {
		return "", false
	}
	return claims.User, true
}

func sessionUser(r *http.Request) (string, bool) {
	cookie, err := r.Cookie("session")
	if err != nil {
		return "", false
	}
	return parseSessionToken(cookie.Value)
}

// --- Middleware ---

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := sessionUser(r); !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"unauthorized"}`))
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

// GET /auth/me
func handleAuthMe(w http.ResponseWriter, r *http.Request) {
	user, ok := sessionUser(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"user": user})
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
	if err := issueSession(w, body.Username); err != nil {
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
		Secure:   tlsEnabled(),
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

	var claims struct {
		Email string `json:"email"`
		Sub   string `json:"sub"`
	}
	idToken.Claims(&claims)

	user := claims.Email
	if user == "" {
		user = claims.Sub
	}

	if err := issueSession(w, user); err != nil {
		respondError(w, http.StatusInternalServerError, "session error")
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

// POST /auth/logout
func handleLogout(w http.ResponseWriter, r *http.Request) {
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
