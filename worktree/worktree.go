// Package worktree runs the git commands behind a feature's worktree.
package worktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Path is where the worktree of the feature with slug in project projectID lives.
func Path(dataDir string, projectID int64, slug string) string {
	return filepath.Join(dataDir, "worktrees", strconv.FormatInt(projectID, 10), slug)
}

// Branch is the branch a feature works on.
func Branch(slug string) string { return "feature/" + slug }

// Ensure makes dir a worktree of repo on branch. It does nothing when dir already is one.
// The branch is made from the HEAD of repo, or reused when it exists.
func Ensure(ctx context.Context, repo, dir, branch string) error {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return nil
	}
	args := []string{"worktree", "add", "-b", branch, dir, "HEAD"}
	_, err := git(ctx, repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	var exit *exec.ExitError
	switch {
	case err == nil:
		args = []string{"worktree", "add", dir, branch}
	case !errors.As(err, &exit) || exit.ExitCode() != 1:
		return err
	}
	_, err = git(ctx, repo, args...)
	return err
}

// Dirty reports whether the checkout at repo has uncommitted changes.
func Dirty(ctx context.Context, repo string) (bool, error) {
	out, err := git(ctx, repo, "status", "--porcelain")
	return out != "", err
}

// IsRepo reports whether dir is inside a git work tree.
func IsRepo(ctx context.Context, dir string) bool {
	_, err := git(ctx, dir, "rev-parse", "--is-inside-work-tree")
	return err == nil
}

// Checkpoint stores HEAD and the whole working tree of the checkout at dir, untracked files
// included and ignored ones left out, as a commit at ref. The index, HEAD and the files are
// not touched: the tree is built in a temporary index.
func Checkpoint(ctx context.Context, dir, ref string) error {
	head, err := git(ctx, dir, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return fmt.Errorf("checkpoint %s: no commit to snapshot: %w", dir, err)
	}
	tmp, err := os.MkdirTemp("", "reef-checkpoint-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	index := []string{"GIT_INDEX_FILE=" + filepath.Join(tmp, "index")}
	if _, err := gitEnv(ctx, dir, index, "read-tree", "HEAD"); err != nil {
		return err
	}
	if _, err := gitEnv(ctx, dir, index, "add", "-A"); err != nil {
		return err
	}
	tree, err := gitEnv(ctx, dir, index, "write-tree")
	if err != nil {
		return err
	}
	commit, err := git(ctx, dir, "-c", "user.name=Reef", "-c", "user.email=reef@localhost",
		"commit-tree", tree, "-p", head, "-m", "reef checkpoint")
	if err != nil {
		return err
	}
	_, err = git(ctx, dir, "update-ref", ref, commit)
	return err
}

func git(ctx context.Context, repo string, args ...string) (string, error) {
	return gitEnv(ctx, repo, nil, args...)
}

func gitEnv(ctx context.Context, repo string, env []string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...)
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		return "", fmt.Errorf("git %s in %s: %w: %s", strings.Join(args, " "), repo, err, text)
	}
	return text, nil
}
