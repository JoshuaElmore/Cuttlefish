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
	Path       string         `json:"path"`
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
type SearchRule struct {
	Field     string `json:"field"`     // path | uid | gid | file_type
	Operator  string `json:"operator"`  // contains | equals | starts_with | regex | regex_i | not_equals | gt | lt
	Value     string `json:"value"`     // raw value; numeric fields parse this to an int
	Connector string `json:"connector"` // AND | OR (relative to the previous rule)
}

// SearchRequest is the JSON body for POST /api/search.
type SearchRequest struct {
	Rules  []SearchRule `json:"rules"`
	SortBy string       `json:"sort_by"` // path | size_bytes | uid | gid | file_type | mtime | atime | ctime
	Order  string       `json:"order"`   // ASC | DESC
	Limit  int          `json:"limit"`
	Offset int          `json:"offset"`
}
