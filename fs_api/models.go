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
