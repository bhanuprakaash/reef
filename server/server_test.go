package server

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h := New(st, t.TempDir(), "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)
	return ts
}

func do(t *testing.T, method, url, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, strings.TrimSpace(string(b))
}

func TestProjectStatus(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		give       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "create",
			method:     http.MethodPost,
			path:       "/projects",
			give:       `{"name":"reef","path":"/"}`,
			wantStatus: 201,
			wantBody:   `"name":"reef","path":"/"`,
		},
		{
			name:       "path is not a directory",
			method:     http.MethodPost,
			path:       "/projects",
			give:       `{"name":"reef","path":"/no/such/dir"}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: project path \"/no/such/dir\": not a directory"}`,
		},
		{
			name:       "path is relative",
			method:     http.MethodPost,
			path:       "/projects",
			give:       `{"name":"reef","path":"repo"}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: project path \"repo\": not an absolute path"}`,
		},
		{
			name:       "unknown id",
			method:     http.MethodGet,
			path:       "/projects/999",
			wantStatus: 404,
			wantBody:   `{"error":"project 999: not found"}`,
		},
		{
			name:       "bad json",
			method:     http.MethodPost,
			path:       "/projects",
			give:       `{"name":`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: bad JSON: unexpected EOF"}`,
		},
		{
			name:       "data after the body",
			method:     http.MethodPost,
			path:       "/projects",
			give:       `{"name":"reef","path":"/tmp"} {"name":"x"}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: bad JSON: unexpected data after the body"}`,
		},
		{
			name:       "missing name",
			method:     http.MethodPost,
			path:       "/projects",
			give:       `{"path":"/tmp"}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: project name is required"}`,
		},
		{
			name:       "unknown path",
			method:     http.MethodGet,
			path:       "/nope",
			wantStatus: 404,
			wantBody:   `{"error":"not found"}`,
		},
		{
			name:       "wrong method",
			method:     http.MethodPatch,
			path:       "/projects",
			wantStatus: 405,
			wantBody:   `{"error":"method not allowed"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := newTestServer(t)
			status, body := do(t, tt.method, ts.URL+tt.path, tt.give)
			if status != tt.wantStatus {
				t.Errorf("status = %d, want %d", status, tt.wantStatus)
			}
			if !strings.Contains(body, tt.wantBody) {
				t.Errorf("body = %s, want it to contain %s", body, tt.wantBody)
			}
		})
	}
}

func TestProjectRoundTrip(t *testing.T) {
	ts := newTestServer(t)
	firstPath, secondPath := t.TempDir(), t.TempDir()
	firstPath, err := store.ResolvePath(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	secondPath, err = store.ResolvePath(secondPath)
	if err != nil {
		t.Fatal(err)
	}

	status, body := do(t, http.MethodPost, ts.URL+"/projects",
		fmt.Sprintf(`{"name":"reef","path":%q}`, firstPath))
	if status != 201 || !strings.HasPrefix(body,
		fmt.Sprintf(`{"id":1,"name":"reef","path":%q,"createdAt":"`, firstPath)) {
		t.Fatalf("create = %d %s", status, body)
	}

	status, body = do(t, http.MethodPut, ts.URL+"/projects/1",
		fmt.Sprintf(`{"name":"reef2","path":%q}`, secondPath))
	if status != 200 || !strings.HasPrefix(body,
		fmt.Sprintf(`{"id":1,"name":"reef2","path":%q,"createdAt":"`, secondPath)) {
		t.Fatalf("update = %d %s", status, body)
	}

	if status, body = do(t, http.MethodPut, ts.URL+"/projects/1", `{"name":"reef2","path":"/no/such/dir"}`); status != 400 {
		t.Fatalf("update with a bad path = %d %s", status, body)
	}

	status, body = do(t, http.MethodGet, ts.URL+"/projects", "")
	if status != 200 || !strings.HasPrefix(body,
		fmt.Sprintf(`[{"id":1,"name":"reef2","path":%q,`, secondPath)) {
		t.Fatalf("list = %d %s", status, body)
	}

	if status, body = do(t, http.MethodDelete, ts.URL+"/projects/1", ""); status != 204 || body != "" {
		t.Fatalf("delete = %d %s", status, body)
	}

	status, body = do(t, http.MethodGet, ts.URL+"/projects", "")
	if status != 200 || body != "[]" {
		t.Fatalf("list after delete = %d %s", status, body)
	}
}

func TestProjectPathsResolveSymlinks(t *testing.T) {
	ts := newTestServer(t)
	target := t.TempDir()
	target, err := store.ResolvePath(target)
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "project")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	status, body := do(t, http.MethodPost, ts.URL+"/projects",
		fmt.Sprintf(`{"name":"reef","path":%q}`, link))
	if status != http.StatusCreated || !strings.Contains(body, fmt.Sprintf(`"path":%q`, target)) {
		t.Fatalf("create with symlink = %d %s, want path %q", status, body, target)
	}

	nextTarget := t.TempDir()
	nextTarget, err = store.ResolvePath(nextTarget)
	if err != nil {
		t.Fatal(err)
	}
	nextLink := filepath.Join(t.TempDir(), "project")
	if err := os.Symlink(nextTarget, nextLink); err != nil {
		t.Fatal(err)
	}
	status, body = do(t, http.MethodPut, ts.URL+"/projects/1",
		fmt.Sprintf(`{"name":"reef","path":%q}`, nextLink))
	if status != http.StatusOK || !strings.Contains(body, fmt.Sprintf(`"path":%q`, nextTarget)) {
		t.Fatalf("update with symlink = %d %s, want path %q", status, body, nextTarget)
	}
}
