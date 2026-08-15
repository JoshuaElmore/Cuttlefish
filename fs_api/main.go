package main

// @title File System Index API
// @version 1.0
// @description API for querying a high-performance filesystem index.
// @host localhost:8080
// @BasePath /
// @securityDefinitions.apikeys.ApiKeyAuth
// @in header
// @name Authorization

import (
	"context"
	"database/sql"
	"fmt"
	"html"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	_ "fs_api/docs"
	_ "github.com/lib/pq"
	httpSwagger "github.com/swaggo/http-swagger"
)

var db *sql.DB

// titleTag matches the single <title>…</title> element Vite emits in index.html.
var titleTag = regexp.MustCompile(`(?is)<title>.*?</title>`)

// renderIndexHTML reads the built index.html and substitutes the configured
// page title. Doing it here rather than in the UI build keeps the title a
// deployment setting — one binary, one config file, no npm rebuild to retitle
// an instance — and it lands in the initial HTML, so the tab never flashes the
// default first. Returns nil when the file can't be read or has no <title>,
// which the caller treats as "serve the file as-is".
func renderIndexHTML(path, title string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if !titleTag.Match(raw) {
		return nil, fmt.Errorf("no <title> element in %s", path)
	}
	// Escaped: the title comes from a config file, but it is still text being
	// spliced into markup.
	return titleTag.ReplaceAll(raw, []byte("<title>"+html.EscapeString(title)+"</title>")), nil
}

func main() {
	loadConfig("fs_config.yml")

	if config.Auth.Mode() == "oidc" {
		if err := initOIDC(context.Background()); err != nil {
			log.Fatalf("OIDC init failed: %v", err)
		}
		log.Printf("Auth mode: OIDC (%s)", config.Auth.OIDC.Issuer)
	} else {
		log.Printf("Auth mode: local (user: %s)", config.Auth.Local.Username)
	}

	var err error
	db, err = sql.Open("postgres", config.Database.ConnStr())
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	checkPathHashWidth()

	exeDir := filepath.Dir(func() string { p, _ := os.Executable(); return p }())
	staticPath := filepath.Join(exeDir, "ui", "build")
	if _, err := os.Stat(staticPath); os.IsNotExist(err) {
		staticPath = "ui/build"
	}

	// Auth routes (unauthenticated)
	http.HandleFunc("/auth/mode", handleAuthMode)
	http.HandleFunc("/auth/me", handleAuthMe)
	http.HandleFunc("/auth/logout", handleLogout)
	if config.Auth.Mode() == "oidc" {
		http.HandleFunc("/auth/oidc/start", handleOIDCStart)
		http.HandleFunc("/auth/callback", handleOIDCCallback)
	} else {
		http.HandleFunc("/auth/login", handleLocalLogin)
	}

	// API routes (require session)
	http.HandleFunc("/swagger/", httpSwagger.WrapHandler)
	http.HandleFunc("/api/list", authMiddleware(loggingMiddleware(ListDirectory)))
	http.HandleFunc("/api/file/stats", authMiddleware(loggingMiddleware(GetFileStats)))
	http.HandleFunc("/api/dir/stats", authMiddleware(loggingMiddleware(GetDirStats)))
	http.HandleFunc("/api/user/list", authMiddleware(loggingMiddleware(ListUserStats)))
	http.HandleFunc("/api/group/list", authMiddleware(loggingMiddleware(ListGroupStats)))
	http.HandleFunc("/api/search", authMiddleware(loggingMiddleware(SearchFiles)))
	http.HandleFunc("/api/scans", authMiddleware(loggingMiddleware(ListScanSessions)))

	// MCP endpoint (same session auth as /api/*, or its own listener).
	registerMCP(http.DefaultServeMux)

	indexPath := filepath.Join(staticPath, "index.html")
	indexHTML, err := renderIndexHTML(indexPath, config.UI.Title)
	if err != nil {
		log.Printf("warning: serving index.html unmodified (%v)", err)
	}

	// The SPA fallback and "/" both have to go through serveIndex, not the file
	// server — otherwise the on-disk index.html is served with the title Vite
	// baked in, ignoring ui.title.
	serveIndex := func(w http.ResponseWriter, r *http.Request) {
		if indexHTML == nil {
			http.ServeFile(w, r, indexPath)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(indexHTML)
	}

	fileServer := http.FileServer(http.Dir(staticPath))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/swagger/") || strings.HasPrefix(r.URL.Path, "/auth/") {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			serveIndex(w, r)
			return
		}
		if _, err := os.Stat(filepath.Join(staticPath, r.URL.Path)); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}
		serveIndex(w, r)
	})

	addr := config.Server.ListenAddr
	if addr == "" {
		addr = ":8080"
	}

	if config.Server.TLSCert != "" && config.Server.TLSKey != "" {
		fmt.Printf("Server starting on %s (TLS)...\n", addr)
		log.Fatal(http.ListenAndServeTLS(addr, config.Server.TLSCert, config.Server.TLSKey, nil))
	} else {
		fmt.Printf("Server starting on %s (plain HTTP)...\n", addr)
		log.Fatal(http.ListenAndServe(addr, nil))
	}
}
