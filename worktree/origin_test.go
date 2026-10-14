package worktree

import (
	"context"
	"testing"
)

func TestOrigin_AndRestoreOfABranch(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	run(t, repo, "checkout", "-b", "feature/x")
	head := run(t, repo, "rev-parse", "HEAD")
	if err := Checkpoint(ctx, repo, "refs/reef/checkpoints/1"); err != nil {
		t.Fatal(err)
	}
	dir, branch, err := Origin(ctx, repo, "refs/reef/checkpoints/1")
	if err != nil || dir != repo || branch != "feature/x" {
		t.Errorf("Origin = %q, %q, %v, want %q, feature/x", dir, branch, err, repo)
	}

	run(t, repo, "checkout", "-b", "other")
	run(t, repo, "commit", "--allow-empty", "-m", "on other")
	run(t, repo, "branch", "-f", "feature/x", "other")
	if err := Restore(ctx, repo, "refs/reef/checkpoints/1"); err != nil {
		t.Fatal(err)
	}

	if got := run(t, repo, "branch", "--show-current"); got != "feature/x" {
		t.Errorf("branch = %q, want feature/x", got)
	}
	if got := run(t, repo, "rev-parse", "feature/x"); got != head {
		t.Errorf("feature/x = %q, want %q", got, head)
	}
}

func TestOrigin_DetachedHeadRestoresInPlace(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	head := run(t, repo, "rev-parse", "HEAD")
	run(t, repo, "checkout", "--detach")
	if err := Checkpoint(ctx, repo, "refs/reef/checkpoints/1"); err != nil {
		t.Fatal(err)
	}
	if _, branch, err := Origin(ctx, repo, "refs/reef/checkpoints/1"); err != nil || branch != "" {
		t.Errorf("Origin branch = %q, %v, want none", branch, err)
	}

	run(t, repo, "commit", "--allow-empty", "-m", "later")
	if err := Restore(ctx, repo, "refs/reef/checkpoints/1"); err != nil {
		t.Fatal(err)
	}

	if got := run(t, repo, "rev-parse", "HEAD"); got != head {
		t.Errorf("HEAD = %q, want %q", got, head)
	}
	if got := run(t, repo, "branch", "--show-current"); got != "" {
		t.Errorf("branch = %q, want detached", got)
	}
}
