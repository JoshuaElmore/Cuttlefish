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

	files, err := queryChildren(r.Context(), path, includeStats, 0, 0)
	if err != nil {
		respondQueryError(w, err, "list children")
		return
	}
	respondJSON(w, http.StatusOK, files)
}

// GetFileStats handles GET /api/file/stats?path=/some/file
// @Summary Get file metadata
// @Description Returns metadata for a single non-directory entry (file, symlink, socket, FIFO or device node) at the specified path. Directories are served by /api/dir/stats.
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

	f, err := queryEntry(r.Context(), path, "File not found")
	if err != nil {
		respondQueryError(w, err, "file stats")
		return
	}

	// Everything that isn't a directory is served here, not just regular files.
	// Symlinks (3) and sockets/FIFOs/device nodes (0) are indexed and listed like
	// any other entry, so rejecting them made them unselectable in the UI while
	// still appearing in the listing.
	if f.FileType == 2 {
		respondError(w, http.StatusBadRequest, "The provided path is a directory")
		return
	}

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

	f, err := queryEntry(r.Context(), path, "Directory not found")
	if err != nil {
		respondQueryError(w, err, "dir stats")
		return
	}

	if f.FileType != 2 {
		respondError(w, http.StatusBadRequest, "The provided path is not a directory")
		return
	}

	if includeStats {
		// A failure here costs the aggregates, not the response: the entry's own
		// metadata is already loaded and is what the endpoint promises.
		agg, err := queryDirAggregates(r.Context(), path)
		if err != nil {
			log.Printf("Query error (dir aggregates): %v", err)
		}
		f.Aggregates = agg
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
	stats, err := queryIdentityStats(
		r.Context(),
		idType,
		r.URL.Query().Get("sort_by"),
		r.URL.Query().Get("order"),
		queryInt(r, "limit", 0),
		queryInt(r, "offset", 0),
	)
	if err != nil {
		respondQueryError(w, err, "list "+idType+"s")
		return
	}
	respondJSON(w, http.StatusOK, stats)
}

// queryInt reads an integer query parameter, falling back to def when it is
// absent or unparseable. Range checking is the query layer's job (clampLimit).
func queryInt(r *http.Request, name string, def int) int {
	if v, err := strconv.Atoi(r.URL.Query().Get(name)); err == nil {
		return v
	}
	return def
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
	sessions, err := queryScanSessions(r.Context(), queryInt(r, "limit", 0), queryInt(r, "offset", 0))
	if err != nil {
		respondQueryError(w, err, "list scans")
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
