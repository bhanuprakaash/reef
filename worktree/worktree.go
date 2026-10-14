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

// ErrNoCommit is returned by Checkpoint when there is nothing it can cleanly snapshot: a repo
// whose HEAD has no commit yet, or one with an unmerged index mid-merge.
var ErrNoCommit = errors.New("no commit to snapshot")

// ErrDirty is returned by Remove for a worktree with uncommitted changes.
var ErrDirty = errors.New("uncommitted changes")

// Path is where the worktree of the feature with slug in project projectID lives.
func Path(dataDir string, projectID int64, slug string) string {
	return filepath.Join(dataDir, "worktrees", strconv.FormatInt(projectID, 10), slug)
}

// Branch is the branch a feature works on.
func Branch(slug string) string { return "feature/" + slug }

// Ensure makes dir a worktree of repo on branch. It does nothing when dir already is one.
// The branch is made from the HEAD of repo, or reused when it exists. A worktree whose folder
// is gone is made again. A repo with no commit has no HEAD to make a branch from, and fails,
// as does a branch that another worktree has checked out.
func Ensure(ctx context.Context, repo, dir, branch string) error {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return nil
	}
	if _, err := git(ctx, repo, "rev-parse", "--verify", "HEAD"); err != nil {
		return fmt.Errorf("features need the project folder to be a git repo with a commit: %s: %w", repo, err)
	}
	if _, err := git(ctx, repo, "worktree", "prune"); err != nil {
		return err
	}
	args := []string{"worktree", "add", "-b", branch, dir, "HEAD"}
	_, err := git(ctx, repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	var exit *exec.ExitError
	switch {
	case err == nil:
		at, err := checkedOutAt(ctx, repo, branch)
		if err != nil {
			return err
		}
		if at != "" {
			return fmt.Errorf("branch %s is checked out at %s; switch that checkout to another branch", branch, at)
		}
		args = []string{"worktree", "add", dir, branch}
	case !errors.As(err, &exit) || exit.ExitCode() != 1:
		return err
	}
	_, err = git(ctx, repo, args...)
	return err
}

// Clean returns ErrDirty when the worktree at dir has uncommitted changes. A dir that is no
// worktree is clean.
func Clean(ctx context.Context, dir string) error {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return nil
	}
	dirty, err := Dirty(ctx, dir)
	if err != nil {
		return err
	}
	if dirty {
		return fmt.Errorf("%s: %w", dir, ErrDirty)
	}
	return nil
}

// Remove removes the worktree at dir of repo, and keeps its branch. A dir that is no worktree
// is not an error. A worktree with uncommitted changes is left as it is and returns ErrDirty.
func Remove(ctx context.Context, repo, dir string) error {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return nil
	}
	if err := Clean(ctx, dir); err != nil {
		return err
	}
	_, err := git(ctx, repo, "worktree", "remove", dir)
	return err
}

// DeleteRefs deletes refs in repo. A ref that does not exist is not an error.
func DeleteRefs(ctx context.Context, repo string, refs []string) error {
	for _, ref := range refs {
		if _, err := git(ctx, repo, "update-ref", "-d", ref); err != nil {
			return err
		}
	}
	return nil
}

// checkedOutAt is the path of the worktree of repo that has branch checked out, or "" when
// none has.
func checkedOutAt(ctx context.Context, repo, branch string) (string, error) {
	out, err := git(ctx, repo, "worktree", "list", "--porcelain")
	if err != nil {
		return "", err
	}
	var path string
	for _, line := range strings.Split(out, "\n") {
		if v, ok := strings.CutPrefix(line, "worktree "); ok {
			path = v
		} else if line == "branch refs/heads/"+branch {
			return path, nil
		}
	}
	return "", nil
}

// Dirty reports whether the checkout at repo has uncommitted changes.
func Dirty(ctx context.Context, repo string) (bool, error) {
	out, err := git(ctx, repo, "status", "--porcelain")
	return out != "", err
}

// IsRepo reports whether dir is itself the top level of a git work tree, not merely a folder
// inside one: a checkpoint or restore of dir must not reach outside it.
func IsRepo(ctx context.Context, dir string) bool {
	top, err := git(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return false
	}
	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return false
	}
	got, err := filepath.EvalSymlinks(top)
	return err == nil && got == want
}

// HasCommit reports whether dir is a git repo whose HEAD points at a commit.
func HasCommit(ctx context.Context, dir string) bool {
	_, err := git(ctx, dir, "rev-parse", "--verify", "--quiet", "HEAD")
	return err == nil
}

// Checkpoint stores HEAD and the whole working tree of the checkout at dir, untracked files
// included and ignored ones left out, as a commit at ref. Its first parent is HEAD; its second
// is a commit of the index as it was, so Restore can bring back what was staged. The index,
// HEAD and the files are not touched: the tree is built in a temporary index that starts as a
// copy of the real one, so files git already knows are not read again. A repo with no commit,
// or an index with unmerged entries mid-merge, returns ErrNoCommit.
func Checkpoint(ctx context.Context, dir, ref string) error {
	head, err := git(ctx, dir, "rev-parse", "--verify", "--quiet", "HEAD")
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return fmt.Errorf("checkpoint %s: %w", dir, ErrNoCommit)
	}
	if err != nil {
		return err
	}
	if unmerged, err := git(ctx, dir, "ls-files", "-u"); err != nil {
		return err
	} else if unmerged != "" {
		return fmt.Errorf("checkpoint %s: unmerged index: %w", dir, ErrNoCommit)
	}
	tmp, err := os.MkdirTemp("", "reef-checkpoint-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	index := []string{"GIT_INDEX_FILE=" + filepath.Join(tmp, "index")}
	if err := seedIndex(ctx, dir, filepath.Join(tmp, "index")); err != nil {
		return err
	}
	if _, err := gitEnv(ctx, dir, index, "add", "-A"); err != nil {
		return err
	}
	tree, err := gitEnv(ctx, dir, index, "write-tree")
	if err != nil {
		return err
	}
	staged, err := git(ctx, dir, "write-tree")
	if err != nil {
		return err
	}
	stagedCommit, err := commitTree(ctx, dir, staged, "reef checkpoint", head)
	if err != nil {
		return err
	}
	branch, err := git(ctx, dir, "branch", "--show-current")
	if err != nil {
		return err
	}
	message := fmt.Sprintf("reef checkpoint\n\ndir: %s\nbranch: %s", dir, branch)
	commit, err := commitTree(ctx, dir, tree, message, head, stagedCommit)
	if err != nil {
		return err
	}
	_, err = git(ctx, dir, "update-ref", ref, commit)
	return err
}

// Origin is the directory and the branch the checkpoint at ref was taken in. The branch is
// empty when HEAD was detached. repo is any checkout of the repository that holds ref.
func Origin(ctx context.Context, repo, ref string) (dir, branch string, err error) {
	msg, err := git(ctx, repo, "log", "-1", "--format=%B", ref)
	if err != nil {
		return "", "", err
	}
	for _, line := range strings.Split(msg, "\n") {
		if v, ok := strings.CutPrefix(line, "dir: "); ok {
			dir = v
		} else if v, ok := strings.CutPrefix(line, "branch: "); ok {
			branch = v
		}
	}
	return dir, branch, nil
}

// Restore puts the checkout at dir back as Checkpoint saw it at ref: the branch it was on,
// checked out at the HEAD it had, the files as they were, untracked ones included, and the
// same changes staged. A checkpoint of a detached HEAD resets whatever is checked out. Files
// made since are removed. Ignored files are left alone.
func Restore(ctx context.Context, dir, ref string) error {
	_, branch, err := Origin(ctx, dir, ref)
	if err != nil {
		return err
	}
	back := []string{"reset", "--hard", ref + "^1"}
	if branch != "" {
		back = []string{"checkout", "--force", "-B", branch, ref + "^1"}
	}
	steps := [][]string{
		back,
		{"clean", "-fd"},
		{"read-tree", "--reset", "-u", ref},
		{"read-tree", ref + "^2"},
	}
	for _, args := range steps {
		if _, err := git(ctx, dir, args...); err != nil {
			return err
		}
	}
	return nil
}

// seedIndex copies the real index of the checkout at dir to to. A checkout without an index
// file starts with an empty one.
func seedIndex(ctx context.Context, dir, to string) error {
	from, err := git(ctx, dir, "rev-parse", "--path-format=absolute", "--git-path", "index")
	if err != nil {
		return err
	}
	data, err := os.ReadFile(from)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return os.WriteFile(to, data, 0o600)
}

func commitTree(ctx context.Context, dir, tree, message string, parents ...string) (string, error) {
	args := []string{"-c", "user.name=Reef", "-c", "user.email=reef@localhost", "commit-tree", tree}
	for _, p := range parents {
		args = append(args, "-p", p)
	}
	return git(ctx, dir, append(args, "-m", message)...)
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
