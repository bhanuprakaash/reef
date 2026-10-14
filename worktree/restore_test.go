package worktree

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRestore(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	file := func(name string) string { return filepath.Join(repo, name) }
	write := func(name, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(file(name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file(name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	read := func(name string) string {
		t.Helper()
		b, err := os.ReadFile(file(name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	write(".gitignore", "*.log\n")
	write("b.txt", "b")
	run(t, repo, "add", ".")
	run(t, repo, "commit", "-m", "second")

	write("a.txt", "a staged")
	run(t, repo, "add", "a.txt")
	write("a.txt", "a staged and more")
	write("b.txt", "b unstaged")
	write("staged-new.txt", "s")
	run(t, repo, "add", "staged-new.txt")
	write("new.txt", "n")
	write("dir/deep.txt", "d")
	write("x.log", "ignored")
	head := run(t, repo, "rev-parse", "HEAD")
	status := run(t, repo, "status", "--porcelain")
	if want := "MM a.txt\n M b.txt\nA  staged-new.txt\n?? dir/\n?? new.txt"; status != want {
		t.Fatalf("status before = %q, want %q", status, want)
	}
	if err := Checkpoint(ctx, repo, "refs/reef/checkpoints/3"); err != nil {
		t.Fatal(err)
	}

	write("a.txt", "a by the turn")
	run(t, repo, "rm", "-f", "b.txt")
	run(t, repo, "rm", "-f", "staged-new.txt")
	write("created.txt", "c")
	write("dir/created.txt", "c")
	write("later.log", "ignored too")
	run(t, repo, "add", "created.txt")
	run(t, repo, "commit", "-m", "by the turn")
	write("after-commit.txt", "ac")
	if err := os.Remove(file("new.txt")); err != nil {
		t.Fatal(err)
	}

	if err := Restore(ctx, repo, "refs/reef/checkpoints/3"); err != nil {
		t.Fatal(err)
	}

	if got := run(t, repo, "rev-parse", "HEAD"); got != head {
		t.Errorf("HEAD = %q, want %q", got, head)
	}
	if got := run(t, repo, "log", "--format=%s"); got != "second\nfirst" {
		t.Errorf("log = %q, want second and first", got)
	}
	if got := run(t, repo, "status", "--porcelain"); got != status {
		t.Errorf("status = %q, want %q", got, status)
	}
	for name, want := range map[string]string{
		"a.txt": "a staged and more", "b.txt": "b unstaged", "staged-new.txt": "s",
		"new.txt": "n", "dir/deep.txt": "d", "x.log": "ignored", "later.log": "ignored too",
	} {
		if got := read(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	for _, name := range []string{"created.txt", "dir/created.txt", "after-commit.txt"} {
		if _, err := os.Stat(file(name)); !os.IsNotExist(err) {
			t.Errorf("%s exists after restore, want it removed (err = %v)", name, err)
		}
	}
	if got := run(t, repo, "show", ":a.txt"); got != "a staged" {
		t.Errorf("staged a.txt = %q, want %q", got, "a staged")
	}
}

func TestRestore_CleanTree(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	if err := Checkpoint(ctx, repo, "refs/reef/checkpoints/1"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, repo, "commit", "-am", "later")

	if err := Restore(ctx, repo, "refs/reef/checkpoints/1"); err != nil {
		t.Fatal(err)
	}

	if got := run(t, repo, "status", "--porcelain"); got != "" {
		t.Errorf("status = %q, want clean", got)
	}
	if got := run(t, repo, "log", "--format=%s"); got != "first" {
		t.Errorf("log = %q, want first", got)
	}
}

func TestRestore_NoCheckpoint(t *testing.T) {
	repo := newRepo(t)
	if err := Restore(context.Background(), repo, "refs/reef/checkpoints/9"); err == nil {
		t.Error("Restore of a missing ref succeeded, want an error")
	}
	if got := run(t, repo, "status", "--porcelain"); got != "" {
		t.Errorf("status = %q, want the tree untouched", got)
	}
}
