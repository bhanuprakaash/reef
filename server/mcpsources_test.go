package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/space"
	"github.com/bhanuprakaash/reef/store"
)

func writeConfig(t *testing.T, path, body string) {
	t.Helper()
	writeFile(t, path, body, 0o755)
}

func TestAvailableAndImportConnections(t *testing.T) {
	home, repo, bin := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	writeConfig(t, filepath.Join(home, ".claude.json"), `{
		"mcpServers": {"notes": {"command": "notes-mcp", "args": ["--ro"], "env": {"KEY": "v"}},
			"feed": {"type": "sse", "url": "https://feed.io/sse"}},
		"projects": {`+strconv.Quote(repo)+`: {"mcpServers": {"local": {"type": "http", "url": "http://localhost:8114/mcp", "headers": {"X-Key": "k"}}}}}}`)
	writeConfig(t, filepath.Join(repo, ".mcp.json"), `{"mcpServers": {"repo.db": {"command": "db-mcp"}}}`)
	writeConfig(t, filepath.Join(home, ".claude/settings.json"), `{"enabledPlugins": {"slack@market": true, "off@market": false}}`)
	writeConfig(t, filepath.Join(home, ".claude/plugins/installed_plugins.json"), `{"plugins": {
		"slack@market": [{"scope": "user", "installPath": `+strconv.Quote(filepath.Join(home, "p/slack"))+`}],
		"off@market": [{"scope": "user", "installPath": `+strconv.Quote(filepath.Join(home, "p/off"))+`}]}}`)
	writeConfig(t, filepath.Join(home, "p/slack/.mcp.json"),
		`{"slack": {"type": "http", "url": "https://mcp.slack.com/mcp", "oauth": {"clientId": "id1", "callbackPort": 3118}}}`)
	writeConfig(t, filepath.Join(home, "p/off/.mcp.json"), `{"mcpServers": {"hidden": {"command": "h"}}}`)
	writeConfig(t, filepath.Join(bin, "codex"), "#!/bin/sh\n"+
		`echo '[{"name":"cx","enabled":true,"transport":{"type":"stdio","command":"cx-mcp","args":["a"],"env":{"T":"s"}}},`+
		`{"name":"gone","enabled":false,"transport":{"type":"stdio","command":"g"}}]'`+"\n")

	st, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	p, err := st.CreateProject(context.Background(), store.Project{Name: "shop", Path: repo})
	if err != nil {
		t.Fatal(err)
	}
	h := New(st, testMemory(t, st), t.TempDir(), "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)
	project := strconv.FormatInt(p.ID, 10)

	tests := []struct {
		name   string
		method string
		path   string
		body   string
		status int
		want   string
	}{
		{"with project", http.MethodGet, "/connections/available?projectId=" + project, "", 200,
			`[{"source":"claude user","name":"notes","kind":"command","command":"notes-mcp","args":["****"],"env":{"KEY":"****"},"added":false},` +
				`{"source":"claude project","name":"local","kind":"url","url":"http://localhost:8114/mcp","headers":{"X-Key":"****"},"added":false},` +
				`{"source":"claude project","name":"repo.db","kind":"command","command":"db-mcp","added":false},` +
				`{"source":"claude plugin slack","name":"slack","kind":"url","url":"https://mcp.slack.com/mcp","added":false},` +
				`{"source":"codex","name":"cx","kind":"command","command":"cx-mcp","args":["****"],"env":{"T":"****"},"added":false}]`},
		{"import copies the secrets", http.MethodPost, "/connections/import",
			`{"source":"claude project","name":"repo.db","projectId":` + project + `}`, 201,
			`"name":"repo-db","command":"db-mcp"`},
		{"import a plugin url", http.MethodPost, "/connections/import",
			`{"source":"claude plugin slack","name":"slack"}`, 201,
			`"name":"slack","url":"https://mcp.slack.com/mcp","oauth":{"clientId":"id1","callbackPort":3118}`},
		{"import twice", http.MethodPost, "/connections/import",
			`{"source":"claude plugin slack","name":"slack"}`, 409, `a connection named \"slack\" already exists`},
		{"unknown", http.MethodPost, "/connections/import", `{"source":"codex","name":"nope"}`, 404, `no codex server named \"nope\"`},
		{"project scope needs the project", http.MethodPost, "/connections/import",
			`{"source":"claude project","name":"local"}`, 404, `no claude project server named \"local\"`},
		{"added", http.MethodGet, "/connections/available", "", 200,
			`[{"source":"claude user","name":"notes","kind":"command","command":"notes-mcp","args":["****"],"env":{"KEY":"****"},"added":false},` +
				`{"source":"claude plugin slack","name":"slack","kind":"url","url":"https://mcp.slack.com/mcp","added":true,"connectionId":2},` +
				`{"source":"codex","name":"cx","kind":"command","command":"cx-mcp","args":["****"],"env":{"T":"****"},"added":false}]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := do(t, tt.method, ts.URL+tt.path, tt.body)
			if status != tt.status || !strings.Contains(body, tt.want) {
				t.Errorf("%s %s = %d %s, want %d containing %s", tt.method, tt.path, status, body, tt.status, tt.want)
			}
		})
	}
	slack, err := st.Connection(t.Context(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if slack.OAuth == nil || slack.OAuth.ClientID != "id1" || slack.OAuth.CallbackPort != 3118 {
		t.Errorf("slack oauth = %+v, want clientId id1 and port 3118", slack.OAuth)
	}
}

func TestConnectionHeaders(t *testing.T) {
	st, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h := New(st, testMemory(t, st), t.TempDir(), "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)

	tests := []struct {
		name   string
		method string
		path   string
		body   string
		status int
		want   string
	}{
		{"create masks", http.MethodPost, "/connections",
			`{"name":"oa","url":"http://h/mcp","headers":{"Authorization":"Bearer abc"}}`, 201, `"headers":{"Authorization":"****"}`},
		{"list masks", http.MethodGet, "/connections", "", 200, `"headers":{"Authorization":"****"}`},
		{"edit keeps masked", http.MethodPut, "/connections/1",
			`{"name":"oa","url":"http://h/mcp","headers":{"Authorization":"****","X-N":"2"}}`, 200, `"headers":{"Authorization":"****","X-N":"****"}`},
		{"mask without stored value", http.MethodPut, "/connections/1",
			`{"name":"oa","url":"http://h/mcp","headers":{"Other":"****"}}`, 400, `header Other is masked`},
		{"headers need a url", http.MethodPost, "/connections",
			`{"name":"bad","command":"c","headers":{"A":"b"}}`, 400, `headers and oauth need a url`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := do(t, tt.method, ts.URL+tt.path, tt.body)
			if status != tt.status || !strings.Contains(body, tt.want) {
				t.Errorf("%s %s = %d %s, want %d containing %s", tt.method, tt.path, status, body, tt.status, tt.want)
			}
		})
	}
	oa, err := st.Connection(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if oa.Headers["Authorization"] != "Bearer abc" || oa.Headers["X-N"] != "2" {
		t.Errorf("headers = %v, want the stored Authorization kept and X-N=2", oa.Headers)
	}
}

func TestLoginConnection(t *testing.T) {
	st, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := t.Context()
	for _, c := range []store.Connection{
		{Name: "slack", URL: "https://mcp.slack.com/mcp", OAuth: &agent.OAuth{ClientID: "id1", CallbackPort: 3118}},
		{Name: "keyed", URL: "http://h/mcp", Headers: map[string]string{"Authorization": "Bearer k"}},
		{Name: "local", Command: "db-mcp"},
		{Name: "lonely", URL: "https://x.io/mcp"},
	} {
		if _, err := st.CreateConnection(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	for _, b := range []store.Bot{
		{Name: "msg", Backend: "claude", Connections: []int64{1}},
		{Name: "rev", Backend: "codex", Connections: []int64{1}},
		{Name: "dev", Backend: "claude", Connections: []int64{1}},
	} {
		if _, err := st.CreateBot(ctx, b); err != nil {
			t.Fatal(err)
		}
	}
	var calls []string
	dir := t.TempDir()
	h := newHandler(st, nil, dir, space.NewTrees(dir), nil, func(s *server) {
		s.login = func(_ context.Context, backend, name string, srv agent.MCPServer) (string, error) {
			calls = append(calls, backend+" "+name+" "+srv.URL)
			if backend == "codex" {
				return "", errors.New("not registered")
			}
			return "Authentication successful", nil
		}
	})
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)

	tests := []struct {
		name   string
		id     string
		status int
		want   string
		calls  []string
	}{
		{"each backend of its Bots once", "1", 200,
			`[{"backend":"claude","ok":true,"output":"Authentication successful"},` +
				`{"backend":"codex","ok":false,"output":"not registered"}]`,
			[]string{"claude slack https://mcp.slack.com/mcp", "codex slack https://mcp.slack.com/mcp"}},
		{"claude when no Bot selects it", "4", 200, `[{"backend":"claude","ok":true`,
			[]string{"claude lonely https://x.io/mcp"}},
		{"headers sign in by themselves", "2", 400, "only a url connection without headers", nil},
		{"a command has no sign-in", "3", 400, "only a url connection without headers", nil},
		{"unknown connection", "9", 404, "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls = nil
			status, body := do(t, http.MethodPost, ts.URL+"/connections/"+tt.id+"/login", "")
			if status != tt.status || !strings.Contains(body, tt.want) {
				t.Errorf("login %s = %d %s, want %d containing %s", tt.id, status, body, tt.status, tt.want)
			}
			if !slices.Equal(calls, tt.calls) {
				t.Errorf("logins = %v, want %v", calls, tt.calls)
			}
		})
	}
}
