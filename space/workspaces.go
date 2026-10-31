package space

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/bhanuprakaash/reef/store"
	"github.com/bhanuprakaash/reef/worktree"
)

var _notSlug = regexp.MustCompile(`[^a-z0-9]+`)

// WorkRoot is the one folder the agents of the Work item with tree key key work from; each
// target's workspace is a folder inside it.
func WorkRoot(dataDir, key string) string {
	return filepath.Join(dataDir, "worktrees", "item-"+key)
}

// WorkspaceDir is where the workspace of the Work item with tree key key for project p lives.
func WorkspaceDir(dataDir, key string, p store.Project) string {
	slug := strings.Trim(_notSlug.ReplaceAllString(strings.ToLower(p.Name), "-"), "-")
	if slug == "" {
		slug = "project"
	}
	return filepath.Join(WorkRoot(dataDir, key), strconv.FormatInt(p.ID, 10)+"-"+slug)
}

// EnsureWorkspaces makes the workspace of every target of item, one per repository, and returns
// their rows. Each row is recorded as `creating` before git runs, so a cut creation is found
// again and finished in the same folder, keeping its edits. A target whose base branch or
// repository cannot be found is `paused` with a note, and the others go on. A base that could
// only be taken from the local branch is `ready` with a note saying origin was not refreshed.
// Notes are also posted in the item's active Space. A failure that is not about the repository
// leaves the row `creating` and is returned after the other targets are done.
func EnsureWorkspaces(ctx context.Context, st *store.Store, trees *Trees, dataDir string, item store.WorkItem) ([]store.Workspace, error) {
	topID, err := strconv.ParseInt(item.TreeKey(), 10, 64)
	if err != nil {
		return nil, err
	}
	var errs []error
	for _, t := range item.Targets {
		p, err := st.Project(ctx, t.ProjectID)
		if err != nil {
			return nil, err
		}
		w, err := st.EnsureWorkspace(ctx, store.Workspace{
			ProjectID: p.ID, WorkItemID: &topID, Dir: WorkspaceDir(dataDir, item.TreeKey(), p),
			Branch: worktree.Branch(item.TreeKey()), Base: p.BaseBranch,
		})
		if err != nil {
			return nil, err
		}
		if err := ensureWorkspace(ctx, st, trees, p, w, topID); err != nil {
			if ctx.Err() != nil {
				return nil, err
			}
			errs = append(errs, fmt.Errorf("workspace of %s: %w", p.Name, err))
		}
	}
	rows, err := st.WorkItemWorkspaces(ctx, topID)
	if err != nil {
		return nil, err
	}
	return rows, errors.Join(errs...)
}

// targetLines tells an agent where each repository of item f is, relative to its working
// folder, on which branch and from which base. A pull request you open targets that base.
func targetLines(ctx context.Context, st *store.Store, b *strings.Builder, f store.WorkItem) error {
	topID, err := strconv.ParseInt(f.TreeKey(), 10, 64)
	if err != nil {
		return err
	}
	ws, err := st.WorkItemWorkspaces(ctx, topID)
	if err != nil {
		return err
	}
	if len(ws) == 0 {
		return nil
	}
	b.WriteString("Repositories you may change, each in its own folder under your working folder; " +
		"a pull request targets the base branch:\n")
	for _, w := range ws {
		p, err := st.Project(ctx, w.ProjectID)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "- %s: %s/ on branch %s", p.Name, filepath.Base(w.Dir), w.Branch)
		base := cmp.Or(w.Base, w.BaseRef)
		switch {
		case w.State == store.WorkspacePaused:
			fmt.Fprintf(b, ", paused: %s", w.Note)
		case w.BaseSource != "":
			fmt.Fprintf(b, ", base %s (%s)", base, w.BaseSource)
			if w.Note != "" {
				fmt.Fprintf(b, ": %s", w.Note)
			}
		default:
			fmt.Fprintf(b, ", base %s", base)
		}
		b.WriteString("\n")
	}
	return nil
}

// readyWorkspaces lists the workspaces of the item with tree key key that can be worked in.
func readyWorkspaces(ctx context.Context, st *store.Store, key string) ([]store.Workspace, error) {
	topID, err := strconv.ParseInt(key, 10, 64)
	if err != nil {
		return nil, err
	}
	all, err := st.WorkItemWorkspaces(ctx, topID)
	if err != nil {
		return nil, err
	}
	var ready []store.Workspace
	for _, w := range all {
		if w.State == store.WorkspaceReady {
			ready = append(ready, w)
		}
	}
	return ready, nil
}

func ensureWorkspace(ctx context.Context, st *store.Store, trees *Trees, p store.Project, w store.Workspace, topID int64) error {
	if _, err := os.Stat(filepath.Join(w.Dir, ".git")); err == nil && w.State == store.WorkspaceReady {
		return nil
	}
	unlock, err := trees.Lock(ctx, "add "+w.Dir)
	if err != nil {
		return err
	}
	defer unlock()
	before := w.Note
	w.Base = p.BaseBranch
	b, err := worktree.Create(ctx, p.Path, w.Dir, w.Branch, p.BaseBranch)
	switch {
	case errors.Is(err, worktree.ErrNoBase) || errors.Is(err, worktree.ErrNoRepo):
		w.State, w.Note = store.WorkspacePaused, oneLine(err)
		err = nil
	case err != nil:
		w.State, w.Note = store.WorkspaceCreating, oneLine(err)
		if serr := st.SetWorkspace(ctx, w); serr != nil {
			return errors.Join(err, serr)
		}
		return err
	default:
		w.State, w.Note = store.WorkspaceReady, ""
		if b.Ref != "" {
			w.BaseRef, w.BaseSource, w.Note = b.Ref, b.Source, b.Note
		}
	}
	if err := st.SetWorkspace(ctx, w); err != nil {
		return err
	}
	if w.Note != "" && w.Note != before {
		return noteWorkspace(ctx, st, topID, p, w.Note)
	}
	return nil
}

func noteWorkspace(ctx context.Context, st *store.Store, topID int64, p store.Project, note string) error {
	sp, err := st.WorkItemActiveSpace(ctx, topID)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = st.CreateMessage(ctx, store.Message{
		SpaceID: sp.ID, Author: store.ReefAuthor, Body: fmt.Sprintf("Workspace for %s: %s", p.Name, note),
	})
	return err
}

func oneLine(err error) string {
	line, _, _ := strings.Cut(err.Error(), "\n")
	return line
}
