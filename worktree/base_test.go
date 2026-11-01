package worktree

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func commitFile(t *testing.T, repo, name string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, name), []byte(name), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, repo, "add", ".")
	run(t, repo, "commit", "-m", name)
	return run(t, repo, "rev-parse", "HEAD")
}

func TestEnsure_BaseBranchIsFetchedFromOrigin(t *testing.T) {
	remote := t.TempDir()
	run(t, remote, "init", "--bare", "-b", "main")
	repo := newRepo(t)
	run(t, repo, "remote", "add", "origin", remote)
	run(t, repo, "push", "origin", "main")
	run(t, repo, "checkout", "-b", "dev")
	commitFile(t, repo, "dev1.txt")
	run(t, repo, "push", "origin", "dev")
	run(t, repo, "checkout", "main")
	other := t.TempDir()
	run(t, other, "clone", "-b", "dev", remote, ".")
	newDev := commitFile(t, other, "dev2.txt")
	run(t, other, "push", "origin", "dev")
	main := run(t, repo, "rev-parse", "main")
	dir := filepath.Join(t.TempDir(), "login")

	if _, err := Create(context.Background(), repo, dir, "feature/login", "dev"); err != nil {
		t.Fatal(err)
	}
	if got := run(t, dir, "rev-parse", "HEAD"); got != newDev {
		t.Errorf("HEAD = %q, want origin/dev's %q", got, newDev)
	}
	if got, err := Base(context.Background(), repo, dir, "dev"); err != nil || got != newDev {
		t.Errorf("Base = %q, %v; want origin/dev's %q", got, err, newDev)
	}

	empty := filepath.Join(t.TempDir(), "search")
	if _, err := Create(context.Background(), repo, empty, "feature/search", ""); err != nil {
		t.Fatal(err)
	}
	if got := run(t, empty, "rev-parse", "HEAD"); got != main {
		t.Errorf("HEAD with no base = %q, want local main's %q", got, main)
	}
}

func TestResolveBase_EmptyBaseWithOriginButNoDefaultSaysSo(t *testing.T) {
	remote := t.TempDir()
	run(t, remote, "init", "--bare", "-b", "main")
	repo := newRepo(t)
	run(t, repo, "remote", "add", "origin", remote)
	sha := run(t, repo, "rev-parse", "--short", "main")

	b, err := ResolveBase(context.Background(), repo, "")

	want := "origin's default branch is unknown; using local main at " + sha
	if err != nil || b.Source != "local" || b.Ref != "refs/heads/main" || b.Note != want {
		t.Errorf("ResolveBase = %+v, %v; want local refs/heads/main with note %q", b, err, want)
	}
	plain := newRepo(t)
	if b, err := ResolveBase(context.Background(), plain, ""); err != nil || b.Note != "" {
		t.Errorf("ResolveBase of a repo with no origin = %+v, %v; want no note", b, err)
	}
}

func TestEnsure_BaseBranchWithoutRemote(t *testing.T) {
	repo := newRepo(t)
	run(t, repo, "checkout", "-b", "dev")
	dev := commitFile(t, repo, "dev.txt")
	run(t, repo, "checkout", "main")

	tests := []struct {
		name string
		give string
		want string
	}{
		{"local branch", "dev", dev},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "login")
			branch := "feature/login" + strconv.Itoa(i)
			if _, err := Create(context.Background(), repo, dir, branch, tt.give); err != nil {
				t.Fatal(err)
			}
			if got := run(t, dir, "rev-parse", "HEAD"); got != tt.want {
				t.Errorf("HEAD = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidBranch(t *testing.T) {
	tests := []struct {
		give string
		want bool
	}{
		{"dev", true},
		{"release/1.0", true},
		{"-x", false},
		{"a b", false},
		{"a..b", false},
		{"@{-1}", false},
	}
	for _, tt := range tests {
		if got := ValidBranch(context.Background(), tt.give); got != tt.want {
			t.Errorf("ValidBranch(%q) = %v, want %v", tt.give, got, tt.want)
		}
	}
}
