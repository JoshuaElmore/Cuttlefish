package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
)

// AccessGrant gives every member of an IdP group read access to one path prefix.
type AccessGrant struct {
	ID         int       `json:"id"`
	GroupName  string    `json:"group_name"`
	PathPrefix string    `json:"path_prefix"`
	CreatedBy  string    `json:"created_by"`
	CreatedAt  time.Time `json:"created_at"`
}

// ActiveSession is a live session as the admin page lists it. ID is a short
// prefix of the stored hash, for display only; revocation is per user.
type ActiveSession struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Groups    []string  `json:"groups"`
	CreatedAt time.Time `json:"created_at"`
	CheckedAt time.Time `json:"checked_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// AdminGrants handles /api/admin/grants (GET list, POST create, DELETE ?id=).
// @Summary Manage path access grants (admin only)
// @Description GET lists grants; POST {group_name, path_prefix} creates one; DELETE ?id= removes one.
// @Param id query int false "Grant id (DELETE only)"
// @Success 200 {array} AccessGrant
// @Failure 400 {string} Bad Request
// @Failure 403 {string} Forbidden
// @Failure 409 {string} Conflict
// @Router /api/admin/grants [get]
func AdminGrants(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		rows, err := db.Query(`SELECT id, group_name, path_prefix, created_by, created_at FROM access_grants ORDER BY group_name, path_prefix`)
		if err != nil {
			log.Printf("Query error (grants): %v", err)
			respondError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		defer rows.Close()
		grants := []AccessGrant{}
		for rows.Next() {
			var g AccessGrant
			if err := rows.Scan(&g.ID, &g.GroupName, &g.PathPrefix, &g.CreatedBy, &g.CreatedAt); err != nil {
				log.Printf("Scan error (grants): %v", err)
				respondError(w, http.StatusInternalServerError, "Internal server error")
				return
			}
			grants = append(grants, g)
		}
		respondJSON(w, http.StatusOK, grants)

	case http.MethodPost:
		var body struct {
			GroupName  string `json:"group_name"`
			PathPrefix string `json:"path_prefix"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
			respondError(w, http.StatusBadRequest, "Invalid request body")
			return
		}
		group := strings.TrimSpace(body.GroupName)
		prefix := strings.TrimSpace(body.PathPrefix)
		if group == "" || !strings.HasPrefix(prefix, "/") {
			respondError(w, http.StatusBadRequest, "group_name is required and path_prefix must be an absolute path")
			return
		}
		// Stored canonical, since access.allows compares canonical paths.
		prefix = path.Clean(prefix)
		g := AccessGrant{GroupName: group, PathPrefix: prefix, CreatedBy: accessFrom(r).User}
		err := db.QueryRow(`
			INSERT INTO access_grants (group_name, path_prefix, created_by) VALUES ($1, $2, $3)
			RETURNING id, created_at`, g.GroupName, g.PathPrefix, g.CreatedBy).Scan(&g.ID, &g.CreatedAt)
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			respondError(w, http.StatusConflict, "that group already has this prefix")
			return
		} else if err != nil {
			log.Printf("Insert error (grants): %v", err)
			respondError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		log.Printf("admin %q granted %q access to %s", g.CreatedBy, g.GroupName, g.PathPrefix)
		respondJSON(w, http.StatusCreated, g)

	case http.MethodDelete:
		id, err := strconv.Atoi(r.URL.Query().Get("id"))
		if err != nil {
			respondError(w, http.StatusBadRequest, "id parameter is required")
			return
		}
		if _, err := db.Exec(`DELETE FROM access_grants WHERE id = $1`, id); err != nil {
			log.Printf("Delete error (grants): %v", err)
			respondError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		log.Printf("admin %q deleted grant %d", accessFrom(r).User, id)
		w.WriteHeader(http.StatusNoContent)

	default:
		respondError(w, http.StatusMethodNotAllowed, "GET, POST or DELETE required")
	}
}

// AdminSessions handles /api/admin/sessions (GET list, DELETE ?user= revokes all of that user's sessions).
// @Summary List or revoke active sessions (admin only)
// @Param user query string false "Username whose sessions to revoke (DELETE only)"
// @Success 200 {array} ActiveSession
// @Failure 403 {string} Forbidden
// @Router /api/admin/sessions [get]
func AdminSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		rows, err := db.Query(`
			SELECT id_hash, username, groups, created_at, checked_at, expires_at
			FROM sessions WHERE expires_at > now() ORDER BY username, created_at DESC`)
		if err != nil {
			log.Printf("Query error (sessions): %v", err)
			respondError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		defer rows.Close()
		sessions := []ActiveSession{}
		for rows.Next() {
			var s ActiveSession
			var idHash []byte
			if err := rows.Scan(&idHash, &s.Username, pq.Array(&s.Groups), &s.CreatedAt, &s.CheckedAt, &s.ExpiresAt); err != nil {
				log.Printf("Scan error (sessions): %v", err)
				respondError(w, http.StatusInternalServerError, "Internal server error")
				return
			}
			s.ID = hex.EncodeToString(idHash[:4])
			sessions = append(sessions, s)
		}
		respondJSON(w, http.StatusOK, sessions)

	case http.MethodDelete:
		user := r.URL.Query().Get("user")
		if user == "" {
			respondError(w, http.StatusBadRequest, "user parameter is required")
			return
		}
		res, err := db.Exec(`DELETE FROM sessions WHERE username = $1`, user)
		if err != nil {
			log.Printf("Delete error (sessions): %v", err)
			respondError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		n, _ := res.RowsAffected()
		log.Printf("admin %q revoked %d session(s) of %q", accessFrom(r).User, n, user)
		w.WriteHeader(http.StatusNoContent)

	default:
		respondError(w, http.StatusMethodNotAllowed, "GET or DELETE required")
	}
}
