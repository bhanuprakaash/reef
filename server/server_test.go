package server

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/bhanuprakaash/reef/memory"
	"github.com/bhanuprakaash/reef/store"
)

// _migratedDB is the database file of a fresh store. Seeding a test store with it skips
// the migrations, which are most of this package's test time.
var _migratedDB = sync.OnceValues(func() ([]byte, error) {
	dir, err := os.MkdirTemp("", "reef-store")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	st, err := store.Open(dir)
	if err != nil {
		return nil, err
	}
	if err := st.Close(); err != nil {
		return nil, err
	}
	return os.ReadFile(filepath.Join(dir, "reef.db"))
})

// openStore is store.Open on dir, seeded with a migrated database when dir has none.
func openStore(dir string) (*store.Store, error) {
	path := filepath.Join(dir, "reef.db")
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		db, err := _migratedDB()
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, db, 0o644); err != nil {
			return nil, err
		}
	}
	return store.Open(dir)
}

func testMemory(t *testing.T, st *store.Store) *memory.Store {
	t.Helper()
	ms, err := memory.Open(st.DB(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return ms
}

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	ts, _ := newEmbedServer(t, nil)
	return ts
}

// approveForSpace marks the backlog items ids approved, owned by bot 1 where they have no owner, so
// POST /spaces takes them without the queue starting them.
func approveForSpace(t *testing.T, st *store.Store, ids ...int64) {
	t.Helper()
	for _, id := range ids {
		for _, q := range []string{
			"UPDATE work_item SET owner_bot_id = COALESCE(owner_bot_id, 1), approved_at = '2026-01-01T00:00:00Z', " +
				"approval_key = 'by-hand-' || id WHERE id = ?",
			"INSERT OR IGNORE INTO work_item_participant (work_item_id, bot_id) " +
				"SELECT id, owner_bot_id FROM work_item WHERE id = ?",
		} {
			if _, err := st.DB().Exec(q, id); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// newEmbedServer is newTestServer with the embedder e, and the store it serves.
func newEmbedServer(t *testing.T, e memory.Embedder) (*httptest.Server, *store.Store) {
	t.Helper()
	st, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ms, err := memory.Open(st.DB(), e)
	if err != nil {
		t.Fatal(err)
	}
	h := New(st, ms, t.TempDir(), "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)
	return ts, st
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
		fmt.Sprintf(`{"id":1,"name":"reef","path":%q,"baseBranch":"","purpose":"","icon":"","colour":"","createdAt":"`, firstPath)) {
		t.Fatalf("create = %d %s", status, body)
	}

	status, body = do(t, http.MethodPut, ts.URL+"/projects/1",
		fmt.Sprintf(`{"name":"reef2","path":%q}`, secondPath))
	if status != 200 || !strings.HasPrefix(body,
		fmt.Sprintf(`{"id":1,"name":"reef2","path":%q,"baseBranch":"","purpose":"","icon":"","colour":"","createdAt":"`, secondPath)) {
		t.Fatalf("update = %d %s", status, body)
	}

	if status, body = do(t, http.MethodPut, ts.URL+"/projects/1", `{"name":"reef2","path":"/no/such/dir"}`); status != 400 {
		t.Fatalf("update with a bad path = %d %s", status, body)
	}

	status, body = do(t, http.MethodPut, ts.URL+"/projects/1",
		fmt.Sprintf(`{"name":"reef2","path":%q,"baseBranch":"dev"}`, secondPath))
	if status != 200 || !strings.HasPrefix(body,
		fmt.Sprintf(`{"id":1,"name":"reef2","path":%q,"baseBranch":"dev","purpose":"","icon":"","colour":"","createdAt":"`, secondPath)) {
		t.Fatalf("update with a base branch = %d %s", status, body)
	}

	status, body = do(t, http.MethodPut, ts.URL+"/projects/1",
		fmt.Sprintf(`{"name":"reef2","path":%q,"baseBranch":"dev","icon":"🚀","colour":"#2F72C0"}`, secondPath))
	if status != 200 || !strings.HasPrefix(body,
		fmt.Sprintf(`{"id":1,"name":"reef2","path":%q,"baseBranch":"dev","purpose":"","icon":"🚀","colour":"#2F72C0","createdAt":"`, secondPath)) {
		t.Fatalf("update with an icon = %d %s", status, body)
	}

	if status, body = do(t, http.MethodPut, ts.URL+"/projects/1",
		fmt.Sprintf(`{"name":"reef2","path":%q,"colour":"blue"}`, secondPath)); status != 400 {
		t.Fatalf("update with a bad colour = %d %s", status, body)
	}

	if status, body = do(t, http.MethodPut, ts.URL+"/projects/1",
		fmt.Sprintf(`{"name":"reef2","path":%q,"baseBranch":"a b"}`, secondPath)); status != 400 {
		t.Fatalf("update with a bad base branch = %d %s", status, body)
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
