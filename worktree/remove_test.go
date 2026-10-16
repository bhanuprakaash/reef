package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemove(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	dir := Path(t.TempDir(), 1, "login")
	if err := Ensure(ctx, repo, dir, "feature/login", ""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Remove(ctx, repo, dir); !errors.Is(err, ErrDirty) {
		t.Fatalf("Remove of a dirty worktree = %v, want ErrDirty", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("dirty worktree is gone: %v", err)
	}

	run(t, dir, "add", ".")
	run(t, dir, "commit", "-m", "work")
	if err := Remove(ctx, repo, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("worktree folder after Remove: err = %v, want not exist", err)
	}
	if got := run(t, repo, "worktree", "list", "--porcelain"); strings.Contains(got, dir) {
		t.Errorf("worktree list still has %s:\n%s", dir, got)
	}
	run(t, repo, "rev-parse", "--verify", "refs/heads/feature/login")

	if err := Remove(ctx, repo, dir); err != nil {
		t.Errorf("Remove of a missing worktree = %v, want nil", err)
	}
}

func TestDeleteRefs(t *testing.T) {
	repo := newRepo(t)
	run(t, repo, "update-ref", "refs/reef/checkpoints/1", "HEAD")
	run(t, repo, "update-ref", "refs/reef/checkpoints/2", "HEAD")

	refs := []string{"refs/reef/checkpoints/1", "refs/reef/checkpoints/9"}
	if err := DeleteRefs(context.Background(), repo, refs); err != nil {
		t.Fatal(err)
	}
	got := run(t, repo, "for-each-ref", "--format=%(refname)", "refs/reef")
	if got != "refs/reef/checkpoints/2" {
		t.Errorf("refs left = %q, want only refs/reef/checkpoints/2", got)
	}
}
