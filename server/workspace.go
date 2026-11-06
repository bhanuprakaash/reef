package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/bhanuprakaash/reef/space"
	"github.com/bhanuprakaash/reef/store"
	"github.com/bhanuprakaash/reef/worktree"
)

// itemWorkspaces lists the workspaces recorded for the targets of f, the parent's for a
// sub-item.
func (s *server) itemWorkspaces(ctx context.Context, f store.WorkItem) ([]store.Workspace, error) {
	top := f.ID
	if f.ParentID != nil {
		top = *f.ParentID
	}
	return s.st.WorkItemWorkspaces(ctx, top)
}

// listWorkItemWorkspaces returns the workspaces recorded for the work item's targets, the
// parent's for a sub-item. It never makes one.
func (s *server) listWorkItemWorkspaces(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	f, err := s.st.WorkItem(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	ws, err := s.itemWorkspaces(r.Context(), f)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ws)
}

// ensureWorkItemWorkspaces makes the workspace of every target of the work item and returns
// their rows, including a paused target with its note. Workspaces already made are left as
// they are, so a retry after a cut creation finishes it in the same folder.
func (s *server) ensureWorkItemWorkspaces(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	f, err := s.st.WorkItem(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	ws, err := space.EnsureWorkspaces(r.Context(), s.st, s.trees, s.dataDir, f)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ws)
}

// spaceWorkspace is a quick fix of a conversation as the window and delivery read it: where it
// is, what it started from and what it holds now.
type spaceWorkspace struct {
	store.Workspace
	ProjectName string              `json:"projectName"`
	Head        string              `json:"head"`
	Dirty       bool                `json:"dirty"`
	Requests    []store.RepoRequest `json:"requests"`
}

// listSpaceWorkspaces returns the quick fixes the conversation holds, abandoned ones included,
// with the head commit, whether the tree has uncommitted changes and the pull or merge
// requests opened from it. It never makes one.
func (s *server) listSpaceWorkspaces(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if _, err := s.st.Space(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	ws, err := s.st.SpaceWorkspaces(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]spaceWorkspace, 0, len(ws))
	for _, wk := range ws {
		p, err := s.st.Project(r.Context(), wk.ProjectID)
		if err != nil {
			writeError(w, err)
			return
		}
		reqs, err := s.st.WorkspaceRequests(r.Context(), wk.ID)
		if err != nil {
			writeError(w, err)
			return
		}
		sw := spaceWorkspace{Workspace: wk, ProjectName: p.Name, Requests: reqs}
		if wk.State == store.WorkspaceReady {
			sw.Head = worktree.Head(r.Context(), wk.Dir)
			sw.Dirty, _ = worktree.Dirty(r.Context(), wk.Dir)
		}
		out = append(out, sw)
	}
	writeJSON(w, http.StatusOK, out)
}

// abandonSpaceWorkspace gives up a quick fix: its tree is removed and its branch is kept, so
// committed work stays in the repository, and uncommitted changes are saved first (see
// cleanWorkspace). A turn of the conversation running is 409 and the tree stays, as it does
// when the changes could not be saved.
func (s *server) abandonSpaceWorkspace(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	wid, err := strconv.ParseInt(r.PathValue("workspaceId"), 10, 64)
	if err != nil {
		writeError(w, fmt.Errorf("%w: workspace id %q is not a number", store.ErrInvalid, r.PathValue("workspaceId")))
		return
	}
	ws, err := s.st.SpaceWorkspaces(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	i := slices.IndexFunc(ws, func(wk store.Workspace) bool { return wk.ID == wid })
	if i < 0 {
		writeError(w, fmt.Errorf("%w: Space %d has no workspace %d", store.ErrNotFound, id, wid))
		return
	}
	if err := s.cleanWorkspace(r.Context(), ws[i]); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// getWorkItemDiff returns the changes of every ready workspace of the work item, uncommitted
// and untracked files included, against the commit its branch and the project's base branch last
// shared, or against the workspace's own HEAD with ?scope=current. A file's path starts with the
// folder of its workspace, so each repository is a section of its own. A work item with no
// workspace yet has no changes.
func (s *server) getWorkItemDiff(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	f, err := s.st.WorkItem(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	ws, err := s.itemWorkspaces(r.Context(), f)
	if err != nil {
		writeError(w, err)
		return
	}
	diffs := []worktree.FileDiff{}
	for _, wk := range ws {
		if _, err := os.Stat(filepath.Join(wk.Dir, ".git")); err != nil {
			continue
		}
		base := "HEAD"
		if r.URL.Query().Get("scope") != "current" {
			p, err := s.st.Project(r.Context(), wk.ProjectID)
			if err == nil {
				base, err = worktree.Base(r.Context(), p.Path, wk.Dir, p.BaseBranch)
			}
			if err != nil {
				writeError(w, err)
				return
			}
		}
		files, err := worktree.Diff(r.Context(), wk.Dir, base)
		if err != nil {
			writeError(w, err)
			return
		}
		for _, d := range files {
			d.Path = path.Join(filepath.Base(wk.Dir), d.Path)
			diffs = append(diffs, d)
		}
	}
	writeJSON(w, http.StatusOK, diffs)
}

// removeWorkspaces removes the worktree of every workspace in ws, keeping the branches. A
// workspace with uncommitted changes is 409 and nothing is removed. A project folder that is
// gone is not git cleaned up: the workspace folder is just removed.
func (s *server) removeWorkspaces(ctx context.Context, ws []store.Workspace) error {
	projects := map[int64]store.Project{}
	for _, w := range ws {
		p, err := s.st.Project(ctx, w.ProjectID)
		if err != nil {
			return err
		}
		projects[w.ProjectID] = p
		if !projectGone(p.Path) {
			if err := treeConflict(w, worktree.Clean(ctx, w.Dir)); err != nil {
				return err
			}
		}
	}
	for _, w := range ws {
		p := projects[w.ProjectID]
		var err error
		if projectGone(p.Path) {
			err = os.RemoveAll(w.Dir)
		} else {
			err = treeConflict(w, worktree.Remove(ctx, p.Path, w.Dir))
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func treeConflict(w store.Workspace, err error) error {
	if errors.Is(err, worktree.ErrDirty) {
		what := fmt.Sprintf("workspace %d", w.ID)
		if w.WorkItemID != nil {
			what = fmt.Sprintf("work item %d", *w.WorkItemID)
		}
		return fmt.Errorf("%w: %s has uncommitted changes in %s; commit or discard them first",
			store.ErrConflict, what, w.Dir)
	}
	return err
}

// checkpointRefs lists the checkpoint refs of the turns of the Spaces that match f, by the
// project whose repository holds them.
func (s *server) checkpointRefs(ctx context.Context, f store.SpaceFilter) (map[int64][]string, error) {
	cs, err := s.st.Spaces(ctx, f)
	if err != nil {
		return nil, err
	}
	refs := map[int64][]string{}
	for _, c := range cs {
		ts, err := s.st.Turns(ctx, store.TurnFilter{SpaceID: &c.ID})
		if err != nil {
			return nil, err
		}
		for _, t := range ts {
			for _, c := range t.Checkpoints {
				refs[c.ProjectID] = append(refs[c.ProjectID], c.Ref)
			}
		}
	}
	return refs, nil
}

// deleteRefs deletes the checkpoint refs in the repository of each project. A project folder
// that is gone has none to clean up.
func (s *server) deleteRefs(ctx context.Context, refs map[int64][]string) error {
	for id, rs := range refs {
		p, err := s.st.Project(ctx, id)
		if err != nil {
			return err
		}
		if projectGone(p.Path) {
			continue
		}
		if err := worktree.DeleteRefs(ctx, p.Path, rs); err != nil {
			return err
		}
	}
	return nil
}
