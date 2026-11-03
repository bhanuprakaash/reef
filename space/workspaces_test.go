package space

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

func gitRepoIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)
	if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func newGitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	gitRepoIn(t, repo, "init", "-b", "main")
	gitRepoIn(t, repo, "commit", "--allow-empty", "-m", "first")
	return repo
}

func TestEnsureWorkspaces_ProposedTargetGetsNoWorkspaceAndIsNotListed(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	infra, err := st.CreateProject(ctx, store.Project{Name: "infra", Path: newGitRepo(t)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddWorkItemTarget(ctx, f.ID, infra.ID, "proposed by Bob"); err != nil {
		t.Fatal(err)
	}
	f, err = st.WorkItem(ctx, f.ID)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()

	ws, err := EnsureWorkspaces(ctx, st, NewTrees(t.TempDir()), dir, f)

	if err != nil || len(ws) != 1 || ws[0].ProjectID != f.ProjectID {
		t.Fatalf("workspaces = %+v, %v, want only the home project's", ws, err)
	}
	if _, err := os.Stat(filepath.Join(WorkRoot(dir, "1"), "2-infra")); !os.IsNotExist(err) {
		t.Errorf("folder of the proposed target: %v, want none", err)
	}
	var b strings.Builder
	if err := targetLines(ctx, st, &b, f); err != nil {
		t.Fatal(err)
	}
	if got := b.String(); strings.Contains(got, "infra") ||
		!strings.Contains(got, "Your folder "+WorkRoot(dir, "1")+" is not a repository") {
		t.Errorf("target lines = %q, want the home project only and the note that the root is no repository", got)
	}
}

func TestEnsureWorkspaces_DeletedTargetGetsNoWorkspaceAndIsNotWritable(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	infra, err := st.CreateProject(ctx, store.Project{Name: "infra", Path: newGitRepo(t)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddWorkItemTarget(ctx, f.ID, infra.ID, store.ByUser); err != nil {
		t.Fatal(err)
	}
	f, err = st.WorkItem(ctx, f.ID)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if ws, err := EnsureWorkspaces(ctx, st, NewTrees(t.TempDir()), dir, f); err != nil || len(ws) != 2 {
		t.Fatalf("workspaces = %+v, %v, want both", ws, err)
	}

	if _, err := st.MarkProjectDeleted(ctx, infra.ID); err != nil {
		t.Fatal(err)
	}

	ready, err := readyWorkspaces(ctx, st, "1")
	if err != nil || len(ready) != 1 || ready[0].ProjectID != f.ProjectID {
		t.Errorf("writable workspaces = %+v, %v, want only the home project's", ready, err)
	}
	if _, err := os.Stat(filepath.Join(WorkRoot(dir, "1"), "2-infra")); err != nil {
		t.Errorf("folder of the deleted target: %v, want kept", err)
	}
}

func TestEnsureWorkspaces_OneFailingTargetNamesItselfAndDoesNotStopTheOther(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	infraRepo := newGitRepo(t)
	infra, err := st.CreateProject(ctx, store.Project{Name: "infra", Path: infraRepo})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddWorkItemTarget(ctx, f.ID, infra.ID, "user"); err != nil {
		t.Fatal(err)
	}
	f, err = st.WorkItem(ctx, f.ID)
	if err != nil {
		t.Fatal(err)
	}
	gitRepoIn(t, infraRepo, "worktree", "add", "-b", "work-item/1", filepath.Join(t.TempDir(), "other"))

	ws, err := EnsureWorkspaces(ctx, st, NewTrees(t.TempDir()), t.TempDir(), f)

	if err == nil || len(ws) != 2 || ws[0].State != store.WorkspaceReady || ws[1].State != store.WorkspaceCreating {
		t.Fatalf("workspaces = %+v, %v, want shop ready and infra still creating, with an error", ws, err)
	}
	if err := requireReadyWorkspace(ctx, st, ws, err); err != nil {
		t.Errorf("requireReadyWorkspace with shop ready = %v, want nil", err)
	}
	ws[0].State, ws[0].Note = store.WorkspacePaused, "no base"
	err = requireReadyWorkspace(ctx, st, ws, nil)
	if err == nil || !strings.HasPrefix(err.Error(), "no repository of this Work item is ready: shop: no base; "+
		"infra: branch work-item/1 is checked out at") {
		t.Errorf("requireReadyWorkspace with none ready = %v, want each repository's own note", err)
	}
}

func TestEnsureWorkspaces_CutCreationKeepsTheLocalBaseAndItsNote(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	p, err := st.Project(ctx, f.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	gitRepoIn(t, p.Path, "remote", "add", "origin", filepath.Join(t.TempDir(), "gone"))
	p.BaseBranch = "main"
	if _, err := st.UpdateProject(ctx, p); err != nil {
		t.Fatal(err)
	}
	trees := NewTrees(t.TempDir())
	dir := t.TempDir()
	ws, err := EnsureWorkspaces(ctx, st, trees, dir, f)
	if err != nil || len(ws) != 1 || ws[0].BaseSource != "local" || ws[0].Note == "" {
		t.Fatalf("first creation = %+v, %v, want ready from local main with a note", ws, err)
	}
	cut := ws[0]
	cut.State, cut.BaseRef, cut.BaseSource, cut.Note = store.WorkspaceCreating, "", "", ""
	if err := st.SetWorkspace(ctx, cut); err != nil {
		t.Fatal(err)
	}

	ws, err = EnsureWorkspaces(ctx, st, trees, dir, f)

	if err != nil || len(ws) != 1 || ws[0].State != store.WorkspaceReady || ws[0].BaseSource != "local" ||
		ws[0].BaseRef != "refs/heads/main" || !strings.Contains(ws[0].Note, "origin could not be refreshed") {
		t.Errorf("after the retry = %+v, %v, want ready with the local base and the note again", ws, err)
	}
}
