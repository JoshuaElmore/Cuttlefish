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
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"
	httpSwagger "github.com/swaggo/http-swagger"
	_ "fs_api/docs"
)

var db *sql.DB

// --- Models ---

type UserStats struct {
	IDType         string `json:"id_type"`
	IDValue        int    `json:"id_value"`
	TotalSizeBytes int64  `json:"total_size_bytes"`
	FileCount      int    `json:"file_count"`
	Name           string `json:"name"`
}

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

// --- Middleware ---

func loggingMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		log.Printf("--> %s %s", r.Method, r.URL.String())
		next(w, r)
		log.Printf("<-- %s %s took %v", r.Method, r.URL.String(), time.Since(start))
	}
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("JSON encoding error: %v", err)
	}
}

func respondError(w http.ResponseWriter, code int, message string) {
	respondJSON(w, code, map[string]string{"error": message})
}

// --- Handlers ---

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
		respondError(w, http.StatusBadRequest, "path parameter is required")
		return
	}
	includeStats := r.URL.Query().Get("include_stats") == "true"

	var parentHash []byte
	err := db.QueryRow("SELECT path_hash FROM filesystem_index WHERE path = $1", path).Scan(&parentHash)
	if err == sql.ErrNoRows {
		respondError(w, http.StatusNotFound, "Directory not found")
		return
	} else if err != nil {
		log.Printf("Query error (resolve hash): %v", err)
		respondError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	query := `
		SELECT f.path, f.size_bytes, f.file_type, f.permissions, 
		       f.uid, f.gid, u.name, g.name, f.mtime, f.atime, f.ctime
		FROM filesystem_index f
		LEFT JOIN identity_map u ON f.uid = u.id AND u.id_type = 'uid'
		LEFT JOIN identity_map g ON f.gid = g.id AND g.id_type = 'gid'
		WHERE f.parent_hash = $1
	`
	if includeStats {
		query = `
			SELECT f.path, f.size_bytes, f.file_type, f.permissions, 
			       f.uid, f.gid, u.name, g.name, f.mtime, f.atime, f.ctime,
			       s.total_size_bytes, s.file_count, s.mtime_first, s.mtime_last,
			       s.atime_first, s.atime_last, s.ctime_first, s.ctime_last
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
		respondError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	defer rows.Close()

	var files []FileInfo
	for rows.Next() {
		var f FileInfo
		var scanErr error
		var userName, groupName sql.NullString

		if includeStats {
			var totalSize sql.NullInt64
			var fileCount sql.NullInt32
			var mf, ml, af, al, cf, cl sql.NullInt64

			scanErr = rows.Scan(
				&f.Path, &f.SizeBytes, &f.FileType, &f.Perms, &f.UID, &f.GID, &userName, &groupName, &f.MTime, &f.ATime, &f.CTime,
				&totalSize, &fileCount, &mf, &ml, &af, &al, &cf, &cl,
			)
			if totalSize.Valid {
				f.Aggregates = &DirAggregates{
					TotalSize: totalSize.Int64, FileCount: int(fileCount.Int32),
					MTimeFirst: mf.Int64, MTimeLast: ml.Int64,
					ATimeFirst: af.Int64, ATimeLast: al.Int64,
					CTimeFirst: cf.Int64, CTimeLast: cl.Int64,
				}
			}
		} else {
			scanErr = rows.Scan(&f.Path, &f.SizeBytes, &f.FileType, &f.Perms, &f.UID, &f.GID, &userName, &groupName, &f.MTime, &f.ATime, &f.CTime)
		}

		if scanErr != nil {
			log.Printf("Scan error: %v", scanErr)
			continue
		}

		if userName.Valid { f.User = userName.String } else { f.User = fmt.Sprintf("%d", f.UID) }
		if groupName.Valid { f.Group = groupName.String } else { f.Group = fmt.Sprintf("%d", f.GID) }
		f.Perms = formatPermissions(f.Perms)
		files = append(files, f)
	}

	respondJSON(w, http.StatusOK, files)
}

func GetFileStats(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		respondError(w, http.StatusBadRequest, "path parameter is required")
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
		respondError(w, http.StatusNotFound, "File not found")
		return
	} else if err != nil {
		log.Printf("Query error: %v", err)
		respondError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	if f.FileType != 1 {
		respondError(w, http.StatusBadRequest, "The provided path is not a file")
		return
	}

	f.UID, f.GID = uid, gid
	if userName.Valid { f.User = userName.String } else { f.User = fmt.Sprintf("%d", uid) }
	if groupName.Valid { f.Group = groupName.String } else { f.Group = fmt.Sprintf("%d", gid) }
	f.Perms = formatPermissions(f.Perms)

	respondJSON(w, http.StatusOK, f)
}

func GetDirStats(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		respondError(w, http.StatusBadRequest, "path parameter is required")
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
		respondError(w, http.StatusNotFound, "Directory not found")
		return
	} else if err != nil {
		log.Printf("Query error: %v", err)
		respondError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	if f.FileType != 2 {
		respondError(w, http.StatusBadRequest, "The provided path is not a directory")
		return
	}

	f.UID, f.GID = uid, gid
	if userName.Valid { f.User = userName.String } else { f.User = fmt.Sprintf("%d", uid) }
	if groupName.Valid { f.Group = groupName.String } else { f.Group = fmt.Sprintf("%d", gid) }
	f.Perms = formatPermissions(f.Perms)

	if includeStats {
		var totalSize sql.NullInt64
		var fileCount sql.NullInt32
		var mf, ml, af, al, cf, cl sql.NullInt64

		statQuery := `SELECT total_size_bytes, file_count, mtime_first, mtime_last, atime_first, atime_last, ctime_first, ctime_last FROM dir_stats WHERE path = $1`
		err := db.QueryRow(statQuery, path).Scan(&totalSize, &fileCount, &mf, &ml, &af, &al, &cf, &cl)
		if err == nil && totalSize.Valid {
			f.Aggregates = &DirAggregates{
				TotalSize: totalSize.Int64, FileCount: int(fileCount.Int32),
				MTimeFirst: mf.Int64, MTimeLast: ml.Int64,
				ATimeFirst: af.Int64, ATimeLast: al.Int64,
				CTimeFirst: cf.Int64, CTimeLast: cl.Int64,
			}
		}
	}

	respondJSON(w, http.StatusOK, f)
}

// ListUserStats handles GET /api/user/stats
// @Summary List all users with usage statistics
// @Description Returns a sorted list of all users and their total data usage.
// @Param sort_by query string false "Field to sort by (total_size_bytes, file_count, name). Default: total_size_bytes"
// @Param order query string false "Sort order (ASC, DESC). Default: DESC"
// @Param limit query int false "Number of items to return. Default: 100"
// @Param offset query int false "Number of items to skip. Default: 0"
// @Success 200 {array} UserStats
// @Failure 500 {string} Internal Server Error
// @Router /api/user/stats [get]
func ListUserStats(w http.ResponseWriter, r *http.Request) {
	sortBy := r.URL.Query().Get("sort_by")
	if sortBy == "" { sortBy = "total_size_bytes" }
	allowedSorts := map[string]string{"total_size_bytes": "s.total_size_bytes", "file_count": "s.file_count", "name": "COALESCE(i.name, CAST(s.id_value AS TEXT))"}
	col, ok := allowedSorts[sortBy]
	if !ok { col = "s.total_size_bytes" }

	order := r.URL.Query().Get("order")
	if order != "ASC" && order != "DESC" { order = "DESC" }

	limit := 100
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil { limit = l }
	offset := 0
	if o, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil { offset = o }

	query := fmt.Sprintf(`
		SELECT s.id_type, s.id_value, s.total_size_bytes, s.file_count, 
		       COALESCE(i.name, CAST(s.id_value AS TEXT))
		FROM user_stats s
		LEFT JOIN identity_map i ON s.id_value = i.id AND s.id_type = i.id_type
		WHERE s.id_type = 'uid'
		ORDER BY %s %s
		LIMIT %d OFFSET %d
	`, col, order, limit, offset)

	rows, err := db.Query(query)
	if err != nil {
		log.Printf("Query error (list users): %v", err)
		respondError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	defer rows.Close()

	var stats []UserStats
	for rows.Next() {
		var s UserStats
		if err := rows.Scan(&s.IDType, &s.IDValue, &s.TotalSizeBytes, &s.FileCount, &s.Name); err != nil {
			log.Printf("Scan error: %v", err)
			continue
		}
		stats = append(stats, s)
	}
	respondJSON(w, http.StatusOK, stats)
}

// ListGroupStats handles GET /api/group/stats
// @Summary List all groups with usage statistics
// @Description Returns a sorted list of all groups and their total data usage.
// @Param sort_by query string false "Field to sort by (total_size_bytes, file_count, name). Default: total_size_bytes"
// @Param order query string false "Sort order (ASC, DESC). Default: DESC"
// @Param limit query int false "Number of items to return. Default: 100"
// @Param offset query int false "Number of items to skip. Default: 0"
// @Success 200 {array} UserStats
// @Failure 500 {string} Internal Server Error
// @Router /api/group/stats [get]
func ListGroupStats(w http.ResponseWriter, r *http.Request) {
	sortBy := r.URL.Query().Get("sort_by")
	if sortBy == "" { sortBy = "total_size_bytes" }
	allowedSorts := map[string]string{"total_size_bytes": "s.total_size_bytes", "file_count": "s.file_count", "name": "COALESCE(i.name, CAST(s.id_value AS TEXT))"}
	col, ok := allowedSorts[sortBy]
	if !ok { col = "s.total_size_bytes" }

	order := r.URL.Query().Get("order")
	if order != "ASC" && order != "DESC" { order = "DESC" }

	limit := 100
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil { limit = l }
	offset := 0
	if o, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil { offset = o }

	query := fmt.Sprintf(`
		SELECT s.id_type, s.id_value, s.total_size_bytes, s.file_count, 
		       COALESCE(i.name, CAST(s.id_value AS TEXT))
		FROM user_stats s
		LEFT JOIN identity_map i ON s.id_value = i.id AND s.id_type = i.id_type
		WHERE s.id_type = 'gid'
		ORDER BY %s %s
		LIMIT %d OFFSET %d
	`, col, order, limit, offset)

	rows, err := db.Query(query)
	if err != nil {
		log.Printf("Query error (list groups): %v", err)
		respondError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	defer rows.Close()

	var stats []UserStats
	for rows.Next() {
		var s UserStats
		if err := rows.Scan(&s.IDType, &s.IDValue, &s.TotalSizeBytes, &s.FileCount, &s.Name); err != nil {
			log.Printf("Scan error: %v", err)
			continue
		}
		stats = append(stats, s)
	}
	respondJSON(w, http.StatusOK, stats)
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
	if _, err := os.Stat(staticPath); os.IsNotExist(err) {
		staticPath = "ui/build"
	}

	http.HandleFunc("/swagger/", httpSwagger.WrapHandler)
	http.HandleFunc("/api/list", loggingMiddleware(ListDirectory))
	http.HandleFunc("/api/file/stats", loggingMiddleware(GetFileStats))
	http.HandleFunc("/api/dir/stats", loggingMiddleware(GetDirStats))
	http.HandleFunc("/api/user/stats", loggingMiddleware(ListUserStats))
	http.HandleFunc("/api/group/stats", loggingMiddleware(ListGroupStats))

	fs := http.FileServer(http.Dir(staticPath))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fs.ServeHTTP(w, r)
	})

	fmt.Println("Server starting on :8080...")
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
		if digit&4 != 0 { res.WriteByte(read) } else { res.WriteByte(dash) }
		if digit&2 != 0 { res.WriteByte(write) } else { res.WriteByte(dash) }
		if digit&1 != 0 { res.WriteByte(exec) } else { res.WriteByte(dash) }
	}
	return res.String()
}
