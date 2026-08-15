package main

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
	// Path is a UTF-8 rendering, which is lossy when the real name is not
	// valid UTF-8 (Unix filenames are arbitrary bytes). PathRaw carries the
	// exact bytes in exactly that case and is absent otherwise, so its presence
	// is the signal that Path cannot be round-tripped: two entries in one
	// listing may share an identical Path and differ only in PathRaw. Callers
	// that need to address such an entry must percent-encode PathRaw's decoded
	// bytes into ?path=, not send Path back. JSON-encoded as base64, since JSON
	// strings cannot hold arbitrary bytes.
	Path       string         `json:"path"`
	PathRaw    []byte         `json:"path_raw,omitempty"`
	SizeBytes  int64          `json:"size_bytes"`
	FileType   int            `json:"file_type"`
	Perms      string         `json:"permissions"`
	UID        int            `json:"uid"`
	GID        int            `json:"gid"`
	User       string         `json:"user"`
	Group      string         `json:"group"`
	MTime      int64          `json:"mtime"`
	ATime      int64          `json:"atime"`
	CTime      int64          `json:"ctime"`
	Aggregates *DirAggregates `json:"aggregates,omitempty"`
}

// SearchRule is a single condition in an advanced search query. Rules are joined
// together by their Connector ("AND" / "OR"); the first rule's connector is ignored.
//
// Connector and Negate carry `omitempty` because this type is also the source of
// the MCP search tool's input schema, where a field without it is inferred as
// required — which would force a caller to spell out a connector on the very
// first rule, where it means nothing. Nothing encodes a SearchRule, so the tag
// has no effect on the REST API, which only ever decodes one.
//
// The jsonschema tags are the only documentation the calling model gets; the
// authoritative lists of fields and operators live in searchFields and
// buildCondition in search.go. The trailing comments say the same thing for
// Swagger, which reads them and not the tags — keep the two in step.
type SearchRule struct {
	Field     string `json:"field" jsonschema:"column to test: path, user, group (text) or uid, gid, file_type, size_bytes, mtime, atime, ctime, dir_total_size, dir_file_count, dir_mtime_first, dir_mtime_last, dir_atime_first, dir_atime_last, dir_ctime_first, dir_ctime_last (numeric)"` // path | user | group | uid | gid | file_type | size_bytes | mtime | atime | ctime | dir_*
	Operator  string `json:"operator" jsonschema:"text fields: contains, equals, starts_with, ends_with, regex, regex_i; numeric fields: equals, not_equals, gt, lt"`                                                                                                                          // contains | equals | starts_with | ends_with | regex | regex_i | not_equals | gt | lt
	Value     string `json:"value" jsonschema:"value to compare against, always as a string; size fields accept suffixes such as 500M or 2G, time fields take Unix epoch seconds"`                                                                                                             // raw value; numeric fields parse this to an int, size fields accept 500M / 2G
	Connector string `json:"connector,omitempty" jsonschema:"AND or OR, relative to the preceding rule; ignored on the first rule and defaults to AND"`                                                                                                                                        // AND | OR (relative to the previous rule)
	Negate    bool   `json:"negate,omitempty" jsonschema:"invert this condition; rows where the field is null count as not matching, so they survive the inversion"`                                                                                                                           // when true the whole condition is inverted ("path NOT contains foo")
}

// SearchRequest is the JSON body for POST /api/search.
type SearchRequest struct {
	Rules  []SearchRule `json:"rules"`
	SortBy string       `json:"sort_by"` // path | size_bytes | uid | gid | file_type | mtime | atime | ctime
	Order  string       `json:"order"`   // ASC | DESC
	Limit  int          `json:"limit"`
	Offset int          `json:"offset"`
}

// ScanSession is one fs_indexer or fs_aggregator run. EndedAt is 0 while the
// scan is still running. FilesScanned is written directly by the scanning
// process (see fs_common::report_scan_progress/end_scan_session) rather than
// derived by counting filesystem_index — a deliberate denormalization, since
// it's the only place fs_aggregator's row count can live and it keeps this
// list cheap regardless of index size.
type ScanSession struct {
	SessionID    string `json:"session_id"`
	ScanType     string `json:"scan_type"` // indexer | aggregator
	Status       string `json:"status"`    // running | success | failed
	StartedAt    int64  `json:"started_at"`
	EndedAt      int64  `json:"ended_at,omitempty"`
	FilesScanned int64  `json:"files_scanned"`
}
