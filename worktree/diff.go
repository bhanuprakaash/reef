package worktree

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// FileDiff is one file's changes in a Diff.
type FileDiff struct {
	Path   string `json:"path"`
	Status string `json:"status"` // added, modified or deleted
	Hunks  []Hunk `json:"hunks"`
}

// Hunk is one unified-diff hunk of a FileDiff.
type Hunk struct {
	Header string `json:"header"`
	Lines  []Line `json:"lines"`
}

// Line is one line of a Hunk. Old and New are its line number in the old and new file; a line
// carries only the one its Kind has.
type Line struct {
	Kind string `json:"kind"` // add, del or ctx
	Old  int    `json:"old,omitempty"`
	New  int    `json:"new,omitempty"`
	Text string `json:"text"`
}

var _hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

// Base is the commit dir's worktree was made from: where its branch and repo's current HEAD
// last shared history.
func Base(ctx context.Context, repo, dir string) (string, error) {
	head, err := git(ctx, repo, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return git(ctx, dir, "merge-base", "HEAD", head)
}

// Diff is the changes of the worktree at dir against base, a commit reachable from dir,
// uncommitted and untracked files included. The real index is not touched: the tree diffed is
// built in a temporary index, the same way Checkpoint builds one.
func Diff(ctx context.Context, dir, base string) ([]FileDiff, error) {
	tmp, err := os.MkdirTemp("", "reef-diff-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	index := []string{"GIT_INDEX_FILE=" + filepath.Join(tmp, "index")}
	if err := seedIndex(ctx, dir, filepath.Join(tmp, "index")); err != nil {
		return nil, err
	}
	if _, err := gitEnv(ctx, dir, index, "add", "-A"); err != nil {
		return nil, err
	}
	tree, err := gitEnv(ctx, dir, index, "write-tree")
	if err != nil {
		return nil, err
	}
	out, err := git(ctx, dir, "diff", base, tree)
	if err != nil {
		return nil, err
	}
	return parseDiff(out), nil
}

// parseDiff turns the unified diff text of `git diff` into one FileDiff per file.
func parseDiff(out string) []FileDiff {
	var files []FileDiff
	var cur *FileDiff
	var hunk *Hunk
	var oldLine, newLine int
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			if cur != nil {
				files = append(files, *cur)
			}
			cur = &FileDiff{Status: "modified"}
			hunk = nil
		case cur == nil:
			continue
		case strings.HasPrefix(line, "new file mode"):
			cur.Status = "added"
		case strings.HasPrefix(line, "deleted file mode"):
			cur.Status = "deleted"
		case strings.HasPrefix(line, "--- "):
			if p := strings.TrimPrefix(line, "--- a/"); p != line {
				cur.Path = p
			}
		case strings.HasPrefix(line, "+++ "):
			if p := strings.TrimPrefix(line, "+++ b/"); p != line {
				cur.Path = p
			}
		case strings.HasPrefix(line, "@@ "):
			if m := _hunkHeader.FindStringSubmatch(line); m != nil {
				oldLine, _ = strconv.Atoi(m[1])
				newLine, _ = strconv.Atoi(m[2])
			}
			cur.Hunks = append(cur.Hunks, Hunk{Header: line})
			hunk = &cur.Hunks[len(cur.Hunks)-1]
		case hunk == nil:
			continue
		case strings.HasPrefix(line, "+"):
			hunk.Lines = append(hunk.Lines, Line{Kind: "add", New: newLine, Text: line[1:]})
			newLine++
		case strings.HasPrefix(line, "-"):
			hunk.Lines = append(hunk.Lines, Line{Kind: "del", Old: oldLine, Text: line[1:]})
			oldLine++
		case strings.HasPrefix(line, " "):
			hunk.Lines = append(hunk.Lines, Line{Kind: "ctx", Old: oldLine, New: newLine, Text: line[1:]})
			oldLine++
			newLine++
		}
	}
	if cur != nil {
		files = append(files, *cur)
	}
	return files
}
