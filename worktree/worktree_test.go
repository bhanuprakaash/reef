package worktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)
	out, err := exec.Command("git", full...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func newRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	run(t, repo, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, repo, "add", ".")
	run(t, repo, "commit", "-m", "first")
	return repo
}

func TestPath(t *testing.T) {
	if got, want := Path("/data", 7, "login"), "/data/worktrees/7/login"; got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
}

func TestEnsure_NewBranch(t *testing.T) {
	repo := newRepo(t)
	dir := Path(t.TempDir(), 1, "login")

	if err := Ensure(context.Background(), repo, dir, "feature/login"); err != nil {
		t.Fatal(err)
	}
	if got := run(t, dir, "branch", "--show-current"); got != "feature/login" {
		t.Errorf("branch = %q, want feature/login", got)
	}
	if got, want := run(t, dir, "rev-parse", "HEAD"), run(t, repo, "rev-parse", "HEAD"); got != want {
		t.Errorf("HEAD = %q, want the repo's %q", got, want)
	}
}

func TestEnsure_ReusesWorktree(t *testing.T) {
	repo := newRepo(t)
	dir := Path(t.TempDir(), 1, "login")
	for range 2 {
		if err := Ensure(context.Background(), repo, dir, "feature/login"); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(strings.Split(run(t, repo, "worktree", "list"), "\n")); got != 2 {
		t.Errorf("worktree list has %d lines, want 2: the checkout and one worktree", got)
	}
}

func TestEnsure_ReusesBranch(t *testing.T) {
	repo := newRepo(t)
	run(t, repo, "branch", "feature/login")
	dir := Path(t.TempDir(), 1, "login")

	if err := Ensure(context.Background(), repo, dir, "feature/login"); err != nil {
		t.Fatal(err)
	}
	if got := run(t, dir, "branch", "--show-current"); got != "feature/login" {
		t.Errorf("branch = %q, want feature/login", got)
	}
}

func TestEnsure_NotARepo(t *testing.T) {
	err := Ensure(context.Background(), t.TempDir(), filepath.Join(t.TempDir(), "w"), "feature/login")
	if err == nil || !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("Ensure error = %v, want one saying not a git repository", err)
	}
}

func TestDirty(t *testing.T) {
	repo := newRepo(t)
	if got, err := Dirty(context.Background(), repo); err != nil || got {
		t.Errorf("Dirty on a clean checkout = %v, %v, want false", got, err)
	}
	if err := os.WriteFile(filepath.Join(repo, "b.txt"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := Dirty(context.Background(), repo); err != nil || !got {
		t.Errorf("Dirty with a new file = %v, %v, want true", got, err)
	}
}
