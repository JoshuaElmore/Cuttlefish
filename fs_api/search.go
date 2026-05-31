package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
)

// searchFields maps an allowlisted request field name to its SQL column.
// numeric=true means the value is parsed as an integer before binding.
// sizeHuman=true additionally accepts unit suffixes (K/M/G/T/P).
var searchFields = map[string]struct {
	column    string
	numeric   bool
	sizeHuman bool
}{
	"path":            {"f.path", false, false},
	"uid":             {"f.uid", true, false},
	"gid":             {"f.gid", true, false},
	"user":            {"u.name", false, false},
	"group":           {"g.name", false, false},
	"file_type":       {"f.file_type", true, false},
	"size_bytes":      {"f.size_bytes", true, true},
	"mtime":           {"f.mtime", true, false},
	"atime":           {"f.atime", true, false},
	"ctime":           {"f.ctime", true, false},
	"dir_total_size":  {"s.total_size_bytes", true, true},
	"dir_file_count":  {"s.file_count", true, false},
	"dir_mtime_first": {"s.mtime_first", true, false},
	"dir_mtime_last":  {"s.mtime_last", true, false},
	"dir_atime_first": {"s.atime_first", true, false},
	"dir_atime_last":  {"s.atime_last", true, false},
	"dir_ctime_first": {"s.ctime_first", true, false},
	"dir_ctime_last":  {"s.ctime_last", true, false},
}

// parseHumanSize converts strings like "1G", "500M", "2.5T" to bytes.
// Falls back to plain integer parsing when no unit suffix is present.
// Suffixes are checked longest-first so "KB" is matched before "K", etc.
func parseHumanSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty size value")
	}

	units := []struct {
		suffix string
		mult   int64
	}{
		{"PB", 1 << 50}, {"TB", 1 << 40}, {"GB", 1 << 30}, {"MB", 1 << 20}, {"KB", 1 << 10},
		{"P", 1 << 50}, {"T", 1 << 40}, {"G", 1 << 30}, {"M", 1 << 20}, {"K", 1 << 10},
	}

	upper := strings.ToUpper(s)
	for _, u := range units {
		if strings.HasSuffix(upper, u.suffix) {
			numStr := strings.TrimSpace(s[:len(s)-len(u.suffix)])
			f, err := strconv.ParseFloat(numStr, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid size value %q", s)
			}
			return int64(f * float64(u.mult)), nil
		}
	}

	return strconv.ParseInt(s, 10, 64)
}

// searchSortColumns is the allowlist of columns the result set may be ordered by.
var searchSortColumns = map[string]string{
	"path":            "f.path",
	"size_bytes":      "f.size_bytes",
	"uid":             "f.uid",
	"gid":             "f.gid",
	"file_type":       "f.file_type",
	"mtime":           "f.mtime",
	"atime":           "f.atime",
	"ctime":           "f.ctime",
	"dir_total_size":  "s.total_size_bytes",
	"dir_file_count":  "s.file_count",
	"dir_mtime_first": "s.mtime_first",
	"dir_mtime_last":  "s.mtime_last",
	"dir_atime_first": "s.atime_first",
	"dir_atime_last":  "s.atime_last",
	"dir_ctime_first": "s.ctime_first",
	"dir_ctime_last":  "s.ctime_last",
}

const searchMaxLimit = 1000

// SearchFiles handles POST /api/search
// @Summary Advanced filesystem search
// @Description Search files and directories using AND/OR conditions on path, uid, gid, file_type and size, with regex support and sortable output.
// @Accept json
// @Produce json
// @Param request body SearchRequest true "Search query"
// @Success 200 {array} FileInfo
// @Failure 400 {string} Bad Request
// @Failure 500 {string} Internal Server Error
// @Router /api/search [post]
func SearchFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	var req SearchRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if len(req.Rules) == 0 {
		respondError(w, http.StatusBadRequest, "At least one search rule is required")
		return
	}

	whereClause, args, err := buildSearchWhere(req.Rules)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// ORDER BY column and direction come from allowlists; never from user text directly.
	sortCol, ok := searchSortColumns[req.SortBy]
	if !ok {
		sortCol = "f.path"
	}
	order := "ASC"
	if strings.EqualFold(req.Order, "DESC") {
		order = "DESC"
	}

	limit := req.Limit
	if limit <= 0 || limit > searchMaxLimit {
		limit = 200
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	// limit/offset are appended as bind parameters after the rule values.
	limitParam := len(args) + 1
	offsetParam := len(args) + 2
	args = append(args, limit, offset)

	query := fmt.Sprintf(`
		SELECT f.path, f.size_bytes, f.file_type, f.permissions,
		       f.uid, f.gid, u.name, g.name, f.mtime, f.atime, f.ctime,
		       s.total_size_bytes, s.file_count,
		       s.mtime_first, s.mtime_last,
		       s.atime_first, s.atime_last,
		       s.ctime_first, s.ctime_last
		FROM filesystem_index f
		LEFT JOIN identity_map u ON f.uid = u.id AND u.id_type = 'uid'
		LEFT JOIN identity_map g ON f.gid = g.id AND g.id_type = 'gid'
		LEFT JOIN dir_stats s ON f.path_hash = s.path_hash
		WHERE %s
		ORDER BY %s %s
		LIMIT $%d OFFSET $%d
	`, whereClause, sortCol, order, limitParam, offsetParam)

	rows, err := db.Query(query, args...)
	if err != nil {
		log.Printf("Query error (search): %v", err)
		respondError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	defer rows.Close()

	var files []FileInfo
	for rows.Next() {
		var f FileInfo
		var userName, groupName sql.NullString
		var totalSize sql.NullInt64
		var fileCount sql.NullInt32
		var mf, ml, af, al, cf, cl sql.NullInt64
		if err := rows.Scan(
			&f.Path, &f.SizeBytes, &f.FileType, &f.Perms, &f.UID, &f.GID,
			&userName, &groupName, &f.MTime, &f.ATime, &f.CTime,
			&totalSize, &fileCount, &mf, &ml, &af, &al, &cf, &cl,
		); err != nil {
			log.Printf("Scan error: %v", err)
			continue
		}
		resolveIdentity(&f, userName, groupName)
		f.Perms = formatPermissions(f.Perms)
		if totalSize.Valid {
			f.Aggregates = &DirAggregates{
				TotalSize:  totalSize.Int64,
				FileCount:  int(fileCount.Int32),
				MTimeFirst: mf.Int64, MTimeLast: ml.Int64,
				ATimeFirst: af.Int64, ATimeLast: al.Int64,
				CTimeFirst: cf.Int64, CTimeLast: cl.Int64,
			}
		}
		files = append(files, f)
	}

	respondJSON(w, http.StatusOK, files)
}

// buildSearchWhere turns the rule list into a parameterized WHERE clause. Field
// names and operators are mapped through allowlists, and every user value is
// passed as a bind parameter ($1, $2, …) so the clause is injection-safe.
func buildSearchWhere(rules []SearchRule) (string, []interface{}, error) {
	var sb strings.Builder
	var args []interface{}

	for i, rule := range rules {
		field, ok := searchFields[rule.Field]
		if !ok {
			return "", nil, fmt.Errorf("unknown search field: %q", rule.Field)
		}

		cond, err := buildCondition(field.column, field.numeric, field.sizeHuman, rule.Operator, rule.Value, &args)
		if err != nil {
			return "", nil, err
		}

		if i > 0 {
			connector := "AND"
			if strings.EqualFold(rule.Connector, "OR") {
				connector = "OR"
			}
			sb.WriteString(" " + connector + " ")
		}
		sb.WriteString(cond)
	}

	return sb.String(), args, nil
}

// buildCondition appends the rule's value to args and returns a single SQL
// predicate referencing it by position. The returned predicate is wrapped in
// parentheses so callers can join predicates with AND/OR safely.
func buildCondition(column string, numeric, sizeHuman bool, operator, value string, args *[]interface{}) (string, error) {
	if numeric {
		var n int64
		var err error
		if sizeHuman {
			n, err = parseHumanSize(value)
		} else {
			n, err = strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		}
		if err != nil {
			return "", fmt.Errorf("invalid value for %s: %v", column, err)
		}
		op, ok := map[string]string{
			"equals":     "=",
			"not_equals": "<>",
			"gt":         ">",
			"lt":         "<",
		}[operator]
		if !ok {
			return "", fmt.Errorf("operator %q is not valid for numeric fields", operator)
		}
		*args = append(*args, n)
		return fmt.Sprintf("(%s %s $%d)", column, op, len(*args)), nil
	}

	// Text column (path).
	switch operator {
	case "equals":
		*args = append(*args, value)
		return fmt.Sprintf("(%s = $%d)", column, len(*args)), nil
	case "contains":
		*args = append(*args, "%"+escapeLike(value)+"%")
		return fmt.Sprintf("(%s ILIKE $%d)", column, len(*args)), nil
	case "starts_with":
		*args = append(*args, escapeLike(value)+"%")
		return fmt.Sprintf("(%s ILIKE $%d)", column, len(*args)), nil
	case "regex":
		*args = append(*args, value)
		return fmt.Sprintf("(%s ~ $%d)", column, len(*args)), nil
	case "regex_i":
		*args = append(*args, value)
		return fmt.Sprintf("(%s ~* $%d)", column, len(*args)), nil
	default:
		return "", fmt.Errorf("operator %q is not valid for text fields", operator)
	}
}

// escapeLike neutralizes LIKE wildcards in user input so contains/starts_with
// match the literal characters the user typed.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}
