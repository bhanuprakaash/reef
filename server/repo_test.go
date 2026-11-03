package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

// writeFakeRepoCLI writes a fake gh or glab named name that answers `api --hostname HOST user`
// with userJSON. $FAKE_CLI_ERR set makes it write that to stderr,
// with a token line as the real CLIs can, and exit nonzero.
func writeFakeRepoCLI(t *testing.T, binDir, name, userJSON string) {
	t.Helper()
	script := "#!/bin/sh\n" +
		"if [ -n \"$FAKE_CLI_ERR\" ]; then echo \"$FAKE_CLI_ERR\" >&2; echo 'token: glpat-secret123' >&2; exit 1; fi\n" +
		"echo '" + userJSON + "'\n"
	if err := os.WriteFile(filepath.Join(binDir, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestRepoConnect_ShowsAccountThenReasonWhenLoginFails(t *testing.T) {
	ts := newTestServer(t)
	binDir := t.TempDir()
	writeFakeRepoCLI(t, binDir, "glab", `{"username":"ada","token":"glpat-secret123"}`)
	writeFakeRepoCLI(t, binDir, "gh", `{"login":"grace"}`)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	status, body := do(t, http.MethodPost, ts.URL+"/repo-connections", `{"provider":"gitlab"}`)
	if status != http.StatusOK {
		t.Fatalf("connect = %d %s", status, body)
	}
	got := decodeRepoConnection(t, body)
	if got.ID != 1 || got.Host != "gitlab.com" || got.Account != "ada" || got.State != "connected" {
		t.Errorf("connected = %+v, want id 1 gitlab.com ada connected", got)
	}

	t.Setenv("FAKE_CLI_ERR", "glab: 401 Unauthorized (HTTP 401)")
	status, body = do(t, http.MethodPost, ts.URL+"/repo-connections/1/check", "")
	if status != http.StatusOK {
		t.Fatalf("check = %d %s", status, body)
	}
	got = decodeRepoConnection(t, body)
	if got.State != "disconnected" || got.Reason != "login rejected: the token is expired or revoked" {
		t.Errorf("after 401 = %+v, want disconnected with the 401 reason", got)
	}

	t.Setenv("PATH", t.TempDir())
	_, body = do(t, http.MethodPost, ts.URL+"/repo-connections", `{"provider":"github"}`)
	if got := decodeRepoConnection(t, body); got.State != "disconnected" || got.Reason != "gh is not on PATH" {
		t.Errorf("no gh = %+v, want disconnected, gh is not on PATH", got)
	}

	_, list := do(t, http.MethodGet, ts.URL+"/repo-connections", "")
	if strings.Contains(list, "glpat-") || strings.Contains(list, "secret123") {
		t.Errorf("connection list leaks a token: %s", list)
	}
	if status, _ := do(t, http.MethodPost, ts.URL+"/repo-connections", `{"provider":"gitlab","host":"git.example.com"}`); status != http.StatusBadRequest {
		t.Errorf("connect to another host = %d, want 400", status)
	}
}

func TestProjectRemote_DetectsFromOriginAndBinds(t *testing.T) {
	ts := newTestServer(t)
	binDir := t.TempDir()
	writeFakeRepoCLI(t, binDir, "glab", `{"username":"ada"}`)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	repo := t.TempDir()
	gitIn(t, repo, "init", "-b", "main")
	gitIn(t, repo, "remote", "add", "origin", "https://oauth2:glpat-secret123@gitlab.com/group/sub/app.git")
	do(t, http.MethodPost, ts.URL+"/projects", `{"name":"app","path":"`+repo+`"}`)

	status, body := do(t, http.MethodGet, ts.URL+"/projects/1/remote", "")
	if status != http.StatusOK || body != `{"provider":"gitlab","host":"gitlab.com","path":"group/sub/app","connectionId":null}` {
		t.Fatalf("detect before connecting = %d %s", status, body)
	}
	do(t, http.MethodPost, ts.URL+"/repo-connections", `{"provider":"gitlab"}`)
	_, body = do(t, http.MethodGet, ts.URL+"/projects/1/remote", "")
	if !strings.HasSuffix(body, `"connectionId":1}`) {
		t.Errorf("detect after connecting = %s, want connectionId 1", body)
	}

	status, body = do(t, http.MethodPut, ts.URL+"/projects/1/remote", `{"connectionId":1,"path":"group/other.git"}`)
	var p store.Project
	if err := json.Unmarshal([]byte(body), &p); err != nil || status != http.StatusOK {
		t.Fatalf("bind = %d %s", status, body)
	}
	if p.RepoConnectionID == nil || *p.RepoConnectionID != 1 || p.RemotePath != "group/other" {
		t.Errorf("bound project = %+v, want connection 1 and group/other", p)
	}
	if strings.Contains(body, "glpat-") {
		t.Errorf("project leaks the origin credential: %s", body)
	}

	status, _ = do(t, http.MethodPut, ts.URL+"/projects/1", `{"name":"app","path":"`+repo+`","purpose":"x"}`)
	_, body = do(t, http.MethodGet, ts.URL+"/projects/1", "")
	if status != http.StatusOK || !strings.Contains(body, `"remotePath":"group/other"`) {
		t.Errorf("project after a plain save = %s, want the binding kept", body)
	}
}

func TestParseRepoRemote(t *testing.T) {
	tests := []struct {
		name    string
		give    string
		want    repoRemote
		wantErr bool
	}{
		{"https", "https://github.com/o/r.git", repoRemote{"github", "github.com", "o/r"}, false},
		{"scp", "git@gitlab.com:g/s/r.git\n", repoRemote{"gitlab", "gitlab.com", "g/s/r"}, false},
		{"ssh", "ssh://git@gitlab.com/g/r", repoRemote{"gitlab", "gitlab.com", "g/r"}, false},
		{"other host", "https://git.example.com/o/r.git", repoRemote{}, true},
		{"github nested", "https://github.com/o/s/r.git", repoRemote{}, true},
		{"no repo", "https://gitlab.com/g", repoRemote{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRepoRemote(tt.give)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("parseRepoRemote(%q) = %+v, %v; want %+v, error %v", tt.give, got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func decodeRepoConnection(t *testing.T, body string) store.RepoConnection {
	t.Helper()
	var c store.RepoConnection
	if err := json.Unmarshal([]byte(body), &c); err != nil {
		t.Fatalf("decode %q: %v", body, err)
	}
	return c
}
