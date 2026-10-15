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

// _diffFlags pin git diff's output shape against the user's own git config (diff.noprefix,
// color.ui, diff.external, diff.renames, core.quotePath).
var _diffFlags = []string{
	"-c", "core.quotePath=false",
	"diff", "--no-renames", "--no-color", "--no-ext-diff", "--src-prefix=a/", "--dst-prefix=b/",
}

// fileMeta is one file's path and status from a `--name-status -z` listing, which gives an
// unambiguous path even for non-ASCII names or an empty file, unlike the `--- `/`+++ ` lines.
type fileMeta struct {
	path   string
	status string
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
	metas, err := nameStatus(ctx, dir, base, tree)
	if err != nil {
		return nil, err
	}
	out, err := git(ctx, dir, append(append([]string{}, _diffFlags...), base, tree)...)
	if err != nil {
		return nil, err
	}
	return parseDiff(out, metas), nil
}

// nameStatus lists each changed file's path and status (added, modified, deleted), read from a
// `-z`-terminated name-status listing so a path is never truncated or misread on a quoting rule.
func nameStatus(ctx context.Context, dir, base, tree string) ([]fileMeta, error) {
	args := append(append([]string{}, _diffFlags...), "-z", "--name-status", base, tree)
	out, err := git(ctx, dir, args...)
	if err != nil {
		return nil, err
	}
	var metas []fileMeta
	fields := strings.Split(out, "\x00")
	for i := 0; i+1 < len(fields); i += 2 {
		if fields[i] == "" {
			continue
		}
		// A type change (file <-> symlink) is one name-status record but two `diff --git`
		// blocks in the patch: one deleting the old type, one adding the new.
		if fields[i][:1] == "T" {
			metas = append(metas,
				fileMeta{path: fields[i+1], status: "deleted"},
				fileMeta{path: fields[i+1], status: "added"})
			continue
		}
		metas = append(metas, fileMeta{path: fields[i+1], status: statusWord(fields[i])})
	}
	return metas, nil
}

func statusWord(code string) string {
	switch code[:1] {
	case "A":
		return "added"
	case "D":
		return "deleted"
	default:
		return "modified"
	}
}

// parseDiff turns the unified diff text of `git diff` into one FileDiff per file, taking each
// file's path and status from metas in order rather than from the `--- `/`+++ ` lines, which
// are ambiguous for an empty file, a binary file or a non-ASCII name.
func parseDiff(out string, metas []fileMeta) []FileDiff {
	var files []FileDiff
	var cur *FileDiff
	var hunk *Hunk
	var oldLine, newLine int
	next := 0
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			if cur != nil {
				files = append(files, finishFile(*cur))
			}
			cur = &FileDiff{}
			if next < len(metas) {
				cur.Path, cur.Status = metas[next].path, metas[next].status
				next++
			}
			hunk = nil
		case cur == nil:
			continue
		case strings.HasPrefix(line, "Binary files "):
			continue
		// `--- `/`+++ ` are file headers only before the first hunk; once a hunk is open, a
		// line with that prefix is removed/added file content (e.g. a comment starting "-- ").
		case hunk == nil && strings.HasPrefix(line, "--- "):
			continue
		case hunk == nil && strings.HasPrefix(line, "+++ "):
			continue
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
		files = append(files, finishFile(*cur))
	}
	return files
}

// finishFile makes Hunks a non-nil empty slice so it marshals as `[]`, never `null`.
func finishFile(f FileDiff) FileDiff {
	if f.Hunks == nil {
		f.Hunks = []Hunk{}
	}
	return f
}
