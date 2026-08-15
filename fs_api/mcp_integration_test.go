package main

// End-to-end tests for the MCP server against a real PostgreSQL instance.
//
// Skipped unless CUTTLEFISH_TEST_DSN names a database the test may write to:
//
//	CUTTLEFISH_TEST_DSN="host=localhost user=postgres password=postgres dbname=fs_index sslmode=disable" go test ./fs_api/...
//
// Everything is created inside a throwaway schema that is dropped when the test
// finishes, so the tables here never touch a real index. The connection is
// checked with current_schema() before any DDL runs: if search_path did not
// take, the test fails rather than creating tables in public.

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const testSchema = "cuttlefish_mcp_test"

// testDB points the package-level db handle at a scratch schema, seeds it, and
// restores everything afterwards.
func testDB(t testing.TB) {
	t.Helper()

	dsn := os.Getenv("CUTTLEFISH_TEST_DSN")
	if dsn == "" {
		t.Skip("set CUTTLEFISH_TEST_DSN to run the database-backed MCP tests")
	}

	// Create the schema over a plain connection first: search_path cannot point
	// at a schema that does not exist yet.
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open admin connection: %v", err)
	}
	// Registered before the drop below so it runs after it: t.Cleanup is LIFO,
	// and the drop needs a live connection. A plain defer would close the
	// handle when this helper returns, long before the test body runs.
	t.Cleanup(func() { admin.Close() })

	if _, err := admin.Exec("DROP SCHEMA IF EXISTS " + testSchema + " CASCADE"); err != nil {
		t.Fatalf("drop stale test schema: %v", err)
	}
	if _, err := admin.Exec("CREATE SCHEMA " + testSchema); err != nil {
		t.Fatalf("create test schema: %v", err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP SCHEMA IF EXISTS " + testSchema + " CASCADE"); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
	})

	scoped, err := sql.Open("postgres", withSearchPath(dsn, testSchema))
	if err != nil {
		t.Fatalf("open scoped connection: %v", err)
	}

	var current string
	if err := scoped.QueryRow("SELECT current_schema()").Scan(&current); err != nil {
		scoped.Close()
		t.Fatalf("check search_path: %v", err)
	}
	if current != testSchema {
		scoped.Close()
		t.Fatalf("search_path did not take: current_schema() = %q, want %q; "+
			"refusing to create test tables in the wrong schema", current, testSchema)
	}

	saved := db
	db = scoped
	t.Cleanup(func() {
		scoped.Close()
		db = saved
	})

	seedTestIndex(t)
}

// withSearchPath pins a connection to one schema. lib/pq accepts both the
// keyword/value and URL DSN forms, and passes any parameter it doesn't
// recognize through as a server runtime setting.
func withSearchPath(dsn, schema string) string {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		sep := "?"
		if strings.Contains(dsn, "?") {
			sep = "&"
		}
		return dsn + sep + "search_path=" + schema
	}
	return dsn + " search_path=" + schema
}

// testEntry mirrors one filesystem_index row.
type testEntry struct {
	path     string
	size     int64
	fileType int
	perms    string
	uid, gid int
	mtime    int64
}

// testTree is the fixture every assertion below is written against.
var testTree = []testEntry{
	{"/", 4096, 2, "755", 0, 0, 1000},
	{"/home", 4096, 2, "755", 0, 0, 1100},
	{"/home/alice", 4096, 2, "750", 1000, 1000, 1200},
	{"/home/alice/notes.txt", 1024, 1, "644", 1000, 1000, 1300},
	{"/home/alice/archive.bin", 3 << 30, 1, "600", 1000, 1000, 1400},
	{"/home/bob", 4096, 2, "750", 1001, 1001, 1500},
	{"/home/bob/server.log", 512, 1, "644", 1001, 1001, 1600},
	{"/home/bob/current.log", 0, 3, "777", 1001, 1001, 1700},
	{"/empty", 4096, 2, "755", 0, 0, 1800},
}

func seedTestIndex(t testing.TB) {
	t.Helper()

	// Same shape as the tables fs_indexer and fs_aggregator create, minus the
	// UNLOGGED qualifier, which is irrelevant to a query and is a rewrite this
	// test has no reason to pay for.
	ddl := `
		CREATE TABLE filesystem_index (
			path TEXT, path_raw BYTEA, path_hash BYTEA PRIMARY KEY, parent_hash BYTEA,
			size_bytes BIGINT, file_type INTEGER, permissions TEXT,
			uid INTEGER, gid INTEGER, atime BIGINT, mtime BIGINT, ctime BIGINT, metadata TEXT
		);
		CREATE TABLE identity_map (
			id INTEGER, id_type TEXT CHECK (id_type IN ('uid','gid')), name TEXT NOT NULL,
			PRIMARY KEY (id, id_type)
		);
		CREATE TABLE dir_stats (
			path_hash BYTEA PRIMARY KEY, path TEXT, total_size_bytes BIGINT,
			mtime_first BIGINT, mtime_last BIGINT, atime_first BIGINT, atime_last BIGINT,
			ctime_first BIGINT, ctime_last BIGINT, file_count BIGINT
		);
		CREATE TABLE user_stats (
			id_type TEXT, id_value INT, total_size_bytes BIGINT, file_count BIGINT,
			PRIMARY KEY (id_type, id_value)
		);
		CREATE TABLE scan_sessions (
			session_id TEXT PRIMARY KEY, scan_type TEXT NOT NULL DEFAULT 'indexer',
			status TEXT NOT NULL DEFAULT 'running', files_scanned BIGINT NOT NULL DEFAULT 0,
			started_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP, ended_at TIMESTAMP
		);
	`
	if _, err := db.Exec(ddl); err != nil {
		t.Fatalf("create test tables: %v", err)
	}

	for _, e := range testTree {
		var parent any
		if e.path != "/" {
			parentPath := e.path[:strings.LastIndex(e.path, "/")]
			if parentPath == "" {
				parentPath = "/"
			}
			parent = pathHash(parentPath)
		}
		_, err := db.Exec(`
			INSERT INTO filesystem_index
			  (path, path_hash, parent_hash, size_bytes, file_type, permissions, uid, gid, atime, mtime, ctime, metadata)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'')
		`, e.path, pathHash(e.path), parent, e.size, e.fileType, e.perms, e.uid, e.gid, e.mtime, e.mtime, e.mtime)
		if err != nil {
			t.Fatalf("insert %s: %v", e.path, err)
		}
	}

	for _, row := range []struct {
		id     int
		idType string
		name   string
	}{
		{0, "uid", "root"}, {1000, "uid", "alice"}, {1001, "uid", "bob"},
		{0, "gid", "root"}, {1000, "gid", "alice"}, {1001, "gid", "bob"},
	} {
		if _, err := db.Exec(`INSERT INTO identity_map (id, id_type, name) VALUES ($1,$2,$3)`,
			row.id, row.idType, row.name); err != nil {
			t.Fatalf("insert identity %s: %v", row.name, err)
		}
	}

	// Directory rollups. Only /home/alice and /home get one, so the tests can
	// also cover the "aggregator hasn't reached this directory" case.
	for _, d := range []struct {
		path      string
		totalSize int64
		count     int64
	}{
		{"/home/alice", (3 << 30) + 1024, 2},
		{"/home", (3 << 30) + 1024 + 512, 5},
	} {
		if _, err := db.Exec(`
			INSERT INTO dir_stats (path_hash, path, total_size_bytes, mtime_first, mtime_last,
			                       atime_first, atime_last, ctime_first, ctime_last, file_count)
			VALUES ($1,$2,$3,1200,1700,1200,1700,1200,1700,$4)
		`, pathHash(d.path), d.path, d.totalSize, d.count); err != nil {
			t.Fatalf("insert dir_stats %s: %v", d.path, err)
		}
	}

	for _, s := range []struct {
		idType string
		id     int
		size   int64
		count  int64
	}{
		{"uid", 1000, (3 << 30) + 1024, 2},
		{"uid", 1001, 512, 2},
		{"gid", 1000, (3 << 30) + 1024, 2},
		{"gid", 1001, 512, 2},
	} {
		if _, err := db.Exec(`INSERT INTO user_stats (id_type, id_value, total_size_bytes, file_count)
			VALUES ($1,$2,$3,$4)`, s.idType, s.id, s.size, s.count); err != nil {
			t.Fatalf("insert user_stats %s/%d: %v", s.idType, s.id, err)
		}
	}

	if _, err := db.Exec(`
		INSERT INTO scan_sessions (session_id, scan_type, status, files_scanned, started_at, ended_at) VALUES
		  ('sess-indexer',    'indexer',    'success', 9, NOW() - INTERVAL '2 hours', NOW() - INTERVAL '110 minutes'),
		  ('sess-aggregator', 'aggregator', 'success', 9, NOW() - INTERVAL '1 hour',  NOW() - INTERVAL '55 minutes')
	`); err != nil {
		t.Fatalf("insert scan_sessions: %v", err)
	}

	// The same secondary indexes fs_indexer builds after a scan. They make no
	// difference to nine rows, but the benchmark below runs against the same
	// fixture and a plan without them would measure nothing real.
	if _, err := db.Exec(`
		CREATE INDEX idx_fsindex_parent_hash ON filesystem_index (parent_hash);
		CREATE INDEX idx_fsindex_uid ON filesystem_index (uid);
		CREATE INDEX idx_fsindex_gid ON filesystem_index (gid);
		CREATE INDEX idx_fsindex_size ON filesystem_index (size_bytes);
		CREATE INDEX idx_fsindex_mtime ON filesystem_index (mtime);
	`); err != nil {
		t.Fatalf("create secondary indexes: %v", err)
	}
	// Trigram index for substring search, skipped when the extension isn't
	// installable — exactly what fs_indexer does, and what a search then falls
	// back to (a sequential scan) is worth measuring too.
	if _, err := db.Exec(`CREATE EXTENSION IF NOT EXISTS pg_trgm`); err == nil {
		if _, err := db.Exec(`CREATE INDEX idx_fsindex_path_trgm ON filesystem_index USING GIN (path gin_trgm_ops)`); err != nil {
			t.Logf("trigram index not created: %v", err)
		}
	} else {
		t.Logf("pg_trgm not available, substring search will sequentially scan: %v", err)
	}
}

// seedBenchEntries adds n sibling files under /bench, so the benchmark measures
// a listing and a search over something bigger than the correctness fixture.
// Hashes are computed by PostgreSQL with the same construction as
// fs_common::compute_hash — SHA-256 of the path bytes, first pathHashLen bytes.
func seedBenchEntries(b *testing.B, n int) {
	b.Helper()

	if _, err := db.Exec(`
		INSERT INTO filesystem_index
		  (path, path_hash, parent_hash, size_bytes, file_type, permissions, uid, gid, atime, mtime, ctime, metadata)
		VALUES ('/bench', substring(sha256(convert_to('/bench','UTF8')) from 1 for $1),
		        substring(sha256(convert_to('/','UTF8')) from 1 for $1), 4096, 2, '755', 0, 0, 0, 0, 0, '')
	`, pathHashLen); err != nil {
		b.Fatalf("insert /bench: %v", err)
	}

	if _, err := db.Exec(`
		INSERT INTO filesystem_index
		  (path, path_hash, parent_hash, size_bytes, file_type, permissions, uid, gid, atime, mtime, ctime, metadata)
		SELECT x.p,
		       substring(sha256(convert_to(x.p,'UTF8')) from 1 for $1),
		       substring(sha256(convert_to('/bench','UTF8')) from 1 for $1),
		       i * 1024, 1, '644', 1000 + (i % 7), 1000 + (i % 7), 0, i, 0, ''
		FROM generate_series(1, $2) AS i
		CROSS JOIN LATERAL (SELECT '/bench/file_' || i || '.dat' AS p) AS x
	`, pathHashLen, n); err != nil {
		b.Fatalf("seed bench entries: %v", err)
	}
	if _, err := db.Exec(`ANALYZE filesystem_index`); err != nil {
		b.Fatalf("analyze: %v", err)
	}
}

// BenchmarkMCPTools measures a full tool call — JSON-RPC in, structured result
// out — for the query shapes a client hits most. Run it with the same
// CUTTLEFISH_TEST_DSN the integration tests use:
//
//	go test -bench MCPTools -benchtime 100x ./fs_api/
func BenchmarkMCPTools(b *testing.B) {
	testDB(b)
	seedBenchEntries(b, 20000)
	cs, ctx := connectMCP(b)

	benchmarks := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"get_entry_stats", "get_entry_stats", map[string]any{"path": "/bench/file_9999.dat"}},
		{"list_directory_page", "list_directory", map[string]any{"path": "/bench", "limit": 100}},
		{"list_directory_deep_page", "list_directory", map[string]any{"path": "/bench", "limit": 100, "offset": 15000}},
		{"search_substring", "search_files", map[string]any{
			"rules": []map[string]any{{"field": "path", "operator": "contains", "value": "file_123"}},
		}},
		{"search_size_sorted", "search_files", map[string]any{
			"rules":   []map[string]any{{"field": "size_bytes", "operator": "gt", "value": "10M"}},
			"sort_by": "size_bytes", "order": "DESC",
		}},
		{"list_user_stats", "list_user_stats", nil},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			for b.Loop() {
				res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: bm.tool, Arguments: bm.args})
				if err != nil {
					b.Fatalf("%s: %v", bm.tool, err)
				}
				if res.IsError {
					b.Fatalf("%s: %s", bm.tool, toolErrorText(res))
				}
			}
		})
	}
}

// callTool runs a tool and decodes its structured output, failing the test on
// either a protocol error or a tool error.
func callTool[T any](t *testing.T, cs *mcp.ClientSession, ctx context.Context, name string, args map[string]any) T {
	t.Helper()
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if res.IsError {
		t.Fatalf("%s returned an error: %s", name, toolErrorText(res))
	}
	if res.StructuredContent == nil {
		t.Fatalf("%s returned no structured content", name)
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("%s: remarshal output: %v", name, err)
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("%s: decode output: %v (raw: %s)", name, err, raw)
	}
	return out
}

// callToolExpectError runs a tool that is supposed to fail and returns the
// message. A tool error must arrive as IsError, not as a protocol error: the
// distinction is what lets a model retry with different arguments.
func callToolExpectError(t *testing.T, cs *mcp.ClientSession, ctx context.Context, name string, args map[string]any) string {
	t.Helper()
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: expected a tool error, got a protocol error: %v", name, err)
	}
	if !res.IsError {
		t.Fatalf("%s succeeded, expected an error", name)
	}
	return toolErrorText(res)
}

func toolErrorText(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func TestMCPGetEntryStats(t *testing.T) {
	testDB(t)
	cs, ctx := connectMCP(t)

	t.Run("file", func(t *testing.T) {
		out := callTool[mcpEntryOutput](t, cs, ctx, "get_entry_stats", map[string]any{
			"path": "/home/alice/notes.txt",
		})
		e := out.Entry
		if e.Path != "/home/alice/notes.txt" {
			t.Errorf("path = %q", e.Path)
		}
		if e.SizeBytes != 1024 {
			t.Errorf("size_bytes = %d, want 1024", e.SizeBytes)
		}
		if e.FileType != 1 {
			t.Errorf("file_type = %d, want 1", e.FileType)
		}
		if e.User != "alice" || e.Group != "alice" {
			t.Errorf("user/group = %q/%q, want alice/alice", e.User, e.Group)
		}
		// Same rendering the REST API performs, not the raw octal.
		if e.Perms != "rw-r--r--" {
			t.Errorf("permissions = %q, want rw-r--r--", e.Perms)
		}
		if e.Aggregates != nil {
			t.Errorf("a regular file carries aggregates: %+v", e.Aggregates)
		}
	})

	t.Run("directory with rollups", func(t *testing.T) {
		out := callTool[mcpEntryOutput](t, cs, ctx, "get_entry_stats", map[string]any{
			"path": "/home/alice",
		})
		if out.Entry.Aggregates == nil {
			t.Fatal("directory returned no aggregates")
		}
		if got, want := out.Entry.Aggregates.TotalSize, int64((3<<30)+1024); got != want {
			t.Errorf("total_size_bytes = %d, want %d", got, want)
		}
		if got := out.Entry.Aggregates.FileCount; got != 2 {
			t.Errorf("file_count = %d, want 2", got)
		}
	})

	t.Run("include_stats false", func(t *testing.T) {
		out := callTool[mcpEntryOutput](t, cs, ctx, "get_entry_stats", map[string]any{
			"path":          "/home/alice",
			"include_stats": false,
		})
		if out.Entry.Aggregates != nil {
			t.Errorf("include_stats=false still returned aggregates")
		}
	})

	t.Run("directory the aggregator has not reached", func(t *testing.T) {
		out := callTool[mcpEntryOutput](t, cs, ctx, "get_entry_stats", map[string]any{
			"path": "/home/bob",
		})
		if out.Entry.Aggregates != nil {
			t.Errorf("aggregates present for a directory with no dir_stats row")
		}
	})

	t.Run("symlink resolves like any other entry", func(t *testing.T) {
		out := callTool[mcpEntryOutput](t, cs, ctx, "get_entry_stats", map[string]any{
			"path": "/home/bob/current.log",
		})
		if out.Entry.FileType != 3 {
			t.Errorf("file_type = %d, want 3", out.Entry.FileType)
		}
	})

	t.Run("missing path", func(t *testing.T) {
		msg := callToolExpectError(t, cs, ctx, "get_entry_stats", map[string]any{
			"path": "/nope/not/here",
		})
		if !strings.Contains(strings.ToLower(msg), "no entry") {
			t.Errorf("error message = %q, want it to say the path isn't indexed", msg)
		}
	})
}

func TestMCPListDirectory(t *testing.T) {
	testDB(t)
	cs, ctx := connectMCP(t)

	t.Run("children", func(t *testing.T) {
		out := callTool[mcpListDirectoryOutput](t, cs, ctx, "list_directory", map[string]any{
			"path": "/home",
		})
		if out.Count != 2 || len(out.Entries) != 2 {
			t.Fatalf("count = %d, entries = %d, want 2 and 2", out.Count, len(out.Entries))
		}
		// A limited listing is ordered by path, so this is deterministic.
		if out.Entries[0].Path != "/home/alice" || out.Entries[1].Path != "/home/bob" {
			t.Errorf("entries = %q, %q; want /home/alice, /home/bob", out.Entries[0].Path, out.Entries[1].Path)
		}
		if out.Truncated {
			t.Error("truncated set on a complete listing")
		}
		if out.Entries[0].Aggregates == nil {
			t.Error("child directory /home/alice carries no aggregates")
		}
	})

	t.Run("paging reports truncation", func(t *testing.T) {
		first := callTool[mcpListDirectoryOutput](t, cs, ctx, "list_directory", map[string]any{
			"path": "/home", "limit": 1,
		})
		if len(first.Entries) != 1 || !first.Truncated {
			t.Fatalf("limit=1 gave %d entries, truncated=%v; want 1 and true", len(first.Entries), first.Truncated)
		}
		if first.Entries[0].Path != "/home/alice" {
			t.Errorf("first page = %q, want /home/alice", first.Entries[0].Path)
		}

		second := callTool[mcpListDirectoryOutput](t, cs, ctx, "list_directory", map[string]any{
			"path": "/home", "limit": 1, "offset": 1,
		})
		if len(second.Entries) != 1 || second.Truncated {
			t.Fatalf("second page gave %d entries, truncated=%v; want 1 and false", len(second.Entries), second.Truncated)
		}
		if second.Entries[0].Path != "/home/bob" {
			t.Errorf("second page = %q, want /home/bob", second.Entries[0].Path)
		}
	})

	t.Run("indexed but empty directory", func(t *testing.T) {
		out := callTool[mcpListDirectoryOutput](t, cs, ctx, "list_directory", map[string]any{
			"path": "/empty",
		})
		if out.Count != 0 {
			t.Errorf("count = %d, want 0", out.Count)
		}
		if out.Entries == nil {
			t.Error("entries is null; an empty listing should be []")
		}
	})

	t.Run("unindexed directory is an error", func(t *testing.T) {
		callToolExpectError(t, cs, ctx, "list_directory", map[string]any{"path": "/no/such/dir"})
	})
}

func TestMCPSearchFiles(t *testing.T) {
	testDB(t)
	cs, ctx := connectMCP(t)

	t.Run("substring on path", func(t *testing.T) {
		out := callTool[mcpSearchOutput](t, cs, ctx, "search_files", map[string]any{
			"rules": []map[string]any{{"field": "path", "operator": "contains", "value": ".log"}},
		})
		if out.Count != 2 {
			t.Fatalf("count = %d, want 2 (%+v)", out.Count, out.Results)
		}
	})

	t.Run("human-readable size threshold", func(t *testing.T) {
		out := callTool[mcpSearchOutput](t, cs, ctx, "search_files", map[string]any{
			"rules": []map[string]any{{"field": "size_bytes", "operator": "gt", "value": "1G"}},
		})
		if out.Count != 1 || out.Results[0].Path != "/home/alice/archive.bin" {
			t.Fatalf("got %+v, want just /home/alice/archive.bin", out.Results)
		}
	})

	t.Run("AND across fields", func(t *testing.T) {
		out := callTool[mcpSearchOutput](t, cs, ctx, "search_files", map[string]any{
			"rules": []map[string]any{
				{"field": "path", "operator": "starts_with", "value": "/home/"},
				{"field": "uid", "operator": "equals", "value": "1001", "connector": "AND"},
			},
		})
		if out.Count != 3 {
			t.Fatalf("count = %d, want 3 (bob's dir, log and symlink): %+v", out.Count, out.Results)
		}
	})

	t.Run("negated rule keeps rows with no match", func(t *testing.T) {
		out := callTool[mcpSearchOutput](t, cs, ctx, "search_files", map[string]any{
			"rules": []map[string]any{
				{"field": "user", "operator": "equals", "value": "alice", "negate": true},
			},
		})
		for _, r := range out.Results {
			if r.User == "alice" {
				t.Errorf("negated rule returned an alice-owned entry: %s", r.Path)
			}
		}
		if out.Count == 0 {
			t.Error("negated rule matched nothing")
		}
	})

	t.Run("sorting and truncation", func(t *testing.T) {
		out := callTool[mcpSearchOutput](t, cs, ctx, "search_files", map[string]any{
			"rules":   []map[string]any{{"field": "file_type", "operator": "equals", "value": "1"}},
			"sort_by": "size_bytes",
			"order":   "DESC",
			"limit":   1,
		})
		if len(out.Results) != 1 || !out.Truncated {
			t.Fatalf("limit=1 gave %d results, truncated=%v", len(out.Results), out.Truncated)
		}
		if out.Results[0].Path != "/home/alice/archive.bin" {
			t.Errorf("largest file = %q, want /home/alice/archive.bin", out.Results[0].Path)
		}
	})

	t.Run("no rules is a tool error", func(t *testing.T) {
		callToolExpectError(t, cs, ctx, "search_files", map[string]any{"rules": []map[string]any{}})
	})

	t.Run("unknown field is a tool error", func(t *testing.T) {
		msg := callToolExpectError(t, cs, ctx, "search_files", map[string]any{
			"rules": []map[string]any{{"field": "contents", "operator": "contains", "value": "secret"}},
		})
		if !strings.Contains(msg, "contents") {
			t.Errorf("error message = %q, want it to name the bad field", msg)
		}
	})

	// The regex operators compile as POSIX ARE inside PostgreSQL, so a bad
	// pattern surfaces as a query failure. It has to reach the caller as their
	// mistake, not as an internal error.
	t.Run("bad regex is the caller's error", func(t *testing.T) {
		msg := callToolExpectError(t, cs, ctx, "search_files", map[string]any{
			"rules": []map[string]any{{"field": "path", "operator": "regex", "value": "[unterminated"}},
		})
		if strings.Contains(msg, "internal server error") {
			t.Errorf("bad regex reported as an internal error: %q", msg)
		}
	})
}

func TestMCPIdentityStats(t *testing.T) {
	testDB(t)
	cs, ctx := connectMCP(t)

	users := callTool[mcpIdentityStatsOutput](t, cs, ctx, "list_user_stats", nil)
	if users.Count != 2 {
		t.Fatalf("user count = %d, want 2", users.Count)
	}
	// Default sort is largest first.
	if users.Stats[0].Name != "alice" {
		t.Errorf("largest user = %q, want alice", users.Stats[0].Name)
	}
	if users.Stats[0].IDType != "uid" {
		t.Errorf("id_type = %q, want uid", users.Stats[0].IDType)
	}

	byName := callTool[mcpIdentityStatsOutput](t, cs, ctx, "list_user_stats", map[string]any{
		"sort_by": "name", "order": "asc",
	})
	if byName.Stats[0].Name != "alice" {
		t.Errorf("first by name = %q, want alice", byName.Stats[0].Name)
	}

	groups := callTool[mcpIdentityStatsOutput](t, cs, ctx, "list_group_stats", nil)
	if groups.Count != 2 || groups.Stats[0].IDType != "gid" {
		t.Fatalf("group stats = %+v, want two gid rows", groups.Stats)
	}
}

func TestMCPScanSessions(t *testing.T) {
	testDB(t)
	cs, ctx := connectMCP(t)

	out := callTool[mcpScanSessionsOutput](t, cs, ctx, "list_scan_sessions", nil)
	if out.Count != 2 {
		t.Fatalf("count = %d, want 2", out.Count)
	}
	// Newest first.
	if out.Sessions[0].ScanType != "aggregator" {
		t.Errorf("first session type = %q, want aggregator", out.Sessions[0].ScanType)
	}
	if out.Sessions[0].Status != "success" || out.Sessions[0].EndedAt == 0 {
		t.Errorf("finished session reported as %+v", out.Sessions[0])
	}
}

func TestMCPIndexStatusResource(t *testing.T) {
	testDB(t)
	cs, ctx := connectMCP(t)

	res, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: mcpStatusURI})
	if err != nil {
		t.Fatalf("resources/read: %v", err)
	}
	if len(res.Contents) != 1 {
		t.Fatalf("got %d content parts, want 1", len(res.Contents))
	}

	var status mcpIndexStatus
	if err := json.Unmarshal([]byte(res.Contents[0].Text), &status); err != nil {
		t.Fatalf("decode status: %v (raw: %s)", err, res.Contents[0].Text)
	}
	if status.PathHashBytes != pathHashLen {
		t.Errorf("path_hash_bytes = %d, want %d", status.PathHashBytes, pathHashLen)
	}
	if status.LatestIndexer == nil || status.LatestIndexer.SessionID != "sess-indexer" {
		t.Errorf("latest_indexer = %+v, want sess-indexer", status.LatestIndexer)
	}
	if status.LatestAggregator == nil || status.LatestAggregator.SessionID != "sess-aggregator" {
		t.Errorf("latest_aggregator = %+v, want sess-aggregator", status.LatestAggregator)
	}
}

// TestMCPMatchesREST is the anti-drift check: the MCP tools and the REST
// endpoints go through the same query layer, so for the same entry they must
// produce the same FileInfo. If someone reimplements one of them, this fails.
func TestMCPMatchesREST(t *testing.T) {
	testDB(t)
	cs, ctx := connectMCP(t)

	const path = "/home/alice/notes.txt"

	out := callTool[mcpEntryOutput](t, cs, ctx, "get_entry_stats", map[string]any{"path": path})
	viaMCP, err := json.Marshal(out.Entry)
	if err != nil {
		t.Fatalf("marshal MCP entry: %v", err)
	}

	w := httptest.NewRecorder()
	GetFileStats(w, httptest.NewRequest(http.MethodGet, "/api/file/stats?path="+path, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("REST status = %d: %s", w.Code, w.Body.String())
	}
	var restEntry FileInfo
	if err := json.Unmarshal(w.Body.Bytes(), &restEntry); err != nil {
		t.Fatalf("decode REST entry: %v", err)
	}
	viaREST, err := json.Marshal(restEntry)
	if err != nil {
		t.Fatalf("marshal REST entry: %v", err)
	}

	if string(viaMCP) != string(viaREST) {
		t.Errorf("MCP and REST disagree about %s:\n  MCP:  %s\n  REST: %s", path, viaMCP, viaREST)
	}
}

// TestMCPOverHTTP drives the real transport: a Streamable HTTP client against
// the handler main.go mounts, through the auth middleware.
func TestMCPOverHTTP(t *testing.T) {
	testDB(t)
	withTestConfig(t, "")
	token := testSessionToken(t)

	mux := http.NewServeMux()
	handler := newMCPHTTPHandler()
	mux.Handle(defaultMCPPath, handler)
	mux.Handle(defaultMCPPath+"/", handler)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx := context.Background()
	endpoint := srv.URL + defaultMCPPath

	t.Run("rejects an unauthenticated client", func(t *testing.T) {
		client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0"}, nil)
		session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: endpoint}, nil)
		if err == nil {
			session.Close()
			t.Fatal("connected without credentials")
		}
	})

	t.Run("accepts a bearer session token", func(t *testing.T) {
		client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0"}, nil)
		session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
			Endpoint:   endpoint,
			HTTPClient: &http.Client{Transport: bearerTransport{token: token}},
		}, nil)
		if err != nil {
			t.Fatalf("connect: %v", err)
		}
		defer session.Close()

		tools, err := session.ListTools(ctx, nil)
		if err != nil {
			t.Fatalf("tools/list over HTTP: %v", err)
		}
		if len(tools.Tools) == 0 {
			t.Fatal("no tools listed over HTTP")
		}

		out := callTool[mcpEntryOutput](t, session, ctx, "get_entry_stats", map[string]any{
			"path": "/home/alice/notes.txt",
		})
		if out.Entry.SizeBytes != 1024 {
			t.Errorf("size over HTTP = %d, want 1024", out.Entry.SizeBytes)
		}
	})
}

// bearerTransport attaches the session token to every request, which is how a
// headless MCP client authenticates against this endpoint.
type bearerTransport struct{ token string }

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	clone := r.Clone(r.Context())
	clone.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(clone)
}
