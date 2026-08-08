package main

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
)

// pathHashLen is the byte width of the path_hash / parent_hash keys and must
// stay equal to fs_common::PATH_HASH_LEN in the Rust binaries that write them.
// A SHA-256 truncated to 128 bits: collisions stay negligible (~10^-23 across
// an index of 10^8 entries) while the two largest indexes in the database, both
// almost entirely key bytes, are half the size they would be at the full
// digest. checkPathHashWidth reports a mismatch at startup.
const pathHashLen = 16

// pathHash mirrors fs_common::compute_hash — the leading pathHashLen bytes of
// SHA-256 of the path string — so path lookups hit the path_hash primary key
// instead of scanning the unindexed path column.
func pathHash(path string) []byte {
	h := sha256.Sum256([]byte(path))
	return h[:pathHashLen]
}

// checkPathHashWidth warns when the index was written with a different hash
// width than pathHashLen. Such an index is unreadable rather than merely stale
// — every path lookup and every parent→child link is keyed by a hash this
// process recomputes from the path, so nothing would ever match and the API
// would answer 404 for a filesystem that is fully indexed. Non-fatal: the
// operator may well be starting the API before the first scan, and an empty or
// absent table is reported as width 0.
func checkPathHashWidth() {
	var width int
	err := db.QueryRow(`
		SELECT COALESCE((SELECT octet_length(path_hash) FROM filesystem_index LIMIT 1), 0)
		WHERE to_regclass('filesystem_index') IS NOT NULL
	`).Scan(&width)
	if err == sql.ErrNoRows || (err == nil && width == 0) {
		log.Printf("warning: filesystem_index is empty or absent; run fs_indexer to populate it")
		return
	}
	if err != nil {
		log.Printf("warning: could not verify the index path-hash width: %v", err)
		return
	}
	if width != pathHashLen {
		log.Printf(
			"warning: filesystem_index holds %d-byte path hashes but this build expects %d; "+
				"path lookups will return nothing until fs_indexer rebuilds the index",
			width, pathHashLen)
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
		respondError(w, http.StatusBadRequest, "path parameter is required")
		return
	}
	includeStats := r.URL.Query().Get("include_stats") == "true"

	// The children link is parent_hash = SHA-256(path), so no resolve query is
	// needed — compute the hash locally.
	parentHash := pathHash(path)

	query := `
		SELECT f.path, f.path_raw, f.size_bytes, f.file_type, f.permissions,
		       f.uid, f.gid, u.name, g.name, f.mtime, f.atime, f.ctime
		FROM filesystem_index f
		LEFT JOIN identity_map u ON f.uid = u.id AND u.id_type = 'uid'
		LEFT JOIN identity_map g ON f.gid = g.id AND g.id_type = 'gid'
		WHERE f.parent_hash = $1
	`
	if includeStats {
		query = `
			SELECT f.path, f.path_raw, f.size_bytes, f.file_type, f.permissions,
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
				&f.Path, &f.PathRaw, &f.SizeBytes, &f.FileType, &f.Perms, &f.UID, &f.GID,
				&userName, &groupName, &f.MTime, &f.ATime, &f.CTime,
				&totalSize, &fileCount, &mf, &ml, &af, &al, &cf, &cl,
			)
			if totalSize.Valid {
				f.Aggregates = &DirAggregates{
					TotalSize:  totalSize.Int64,
					FileCount:  int(fileCount.Int32),
					MTimeFirst: mf.Int64, MTimeLast: ml.Int64,
					ATimeFirst: af.Int64, ATimeLast: al.Int64,
					CTimeFirst: cf.Int64, CTimeLast: cl.Int64,
				}
			}
		} else {
			scanErr = rows.Scan(
				&f.Path, &f.PathRaw, &f.SizeBytes, &f.FileType, &f.Perms, &f.UID, &f.GID,
				&userName, &groupName, &f.MTime, &f.ATime, &f.CTime,
			)
		}

		if scanErr != nil {
			log.Printf("Scan error: %v", scanErr)
			continue
		}

		resolveIdentity(&f, userName, groupName)
		f.Perms = formatPermissions(f.Perms)
		files = append(files, f)
	}

	// Must run before the empty check below: a mid-iteration failure would
	// otherwise look like an empty directory and answer 404.
	if err := rows.Err(); err != nil {
		log.Printf("Row iteration error (list children): %v", err)
		respondError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	if len(files) == 0 {
		// Distinguish an empty directory from a path that isn't indexed at all;
		// this primary-key lookup only runs in the empty/missing case.
		var exists bool
		if err := db.QueryRow("SELECT EXISTS (SELECT 1 FROM filesystem_index WHERE path_hash = $1)", parentHash).Scan(&exists); err != nil {
			log.Printf("Query error (existence check): %v", err)
			respondError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		if !exists {
			respondError(w, http.StatusNotFound, "Directory not found")
			return
		}
	}

	respondJSON(w, http.StatusOK, files)
}

// GetFileStats handles GET /api/file/stats?path=/some/file
// @Summary Get file metadata
// @Description Returns metadata for a single file at the specified path.
// @Param path query string true "The file path"
// @Success 200 {object} FileInfo
// @Failure 400 {string} Bad Request
// @Failure 404 {string} Not Found
// @Failure 500 {string} Internal Server Error
// @Router /api/file/stats [get]
func GetFileStats(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		respondError(w, http.StatusBadRequest, "path parameter is required")
		return
	}

	var f FileInfo
	var userName, groupName sql.NullString
	query := `
		SELECT f.path, f.path_raw, f.size_bytes, f.file_type, f.permissions, f.uid, f.gid,
		       u.name, g.name, f.mtime, f.atime, f.ctime
		FROM filesystem_index f
		LEFT JOIN identity_map u ON f.uid = u.id AND u.id_type = 'uid'
		LEFT JOIN identity_map g ON f.gid = g.id AND g.id_type = 'gid'
		WHERE f.path_hash = $1
	`
	err := db.QueryRow(query, pathHash(path)).Scan(
		&f.Path, &f.PathRaw, &f.SizeBytes, &f.FileType, &f.Perms, &f.UID, &f.GID,
		&userName, &groupName, &f.MTime, &f.ATime, &f.CTime,
	)
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

	resolveIdentity(&f, userName, groupName)
	f.Perms = formatPermissions(f.Perms)
	respondJSON(w, http.StatusOK, f)
}

// GetDirStats handles GET /api/dir/stats?path=/some/dir&include_stats=true
// @Summary Get directory metadata
// @Description Returns metadata for a single directory at the specified path, optionally including aggregated size and time stats.
// @Param path query string true "The directory path"
// @Param include_stats query boolean false "Include aggregated size and time stats from dir_stats table"
// @Success 200 {object} FileInfo
// @Failure 400 {string} Bad Request
// @Failure 404 {string} Not Found
// @Failure 500 {string} Internal Server Error
// @Router /api/dir/stats [get]
func GetDirStats(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		respondError(w, http.StatusBadRequest, "path parameter is required")
		return
	}
	includeStats := r.URL.Query().Get("include_stats") == "true"

	var f FileInfo
	var userName, groupName sql.NullString
	query := `
		SELECT f.path, f.path_raw, f.size_bytes, f.file_type, f.permissions, f.uid, f.gid,
		       u.name, g.name, f.mtime, f.atime, f.ctime
		FROM filesystem_index f
		LEFT JOIN identity_map u ON f.uid = u.id AND u.id_type = 'uid'
		LEFT JOIN identity_map g ON f.gid = g.id AND g.id_type = 'gid'
		WHERE f.path_hash = $1
	`
	err := db.QueryRow(query, pathHash(path)).Scan(
		&f.Path, &f.PathRaw, &f.SizeBytes, &f.FileType, &f.Perms, &f.UID, &f.GID,
		&userName, &groupName, &f.MTime, &f.ATime, &f.CTime,
	)
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

	resolveIdentity(&f, userName, groupName)
	f.Perms = formatPermissions(f.Perms)

	if includeStats {
		var totalSize sql.NullInt64
		var fileCount sql.NullInt32
		var mf, ml, af, al, cf, cl sql.NullInt64
		statQuery := `SELECT total_size_bytes, file_count, mtime_first, mtime_last, atime_first, atime_last, ctime_first, ctime_last FROM dir_stats WHERE path_hash = $1`
		if err := db.QueryRow(statQuery, pathHash(path)).Scan(&totalSize, &fileCount, &mf, &ml, &af, &al, &cf, &cl); err == nil && totalSize.Valid {
			f.Aggregates = &DirAggregates{
				TotalSize:  totalSize.Int64,
				FileCount:  int(fileCount.Int32),
				MTimeFirst: mf.Int64, MTimeLast: ml.Int64,
				ATimeFirst: af.Int64, ATimeLast: al.Int64,
				CTimeFirst: cf.Int64, CTimeLast: cl.Int64,
			}
		}
	}

	respondJSON(w, http.StatusOK, f)
}

// ListUserStats handles GET /api/user/list
// @Summary List all users with usage statistics
// @Description Returns a sorted list of all users and their total data usage.
// @Param sort_by query string false "Field to sort by (total_size_bytes, file_count, name). Default: total_size_bytes"
// @Param order query string false "Sort order (ASC, DESC). Default: DESC"
// @Param limit query int false "Number of items to return. Default: 100"
// @Param offset query int false "Number of items to skip. Default: 0"
// @Success 200 {array} UserStats
// @Failure 500 {string} Internal Server Error
// @Router /api/user/list [get]
func ListUserStats(w http.ResponseWriter, r *http.Request) {
	listIdentityStats("uid", w, r)
}

// ListGroupStats handles GET /api/group/list
// @Summary List all groups with usage statistics
// @Description Returns a sorted list of all groups and their total data usage.
// @Param sort_by query string false "Field to sort by (total_size_bytes, file_count, name). Default: total_size_bytes"
// @Param order query string false "Sort order (ASC, DESC). Default: DESC"
// @Param limit query int false "Number of items to return. Default: 100"
// @Param offset query int false "Number of items to skip. Default: 0"
// @Success 200 {array} UserStats
// @Failure 500 {string} Internal Server Error
// @Router /api/group/list [get]
func ListGroupStats(w http.ResponseWriter, r *http.Request) {
	listIdentityStats("gid", w, r)
}

func listIdentityStats(idType string, w http.ResponseWriter, r *http.Request) {
	var col string
	switch r.URL.Query().Get("sort_by") {
	case "file_count":
		col = "s.file_count"
	case "name":
		col = "COALESCE(i.name, CAST(s.id_value AS TEXT))"
	default:
		col = "s.total_size_bytes"
	}

	order := r.URL.Query().Get("order")
	if order != "ASC" && order != "DESC" {
		order = "DESC"
	}

	limit := 100
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil {
		limit = l
	}
	offset := 0
	if o, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil {
		offset = o
	}

	query := fmt.Sprintf(`
		SELECT s.id_type, s.id_value, s.total_size_bytes, s.file_count,
		       COALESCE(i.name, CAST(s.id_value AS TEXT))
		FROM user_stats s
		LEFT JOIN identity_map i ON s.id_value = i.id AND s.id_type = i.id_type
		WHERE s.id_type = '%s'
		ORDER BY %s %s
		LIMIT %d OFFSET %d
	`, idType, col, order, limit, offset)

	rows, err := db.Query(query)
	if err != nil {
		log.Printf("Query error (list %ss): %v", idType, err)
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
	if err := rows.Err(); err != nil {
		log.Printf("Row iteration error (list %ss): %v", idType, err)
		respondError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	respondJSON(w, http.StatusOK, stats)
}

// ListScanSessions handles GET /api/scans?limit=&offset=
// @Summary List scan sessions
// @Description Returns fs_indexer and fs_aggregator run history (scan_sessions), newest first.
// @Param limit query int false "Number of items to return. Default: 50"
// @Param offset query int false "Number of items to skip. Default: 0"
// @Success 200 {array} ScanSession
// @Failure 500 {string} Internal Server Error
// @Router /api/scans [get]
func ListScanSessions(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil {
		limit = l
	}
	offset := 0
	if o, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil {
		offset = o
	}

	query := `
		SELECT session_id, scan_type, status,
		       EXTRACT(EPOCH FROM started_at)::bigint,
		       EXTRACT(EPOCH FROM ended_at)::bigint,
		       files_scanned
		FROM scan_sessions
		ORDER BY started_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := db.Query(query, limit, offset)
	if err != nil {
		log.Printf("Query error (list scans): %v", err)
		respondError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	defer rows.Close()

	sessions := []ScanSession{}
	for rows.Next() {
		var s ScanSession
		var endedAt sql.NullInt64
		if err := rows.Scan(&s.SessionID, &s.ScanType, &s.Status, &s.StartedAt, &endedAt, &s.FilesScanned); err != nil {
			log.Printf("Scan error: %v", err)
			continue
		}
		if endedAt.Valid {
			s.EndedAt = endedAt.Int64
		}
		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		log.Printf("Row iteration error (list scans): %v", err)
		respondError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	respondJSON(w, http.StatusOK, sessions)
}

func resolveIdentity(f *FileInfo, userName, groupName sql.NullString) {
	if userName.Valid {
		f.User = userName.String
	} else {
		f.User = fmt.Sprintf("%d", f.UID)
	}
	if groupName.Valid {
		f.Group = groupName.String
	} else {
		f.Group = fmt.Sprintf("%d", f.GID)
	}
}

func formatPermissions(perm string) string {
	if len(perm) != 3 {
		return perm
	}
	var res strings.Builder
	for i := 0; i < 3; i++ {
		d := perm[i] - '0'
		if d&4 != 0 {
			res.WriteByte('r')
		} else {
			res.WriteByte('-')
		}
		if d&2 != 0 {
			res.WriteByte('w')
		} else {
			res.WriteByte('-')
		}
		if d&1 != 0 {
			res.WriteByte('x')
		} else {
			res.WriteByte('-')
		}
	}
	return res.String()
}
