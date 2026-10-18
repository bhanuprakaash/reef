package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/bhanuprakaash/reef/store"
	"github.com/bhanuprakaash/reef/worktree"
)

func TestGetChannelFile(t *testing.T) {
	ts := newTestServer(t)
	project, err := store.ResolvePath(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	writeIn(t, outside, "secret.txt", "secret")
	writeIn(t, project, "a.txt", "hello")
	if err := os.Mkdir(filepath.Join(project, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeIn(t, project, "sub/b.go", "package b")
	writeIn(t, project, "bin.dat", "a\x00b")
	if err := os.Symlink(filepath.Join(project, "a.txt"), filepath.Join(project, "in")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(project, "out")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../"+filepath.Base(outside)+"/secret.txt", filepath.Join(project, "relout")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "gone.txt"), filepath.Join(project, "dangling")); err != nil {
		t.Fatal(err)
	}
	writeIn(t, project, "empty.txt", "")
	do(t, http.MethodPost, ts.URL+"/projects", fmt.Sprintf(`{"name":"app","path":%q}`, project))
	do(t, http.MethodPost, ts.URL+"/channels", `{"projectId":1,"name":"s"}`)

	const outsideErr = `{"error":"invalid: path is outside the channel's folder"}`
	tests := []struct {
		name       string
		give       string
		wantStatus int
		wantBody   string
	}{
		{"file", "a.txt", 200, `{"content":"hello","path":"a.txt","size":5,"truncated":false}`},
		{"nested", "sub/b.go", 200, `{"content":"package b","path":"sub/b.go","size":9,"truncated":false}`},
		{"absolute inside", filepath.Join(project, "sub/b.go"), 200,
			`{"content":"package b","path":"sub/b.go","size":9,"truncated":false}`},
		{"symlink inside", "in", 200, `{"content":"hello","path":"in","size":5,"truncated":false}`},
		{"dot dot escape", "../a.txt", 400, outsideErr},
		{"dot dot to outside", "sub/../../" + filepath.Base(outside) + "/secret.txt", 400, outsideErr},
		{"absolute outside", filepath.Join(outside, "secret.txt"), 400, outsideErr},
		{"symlink escape", "out", 400, outsideErr},
		{"relative symlink escape", "relout", 400, outsideErr},
		{"symlink to missing outside", "dangling", 400, outsideErr},
		{"directory", "sub", 400, `{"error":"invalid: path is not a regular file"}`},
		{"binary", "bin.dat", 415, `{"error":"binary file"}`},
		{"missing", "nope.txt", 404, `{"error":"file: not found"}`},
		{"empty file", "empty.txt", 200, `{"content":"","path":"empty.txt","size":0,"truncated":false}`},
		{"empty", "", 400, `{"error":"invalid: path is required"}`},
		{"nul", "a.txt\x00", 400, `{"error":"invalid: path has a NUL byte"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := do(t, http.MethodGet,
				ts.URL+"/channels/1/file?path="+url.QueryEscape(tt.give), "")
			if status != tt.wantStatus || body != tt.wantBody {
				t.Errorf("GET file %q = %d %s, want %d %s", tt.give, status, body, tt.wantStatus, tt.wantBody)
			}
		})
	}
}

func TestGetChannelFile_UnknownChannel(t *testing.T) {
	ts := newTestServer(t)
	status, body := do(t, http.MethodGet, ts.URL+"/channels/9/file?path=a.txt", "")
	if want := `{"error":"channel 9: not found"}`; status != 404 || body != want {
		t.Errorf("GET file of unknown channel = %d %s, want 404 %s", status, body, want)
	}
}

func TestGetChannelFile_MethodNotAllowed(t *testing.T) {
	ts := newTestServer(t)
	status, body := do(t, http.MethodPost, ts.URL+"/channels/1/file?path=a.txt", "")
	if want := `{"error":"method not allowed"}`; status != 405 || body != want {
		t.Errorf("POST file = %d %s, want 405 %s", status, body, want)
	}
}

func TestGetChannelFile_TruncatesOnRuneBoundary(t *testing.T) {
	ts := newTestServer(t)
	project := t.TempDir()
	writeIn(t, project, "big.txt", strings.Repeat("a", _fileCap-1)+"é")
	do(t, http.MethodPost, ts.URL+"/projects", fmt.Sprintf(`{"name":"app","path":%q}`, project))
	do(t, http.MethodPost, ts.URL+"/channels", `{"projectId":1,"name":"s"}`)

	status, body := do(t, http.MethodGet, ts.URL+"/channels/1/file?path=big.txt", "")
	var got struct {
		Path      string `json:"path"`
		Content   string `json:"content"`
		Size      int64  `json:"size"`
		Truncated bool   `json:"truncated"`
	}
	if err := json.Unmarshal([]byte(body), &got); status != 200 || err != nil {
		t.Fatalf("GET big file = %d %.200s, %v", status, body, err)
	}
	if got.Path != "big.txt" || got.Size != 524289 || !got.Truncated ||
		got.Content != strings.Repeat("a", 524287) {
		t.Errorf("GET big file = path %q size %d truncated %v content length %d, "+
			"want big.txt 524289 true 524287", got.Path, got.Size, got.Truncated, len(got.Content))
	}
}

func TestGetChannelFile_Cap(t *testing.T) {
	tests := []struct {
		name          string
		give          int
		wantTruncated bool
		wantLen       int
	}{
		{"at the cap", _fileCap, false, 524288},
		{"one over the cap", _fileCap + 1, true, 524288},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := newTestServer(t)
			project := t.TempDir()
			writeIn(t, project, "big.txt", strings.Repeat("a", tt.give))
			do(t, http.MethodPost, ts.URL+"/projects", fmt.Sprintf(`{"name":"app","path":%q}`, project))
			do(t, http.MethodPost, ts.URL+"/channels", `{"projectId":1,"name":"s"}`)

			status, body := do(t, http.MethodGet, ts.URL+"/channels/1/file?path=big.txt", "")
			var got struct {
				Content   string `json:"content"`
				Size      int    `json:"size"`
				Truncated bool   `json:"truncated"`
			}
			if err := json.Unmarshal([]byte(body), &got); status != 200 || err != nil {
				t.Fatalf("GET big file = %d %.200s, %v", status, body, err)
			}
			if got.Size != tt.give || got.Truncated != tt.wantTruncated || len(got.Content) != tt.wantLen {
				t.Errorf("GET big file = size %d truncated %v content length %d, want %d %v %d",
					got.Size, got.Truncated, len(got.Content), tt.give, tt.wantTruncated, tt.wantLen)
			}
		})
	}
}

func TestGetChannelFile_FifoDoesNotBlock(t *testing.T) {
	ts := newTestServer(t)
	project := t.TempDir()
	fifo := filepath.Join(project, "fifo")
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		t.Fatal(err)
	}
	do(t, http.MethodPost, ts.URL+"/projects", fmt.Sprintf(`{"name":"app","path":%q}`, project))
	do(t, http.MethodPost, ts.URL+"/channels", `{"projectId":1,"name":"s"}`)

	client := &http.Client{Timeout: 2 * time.Second}
	res, err := client.Get(ts.URL + "/channels/1/file?path=fifo")
	if err != nil {
		// Open the fifo's other end so the blocked handler returns and the server can close.
		if w, err := os.OpenFile(fifo, os.O_WRONLY, 0); err == nil {
			_ = w.Close()
		}
		t.Fatalf("GET fifo: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	body := strings.TrimSpace(string(b))
	if want := `{"error":"invalid: path is not a regular file"}`; res.StatusCode != 400 || body != want {
		t.Errorf("GET fifo = %d %s, want 400 %s", res.StatusCode, body, want)
	}
}

func TestGetChannelFile_FeatureWorktreeRoot(t *testing.T) {
	ts := newTestServer(t)
	repo := t.TempDir()
	gitIn(t, repo, "init", "-b", "main")
	writeIn(t, repo, "a.txt", "project")
	gitIn(t, repo, "add", ".")
	gitIn(t, repo, "commit", "-m", "first")
	do(t, http.MethodPost, ts.URL+"/projects", fmt.Sprintf(`{"name":"app","path":%q}`, repo))
	do(t, http.MethodPost, ts.URL+"/features", `{"projectId":1,"slug":"demo","name":"Demo"}`)
	do(t, http.MethodPost, ts.URL+"/channels", `{"projectId":1,"featureId":1,"name":"f"}`)

	status, body := do(t, http.MethodGet, ts.URL+"/channels/1/file?path=a.txt", "")
	if want := `{"content":"project","path":"a.txt","size":7,"truncated":false}`; status != 200 || body != want {
		t.Fatalf("GET file before the worktree = %d %s, want 200 %s", status, body, want)
	}

	_, body = do(t, http.MethodPost, ts.URL+"/features/1/worktree", "")
	var tree struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(body), &tree); err != nil {
		t.Fatal(err)
	}
	writeIn(t, tree.Path, "a.txt", "worktree")

	status, body = do(t, http.MethodGet, ts.URL+"/channels/1/file?path=a.txt", "")
	if want := `{"content":"worktree","path":"a.txt","size":8,"truncated":false}`; status != 200 || body != want {
		t.Errorf("GET file in the worktree = %d %s, want 200 %s", status, body, want)
	}
	status, body = do(t, http.MethodGet,
		ts.URL+"/channels/1/file?path="+url.QueryEscape(filepath.Join(repo, "a.txt")), "")
	if want := `{"error":"invalid: path is outside the channel's folder"}`; status != 400 || body != want {
		t.Errorf("GET project file from the worktree = %d %s, want 400 %s", status, body, want)
	}
}

func TestGetChannelFile_SymlinkedRoot(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	data := filepath.Join(t.TempDir(), "data")
	if err := os.Symlink(t.TempDir(), data); err != nil {
		t.Fatal(err)
	}
	h := New(st, data, "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)
	repo := t.TempDir()
	gitIn(t, repo, "init", "-b", "main")
	writeIn(t, repo, "a.txt", "project")
	gitIn(t, repo, "add", ".")
	gitIn(t, repo, "commit", "-m", "first")
	do(t, http.MethodPost, ts.URL+"/projects", fmt.Sprintf(`{"name":"app","path":%q}`, repo))
	do(t, http.MethodPost, ts.URL+"/features", `{"projectId":1,"slug":"demo","name":"Demo"}`)
	do(t, http.MethodPost, ts.URL+"/channels", `{"projectId":1,"featureId":1,"name":"f"}`)
	do(t, http.MethodPost, ts.URL+"/features/1/worktree", "")
	tree := worktree.Path(data, 1, "demo")
	resolved, err := filepath.EvalSymlinks(tree)
	if err != nil {
		t.Fatal(err)
	}

	for _, give := range []string{filepath.Join(tree, "a.txt"), filepath.Join(resolved, "a.txt")} {
		status, body := do(t, http.MethodGet, ts.URL+"/channels/1/file?path="+url.QueryEscape(give), "")
		if want := `{"content":"project","path":"a.txt","size":7,"truncated":false}`; status != 200 || body != want {
			t.Errorf("GET file %s = %d %s, want 200 %s", give, status, body, want)
		}
	}
}
