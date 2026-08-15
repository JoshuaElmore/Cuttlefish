package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// connectMCP wires an in-memory client to a freshly built server. Nothing here
// touches the database: tools/list, the schemas and input validation are all
// answered before a handler runs, which is exactly the surface these tests are
// about. Tool calls that would reach the query layer live in
// mcp_integration_test.go, behind a real database.
func connectMCP(t testing.TB) (*mcp.ClientSession, context.Context) {
	t.Helper()
	ctx := context.Background()

	server := newMCPServer()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0"}, nil)

	st, ct := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	clientSession, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		clientSession.Close()
		serverSession.Wait()
	})
	return clientSession, ctx
}

// TestMCPServerBuilds is the schema-inference guard. mcp.AddTool panics when it
// cannot derive a JSON schema for a tool's input or output, and FileInfo
// carries a []byte and a pointer struct that make that a real risk — so simply
// constructing the server is the assertion.
func TestMCPServerBuilds(t *testing.T) {
	if s := newMCPServer(); s == nil {
		t.Fatal("newMCPServer returned nil")
	}
}

func TestMCPListTools(t *testing.T) {
	cs, ctx := connectMCP(t)

	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}

	want := map[string]bool{
		"get_entry_stats":    false,
		"list_directory":     false,
		"search_files":       false,
		"list_user_stats":    false,
		"list_group_stats":   false,
		"list_scan_sessions": false,
	}
	for _, tool := range res.Tools {
		if _, ok := want[tool.Name]; !ok {
			t.Errorf("unexpected tool %q", tool.Name)
			continue
		}
		want[tool.Name] = true

		if tool.Description == "" {
			t.Errorf("tool %q has no description", tool.Name)
		}
		if tool.InputSchema == nil {
			t.Errorf("tool %q has no input schema", tool.Name)
		}
		// Every tool here reads; a client that trusts the hint must not be
		// misled into thinking one of them could write.
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Errorf("tool %q is not annotated read-only", tool.Name)
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("tool %q missing from tools/list", name)
		}
	}
}

// TestMCPToolSchemasRequirePath pins the required-argument list, since the
// schema is the only contract a calling model sees.
func TestMCPToolSchemasRequirePath(t *testing.T) {
	cs, ctx := connectMCP(t)

	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}

	required := map[string][]string{
		"get_entry_stats":    {"path"},
		"list_directory":     {"path"},
		"search_files":       {"rules"},
		"list_user_stats":    nil,
		"list_group_stats":   nil,
		"list_scan_sessions": nil,
	}

	for _, tool := range res.Tools {
		raw, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatalf("marshal %s schema: %v", tool.Name, err)
		}
		var schema struct {
			Type       string              `json:"type"`
			Required   []string            `json:"required"`
			Properties map[string]struct{} `json:"properties"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatalf("unmarshal %s schema: %v", tool.Name, err)
		}
		if schema.Type != "object" {
			t.Errorf("tool %q input schema type = %q, want object", tool.Name, schema.Type)
		}
		want := required[tool.Name]
		if len(schema.Required) != len(want) {
			t.Errorf("tool %q required = %v, want %v", tool.Name, schema.Required, want)
			continue
		}
		for i, name := range want {
			if schema.Required[i] != name {
				t.Errorf("tool %q required[%d] = %q, want %q", tool.Name, i, schema.Required[i], name)
			}
		}
	}
}

// TestMCPInputValidation checks that a malformed call is rejected by the schema
// before the handler runs. It matters twice over here: the package-level db is
// nil in this test, so anything that reached the query layer would panic.
func TestMCPInputValidation(t *testing.T) {
	cs, ctx := connectMCP(t)

	tests := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"missing path", "get_entry_stats", map[string]any{}},
		{"wrong path type", "get_entry_stats", map[string]any{"path": 42}},
		{"missing rules", "search_files", map[string]any{}},
		{"limit not a number", "list_user_stats", map[string]any{"limit": "many"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: tc.tool, Arguments: tc.args})
			if err != nil {
				return // rejected as a protocol error, which is also fine
			}
			if !res.IsError {
				t.Fatalf("%s accepted invalid arguments %v", tc.tool, tc.args)
			}
		})
	}
}

// TestMCPBlankPathRejected covers the case the JSON schema cannot: a present
// but empty path would otherwise hash to something and 404 after a round trip
// to the database.
func TestMCPBlankPathRejected(t *testing.T) {
	cs, ctx := connectMCP(t)

	for _, tool := range []string{"get_entry_stats", "list_directory"} {
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{
			Name:      tool,
			Arguments: map[string]any{"path": "   "},
		})
		if err != nil {
			continue
		}
		if !res.IsError {
			t.Errorf("%s accepted a blank path", tool)
		}
	}
}

func TestMCPUnknownTool(t *testing.T) {
	cs, ctx := connectMCP(t)

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "delete_everything"})
	if err == nil && !res.IsError {
		t.Fatal("calling an unknown tool succeeded")
	}
}

func TestMCPListResources(t *testing.T) {
	cs, ctx := connectMCP(t)

	res, err := cs.ListResources(ctx, nil)
	if err != nil {
		t.Fatalf("resources/list: %v", err)
	}
	for _, r := range res.Resources {
		if r.URI == mcpStatusURI {
			if r.MIMEType != "application/json" {
				t.Errorf("index status MIME type = %q, want application/json", r.MIMEType)
			}
			return
		}
	}
	t.Fatalf("resource %q missing from resources/list (got %d resources)", mcpStatusURI, len(res.Resources))
}

// --- paging helpers ---------------------------------------------------------

func TestMCPPage(t *testing.T) {
	tests := []struct {
		limit, want, fetch int
	}{
		{0, mcpDefaultLimit, mcpDefaultLimit + 1},
		{-5, mcpDefaultLimit, mcpDefaultLimit + 1},
		{10, 10, 11},
		{mcpMaxLimit + 1, mcpMaxLimit, mcpMaxLimit + 1},
		{1 << 30, mcpMaxLimit, mcpMaxLimit + 1},
	}
	for _, tc := range tests {
		want, fetch := mcpPage(tc.limit)
		if want != tc.want || fetch != tc.fetch {
			t.Errorf("mcpPage(%d) = (%d, %d), want (%d, %d)", tc.limit, want, fetch, tc.want, tc.fetch)
		}
	}
}

func TestMCPTrim(t *testing.T) {
	rows, truncated := mcpTrim([]int{1, 2, 3, 4}, 3)
	if truncated != true || len(rows) != 3 {
		t.Errorf("over-full page: got (%v, %v), want ([1 2 3], true)", rows, truncated)
	}

	rows, truncated = mcpTrim([]int{1, 2}, 3)
	if truncated != false || len(rows) != 2 {
		t.Errorf("short page: got (%v, %v), want ([1 2], false)", rows, truncated)
	}

	// A nil slice must serialize as [] rather than null: a model reading a tool
	// result should see an empty list, not a missing one.
	rows, truncated = mcpTrim([]int(nil), 3)
	if truncated || rows == nil {
		t.Errorf("nil page: got (%v, %v), want ([], false)", rows, truncated)
	}
	if b, _ := json.Marshal(rows); string(b) != "[]" {
		t.Errorf("nil page marshals to %s, want []", b)
	}
}

func TestClampLimit(t *testing.T) {
	tests := []struct{ limit, def, want int }{
		{0, 50, 50},
		{-1, 50, 50},
		{10, 50, 10},
		{maxListLimit + 1, 50, maxListLimit},
	}
	for _, tc := range tests {
		if got := clampLimit(tc.limit, tc.def); got != tc.want {
			t.Errorf("clampLimit(%d, %d) = %d, want %d", tc.limit, tc.def, got, tc.want)
		}
	}
	if got := clampOffset(-3); got != 0 {
		t.Errorf("clampOffset(-3) = %d, want 0", got)
	}
}

// --- authentication ---------------------------------------------------------

const testSessionSecret = "0123456789abcdef0123456789abcdef"

func withTestConfig(t *testing.T, mcpToken string) {
	t.Helper()
	saved := config
	config = Config{
		Auth: AuthConfig{
			SessionSecret: testSessionSecret,
			Local:         LocalAuthConfig{Username: "admin", Password: "hunter2"},
		},
		MCP: MCPConfig{Path: defaultMCPPath, Token: mcpToken},
	}
	t.Cleanup(func() { config = saved })
}

// testSessionToken mints a session the same way a successful login does, so the
// test can't drift from the real token format.
func testSessionToken(t *testing.T) string {
	t.Helper()
	rec := httptest.NewRecorder()
	if err := issueSession(rec, "alice"); err != nil {
		t.Fatalf("issueSession: %v", err)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "session" {
			return c.Value
		}
	}
	t.Fatal("issueSession set no session cookie")
	return ""
}

func TestBearerToken(t *testing.T) {
	tests := []struct {
		header string
		want   string
		ok     bool
	}{
		{"Bearer abc123", "abc123", true},
		{"bearer abc123", "abc123", true}, // RFC 7235: the scheme is case-insensitive
		{"BEARER  abc123 ", "abc123", true},
		{"", "", false},
		{"Bearer", "", false},
		{"Bearer ", "", false},
		{"Basic abc123", "", false},
		{"Token abc123", "", false},
	}
	for _, tc := range tests {
		r := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		if tc.header != "" {
			r.Header.Set("Authorization", tc.header)
		}
		got, ok := bearerToken(r)
		if got != tc.want || ok != tc.ok {
			t.Errorf("bearerToken(%q) = (%q, %v), want (%q, %v)", tc.header, got, ok, tc.want, tc.ok)
		}
	}
}

func TestMCPAuthMiddleware(t *testing.T) {
	withTestConfig(t, "static-token-that-is-long-enough-32")
	token := testSessionToken(t)

	var reached bool
	handler := mcpAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		name    string
		prepare func(*http.Request)
		want    int
	}{
		{"no credentials", func(*http.Request) {}, http.StatusUnauthorized},
		{"session cookie", func(r *http.Request) {
			r.AddCookie(&http.Cookie{Name: "session", Value: token})
		}, http.StatusOK},
		{"session token as bearer", func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer "+token)
		}, http.StatusOK},
		{"static mcp token", func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer static-token-that-is-long-enough-32")
		}, http.StatusOK},
		{"garbage bearer", func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer not-a-real-token")
		}, http.StatusUnauthorized},
		{"tampered session token", func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer "+token[:len(token)-4]+"AAAA")
		}, http.StatusUnauthorized},
		{"cookie with a bad token", func(r *http.Request) {
			r.AddCookie(&http.Cookie{Name: "session", Value: "nonsense"})
		}, http.StatusUnauthorized},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reached = false
			r := httptest.NewRequest(http.MethodPost, defaultMCPPath, strings.NewReader("{}"))
			tc.prepare(r)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)

			if w.Code != tc.want {
				t.Errorf("status = %d, want %d", w.Code, tc.want)
			}
			if reached != (tc.want == http.StatusOK) {
				t.Errorf("handler reached = %v, want %v", reached, tc.want == http.StatusOK)
			}
			if w.Code == http.StatusUnauthorized && w.Header().Get("WWW-Authenticate") == "" {
				t.Error("401 response carries no WWW-Authenticate header")
			}
		})
	}
}

// TestMCPEmptyTokenNotACredential guards the case where mcp.token is unset: an
// empty configured token must not make an empty bearer credential valid.
func TestMCPEmptyTokenNotACredential(t *testing.T) {
	withTestConfig(t, "")

	for _, header := range []string{"Bearer ", "Bearer  ", "Bearer x"} {
		r := httptest.NewRequest(http.MethodPost, defaultMCPPath, nil)
		r.Header.Set("Authorization", header)
		if user, ok := mcpAuthorized(r); ok {
			t.Errorf("Authorization %q authorized as %q with mcp.token unset", header, user)
		}
	}
}

// TestMCPBehindReverseProxy pins the DisableLocalhostProtection decision in
// newMCPHTTPHandler. The SDK rejects a request that arrives on a loopback
// socket carrying a non-loopback Host header — which is exactly what a
// same-host TLS-terminating proxy sends, and what fs_config_template.yml tells
// operators to deploy. If the guard comes back, this 403s.
func TestMCPBehindReverseProxy(t *testing.T) {
	withTestConfig(t, "")
	token := testSessionToken(t)

	srv := httptest.NewServer(newMCPHTTPHandler())
	defer srv.Close()

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":` +
		`{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"proxied","version":"1"}}}`
	req, err := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	// What nginx forwards with proxy_set_header Host $host.
	req.Host = "cuttlefish.example.com"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+token)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusForbidden {
		t.Fatal("a proxied Host header was rejected; MCP is unreachable behind a same-host reverse proxy")
	}
	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}
}

// --- configuration ----------------------------------------------------------

func TestNormalizeMCPConfig(t *testing.T) {
	no, yes := false, true

	tests := []struct {
		name     string
		in       MCPConfig
		wantErr  bool
		wantPath string
		wantOn   bool
	}{
		{"defaults when absent", MCPConfig{}, false, defaultMCPPath, true},
		{"explicitly disabled", MCPConfig{Enabled: &no}, false, defaultMCPPath, false},
		{"explicitly enabled", MCPConfig{Enabled: &yes}, false, defaultMCPPath, true},
		{"custom path", MCPConfig{Path: "/agent/mcp"}, false, "/agent/mcp", true},
		{"trailing slash trimmed", MCPConfig{Path: "/mcp/"}, false, "/mcp", true},
		{"path must be absolute", MCPConfig{Path: "mcp"}, true, "", true},
		{"path may not shadow the UI", MCPConfig{Path: "/"}, true, "", true},
		{"path may not shadow the API", MCPConfig{Path: "/api"}, true, "", true},
		{"path may not shadow auth", MCPConfig{Path: "/auth/"}, true, "", true},
		{"a bad path on a disabled server is ignored", MCPConfig{Enabled: &no, Path: "/api"}, false, "/api", false},
		{"short token rejected", MCPConfig{Token: "short"}, true, "", true},
		{"template token rejected", MCPConfig{Token: mcpTokenTemplateValue}, true, "", true},
		{"long token accepted", MCPConfig{Token: strings.Repeat("a", mcpTokenMinLen)}, false, defaultMCPPath, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := tc.in
			err := normalizeMCPConfig(&m)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("normalizeMCPConfig(%+v) succeeded, want an error", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeMCPConfig(%+v): %v", tc.in, err)
			}
			if m.Path != tc.wantPath {
				t.Errorf("path = %q, want %q", m.Path, tc.wantPath)
			}
			if m.IsEnabled() != tc.wantOn {
				t.Errorf("enabled = %v, want %v", m.IsEnabled(), tc.wantOn)
			}
		})
	}
}
