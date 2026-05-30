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
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	_ "fs_api/docs"
	_ "github.com/lib/pq"
	httpSwagger "github.com/swaggo/http-swagger"
)

var db *sql.DB

func main() {
	loadConfig("fs_config.toml")

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

	fileServer := http.FileServer(http.Dir(staticPath))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/swagger/") || strings.HasPrefix(r.URL.Path, "/auth/") {
			http.NotFound(w, r)
			return
		}
		if _, err := os.Stat(filepath.Join(staticPath, r.URL.Path)); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(staticPath, "index.html"))
	})

	fmt.Println("Server starting on :8080...")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
