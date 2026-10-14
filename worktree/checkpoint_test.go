package worktree

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckpoint(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repo, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".gitignore", "*.log\n")
	run(t, repo, "add", ".gitignore")
	run(t, repo, "commit", "-m", "ignore")
	write("a.txt", "a2")
	write("staged.txt", "s")
	run(t, repo, "add", "staged.txt")
	write("new.txt", "n")
	write("x.log", "ignored")
	head := run(t, repo, "rev-parse", "HEAD")
	status := run(t, repo, "status", "--porcelain")
	index := run(t, repo, "ls-files", "-s")
	if want := "M a.txt\nA  staged.txt\n?? new.txt"; status != want {
		t.Fatalf("status before = %q, want %q", status, want)
	}

	if err := Checkpoint(ctx, repo, "refs/reef/checkpoints/7"); err != nil {
		t.Fatal(err)
	}

	if got := run(t, repo, "rev-parse", "HEAD"); got != head {
		t.Errorf("HEAD = %q, want %q", got, head)
	}
	if got := run(t, repo, "status", "--porcelain"); got != status {
		t.Errorf("status = %q, want %q", got, status)
	}
	if got := run(t, repo, "ls-files", "-s"); got != index {
		t.Errorf("index = %q, want %q", got, index)
	}
	if got := run(t, repo, "rev-parse", "refs/reef/checkpoints/7^"); got != head {
		t.Errorf("checkpoint parent = %q, want %q", got, head)
	}
	if got, want := run(t, repo, "ls-tree", "-r", "--name-only", "refs/reef/checkpoints/7"),
		".gitignore\na.txt\nnew.txt\nstaged.txt"; got != want {
		t.Errorf("checkpoint files = %q, want %q", got, want)
	}
	for file, want := range map[string]string{"new.txt": "n", "a.txt": "a2", "staged.txt": "s"} {
		if got := run(t, repo, "show", "refs/reef/checkpoints/7:"+file); got != want {
			t.Errorf("checkpoint %s = %q, want %q", file, got, want)
		}
	}
}

func TestCheckpoint_NoCommits(t *testing.T) {
	repo := t.TempDir()
	run(t, repo, "init", "-b", "main")
	err := Checkpoint(context.Background(), repo, "refs/reef/checkpoints/1")
	if !errors.Is(err, ErrNoCommit) {
		t.Errorf("Checkpoint in a repo with no commits: err = %v, want ErrNoCommit", err)
	}
}

func TestCheckpoint_NotARepo(t *testing.T) {
	err := Checkpoint(context.Background(), t.TempDir(), "refs/reef/checkpoints/1")
	if err == nil || errors.Is(err, ErrNoCommit) || !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("Checkpoint in a plain folder: err = %v, want a not a git repository error", err)
	}
}

func TestCheckpoint_SeedsIndex(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	run(t, repo, "update-index", "--skip-worktree", "a.txt")
	if err := os.Remove(filepath.Join(repo, "a.txt")); err != nil {
		t.Fatal(err)
	}

	if err := Checkpoint(ctx, repo, "refs/reef/checkpoints/1"); err != nil {
		t.Fatal(err)
	}

	if got := run(t, repo, "show", "refs/reef/checkpoints/1:a.txt"); got != "a" {
		t.Errorf("checkpoint a.txt = %q, want a: the real index marks it skip-worktree, so a temporary index seeded from it keeps the entry", got)
	}
}

func TestCheckpoint_Unmerged(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	run(t, repo, "checkout", "-b", "other")
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("other"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, repo, "commit", "-am", "other change")
	run(t, repo, "checkout", "main")
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("main"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, repo, "commit", "-am", "main change")
	if err := exec.Command("git", "-C", repo, "merge", "other").Run(); err == nil {
		t.Fatal("merge did not conflict")
	}

	err := Checkpoint(ctx, repo, "refs/reef/checkpoints/1")
	if !errors.Is(err, ErrNoCommit) {
		t.Errorf("Checkpoint with an unmerged index: err = %v, want ErrNoCommit", err)
	}
}

func TestIsRepo(t *testing.T) {
	ctx := context.Background()
	if !IsRepo(ctx, newRepo(t)) {
		t.Error("IsRepo(repo) = false, want true")
	}
	if IsRepo(ctx, t.TempDir()) {
		t.Error("IsRepo(plain folder) = true, want false")
	}
}

func TestIsRepo_Subfolder(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	sub := filepath.Join(repo, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if IsRepo(ctx, sub) {
		t.Error("IsRepo(subfolder) = true, want false")
	}
}

func TestHasCommit(t *testing.T) {
	ctx := context.Background()
	if !HasCommit(ctx, newRepo(t)) {
		t.Error("HasCommit(repo) = false, want true")
	}
	noCommit := t.TempDir()
	run(t, noCommit, "init", "-b", "main")
	if HasCommit(ctx, noCommit) {
		t.Error("HasCommit(repo with no commit) = true, want false")
	}
	if HasCommit(ctx, t.TempDir()) {
		t.Error("HasCommit(plain folder) = true, want false")
	}
}
