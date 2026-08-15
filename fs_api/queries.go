package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
)

// This file is the single query layer over the index. Both front ends — the
// REST handlers in handlers.go/search.go and the MCP tools in mcp.go — call
// these functions rather than writing their own SQL, so the two can never drift
// on what a listing contains or on how an unresolved uid is rendered.

// apiError is a failure that already knows how it should be reported: Status is
// the HTTP status a REST handler answers with, and Message is text that is safe
// to hand back to the caller. The distinction between "not found", "you asked
// for the wrong thing" and "we broke" is made where the query runs, so it has to
// travel with the error — neither front end can re-derive it from a bare error.
// Anything else coming out of this layer is an unexpected server-side failure:
// log it, don't show it.
type apiError struct {
	Status  int
	Message string
}

func (e *apiError) Error() string { return e.Message }

func errNotFound(msg string) error   { return &apiError{Status: http.StatusNotFound, Message: msg} }
func errBadRequest(msg string) error { return &apiError{Status: http.StatusBadRequest, Message: msg} }

// respondQueryError answers a REST request with the status the query layer
// chose, or 500 for anything it didn't classify. what names the operation for
// the server log.
func respondQueryError(w http.ResponseWriter, err error, what string) {
	var ae *apiError
	if errors.As(err, &ae) {
		respondError(w, ae.Status, ae.Message)
		return
	}
	log.Printf("Query error (%s): %v", what, err)
	respondError(w, http.StatusInternalServerError, "Internal server error")
}

// maxListLimit caps every paginated query. The limit reaches PostgreSQL as a
// literal in listIdentityStats' ORDER BY-bearing statement, and an unbounded one
// is a way to ask the server for the whole table in a single response — which
// matters more now that the MCP tools take the same parameter from a model.
// A non-positive limit means "use the default" rather than LIMIT -1, which is a
// hard error in PostgreSQL.
const maxListLimit = 10000

func clampLimit(limit, def int) int {
	if limit <= 0 {
		return def
	}
	if limit > maxListLimit {
		return maxListLimit
	}
	return limit
}

func clampOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}

// entrySelect is the column list every single-entry and listing query shares.
// The identity_map joins are LEFT so an entry whose uid has no name still comes
// back; resolveIdentity renders the bare number in that case.
const entrySelect = `
	SELECT f.path, f.path_raw, f.size_bytes, f.file_type, f.permissions,
	       f.uid, f.gid, u.name, g.name, f.mtime, f.atime, f.ctime
	FROM filesystem_index f
	LEFT JOIN identity_map u ON f.uid = u.id AND u.id_type = 'uid'
	LEFT JOIN identity_map g ON f.gid = g.id AND g.id_type = 'gid'
`

// entrySelectWithStats is entrySelect plus the dir_stats aggregate columns. The
// join is LEFT because non-directories have no dir_stats row, and so do
// directories when fs_aggregator hasn't run since they were indexed.
const entrySelectWithStats = `
	SELECT f.path, f.path_raw, f.size_bytes, f.file_type, f.permissions,
	       f.uid, f.gid, u.name, g.name, f.mtime, f.atime, f.ctime,
	       s.total_size_bytes, s.file_count, s.mtime_first, s.mtime_last,
	       s.atime_first, s.atime_last, s.ctime_first, s.ctime_last
	FROM filesystem_index f
	LEFT JOIN identity_map u ON f.uid = u.id AND u.id_type = 'uid'
	LEFT JOIN identity_map g ON f.gid = g.id AND g.id_type = 'gid'
	LEFT JOIN dir_stats s ON f.path_hash = s.path_hash
`

// scanEntry reads one row of entrySelect (or entrySelectWithStats when
// withStats is set) into a FileInfo, resolving names and permissions.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanEntry(rows rowScanner, withStats bool) (FileInfo, error) {
	var f FileInfo
	var userName, groupName sql.NullString
	var err error

	if withStats {
		var totalSize sql.NullInt64
		var fileCount sql.NullInt64
		var mf, ml, af, al, cf, cl sql.NullInt64
		err = rows.Scan(
			&f.Path, &f.PathRaw, &f.SizeBytes, &f.FileType, &f.Perms, &f.UID, &f.GID,
			&userName, &groupName, &f.MTime, &f.ATime, &f.CTime,
			&totalSize, &fileCount, &mf, &ml, &af, &al, &cf, &cl,
		)
		if err == nil && totalSize.Valid {
			f.Aggregates = &DirAggregates{
				TotalSize:  totalSize.Int64,
				FileCount:  int(fileCount.Int64),
				MTimeFirst: mf.Int64, MTimeLast: ml.Int64,
				ATimeFirst: af.Int64, ATimeLast: al.Int64,
				CTimeFirst: cf.Int64, CTimeLast: cl.Int64,
			}
		}
	} else {
		err = rows.Scan(
			&f.Path, &f.PathRaw, &f.SizeBytes, &f.FileType, &f.Perms, &f.UID, &f.GID,
			&userName, &groupName, &f.MTime, &f.ATime, &f.CTime,
		)
	}
	if err != nil {
		return f, err
	}

	resolveIdentity(&f, userName, groupName)
	f.Perms = formatPermissions(f.Perms)
	return f, nil
}

// queryEntry looks up a single indexed entry by path, whatever its type.
// notFound is the message the 404 carries, so each caller keeps its own
// wording ("File not found" / "Directory not found") while sharing the query.
func queryEntry(ctx context.Context, path, notFound string) (FileInfo, error) {
	row := db.QueryRowContext(ctx, entrySelect+" WHERE f.path_hash = $1", pathHash(path))
	f, err := scanEntry(row, false)
	if errors.Is(err, sql.ErrNoRows) {
		return f, errNotFound(notFound)
	}
	if err != nil {
		return f, fmt.Errorf("entry lookup: %w", err)
	}
	return f, nil
}

// queryDirAggregates fetches one directory's rolled-up totals. A missing row is
// (nil, nil): fs_aggregator may simply not have run since the directory was
// indexed, which is not an error for any caller.
func queryDirAggregates(ctx context.Context, path string) (*DirAggregates, error) {
	var totalSize sql.NullInt64
	var fileCount sql.NullInt64
	var mf, ml, af, al, cf, cl sql.NullInt64
	err := db.QueryRowContext(ctx, `
		SELECT total_size_bytes, file_count, mtime_first, mtime_last,
		       atime_first, atime_last, ctime_first, ctime_last
		FROM dir_stats WHERE path_hash = $1
	`, pathHash(path)).Scan(&totalSize, &fileCount, &mf, &ml, &af, &al, &cf, &cl)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("dir aggregates lookup: %w", err)
	}
	if !totalSize.Valid {
		return nil, nil
	}
	return &DirAggregates{
		TotalSize:  totalSize.Int64,
		FileCount:  int(fileCount.Int64),
		MTimeFirst: mf.Int64, MTimeLast: ml.Int64,
		ATimeFirst: af.Int64, ATimeLast: al.Int64,
		CTimeFirst: cf.Int64, CTimeLast: cl.Int64,
	}, nil
}

// queryChildren lists the direct children of a directory. The children link is
// parent_hash = SHA-256(path), so no resolve query is needed — the hash is
// computed locally and the listing is a single index scan.
//
// A path that is not indexed at all is a 404; an indexed but empty directory is
// an empty listing. Distinguishing the two costs one primary-key lookup, and
// only in the empty case.
//
// A limit of 0 means "every child, in whatever order the index scan produces
// them" — what /api/list has always returned, and what the UI needs to render a
// directory. A positive limit adds ORDER BY path, because paging is meaningless
// without a stable order; the sort is the reason it isn't simply always on.
func queryChildren(ctx context.Context, path string, includeStats bool, limit, offset int) ([]FileInfo, error) {
	parentHash := pathHash(path)

	query := entrySelect + " WHERE f.parent_hash = $1"
	if includeStats {
		query = entrySelectWithStats + " WHERE f.parent_hash = $1"
	}
	args := []any{parentHash}
	if limit > 0 {
		query += " ORDER BY f.path ASC LIMIT $2 OFFSET $3"
		args = append(args, clampLimit(limit, limit), clampOffset(offset))
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list children: %w", err)
	}
	defer rows.Close()

	var files []FileInfo
	for rows.Next() {
		f, err := scanEntry(rows, includeStats)
		if err != nil {
			log.Printf("Scan error (list children): %v", err)
			continue
		}
		files = append(files, f)
	}
	// Must run before the empty check below: a mid-iteration failure would
	// otherwise look like an empty directory and answer 404.
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list children: %w", err)
	}

	if len(files) == 0 {
		var exists bool
		if err := db.QueryRowContext(ctx,
			"SELECT EXISTS (SELECT 1 FROM filesystem_index WHERE path_hash = $1)", parentHash,
		).Scan(&exists); err != nil {
			return nil, fmt.Errorf("existence check: %w", err)
		}
		if !exists {
			return nil, errNotFound("Directory not found")
		}
	}

	return files, nil
}

// identityStatsSortColumns is the allowlist of columns user/group listings may
// be ordered by. listIdentityStats interpolates the value, so nothing outside
// this map may ever reach the statement.
var identityStatsSortColumns = map[string]string{
	"total_size_bytes": "s.total_size_bytes",
	"file_count":       "s.file_count",
	"name":             "COALESCE(i.name, CAST(s.id_value AS TEXT))",
}

// queryIdentityStats returns per-uid or per-gid totals. idType must already be
// 'uid' or 'gid' — it is interpolated, not bound, and the only two callers pass
// a literal.
func queryIdentityStats(ctx context.Context, idType, sortBy, order string, limit, offset int) ([]UserStats, error) {
	col, ok := identityStatsSortColumns[sortBy]
	if !ok {
		col = identityStatsSortColumns["total_size_bytes"]
	}
	if order != "ASC" && order != "DESC" {
		order = "DESC"
	}
	limit = clampLimit(limit, 100)
	offset = clampOffset(offset)

	query := fmt.Sprintf(`
		SELECT s.id_type, s.id_value, s.total_size_bytes, s.file_count,
		       COALESCE(i.name, CAST(s.id_value AS TEXT))
		FROM user_stats s
		LEFT JOIN identity_map i ON s.id_value = i.id AND s.id_type = i.id_type
		WHERE s.id_type = '%s'
		ORDER BY %s %s
		LIMIT %d OFFSET %d
	`, idType, col, order, limit, offset)

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list %ss: %w", idType, err)
	}
	defer rows.Close()

	var stats []UserStats
	for rows.Next() {
		var s UserStats
		if err := rows.Scan(&s.IDType, &s.IDValue, &s.TotalSizeBytes, &s.FileCount, &s.Name); err != nil {
			log.Printf("Scan error (list %ss): %v", idType, err)
			continue
		}
		stats = append(stats, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list %ss: %w", idType, err)
	}
	return stats, nil
}

// queryScanSessions returns fs_indexer / fs_aggregator run history, newest first.
func queryScanSessions(ctx context.Context, limit, offset int) ([]ScanSession, error) {
	limit = clampLimit(limit, 50)
	offset = clampOffset(offset)

	rows, err := db.QueryContext(ctx, `
		SELECT session_id, scan_type, status,
		       EXTRACT(EPOCH FROM started_at)::bigint,
		       EXTRACT(EPOCH FROM ended_at)::bigint,
		       files_scanned
		FROM scan_sessions
		ORDER BY started_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list scans: %w", err)
	}
	defer rows.Close()

	sessions := []ScanSession{}
	for rows.Next() {
		var s ScanSession
		var endedAt sql.NullInt64
		if err := rows.Scan(&s.SessionID, &s.ScanType, &s.Status, &s.StartedAt, &endedAt, &s.FilesScanned); err != nil {
			log.Printf("Scan error (list scans): %v", err)
			continue
		}
		if endedAt.Valid {
			s.EndedAt = endedAt.Int64
		}
		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list scans: %w", err)
	}
	return sessions, nil
}
