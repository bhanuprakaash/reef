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

func git(ctx context.Context, repo string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...).
		CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		return "", fmt.Errorf("git %s in %s: %w: %s", strings.Join(args, " "), repo, err, text)
	}
	return text, nil
}
