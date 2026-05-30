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
	var err error
	connStr := "host=localhost user=postgres password=postgres dbname=fs_index sslmode=disable"
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	exeDir := filepath.Dir(func() string { p, _ := os.Executable(); return p }())
	staticPath := filepath.Join(exeDir, "ui", "build")
	if _, err := os.Stat(staticPath); os.IsNotExist(err) {
		staticPath = "ui/build"
	}

	http.HandleFunc("/swagger/", httpSwagger.WrapHandler)
	http.HandleFunc("/api/list", loggingMiddleware(ListDirectory))
	http.HandleFunc("/api/file/stats", loggingMiddleware(GetFileStats))
	http.HandleFunc("/api/dir/stats", loggingMiddleware(GetDirStats))
	http.HandleFunc("/api/user/list", loggingMiddleware(ListUserStats))
	http.HandleFunc("/api/group/list", loggingMiddleware(ListGroupStats))

	fileServer := http.FileServer(http.Dir(staticPath))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/swagger/") {
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
