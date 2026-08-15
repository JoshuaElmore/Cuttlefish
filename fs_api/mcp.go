package main

// Model Context Protocol server.
//
// The MCP endpoint is a second front end over the same query layer as the REST
// API (queries.go, search.go) — it never talks to the database itself, so the
// two can't drift on what an entry looks like or on how a failure is
// classified. It is read-only by construction: nothing here writes, and every
// tool is annotated ReadOnlyHint so a client can say so before calling.
//
// Transport is Streamable HTTP, mounted on the main server at mcp.path (or on
// its own listener when mcp.listen_addr is set), behind the same session auth
// that guards /api/*.

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	mcpServerName    = "cuttlefish"
	mcpServerVersion = "1.0.0"

	// mcpDefaultLimit is what a tool returns when the caller names no limit.
	// Much smaller than the REST defaults on purpose: these results are read
	// into a model's context window, where a 10 000-entry listing is not a
	// large response but a useless one.
	mcpDefaultLimit = 100
	// mcpMaxLimit caps what a caller may ask for, below the query layer's own
	// maxListLimit for the same reason.
	mcpMaxLimit = 1000

	// mcpSessionTimeout reaps sessions from clients that connected and went
	// away without a DELETE. Without it, every abandoned client leaks its
	// session state for the lifetime of the process.
	mcpSessionTimeout = 30 * time.Minute
)

// mcpInstructions is handed to the client at initialize. It is the one place a
// model learns the vocabulary of this index — the type codes, the fact that the
// data is a point-in-time snapshot rather than the live filesystem, and where
// to look when a path it expects isn't there.
const mcpInstructions = `Cuttlefish indexes a host's filesystem into PostgreSQL and exposes it here.

The data is a snapshot written by the last fs_indexer run, not the live filesystem: an
entry can be missing because it was created after the scan, and a listed entry can be
gone from disk. Call list_scan_sessions or read the cuttlefish://index/status resource
to find out how fresh the snapshot is before drawing conclusions from an absence.

Only metadata is indexed. File contents are never stored and cannot be read through
this server.

file_type codes: 1 = regular file, 2 = directory, 3 = symlink, 0 = anything else
(socket, FIFO, device node).

Sizes are in bytes; size_bytes on a directory is the size of the directory entry itself.
Recursive totals for a directory come from its aggregates field, which is present only
when fs_aggregator has run since the directory was indexed.

Timestamps are Unix epoch seconds.

Paths are absolute and are matched exactly, with no normalization: pass /var/log, not
/var/log/ or a relative path. A path whose real bytes are not valid UTF-8 comes back
with path_raw set, and its path field is a lossy rendering that cannot be used to
address it again.`

// --- Tool input and output types -------------------------------------------
//
// The jsonschema struct tags become the tool's input schema, which is the only
// documentation the calling model gets for each argument. They are written for
// that reader.

type mcpEntryInput struct {
	Path         string `json:"path" jsonschema:"absolute path of the entry, e.g. /var/log/syslog"`
	IncludeStats *bool  `json:"include_stats,omitempty" jsonschema:"attach recursive size/count/time totals when the entry is a directory (default true); ignored for other entry types"`
}

type mcpEntryOutput struct {
	Entry FileInfo `json:"entry"`
}

type mcpListDirectoryInput struct {
	Path         string `json:"path" jsonschema:"absolute path of the directory to list, e.g. /home"`
	IncludeStats *bool  `json:"include_stats,omitempty" jsonschema:"attach recursive totals to each child directory (default true)"`
	Limit        int    `json:"limit,omitempty" jsonschema:"maximum entries to return (default 100, maximum 1000)"`
	Offset       int    `json:"offset,omitempty" jsonschema:"entries to skip, for paging through a large directory"`
}

type mcpListDirectoryOutput struct {
	Path      string     `json:"path"`
	Entries   []FileInfo `json:"entries"`
	Count     int        `json:"count"`
	Truncated bool       `json:"truncated" jsonschema:"true when the directory holds more entries than were returned; raise offset to see the rest"`
}

type mcpSearchInput struct {
	Rules  []SearchRule `json:"rules" jsonschema:"conditions to match, combined left to right by each rule's connector; at least one is required"`
	SortBy string       `json:"sort_by,omitempty" jsonschema:"path | size_bytes | uid | gid | file_type | mtime | atime | ctime | dir_total_size | dir_file_count (default path)"`
	Order  string       `json:"order,omitempty" jsonschema:"ASC or DESC (default ASC)"`
	Limit  int          `json:"limit,omitempty" jsonschema:"maximum results to return (default 100, maximum 1000)"`
	Offset int          `json:"offset,omitempty" jsonschema:"results to skip, for paging"`
}

type mcpSearchOutput struct {
	Results   []FileInfo `json:"results"`
	Count     int        `json:"count"`
	Truncated bool       `json:"truncated" jsonschema:"true when more rows matched than were returned; raise offset to see the rest"`
}

type mcpIdentityStatsInput struct {
	SortBy string `json:"sort_by,omitempty" jsonschema:"total_size_bytes | file_count | name (default total_size_bytes)"`
	Order  string `json:"order,omitempty" jsonschema:"ASC or DESC (default DESC, i.e. largest first)"`
	Limit  int    `json:"limit,omitempty" jsonschema:"maximum rows to return (default 100, maximum 1000)"`
	Offset int    `json:"offset,omitempty" jsonschema:"rows to skip, for paging"`
}

type mcpIdentityStatsOutput struct {
	Stats     []UserStats `json:"stats"`
	Count     int         `json:"count"`
	Truncated bool        `json:"truncated"`
}

type mcpScanSessionsInput struct {
	Limit  int `json:"limit,omitempty" jsonschema:"maximum runs to return, newest first (default 100, maximum 1000)"`
	Offset int `json:"offset,omitempty" jsonschema:"runs to skip, for paging"`
}

type mcpScanSessionsOutput struct {
	Sessions  []ScanSession `json:"sessions"`
	Count     int           `json:"count"`
	Truncated bool          `json:"truncated"`
}

// --- Server construction ----------------------------------------------------

func boolOr(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

// mcpPage resolves a caller-supplied limit into the number of rows to ask the
// query layer for. It requests one extra row: if that row comes back, more
// results exist, which is what lets a tool report truncated honestly instead of
// guessing from a full page.
func mcpPage(limit int) (want, fetch int) {
	if limit <= 0 {
		limit = mcpDefaultLimit
	}
	if limit > mcpMaxLimit {
		limit = mcpMaxLimit
	}
	return limit, limit + 1
}

// mcpTrim cuts an over-fetched page back to size and reports whether it was
// over-full.
func mcpTrim[T any](rows []T, want int) ([]T, bool) {
	if len(rows) > want {
		return rows[:want], true
	}
	if rows == nil {
		// A tool result is JSON: an absent list should read as [], not null.
		return []T{}, false
	}
	return rows, false
}

// mcpToolError converts a query-layer failure into what the caller sees. An
// apiError is the caller's own mistake or a plain "not found" and is safe to
// quote back; anything else is a server fault, which is logged here and
// reported as a bare failure — the same split the REST handlers make, since an
// internal error message can name tables, columns and hosts.
func mcpToolError(err error, what string) error {
	var ae *apiError
	if errors.As(err, &ae) {
		return errors.New(ae.Message)
	}
	log.Printf("MCP error (%s): %v", what, err)
	return fmt.Errorf("%s failed: internal server error", what)
}

// newMCPServer builds the server and registers every tool and resource. It
// takes no arguments and reads no config: the tools close over the package-level
// db handle exactly as the REST handlers do.
func newMCPServer() *mcp.Server {
	readOnly := func(title string) *mcp.ToolAnnotations {
		return &mcp.ToolAnnotations{Title: title, ReadOnlyHint: true, IdempotentHint: true}
	}

	s := mcp.NewServer(&mcp.Implementation{
		Name:        mcpServerName,
		Title:       "Cuttlefish filesystem index",
		Version:     mcpServerVersion,
		Description: "Read-only queries over an indexed host filesystem: entry metadata, directory rollups, ownership totals and search.",
	}, &mcp.ServerOptions{
		Instructions: mcpInstructions,
		HasTools:     true,
		HasResources: true,
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_entry_stats",
		Description: "Look up one indexed filesystem entry by its absolute path and return its metadata: size, type, permissions, owning user and group, and atime/mtime/ctime. Works for any entry type. For a directory it also returns recursive totals (total size, descendant count, first/last timestamps) unless include_stats is false. Returns an error if the path is not in the index.",
		Annotations: readOnly("Get entry metadata"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in mcpEntryInput) (*mcp.CallToolResult, mcpEntryOutput, error) {
		if strings.TrimSpace(in.Path) == "" {
			return nil, mcpEntryOutput{}, errors.New("path is required")
		}
		f, err := queryEntry(ctx, in.Path, "no entry at that path in the index")
		if err != nil {
			return nil, mcpEntryOutput{}, mcpToolError(err, "get_entry_stats")
		}
		if f.FileType == 2 && boolOr(in.IncludeStats, true) {
			agg, err := queryDirAggregates(ctx, in.Path)
			if err != nil {
				// The entry's own metadata is loaded and is what was asked for;
				// losing the rollup is not worth failing the call over.
				log.Printf("MCP error (get_entry_stats aggregates): %v", err)
			}
			f.Aggregates = agg
		}
		return nil, mcpEntryOutput{Entry: f}, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_directory",
		Description: "List the direct children of a directory — one level, not recursive. Each child carries the same metadata get_entry_stats returns, and child directories carry their recursive totals unless include_stats is false. Returns an error if the directory is not in the index; an indexed but empty directory returns no entries.",
		Annotations: readOnly("List directory children"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in mcpListDirectoryInput) (*mcp.CallToolResult, mcpListDirectoryOutput, error) {
		if strings.TrimSpace(in.Path) == "" {
			return nil, mcpListDirectoryOutput{}, errors.New("path is required")
		}
		want, fetch := mcpPage(in.Limit)
		files, err := queryChildren(ctx, in.Path, boolOr(in.IncludeStats, true), fetch, in.Offset)
		if err != nil {
			return nil, mcpListDirectoryOutput{}, mcpToolError(err, "list_directory")
		}
		entries, truncated := mcpTrim(files, want)
		return nil, mcpListDirectoryOutput{
			Path:      in.Path,
			Entries:   entries,
			Count:     len(entries),
			Truncated: truncated,
		}, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "search_files",
		Description: "Search the whole index with a list of conditions joined by AND/OR. " +
			"Each rule names a field, an operator and a value, and may set negate to invert it.\n\n" +
			"Text fields (path, user, group) take contains, equals, starts_with, ends_with, regex " +
			"(POSIX, case-sensitive) or regex_i. Numeric fields (uid, gid, file_type, size_bytes, " +
			"mtime, atime, ctime, dir_total_size, dir_file_count, dir_mtime_first, dir_mtime_last, " +
			"dir_atime_first, dir_atime_last, dir_ctime_first, dir_ctime_last) take equals, " +
			"not_equals, gt or lt. Size values accept unit suffixes: \"500M\", \"2G\", \"1.5T\". " +
			"Time values are Unix epoch seconds. Fields prefixed dir_ match a directory's recursive " +
			"totals and only ever match directories.\n\n" +
			"Example — files over 1 GB under /home owned by uid 1000: rules = " +
			"[{field:\"path\",operator:\"starts_with\",value:\"/home/\"}, " +
			"{field:\"size_bytes\",operator:\"gt\",value:\"1G\",connector:\"AND\"}, " +
			"{field:\"uid\",operator:\"equals\",value:\"1000\",connector:\"AND\"}].",
		Annotations: readOnly("Search the index"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in mcpSearchInput) (*mcp.CallToolResult, mcpSearchOutput, error) {
		want, fetch := mcpPage(in.Limit)
		files, err := executeSearch(ctx, SearchRequest{
			Rules:  in.Rules,
			SortBy: in.SortBy,
			Order:  in.Order,
			Limit:  fetch,
			Offset: in.Offset,
		})
		if err != nil {
			return nil, mcpSearchOutput{}, mcpToolError(err, "search_files")
		}
		results, truncated := mcpTrim(files, want)
		return nil, mcpSearchOutput{Results: results, Count: len(results), Truncated: truncated}, nil
	})

	addIdentityStatsTool(s, "list_user_stats", "uid", readOnly("List storage by user"),
		"Total bytes and entry count per owning user across the entire index, largest first by default. "+
			"Answers \"who is using the space\". Users with no indexed entries do not appear, and a uid "+
			"with no name on this host is reported with its number as the name.")
	addIdentityStatsTool(s, "list_group_stats", "gid", readOnly("List storage by group"),
		"Total bytes and entry count per owning group across the entire index, largest first by default. "+
			"The group-ownership counterpart of list_user_stats.")

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_scan_sessions",
		Description: "List fs_indexer and fs_aggregator runs, newest first: when each started and ended, whether it succeeded, and how many entries it processed. Use this to judge how current the index is, or to check whether a scan is still running before treating a missing path as missing from disk.",
		Annotations: readOnly("List scan history"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in mcpScanSessionsInput) (*mcp.CallToolResult, mcpScanSessionsOutput, error) {
		want, fetch := mcpPage(in.Limit)
		sessions, err := queryScanSessions(ctx, fetch, in.Offset)
		if err != nil {
			return nil, mcpScanSessionsOutput{}, mcpToolError(err, "list_scan_sessions")
		}
		page, truncated := mcpTrim(sessions, want)
		return nil, mcpScanSessionsOutput{Sessions: page, Count: len(page), Truncated: truncated}, nil
	})

	s.AddResource(&mcp.Resource{
		URI:         mcpStatusURI,
		Name:        "index-status",
		Title:       "Index status",
		Description: "Freshness of the index: the most recent fs_indexer and fs_aggregator run, and an estimate of how many entries are indexed. Read this before concluding that something is absent from the filesystem.",
		MIMEType:    "application/json",
	}, mcpReadIndexStatus)

	return s
}

// addIdentityStatsTool registers list_user_stats / list_group_stats, which
// differ only in which half of user_stats they read.
func addIdentityStatsTool(s *mcp.Server, name, idType string, annotations *mcp.ToolAnnotations, description string) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        name,
		Description: description,
		Annotations: annotations,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in mcpIdentityStatsInput) (*mcp.CallToolResult, mcpIdentityStatsOutput, error) {
		want, fetch := mcpPage(in.Limit)
		stats, err := queryIdentityStats(ctx, idType, in.SortBy, strings.ToUpper(in.Order), fetch, in.Offset)
		if err != nil {
			return nil, mcpIdentityStatsOutput{}, mcpToolError(err, name)
		}
		page, truncated := mcpTrim(stats, want)
		return nil, mcpIdentityStatsOutput{Stats: page, Count: len(page), Truncated: truncated}, nil
	})
}

// --- Index status resource --------------------------------------------------

const mcpStatusURI = "cuttlefish://index/status"

type mcpIndexStatus struct {
	// EstimatedEntries is PostgreSQL's own row estimate for filesystem_index,
	// not a COUNT(*): counting a table with 10^8 rows to answer "is this index
	// populated" is not a trade worth making. Omitted when the planner has no
	// estimate yet (a table that has never been analyzed reports -1).
	EstimatedEntries *int64       `json:"estimated_entries,omitempty"`
	LatestIndexer    *ScanSession `json:"latest_indexer,omitempty"`
	LatestAggregator *ScanSession `json:"latest_aggregator,omitempty"`
	PathHashBytes    int          `json:"path_hash_bytes" jsonschema:"hash width this build uses; a stored index of a different width is unreadable"`
}

func mcpReadIndexStatus(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	status := mcpIndexStatus{PathHashBytes: pathHashLen}

	var estimate int64
	err := db.QueryRowContext(ctx, `
		SELECT COALESCE((SELECT reltuples::bigint FROM pg_class WHERE relname = 'filesystem_index'), -1)
	`).Scan(&estimate)
	if err != nil {
		log.Printf("MCP error (index status estimate): %v", err)
	} else if estimate >= 0 {
		status.EstimatedEntries = &estimate
	}

	for _, t := range []struct {
		scanType string
		dst      **ScanSession
	}{
		{"indexer", &status.LatestIndexer},
		{"aggregator", &status.LatestAggregator},
	} {
		s, err := latestScanSession(ctx, t.scanType)
		if err != nil {
			return nil, mcpToolError(err, "read index status")
		}
		*t.dst = s
	}

	body, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return nil, mcpToolError(err, "read index status")
	}
	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{{
			URI:      req.Params.URI,
			MIMEType: "application/json",
			Text:     string(body),
		}},
	}, nil
}

// latestScanSession returns the newest run of one scan type, or nil when that
// binary has never run against this database.
func latestScanSession(ctx context.Context, scanType string) (*ScanSession, error) {
	var s ScanSession
	var endedAt sql.NullInt64
	err := db.QueryRowContext(ctx, `
		SELECT session_id, scan_type, status,
		       EXTRACT(EPOCH FROM started_at)::bigint,
		       EXTRACT(EPOCH FROM ended_at)::bigint,
		       files_scanned
		FROM scan_sessions
		WHERE scan_type = $1
		ORDER BY started_at DESC
		LIMIT 1
	`, scanType).Scan(&s.SessionID, &s.ScanType, &s.Status, &s.StartedAt, &endedAt, &s.FilesScanned)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("latest %s session: %w", scanType, err)
	}
	if endedAt.Valid {
		s.EndedAt = endedAt.Int64
	}
	return &s, nil
}

// --- HTTP transport and auth ------------------------------------------------

// mcpAuthorized reports whether a request carries a credential good for the MCP
// endpoint, and the user it names.
//
// Three ways in, all resolving to the same session mechanism the REST API uses:
// the session cookie (a browser-hosted client on this origin), the same signed
// session token in an Authorization: Bearer header (an MCP client has no cookie
// jar, so it obtains the token from the cookie POST /auth/login sets), and the
// optional static mcp.token for a headless client that cannot log in at all.
func mcpAuthorized(r *http.Request) (string, bool) {
	if user, ok := sessionUser(r); ok {
		return user, true
	}

	raw, ok := bearerToken(r)
	if !ok {
		return "", false
	}
	if user, ok := parseSessionToken(raw); ok {
		return user, true
	}
	// Constant-time so a wrong token leaks nothing about how much of it was
	// right. Guarded on non-empty because an unset mcp.token must not turn an
	// empty Authorization header into a valid credential.
	if config.MCP.Token != "" && subtle.ConstantTimeCompare([]byte(config.MCP.Token), []byte(raw)) == 1 {
		return "mcp-token", true
	}
	return "", false
}

// bearerToken extracts the credential from an Authorization: Bearer header.
// The scheme is matched case-insensitively, as RFC 7235 requires.
func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	const prefix = "bearer "
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	return strings.TrimSpace(h[len(prefix):]), true
}

// mcpAuthMiddleware rejects unauthenticated MCP requests. The WWW-Authenticate
// header is what tells a spec-compliant MCP client that the endpoint wants a
// bearer token rather than that it is simply broken.
func mcpAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := mcpAuthorized(r)
		if !ok {
			w.Header().Set("WWW-Authenticate", `Bearer realm="cuttlefish-mcp"`)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		debugLog("--> MCP %s %s (user %s)", r.Method, r.URL.Path, user)
		next.ServeHTTP(w, r)
	})
}

// newMCPHTTPHandler returns the authenticated Streamable HTTP handler for the
// MCP endpoint. One server instance serves every session: the tools hold no
// per-session state, so there is nothing to keep apart.
func newMCPHTTPHandler() http.Handler {
	server := newMCPServer()
	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{
			SessionTimeout: mcpSessionTimeout,

			// The SDK's DNS-rebinding guard rejects any request whose *local*
			// socket address is loopback while the Host header is not. A
			// reverse proxy on the same host — the deployment this project
			// documents for TLS termination — connects over loopback and
			// forwards the public Host, so leaving this on 403s the normal
			// setup, and does so identically whether the server is bound to
			// 127.0.0.1 or 0.0.0.0.
			//
			// Turning it off costs little here. The attack it prevents is a
			// browser being rebound onto a local MCP server that trusts
			// anything that reaches it; every request to this endpoint needs a
			// credential, and a rebound page cannot get one — cookies are bound
			// to the real origin, and the bearer token never lives in a browser.
			DisableLocalhostProtection: true,
		},
	)
	return mcpAuthMiddleware(handler)
}

// registerMCP mounts the MCP endpoint, either on mux (the main server) or —
// when mcp.listen_addr is set — on a listener of its own, started here in the
// background. It is never both: a deployment that separates the agent-facing
// port from the UI port does so to be able to firewall them differently, which
// a second copy on the main port would undo.
func registerMCP(mux *http.ServeMux) {
	if !config.MCP.IsEnabled() {
		log.Printf("MCP server: disabled")
		return
	}

	handler := newMCPHTTPHandler()

	if addr := config.MCP.ListenAddr; addr != "" {
		mcpMux := http.NewServeMux()
		mcpMux.Handle(config.MCP.Path, handler)
		mcpMux.Handle(config.MCP.Path+"/", handler)
		srv := &http.Server{Addr: addr, Handler: mcpMux}
		go func() {
			var err error
			if tlsEnabled() {
				err = srv.ListenAndServeTLS(config.Server.TLSCert, config.Server.TLSKey)
			} else {
				err = srv.ListenAndServe()
			}
			// Fatal: a deployment that configured a dedicated MCP port and got
			// a live server without one has no way to notice from the outside
			// except by the endpoint never answering.
			log.Fatalf("MCP listener on %s failed: %v", addr, err)
		}()
		log.Printf("MCP server: %s%s (dedicated listener)", addr, config.MCP.Path)
		return
	}

	mux.Handle(config.MCP.Path, handler)
	mux.Handle(config.MCP.Path+"/", handler)
	log.Printf("MCP server: %s (on the main listener)", config.MCP.Path)
}
