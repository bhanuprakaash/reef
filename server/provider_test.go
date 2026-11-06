package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

func TestProviders(t *testing.T) {
	st, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	bin := t.TempDir()
	for _, kind := range []string{"claude", "codex"} {
		script := "#!/bin/sh\necho \"" + kind + " 9.9.9 (detected)\"\n"
		if err := os.WriteFile(filepath.Join(bin, kind), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(other, "claude"), []byte("#!/bin/sh\necho 1.2.3\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	h := New(st, testMemory(t, st), t.TempDir(), "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)

	list := func() []providerView {
		t.Helper()
		status, body := do(t, http.MethodGet, ts.URL+"/providers", "")
		var got []providerView
		if err := json.Unmarshal([]byte(body), &got); status != 200 || err != nil {
			t.Fatalf("GET /providers = %d %s, %v", status, body, err)
		}
		return got
	}
	got := list()
	if len(got) != 2 || got[0] != (providerView{Name: "claude", Kind: "claude", Detected: true,
		Path: filepath.Join(bin, "claude"), Version: "claude 9.9.9 (detected)"}) ||
		got[1].Name != "codex" || got[1].Version != "codex 9.9.9 (detected)" {
		t.Fatalf("GET /providers = %+v, want the detected claude and codex with path and version", got)
	}

	body := `{"name":"work","kind":"claude","folder":"` + other + `"}`
	if status, out := do(t, http.MethodPost, ts.URL+"/providers", body); status != 201 ||
		!strings.Contains(out, `"version":"1.2.3"`) || !strings.Contains(out, `"detected":false`) {
		t.Errorf("POST /providers = %d %s, want 201 with version 1.2.3", status, out)
	}
	if got := list(); len(got) != 3 || got[2].Name != "work" || got[2].Folder != other ||
		got[2].Path != filepath.Join(other, "claude") {
		t.Errorf("GET /providers after add = %+v, want work last", got)
	}

	tests := []struct {
		name string
		body string
		want int
		msg  string
	}{
		{"bad kind", `{"name":"a","kind":"gemini","folder":"` + other + `"}`, 400, "kind must be claude or codex"},
		{"missing folder", `{"name":"a","kind":"claude","folder":"/nonexistent"}`, 400, "not a directory"},
		{"no binary", `{"name":"a","kind":"codex","folder":"` + other + `"}`, 400, "no codex binary in " + other},
		{"duplicate", body, 409, "already exists"},
		{"default name", `{"name":"codex","kind":"claude","folder":"` + other + `"}`, 409, "taken by the one found on PATH"},
		{"unknown field", `{"name":"a","kind":"claude","folder":"` + other + `","path":"x"}`, 400, "bad JSON"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if status, out := do(t, http.MethodPost, ts.URL+"/providers", tt.body); status != tt.want ||
				!strings.Contains(out, tt.msg) {
				t.Errorf("POST /providers = %d %s, want %d containing %q", status, out, tt.want, tt.msg)
			}
		})
	}

	lead := store.Role{Name: "t-lead", Lead: true, Backend: "claude", Provider: "work"}
	team, err := testServerTeam(t, st, context.Background(), store.Team{Name: "t", Roles: []store.Role{lead}})
	if err != nil {
		t.Fatal(err)
	}
	if status, out := do(t, http.MethodDelete, ts.URL+"/providers/work", ""); status != 409 ||
		!strings.Contains(out, `provider \"work\" is used by bot t-lead`) {
		t.Errorf("DELETE /providers/work while a role names it = %d %s, want 409 naming bot t-lead", status, out)
	}
	if _, err := st.UpdateBot(context.Background(), store.Bot{
		ID: team.Roles[0].BotID, Name: "t-lead", Backend: "claude"}); err != nil {
		t.Fatal(err)
	}

	for path, want := range map[string]int{"/providers/claude": 400, "/providers/nope": 404, "/providers/work": 204} {
		if status, out := do(t, http.MethodDelete, ts.URL+path, ""); status != want {
			t.Errorf("DELETE %s = %d %s, want %d", path, status, out, want)
		}
	}
	if got := list(); len(got) != 2 {
		t.Errorf("GET /providers after delete = %+v, want only the detected two", got)
	}
}
