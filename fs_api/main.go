package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

type FileStats struct {
	Path          string `json:"path"`
	SizeBytes     int64  `json:"size_bytes"`
	FileType      int    `json:"file_type"`
	Permissions   string `json:"permissions"`
	UID           int    `json:"uid"`
	GID           int    `json:"gid"`
	Atime         int64  `json:"atime"`
	Mtime         int64  `json:"mtime"`
	Ctime         int64  `json:"ctime"`
	Metadata      string `json:"metadata"`
}

type DirStats struct {
	Path           string `json:"path"`
	TotalSizeBytes int64  `json:"total_size_bytes"`
	LastModified   int64  `json:"last_modified"`
	LastAccessed   int64  `json:"last_accessed"`
	FileCount      int    `json:"file_count"`
}

type Entry struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	FileType int    `json:"type"` // 1: File, 2: Dir
	Mtime    int64  `json:"mtime"`
}

var db *sql.DB

func loggingMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		log.Printf("--> Request: %s %s", r.Method, r.URL.String())
		next(w, r)
		log.Printf("<-- Response: %s %s took %v", r.Method, r.URL.String(), time.Since(start))
	}
}

func main() {
	var err error
	connStr := "host=localhost user=postgres password=postgres dbname=fs_index sslmode=disable"
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	staticPath := filepath.Join(exeDir, "ui", "build")
	
	log.Printf("Serving static files from: %s", staticPath)

	http.HandleFunc("/api/v1/file", loggingMiddleware(getFile))
	http.HandleFunc("/api/v1/dir", loggingMiddleware(getDir))
	http.HandleFunc("/api/v1/list", loggingMiddleware(listDir))

	fs := http.FileServer(http.Dir(staticPath))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fs.ServeHTTP(w, r)
	})

	fmt.Println("Server starting on :8080...")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func listDir(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" { path = "/" }

	// Convert the requested path into its hash for the query
	// In a real production app, we might have a helper to handle this
	// For now, we'll query the DB to find the hash of the requested path.
	var parentHash []byte
	err := db.QueryRow("SELECT path_hash FROM filesystem_index WHERE path = $1", path).Scan(&parentHash)
	if err != nil {
		// If the path isn't in the DB, we can't find its children via parent_hash
		// We fallback to the LIKE query as a safety net
		log.Printf("Path %s not found in index, falling back to LIKE", path)
		fallbackList(w, path)
		return
	}

	// THE DAG QUERY:
	// Find all entries where parent_hash = current_node_hash
	query := "SELECT path, size_bytes, file_type, mtime FROM filesystem_index WHERE parent_hash = $1"
	rows, err := db.Query(query, parentHash)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	
	var entries []Entry
	for rows.Next() {
		var e Entry
		var fullPath string
		if err := rows.Scan(&fullPath, &e.Size, &e.FileType, &e.Mtime); err != nil { continue }
		
		// Extract name from full path
		parts := strings.Split(fullPath, "/")
		name := parts[len(parts)-1]
		if name == "" && fullPath != "/" {
			if len(parts) > 1 {
				name = parts[len(parts)-2]
			}
		}
		
		e.Path = fullPath
		e.Name = name
		entries = append(entries, e)
	}
	
	log.Printf("Path %s (hash: %x) -> Found %d children via DAG", path, parentHash, len(entries))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

func fallbackList(w http.ResponseWriter, path string) {
	normPath := path
	if normPath != "/" && !strings.HasSuffix(normPath, "/") {
		normPath += "/"
	}
	searchPattern := normPath + "%"
	rows, err := db.Query("SELECT path, size_bytes, file_type, mtime FROM filesystem_index WHERE path LIKE $1 LIMIT 1000", searchPattern)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var entries []Entry
	for rows.Next() {
		var e Entry
		var fullPath string
		if err := rows.Scan(&fullPath, &e.Size, &e.FileType, &e.Mtime); err != nil { continue }
		relative := strings.TrimPrefix(fullPath, normPath)
		if relative == "" || strings.Contains(relative, "/") { continue }
		e.Path = fullPath
		e.Name = relative
		entries = append(entries, e)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

func getFile(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	var fs FileStats
	err := db.QueryRow("SELECT path, size_bytes, file_type, permissions, uid, gid, atime, mtime, ctime, metadata FROM filesystem_index WHERE path = $1", path).
		Scan(&fs.Path, &fs.SizeBytes, &fs.FileType, &fs.Permissions, &fs.UID, &fs.GID, &fs.Atime, &fs.Mtime, &fs.Ctime, &fs.Metadata)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(fs)
}

func getDir(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	var ds DirStats
	err := db.QueryRow("SELECT path, total_size_bytes, last_modified, last_accessed, file_count FROM dir_stats WHERE path = $1", path).
		Scan(&ds.Path, &ds.TotalSizeBytes, &ds.LastModified, &ds.LastAccessed, &ds.FileCount)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ds)
}
