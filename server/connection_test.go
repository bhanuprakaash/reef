package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConnectionsAndBotSelection(t *testing.T) {
	st, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h := New(st, testMemory(t, st), t.TempDir(), "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)

	steps := []struct {
		name   string
		method string
		path   string
		body   string
		status int
		want   string
	}{
		{"empty list", http.MethodGet, "/connections", "", 200, `[]`},
		{"create stdio", http.MethodPost, "/connections",
			`{"name":"files","command":"files-mcp","args":["--ro"],"env":{"KEY":"v"}}`, 201, `"name":"files","command":"files-mcp","args":["--ro"],"env":{"KEY":"****"}`},
		{"create http", http.MethodPost, "/connections", `{"name":"web","url":"http://127.0.0.1:9/mcp"}`, 201, `"url":"http://127.0.0.1:9/mcp"`},
		{"both fields", http.MethodPost, "/connections", `{"name":"x","command":"c","url":"http://h"}`, 400, `needs a command or a url, not both`},
		{"name taken", http.MethodPost, "/connections", `{"name":"web","command":"c"}`, 409, `a connection named \"web\" already exists`},
		{"select on a bot", http.MethodPost, "/bots",
			`{"name":"sam","backend":"claude","connections":[1]}`, 201, `"connections":[1]`},
		{"unknown selection", http.MethodPost, "/bots",
			`{"name":"kim","backend":"claude","connections":[9]}`, 400, `unknown connection 9`},
		{"create secret url", http.MethodPost, "/connections", `{"name":"sec","url":"https://u:p@h.io/mcp?token=abc&x=1"}`, 201, `"url":"https://****@h.io/mcp?token=****\u0026x=****"`},
		{"list masks", http.MethodGet, "/connections", "", 200, `"env":{"KEY":"****"}`},
		{"edit keeps masked env", http.MethodPut, "/connections/1", `{"name":"files","command":"files-mcp","env":{"KEY":"****","N":"2"}}`, 200, `"env":{"KEY":"****","N":"****"}`},
		{"edit keeps masked url", http.MethodPut, "/connections/3", `{"name":"sec","url":"https://****@h.io/mcp?token=****\u0026x=****"}`, 200, `token=****\u0026x=****`},
		{"mask without stored value", http.MethodPut, "/connections/1", `{"name":"files","command":"files-mcp","env":{"OTHER":"****"}}`, 400, `env OTHER is masked`},
		{"edit", http.MethodPut, "/connections/2", `{"name":"web","url":"http://127.0.0.1:9/v2"}`, 200, `"url":"http://127.0.0.1:9/v2"`},
		{"delete", http.MethodDelete, "/connections/1", "", 204, ``},
		{"bot lost it", http.MethodGet, "/bots/1", "", 200, `"connections":[]`},
		{"delete missing", http.MethodDelete, "/connections/1", "", 404, `not found`},
	}
	for _, tt := range steps {
		t.Run(tt.name, func(t *testing.T) {
			status, body := do(t, tt.method, ts.URL+tt.path, tt.body)
			if status != tt.status || !strings.Contains(body, tt.want) {
				t.Errorf("%s %s = %d %s, want %d containing %s", tt.method, tt.path, status, body, tt.status, tt.want)
			}
		})
	}
}

func TestMaskedEditKeepsStoredSecrets(t *testing.T) {
	st, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h := New(st, testMemory(t, st), t.TempDir(), "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)

	do(t, http.MethodPost, ts.URL+"/connections", `{"name":"files","command":"f","env":{"KEY":"v"}}`)
	do(t, http.MethodPost, ts.URL+"/connections", `{"name":"web","url":"https://u:p@h.io/mcp?token=abc"}`)
	do(t, http.MethodPut, ts.URL+"/connections/1", `{"name":"files","command":"g","env":{"KEY":"****"}}`)
	do(t, http.MethodPut, ts.URL+"/connections/1", `{"name":"files","command":"h"}`)
	do(t, http.MethodPut, ts.URL+"/connections/2", `{"name":"web","url":"https://****@h.io/mcp?token=****"}`)
	files, err := st.Connection(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if files.Command != "h" || files.Env["KEY"] != "v" {
		t.Errorf("files = %+v, want command h and env KEY=v kept", files)
	}
	web, err := st.Connection(t.Context(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if web.URL != "https://u:p@h.io/mcp?token=abc" {
		t.Errorf("web url = %q, want the stored one", web.URL)
	}
}
