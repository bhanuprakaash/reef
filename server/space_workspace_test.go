package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/bhanuprakaash/reef/space"
	"github.com/bhanuprakaash/reef/store"
)

func TestSpaceWorkspaces_ListAndAbandonKeepTheBranch(t *testing.T) {
	ctx := context.Background()
	ts, st := newEmbedServer(t, nil)
	repo, _ := originRepo(t, "develop")
	p, err := st.CreateProject(ctx, store.Project{Name: "App", Path: repo})
	if err != nil {
		t.Fatal(err)
	}
	p.BaseBranch = "develop"
	if _, err := st.UpdateProject(ctx, p); err != nil {
		t.Fatal(err)
	}
	sp, err := st.CreateBotSpace(ctx, store.Space{Name: "chat"}, *testServerBot(t, st, "architect"))
	if err != nil {
		t.Fatal(err)
	}
	w, err := space.EnsureFixWorkspace(ctx, st, space.NewTrees(t.TempDir()), t.TempDir(), sp.ID, "login", p)
	if err != nil {
		t.Fatal(err)
	}
	writeIn(t, w.Dir, "edit.txt", "x")

	url := fmt.Sprintf("%s/spaces/%d/workspaces", ts.URL, sp.ID)
	list := func() []spaceWorkspace {
		t.Helper()
		status, body := do(t, http.MethodGet, url, "")
		var out []spaceWorkspace
		if status != 200 || json.Unmarshal([]byte(body), &out) != nil || len(out) != 1 {
			t.Fatalf("GET workspaces = %d %s, want one fix", status, body)
		}
		return out
	}
	got := list()[0]
	if got.ProjectName != "App" || got.Branch != fmt.Sprintf("fix/%d-login", sp.ID) || !got.Dirty || got.Head == "" || got.State != "ready" {
		t.Errorf("fix = %+v, want App, its branch, dirty, a head commit, ready", got)
	}

	abandon := fmt.Sprintf("%s/%d/abandon", url, w.ID)
	gitIn(t, w.Dir, "add", ".")
	gitIn(t, w.Dir, "commit", "-m", "fix")
	writeIn(t, w.Dir, "late.txt", "y")
	if status, body := do(t, http.MethodPost, abandon, ""); status != 204 {
		t.Fatalf("abandon = %d %s, want 204", status, body)
	}
	if _, err := os.Stat(w.Dir); !os.IsNotExist(err) {
		t.Errorf("fix folder still there: %v", err)
	}
	if got := gitIn(t, repo, "log", "-1", "--format=%s", w.Branch); got != "fix" {
		t.Errorf("branch %s tip = %q, want the committed fix kept", w.Branch, got)
	}
	got = list()[0]
	if got.State != "removed" || got.SnapshotRef == "" {
		t.Errorf("fix = %s %q, want removed with its snapshot ref", got.State, got.SnapshotRef)
	}
	if out := gitIn(t, repo, "show", got.SnapshotRef+":late.txt"); out != "y" {
		t.Errorf("snapshot late.txt = %q, want the uncommitted file kept", out)
	}
}
