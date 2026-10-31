package skill

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// File is one file of a package: its path relative to the package folder, with "/" separators.
type File struct {
	Path string
	Body []byte
}

// Package is a skill folder read into memory.
type Package struct {
	Name        string
	Description string
	Files       []File
	// Revision is the commit a Git import was cloned at; empty for a folder.
	Revision string
}

// ReadFolder reads the skill folder at dir. It only reads: nothing in it is run or changed.
// It skips .git, and fails on a symlink or a folder without a SKILL.md with a valid name.
func ReadFolder(dir string) (Package, error) {
	root, err := filepath.Abs(dir)
	if err != nil {
		return Package{}, fmt.Errorf("skill folder %q: %w", dir, err)
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return Package{}, fmt.Errorf("skill folder %q is not a folder", dir)
	}
	var p Package
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		switch {
		case rel == ".":
			return nil
		case d.Name() == ".git" && d.IsDir():
			return filepath.SkipDir
		case d.Type()&fs.ModeSymlink != 0:
			return fmt.Errorf("%s is a symlink", filepath.ToSlash(rel))
		case d.IsDir():
			return nil
		case !d.Type().IsRegular():
			return fmt.Errorf("%s is not a regular file", filepath.ToSlash(rel))
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		p.Files = append(p.Files, File{Path: filepath.ToSlash(rel), Body: body})
		return nil
	})
	if err != nil {
		return Package{}, fmt.Errorf("read skill folder %q: %w", dir, err)
	}
	p, err = withManifest(p)
	if errors.Is(err, errNoManifest) {
		return Package{}, fmt.Errorf("skill folder %q has no SKILL.md", dir)
	}
	return p, err
}

var errNoManifest = errors.New("no SKILL.md")

// withManifest fills p's Name and Description from its SKILL.md frontmatter.
func withManifest(p Package) (Package, error) {
	for _, f := range p.Files {
		if f.Path != "SKILL.md" {
			continue
		}
		name, description := frontmatter(string(f.Body))
		if !ValidName(name) {
			return Package{}, fmt.Errorf("SKILL.md needs a name in its frontmatter: letters, digits, :, _ or -, not %q", name)
		}
		p.Name, p.Description = name, description
		return p, nil
	}
	return Package{}, errNoManifest
}

// Build makes a package from files written in Reef. It needs a SKILL.md whose frontmatter name
// is name, and every path must stay inside the folder: relative, "/" separators, no duplicates,
// nothing under .git.
func Build(name string, files []File) (Package, error) {
	seen := map[string]bool{}
	for _, f := range files {
		if !filepath.IsLocal(f.Path) || strings.Contains(f.Path, `\`) || f.Path != path.Clean(f.Path) ||
			f.Path == ".git" || strings.HasPrefix(f.Path, ".git/") {
			return Package{}, fmt.Errorf("skill file path %q must be a plain relative path with / separators", f.Path)
		}
		if seen[f.Path] {
			return Package{}, fmt.Errorf("skill file %q is listed twice", f.Path)
		}
		seen[f.Path] = true
	}
	p, err := withManifest(Package{Files: files})
	if err != nil {
		return Package{}, err
	}
	if p.Name != name {
		return Package{}, fmt.Errorf("SKILL.md names the skill %q, not %q", p.Name, name)
	}
	return p, nil
}

// ReadRepo clones repo (at ref, when set) into a temporary folder and reads the package in its
// sub-folder dir, or at its top when dir is empty. Revision is the commit cloned.
func ReadRepo(ctx context.Context, repo, ref, dir string) (Package, error) {
	if strings.HasPrefix(repo, "-") || strings.HasPrefix(ref, "-") {
		return Package{}, errors.New("repo and ref must not start with -")
	}
	sub := filepath.FromSlash(dir)
	if sub != "" && !filepath.IsLocal(sub) {
		return Package{}, fmt.Errorf("dir %q must stay inside the repo", dir)
	}
	tmp, err := os.MkdirTemp("", "reef-skill-")
	if err != nil {
		return Package{}, fmt.Errorf("make clone folder: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	args := []string{"clone", "--depth", "1"}
	if ref != "" {
		args = append(args, "--branch", ref)
	}
	args = append(args, "--", repo, tmp)
	if out, err := runGit(ctx, "", args...); err != nil {
		return Package{}, fmt.Errorf("clone %q: %s", repo, out)
	}
	rev, err := runGit(ctx, tmp, "rev-parse", "HEAD")
	if err != nil {
		return Package{}, fmt.Errorf("read revision of %q: %s", repo, rev)
	}
	p, err := ReadFolder(filepath.Join(tmp, sub))
	if err != nil {
		return Package{}, fmt.Errorf("repo %q: %w", repo, err)
	}
	p.Revision = strings.TrimSpace(rev)
	return p, nil
}

func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return strings.TrimSpace(out.String()), err
}

var _claudeRuntime = regexp.MustCompile(`\$\{?CLAUDE_[A-Z_]+\}?`)

// Compat says for each backend why p cannot run on it, or "" when it can. The only dependency it
// detects is on Claude's plugin runtime, a CLAUDE_ variable such as ${CLAUDE_PLUGIN_ROOT} in any file.
func (p Package) Compat() map[string]string {
	compat := map[string]string{"claude": "", "codex": ""}
	for _, f := range p.Files {
		if m := _claudeRuntime.Find(f.Body); m != nil {
			compat["codex"] = fmt.Sprintf("%s uses %s, which only Claude sets", f.Path, m)
			break
		}
	}
	return compat
}

// Hash identifies the content of p: its paths and bodies.
func (p Package) Hash() string {
	h := sha256.New()
	for _, f := range slices.SortedFunc(slices.Values(p.Files), func(a, b File) int { return strings.Compare(a.Path, b.Path) }) {
		fmt.Fprintf(h, "%s\x00%d\x00", f.Path, len(f.Body))
		h.Write(f.Body)
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}
