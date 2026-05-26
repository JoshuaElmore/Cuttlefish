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
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/lib/pq"
	httpSwagger "github.com/swaggo/http-swagger"
	_ "fs_api/docs"
)

var db *sql.DB

// DirAggregates holds aggregated statistics for directories
type DirAggregates struct {
	TotalSize  int64 `json:"total_size_bytes"`
	FileCount  int   `json:"file_count"`
	MTimeFirst int64 `json:"mtime_first"`
	MTimeLast  int64 `json:"mtime_last"`
	ATimeFirst int64 `json:"atime_first"`
	ATimeLast  int64 `json:"atime_last"`
	CTimeFirst int64 `json:"ctime_first"`
	CTimeLast  int64 `json:"ctime_last"`
}

// FileInfo represents a single file or directory entry
type FileInfo struct {
	Path       string          `json:"path"`
	SizeBytes  int64           `json:"size_bytes"`
	FileType   int             `json:"file_type"`
	Perms      string          `json:"permissions"`
	UID        int             `json:"uid"`
	GID        int             `json:"gid"`
	User       string          `json:"user"`
	Group      string          `json:"group"`
	MTime      int64           `json:"mtime"`
	ATime      int64           `json:"atime"`
	CTime      int64           `json:"ctime"`
	Aggregates *DirAggregates  `json:"aggregates,omitempty"`
}

// DirStats represents aggregated statistics for a directory
type DirStats struct {
	Path            string `json:"path"`
	TotalSizeBytes  int64  `json:"total_size_bytes"`
	FileCount       int    `json:"file_count"`
	MTimeFirst      int64  `json:"mtime_first"`
	MTimeLast       int64  `json:"mtime_last"`
}

func loggingMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		log.Printf("--> Request: %s %s", r.Method, r.URL.String())
		next(w, r)
		log.Printf("<-- Response: %s %s took %v", r.Method, r.URL.String(), time.Since(start))
	}
}

// ListDirectory handles GET /api/list?path=/some/path&include_stats=true
// @Summary List directory contents
// @Description Returns a list of files and directories within the specified path using high-performance hash lookup internally.
// @Param path query string true "The directory path to list"
// @Param include_stats query boolean false "Include aggregated stats from dir_stats table for directories"
// @Success 200 {array} FileInfo
// @Failure 404 {string} Not Found
// @Failure 500 {string} Internal Server Error
// @Router /api/list [get]
func ListDirectory(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "path parameter is required", http.StatusBadRequest)
		return
	}
	includeStats := r.URL.Query().Get("include_stats") == "true"

	var parentHash []byte
	err := db.QueryRow("SELECT path_hash FROM filesystem_index WHERE path = $1", path).Scan(&parentHash)
	if err == sql.ErrNoRows {
		http.Error(w, "Directory not found", http.StatusNotFound)
		return
	} else if err != nil {
		log.Printf("Query error (resolve hash): %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	query := `
		SELECT f.path, f.size_bytes, f.file_type, f.permissions, 
		       f.uid, f.gid,
		       u.name, g.name,
		       f.mtime, f.atime, f.ctime
		FROM filesystem_index f
		LEFT JOIN identity_map u ON f.uid = u.id AND u.id_type = 'uid'
		LEFT JOIN identity_map g ON f.gid = g.id AND g.id_type = 'gid'
		WHERE f.parent_hash = $1
	`
	
	if includeStats {
		query = `
			SELECT f.path, f.size_bytes, f.file_type, f.permissions, 
			       f.uid, f.gid,
			       u.name, g.name,
			       f.mtime, f.atime, f.ctime,
			       s.total_size_bytes, s.file_count, 
			       s.mtime_first, s.mtime_last,
			       s.atime_first, s.atime_last,
			       s.ctime_first, s.ctime_last
			FROM filesystem_index f
			LEFT JOIN identity_map u ON f.uid = u.id AND u.id_type = 'uid'
			LEFT JOIN identity_map g ON f.gid = g.id AND g.id_type = 'gid'
			LEFT JOIN dir_stats s ON f.path_hash = s.path_hash
			WHERE f.parent_hash = $1
		`
	}

	rows, err := db.Query(query, parentHash)
	if err != nil {
		log.Printf("Query error (list children): %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var files []FileInfo
	for rows.Next() {
		var f FileInfo
		var scanErr error
		
		if includeStats {
			var totalSize sql.NullInt64
			var fileCount sql.NullInt32
			var mf, ml, af, al, cf, cl sql.NullInt64
			var userName, groupName sql.NullString

			scanErr = rows.Scan(
				&f.Path, &f.SizeBytes, &f.FileType, &f.Perms, &f.UID, &f.GID, &userName, &groupName, &f.MTime, &f.ATime, &f.CTime,
				&totalSize, &fileCount, &mf, &ml, &af, &al, &cf, &cl,
			)
			
			if userName.Valid { f.User = userName.String } else { f.User = fmt.Sprintf("%d", f.UID) }
			if groupName.Valid { f.Group = groupName.String } else { f.Group = fmt.Sprintf("%d", f.GID) }
			
			if totalSize.Valid {
				f.Aggregates = &DirAggregates{
					TotalSize:  totalSize.Int64,
					FileCount:  int(fileCount.Int32),
					MTimeFirst: mf.Int64,
					MTimeLast:  ml.Int64,
					ATimeFirst: af.Int64,
					ATimeLast:  al.Int64,
					CTimeFirst: cf.Int64,
					CTimeLast:  cl.Int64,
				}
			}
		} else {
			var userName, groupName sql.NullString
			scanErr = rows.Scan(&f.Path, &f.SizeBytes, &f.FileType, &f.Perms, &f.UID, &f.GID, &userName, &groupName, &f.MTime, &f.ATime, &f.CTime)
			if userName.Valid { f.User = userName.String } else { f.User = fmt.Sprintf("%d", f.UID) }
			if groupName.Valid { f.Group = groupName.String } else { f.Group = fmt.Sprintf("%d", f.GID) }
		}

		if scanErr != nil {
			log.Printf("Scan error: %v", scanErr)
			continue
		}
		f.Perms = formatPermissions(f.Perms)
		files = append(files, f)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
}

// GetFileStats handles GET /api/file/stats?path=/some/file
// @Summary Get detailed file statistics
// @Description Returns detailed metadata for a specific file. Returns 400 if the path is a directory.
// @Param path query string true "The path to the file"
// @Success 200 {object} FileInfo
// @Failure 400 {string} Bad Request - Path is a directory
// @Failure 404 {string} Not Found
// @Failure 500 {string} Internal Server Error
// @Router /api/file/stats [get]
func GetFileStats(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "path parameter is required", http.StatusBadRequest)
		return
	}

	var f FileInfo
	var uid, gid int
	var userName, groupName sql.NullString

	query := `
		SELECT f.path, f.size_bytes, f.file_type, f.permissions, f.uid, f.gid, 
		       u.name, g.name, f.mtime, f.atime, f.ctime
		FROM filesystem_index f
		LEFT JOIN identity_map u ON f.uid = u.id AND u.id_type = 'uid'
		LEFT JOIN identity_map g ON f.gid = g.id AND g.id_type = 'gid'
		WHERE f.path = $1
	`
	err := db.QueryRow(query, path).Scan(&f.Path, &f.SizeBytes, &f.FileType, &f.Perms, &uid, &gid, &userName, &groupName, &f.MTime, &f.ATime, &f.CTime)
	if err == sql.ErrNoRows {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	} else if err != nil {
		log.Printf("Query error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if f.FileType != 1 {
		http.Error(w, "The provided path is not a file", http.StatusBadRequest)
		return
	}

	f.UID = uid
	f.GID = gid
	if userName.Valid { f.User = userName.String } else { f.User = fmt.Sprintf("%d", uid) }
	if groupName.Valid { f.Group = groupName.String } else { f.Group = fmt.Sprintf("%d", gid) }
	f.Perms = formatPermissions(f.Perms)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(f)
}

// GetDirStats handles GET /api/dir/stats?path=/some/path&include_stats=true
// @Summary Get directory statistics
// @Description Returns detailed metadata for a specific directory. If include_stats=true, returns aggregated info from dir_stats.
// @Param path query string true "The directory path for stats"
// @Param include_stats query boolean false "Include aggregated stats from dir_stats table"
// @Success 200 {object} FileInfo
// @Failure 400 {string} Bad Request - Path is not a directory
// @Failure 404 {string} Not Found
// @Failure 500 {string} Internal Server Error
// @Router /api/dir/stats [get]
func GetDirStats(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "path parameter is required", http.StatusBadRequest)
		return
	}
	includeStats := r.URL.Query().Get("include_stats") == "true"

	var f FileInfo
	var uid, gid int
	var userName, groupName sql.NullString

	query := `
		SELECT f.path, f.size_bytes, f.file_type, f.permissions, f.uid, f.gid, 
		       u.name, g.name, f.mtime, f.atime, f.ctime
		FROM filesystem_index f
		LEFT JOIN identity_map u ON f.uid = u.id AND u.id_type = 'uid'
		LEFT JOIN identity_map g ON f.gid = g.id AND g.id_type = 'gid'
		WHERE f.path = $1
	`
	err := db.QueryRow(query, path).Scan(&f.Path, &f.SizeBytes, &f.FileType, &f.Perms, &uid, &gid, &userName, &groupName, &f.MTime, &f.ATime, &f.CTime)
	if err == sql.ErrNoRows {
		http.Error(w, "Directory not found", http.StatusNotFound)
		return
	} else if err != nil {
		log.Printf("Query error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if f.FileType != 2 {
		http.Error(w, "The provided path is not a directory", http.StatusBadRequest)
		return
	}

	f.UID = uid
	f.GID = gid
	if userName.Valid { f.User = userName.String } else { f.User = fmt.Sprintf("%d", uid) }
	if groupName.Valid { f.Group = groupName.String } else { f.Group = fmt.Sprintf("%d", gid) }
	f.Perms = formatPermissions(f.Perms)

	if includeStats {
		var totalSize sql.NullInt64
		var fileCount sql.NullInt32
		var mf, ml, af, al, cf, cl sql.NullInt64

		statQuery := `
			SELECT total_size_bytes, file_count, mtime_first, mtime_last, atime_first, atime_last, ctime_first, ctime_last
			FROM dir_stats
			WHERE path = $1
		`
		err := db.QueryRow(statQuery, path).Scan(&totalSize, &fileCount, &mf, &ml, &af, &al, &cf, &cl)
		if err == nil && totalSize.Valid {
			f.Aggregates = &DirAggregates{
				TotalSize:  totalSize.Int64,
				FileCount:  int(fileCount.Int32),
				MTimeFirst: mf.Int64,
				MTimeLast:  ml.Int64,
				ATimeFirst: af.Int64,
				ATimeLast:  al.Int64,
				CTimeFirst: cf.Int64,
				CTimeLast:  cl.Int64,
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(f)
}

func main() {
	var err error
	connStr := "host=localhost user=postgres password=postgres dbname=fs_index sslmode=disable"
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Static file serving for the UI
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	staticPath := filepath.Join(exeDir, "ui", "build")
	if _, err := os.Stat(staticPath); os.IsNotExist(err) {
		staticPath = "ui/build"
	}
	log.Printf("Serving static files from: %s", staticPath)

	// Swagger UI
	http.HandleFunc("/swagger/", httpSwagger.WrapHandler)

	// API routes
	http.HandleFunc("/api/list", loggingMiddleware(ListDirectory))
	http.HandleFunc("/api/file/stats", loggingMiddleware(GetFileStats))
	http.HandleFunc("/api/dir/stats", loggingMiddleware(GetDirStats))

	// Root handler to serve the UI
	fs := http.FileServer(http.Dir(staticPath))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fs.ServeHTTP(w, r)
	})

	fmt.Println("Server starting on :8080...")
	fmt.Println("Swagger UI available at http://localhost:8080/swagger/index.html")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func formatPermissions(perm string) string {
	if len(perm) != 3 {
		return perm
	}

	const read = 'r'; const write = 'w'; const exec = 'x'; const dash = '-'

	var res strings.Builder
	for i := 0; i < 3; i++ {
		digit := perm[i] - '0'
		if digit&4 != 0 {
			res.WriteByte(read)
		} else {
			res.WriteByte(dash)
		}
		if digit&2 != 0 {
			res.WriteByte(write)
		} else {
			res.WriteByte(dash)
		}
		if digit&1 != 0 {
			res.WriteByte(exec)
		} else {
			res.WriteByte(dash)
		}
	}
	return res.String()
}
