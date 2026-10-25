package main

import (
	"database/sql"
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

// AccessGrant gives every member of a group read access to one path prefix.
// GroupType "idp" matches the IdP groups claim; "unix" matches the Unix groups
// the indexer recorded for the user's mapped Unix name.
type AccessGrant struct {
	ID         int       `json:"id"`
	GroupType  string    `json:"group_type"`
	GroupName  string    `json:"group_name"`
	PathPrefix string    `json:"path_prefix"`
	CreatedBy  string    `json:"created_by"`
	CreatedAt  time.Time `json:"created_at"`
}

// AdminGrants handles /api/admin/grants (GET list, POST create, DELETE ?id=).
// @Summary Manage path access grants (admin only)
// @Description GET lists grants; POST {group_type ("idp" default, or "unix"), group_name, path_prefix} creates one; DELETE ?id= removes one.
// @Param id query int false "Grant id (DELETE only)"
// @Success 200 {array} AccessGrant
// @Failure 400 {string} Bad Request
// @Failure 403 {string} Forbidden
// @Failure 409 {string} Conflict
// @Router /api/admin/grants [get]
func AdminGrants(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		rows, err := db.Query(`SELECT id, group_type, group_name, path_prefix, created_by, created_at FROM access_grants ORDER BY group_type, group_name, path_prefix`)
		if err != nil {
			log.Printf("Query error (grants): %v", err)
			respondError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		defer rows.Close()
		grants := []AccessGrant{}
		for rows.Next() {
			var g AccessGrant
			if err := rows.Scan(&g.ID, &g.GroupType, &g.GroupName, &g.PathPrefix, &g.CreatedBy, &g.CreatedAt); err != nil {
				log.Printf("Scan error (grants): %v", err)
				respondError(w, http.StatusInternalServerError, "Internal server error")
				return
			}
			grants = append(grants, g)
		}
		respondJSON(w, http.StatusOK, grants)

	case http.MethodPost:
		var body struct {
			GroupType  string `json:"group_type"`
			GroupName  string `json:"group_name"`
			PathPrefix string `json:"path_prefix"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
			respondError(w, http.StatusBadRequest, "Invalid request body")
			return
		}
		group := strings.TrimSpace(body.GroupName)
		prefix := strings.TrimSpace(body.PathPrefix)
		groupType := body.GroupType
		if groupType == "" {
			groupType = "idp" // the API predates Unix-group grants
		}
		if groupType != "idp" && groupType != "unix" {
			respondError(w, http.StatusBadRequest, `group_type must be "idp" or "unix"`)
			return
		}
		if group == "" || !strings.HasPrefix(prefix, "/") {
			respondError(w, http.StatusBadRequest, "group_name is required and path_prefix must be an absolute path")
			return
		}
		if groupType == "unix" && group == grantEveryone {
			respondError(w, http.StatusBadRequest, `"*" (everyone) is an IdP-group grant; use group_type "idp"`)
			return
		}
		// Stored canonical, since access.allows compares canonical paths.
		prefix = path.Clean(prefix)
		g := AccessGrant{GroupType: groupType, GroupName: group, PathPrefix: prefix, CreatedBy: accessFrom(r).User}
		err := db.QueryRow(`
			INSERT INTO access_grants (group_type, group_name, path_prefix, created_by) VALUES ($1, $2, $3, $4)
			RETURNING id, created_at`, g.GroupType, g.GroupName, g.PathPrefix, g.CreatedBy).Scan(&g.ID, &g.CreatedAt)
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			respondError(w, http.StatusConflict, "that group already has this prefix")
			return
		} else if err != nil {
			log.Printf("Insert error (grants): %v", err)
			respondError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		log.Printf("admin %q granted %s group %q access to %s", g.CreatedBy, g.GroupType, g.GroupName, g.PathPrefix)
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

// IdentityRule is one IdP → Unix username suffix rewrite (see mapIdentity).
type IdentityRule struct {
	ID          int       `json:"id"`
	MatchSuffix string    `json:"match_suffix"`
	ReplaceWith string    `json:"replace_with"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
}

// IdentityPreview is what an IdP identity maps to under the current rules,
// and the access it would get with IdPGroups.
type IdentityPreview struct {
	Identity string   `json:"identity"`
	UnixUser string   `json:"unix_user"`
	UID      *int     `json:"uid"`    // nil when the scan never saw this user
	Groups   []string `json:"groups"` // Unix groups, from identity_membership
	// IdPGroups is what the access below was computed with: the ?groups= the
	// admin typed, else the groups of the identity's latest live session.
	IdPGroups       []string       `json:"idp_groups"`
	IdPGroupsSource string         `json:"idp_groups_source"` // "given", "session" or "none"
	Role            string         `json:"role"`
	Grants          []MatchedGrant `json:"grants"` // only for the restricted role; others see everything
}

// AdminIdentityRules handles /api/admin/identity-rules (GET list, or GET
// ?preview=<identity>; POST create; DELETE ?id=).
// @Summary Manage IdP → Unix username mapping rules (admin only)
// @Description GET lists rules, or with ?preview= shows what an identity maps to and the access grants it would get; POST {match_suffix, replace_with} creates one; DELETE ?id= removes one.
// @Param preview query string false "IdP identity to map (GET only)"
// @Param groups query string false "Comma-separated IdP groups to preview with (GET ?preview= only); default: the identity's latest live session's groups"
// @Param id query int false "Rule id (DELETE only)"
// @Success 200 {array} IdentityRule
// @Failure 400 {string} Bad Request
// @Failure 403 {string} Forbidden
// @Failure 409 {string} Conflict
// @Router /api/admin/identity-rules [get]
func AdminIdentityRules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if identity := r.URL.Query().Get("preview"); identity != "" {
			previewIdentity(w, identity, r.URL.Query().Get("groups"))
			return
		}
		rows, err := db.Query(`SELECT id, match_suffix, replace_with, created_by, created_at FROM identity_rules ORDER BY match_suffix`)
		if err != nil {
			log.Printf("Query error (identity rules): %v", err)
			respondError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		defer rows.Close()
		rules := []IdentityRule{}
		for rows.Next() {
			var ir IdentityRule
			if err := rows.Scan(&ir.ID, &ir.MatchSuffix, &ir.ReplaceWith, &ir.CreatedBy, &ir.CreatedAt); err != nil {
				log.Printf("Scan error (identity rules): %v", err)
				respondError(w, http.StatusInternalServerError, "Internal server error")
				return
			}
			rules = append(rules, ir)
		}
		respondJSON(w, http.StatusOK, rules)

	case http.MethodPost:
		var body struct {
			MatchSuffix string `json:"match_suffix"`
			ReplaceWith string `json:"replace_with"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
			respondError(w, http.StatusBadRequest, "Invalid request body")
			return
		}
		// Stored lowercased: mapIdentity compares case-insensitively and Unix
		// names are conventionally lowercase.
		ir := IdentityRule{
			MatchSuffix: strings.ToLower(strings.TrimSpace(body.MatchSuffix)),
			ReplaceWith: strings.ToLower(strings.TrimSpace(body.ReplaceWith)),
			CreatedBy:   accessFrom(r).User,
		}
		if ir.MatchSuffix == ir.ReplaceWith {
			respondError(w, http.StatusBadRequest, "match_suffix and replace_with are identical; the rule would do nothing")
			return
		}
		if strings.ContainsAny(ir.ReplaceWith, "/\x00") {
			respondError(w, http.StatusBadRequest, "replace_with must not contain '/'")
			return
		}
		err := db.QueryRow(`
			INSERT INTO identity_rules (match_suffix, replace_with, created_by) VALUES ($1, $2, $3)
			RETURNING id, created_at`, ir.MatchSuffix, ir.ReplaceWith, ir.CreatedBy).Scan(&ir.ID, &ir.CreatedAt)
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			respondError(w, http.StatusConflict, "a rule for that suffix already exists")
			return
		} else if err != nil {
			log.Printf("Insert error (identity rules): %v", err)
			respondError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		log.Printf("admin %q added identity rule %q -> %q", ir.CreatedBy, ir.MatchSuffix, ir.ReplaceWith)
		respondJSON(w, http.StatusCreated, ir)

	case http.MethodDelete:
		id, err := strconv.Atoi(r.URL.Query().Get("id"))
		if err != nil {
			respondError(w, http.StatusBadRequest, "id parameter is required")
			return
		}
		if _, err := db.Exec(`DELETE FROM identity_rules WHERE id = $1`, id); err != nil {
			log.Printf("Delete error (identity rules): %v", err)
			respondError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		log.Printf("admin %q deleted identity rule %d", accessFrom(r).User, id)
		w.WriteHeader(http.StatusNoContent)

	default:
		respondError(w, http.StatusMethodNotAllowed, "GET, POST or DELETE required")
	}
}

func previewIdentity(w http.ResponseWriter, identity, groupsParam string) {
	rules, err := loadIdentityRules()
	if err != nil {
		log.Printf("Query error (identity rules): %v", err)
		respondError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	p := IdentityPreview{Identity: identity, UnixUser: mapIdentity(identity, rules), Groups: []string{},
		IdPGroups: []string{}, IdPGroupsSource: "none", Grants: []MatchedGrant{}}

	for _, g := range strings.Split(groupsParam, ",") {
		if g = strings.TrimSpace(g); g != "" {
			p.IdPGroups = append(p.IdPGroups, g)
		}
	}
	if len(p.IdPGroups) > 0 {
		p.IdPGroupsSource = "given"
	} else {
		err := db.QueryRow(`SELECT groups FROM sessions WHERE lower(username) = lower($1) AND expires_at > now()
			ORDER BY checked_at DESC LIMIT 1`, identity).Scan(pq.Array(&p.IdPGroups))
		if err == nil {
			p.IdPGroupsSource = "session"
		} else if err != sql.ErrNoRows {
			log.Printf("Query error (identity preview session): %v", err)
			respondError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
	}
	p.Role = roleFor(p.IdPGroups)
	if p.Role == roleRestricted {
		if p.Grants, err = matchingGrants(p.IdPGroups, p.UnixUser); err != nil {
			log.Printf("Query error (identity preview grants): %v", err)
			respondError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
	}

	var uid int
	err = db.QueryRow(`SELECT id FROM identity_map WHERE id_type = 'uid' AND name = $1`, p.UnixUser).Scan(&uid)
	if err == nil {
		p.UID = &uid
		rows, err := db.Query(`
			SELECT COALESCE(g.name, CAST(m.gid AS TEXT)) FROM identity_membership m
			LEFT JOIN identity_map g ON g.id = m.gid AND g.id_type = 'gid'
			WHERE m.uid = $1 ORDER BY 1`, uid)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var g string
				if rows.Scan(&g) == nil {
					p.Groups = append(p.Groups, g)
				}
			}
		}
	} else if err != sql.ErrNoRows {
		log.Printf("Query error (identity preview): %v", err)
		respondError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	respondJSON(w, http.StatusOK, p)
}
