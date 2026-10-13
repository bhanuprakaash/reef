package server

import (
	"io"
	"net/http"
	"net/http/httptest"
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
	ts := httptest.NewServer(New(st))
	t.Cleanup(ts.Close)
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
			give:       `{"name":"reef","path":"/tmp/reef"}`,
			wantStatus: 201,
			wantBody:   `"name":"reef","path":"/tmp/reef"`,
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
			give:       `{"name":"reef","path":"/tmp/reef"} {"name":"x"}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: bad JSON: unexpected data after the body"}`,
		},
		{
			name:       "missing name",
			method:     http.MethodPost,
			path:       "/projects",
			give:       `{"path":"/tmp/reef"}`,
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

	status, body := do(t, http.MethodPost, ts.URL+"/projects", `{"name":"reef","path":"/tmp/reef"}`)
	if status != 201 || !strings.HasPrefix(body, `{"id":1,"name":"reef","path":"/tmp/reef","createdAt":"`) {
		t.Fatalf("create = %d %s", status, body)
	}

	status, body = do(t, http.MethodPut, ts.URL+"/projects/1", `{"name":"reef2","path":"/tmp/reef2"}`)
	if status != 200 || !strings.HasPrefix(body, `{"id":1,"name":"reef2","path":"/tmp/reef2","createdAt":"`) {
		t.Fatalf("update = %d %s", status, body)
	}

	status, body = do(t, http.MethodGet, ts.URL+"/projects", "")
	if status != 200 || !strings.HasPrefix(body, `[{"id":1,"name":"reef2","path":"/tmp/reef2",`) {
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
