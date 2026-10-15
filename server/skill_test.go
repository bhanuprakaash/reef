package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

func writeSkill(t *testing.T, path, name, description string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: " + name + "\ndescription: " + description + "\n---\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGetSkills(t *testing.T) {
	home, repo := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	writeSkill(t, filepath.Join(home, ".claude/skills/how/SKILL.md"), "how", "Explain flow.")
	writeSkill(t, filepath.Join(repo, ".claude/skills/repo-skill/SKILL.md"), "repo-skill", "In the repo.")
	writeSkill(t, filepath.Join(home, ".agents/skills/probe/SKILL.md"), "probe", "For codex.")

	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	p, err := st.CreateProject(context.Background(), store.Project{Name: "shop", Path: repo})
	if err != nil {
		t.Fatal(err)
	}
	h := New(st, t.TempDir(), "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)

	tests := []struct {
		name  string
		query string
		code  int
		want  string
	}{
		{"a project", "?project=" + strconv.FormatInt(p.ID, 10), http.StatusOK,
			`{"claude":[{"name":"how","description":"Explain flow."},` +
				`{"name":"repo-skill","description":"In the repo."}],` +
				`"codex":[{"name":"probe","description":"For codex."}]}`},
		{"no project", "", http.StatusBadRequest, ""},
		{"not a number", "?project=x", http.StatusBadRequest, ""},
		{"unknown project", "?project=999", http.StatusNotFound, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, body := do(t, http.MethodGet, ts.URL+"/skills"+tt.query, "")
			if code != tt.code || (tt.want != "" && body != tt.want) {
				t.Errorf("GET /skills%s = %d %s, want %d %s", tt.query, code, body, tt.code, tt.want)
			}
		})
	}
	if code, _ := do(t, http.MethodPost, ts.URL+"/skills", `{}`); code != http.StatusMethodNotAllowed {
		t.Errorf("POST /skills = %d, want 405", code)
	}
}
