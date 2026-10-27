package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/bhanuprakaash/reef/memory"
	"github.com/bhanuprakaash/reef/space"
	"github.com/bhanuprakaash/reef/store"
	"github.com/bhanuprakaash/reef/worktree"
)

func (s *server) workItemRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /work-items", s.listWorkItems)
	mux.HandleFunc("POST /work-items", s.createWorkItem)
	mux.HandleFunc("/work-items", methodNotAllowed)
	mux.HandleFunc("GET /work-items/{id}", s.getWorkItem)
	mux.HandleFunc("PUT /work-items/{id}", s.updateWorkItem)
	mux.HandleFunc("DELETE /work-items/{id}", s.deleteWorkItem)
	mux.HandleFunc("/work-items/{id}", methodNotAllowed)
	mux.HandleFunc("POST /work-items/{id}/close", s.closeWorkItem)
	mux.HandleFunc("/work-items/{id}/close", methodNotAllowed)
	mux.HandleFunc("POST /work-items/{id}/reopen", s.reopenWorkItem)
	mux.HandleFunc("/work-items/{id}/reopen", methodNotAllowed)
	mux.HandleFunc("GET /work-items/{id}/brief", s.getBrief)
	mux.HandleFunc("PUT /work-items/{id}/brief", s.putBrief)
	mux.HandleFunc("/work-items/{id}/brief", methodNotAllowed)
	mux.HandleFunc("GET /work-items/{id}/brief/versions", s.listBriefVersions)
	mux.HandleFunc("/work-items/{id}/brief/versions", methodNotAllowed)
	mux.HandleFunc("GET /work-items/{id}/notes/{role}", s.getRoleNotes)
	mux.HandleFunc("PUT /work-items/{id}/notes/{role}", s.putRoleNotes)
	mux.HandleFunc("DELETE /work-items/{id}/notes/{role}", s.deleteRoleNotes)
	mux.HandleFunc("/work-items/{id}/notes/{role}", methodNotAllowed)
	mux.HandleFunc("GET /work-items/{id}/worktree", s.getWorkItemWorktree)
	mux.HandleFunc("POST /work-items/{id}/worktree", s.ensureWorkItemWorktree)
	mux.HandleFunc("/work-items/{id}/worktree", methodNotAllowed)
	mux.HandleFunc("GET /work-items/{id}/diff", s.getWorkItemDiff)
	mux.HandleFunc("/work-items/{id}/diff", methodNotAllowed)
	mux.HandleFunc("GET /work-items/{id}/pr", s.getWorkItemPR)
	mux.HandleFunc("/work-items/{id}/pr", methodNotAllowed)
	mux.HandleFunc("POST /work-items/{id}/pr/merge", s.mergeWorkItemPR)
	mux.HandleFunc("/work-items/{id}/pr/merge", methodNotAllowed)
}

func (s *server) listWorkItems(w http.ResponseWriter, r *http.Request) {
	f := store.WorkItemFilter{Status: r.URL.Query().Get("status"), Type: r.URL.Query().Get("type")}
	var err error
	if f.ProjectID, err = queryID(r, "project"); err != nil {
		writeError(w, err)
		return
	}
	if f.ParentID, err = queryID(r, "parent"); err != nil {
		writeError(w, err)
		return
	}
	ws, err := s.st.WorkItems(r.Context(), f)
	if err != nil {
		writeError(w, err)
		return
	}
	if ws == nil {
		ws = []store.WorkItem{}
	}
	writeJSON(w, http.StatusOK, ws)
}

func (s *server) createWorkItem(w http.ResponseWriter, r *http.Request) {
	var in store.WorkItem
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	f, err := s.st.CreateWorkItem(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, f)
}

func (s *server) getWorkItem(w http.ResponseWriter, r *http.Request) {
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
	writeJSON(w, http.StatusOK, f)
}

func (s *server) updateWorkItem(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in store.WorkItem
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	in.ID = id
	f, err := s.st.UpdateWorkItem(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, f)
}

// closeWorkItem closes the work item and all its Spaces, and stops their agents. A Space with
// a turn running is 409 and nothing is closed.
func (s *server) closeWorkItem(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	cs, err := s.st.Spaces(r.Context(), store.SpaceFilter{WorkItemID: &id})
	if err != nil {
		writeError(w, err)
		return
	}
	for _, c := range cs {
		if l := s.lookup(c.ID); l != nil {
			l.mu.Lock()
			busy := l.state != _idle
			l.mu.Unlock()
			if busy {
				writeError(w, fmt.Errorf("space %d has a turn running: %w", c.ID, store.ErrConflict))
				return
			}
		}
	}
	lift, err := s.stopSpaces(r.Context(), store.SpaceFilter{WorkItemID: &id})
	if err != nil {
		writeError(w, err)
		return
	}
	defer lift()
	if err := s.st.CloseWorkItem(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) reopenWorkItem(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.st.ReopenWorkItem(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteWorkItem deletes the work item, its sub-items, its worktree and the checkpoint refs of
// its turns, and retires the memory items of the item and its sub-items. The branch is kept. A running turn of its Spaces is stopped first, as for a Space delete.
// A worktree with uncommitted changes is 409 and nothing is removed; so is a Space with a
// turn running elsewhere, or open in reef chat. The worktree goes first: it is the step that
// can refuse or fail, and the next wake makes it again. The refs go last, after the rows that
// name them. A project folder that is gone is not git cleaned up: its worktree folder, if
// any, is just removed.
func (s *server) deleteWorkItem(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	lift, err := s.stopSpaces(r.Context(), store.SpaceFilter{WorkItemID: &id})
	if err != nil {
		writeError(w, err)
		return
	}
	defer lift()
	f, err := s.st.WorkItem(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	p, err := s.st.Project(r.Context(), f.ProjectID)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.spacesInUse(r.Context(), store.SpaceFilter{WorkItemID: &id}); err != nil {
		writeError(w, err)
		return
	}
	refs, err := s.checkpointRefs(r.Context(), store.SpaceFilter{WorkItemID: &id})
	if err != nil {
		writeError(w, err)
		return
	}
	gone := projectGone(p.Path)
	dir := worktree.Path(s.dataDir, p.ID, f.TreeKey())
	if gone {
		err = os.RemoveAll(dir)
	} else {
		err = s.removeWorkItemTree(r.Context(), p, f)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	subs, err := s.st.WorkItems(r.Context(), store.WorkItemFilter{ParentID: &id})
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.st.DeleteWorkItem(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	for _, gone := range append(subs, f) {
		scope := memory.Ref{Kind: memory.ScopeWorkItem, ID: strconv.FormatInt(gone.ID, 10)}
		if err := s.ms.RetireScope(r.Context(), scope); err != nil {
			writeError(w, err)
			return
		}
	}
	if !gone {
		if err := worktree.DeleteRefs(context.WithoutCancel(r.Context()), p.Path, refs); err != nil {
			writeError(w, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// spacesInUse is 409 for the first Space matching f that a `reef chat` has open, or whose
// tree a turn is running on, in this process or another.
func (s *server) spacesInUse(ctx context.Context, f store.SpaceFilter) error {
	cs, err := s.st.Spaces(ctx, f)
	if err != nil {
		return err
	}
	for _, c := range cs {
		if space.InUse(s.dataDir, c.ID) {
			return fmt.Errorf("space %d is open elsewhere: %w", c.ID, store.ErrConflict)
		}
		dir, _, err := space.WorkDir(ctx, s.st, s.dataDir, c.ID)
		if err != nil {
			return err
		}
		unlock, ok := s.trees.TryLock(dir)
		if !ok {
			return fmt.Errorf("%s has a turn running: %w", dir, store.ErrConflict)
		}
		unlock()
	}
	return nil
}

// projectGone reports whether a project's folder no longer exists: git cannot clean up its
// worktrees or checkpoint refs, only their folders can be removed.
func projectGone(path string) bool {
	_, err := os.Stat(path)
	return errors.Is(err, os.ErrNotExist)
}

// checkpointRefs lists the checkpoint refs of the turns of the Spaces that match f.
func (s *server) checkpointRefs(ctx context.Context, f store.SpaceFilter) ([]string, error) {
	cs, err := s.st.Spaces(ctx, f)
	if err != nil {
		return nil, err
	}
	var refs []string
	for _, c := range cs {
		ts, err := s.st.Turns(ctx, store.TurnFilter{SpaceID: &c.ID})
		if err != nil {
			return nil, err
		}
		for _, t := range ts {
			if t.Checkpoint != nil {
				refs = append(refs, *t.Checkpoint)
			}
		}
	}
	return refs, nil
}

// checkWorkItemTree is 409 when the worktree of work item f of project p has uncommitted changes.
func (s *server) checkWorkItemTree(ctx context.Context, p store.Project, f store.WorkItem) error {
	dir := worktree.Path(s.dataDir, p.ID, f.TreeKey())
	return treeConflict(f, dir, worktree.Clean(ctx, dir))
}

// removeWorkItemTree removes the worktree of work item f of project p; 409 when it has
// uncommitted changes.
func (s *server) removeWorkItemTree(ctx context.Context, p store.Project, f store.WorkItem) error {
	dir := worktree.Path(s.dataDir, p.ID, f.TreeKey())
	return treeConflict(f, dir, worktree.Remove(ctx, p.Path, dir))
}

func treeConflict(f store.WorkItem, dir string, err error) error {
	if errors.Is(err, worktree.ErrDirty) {
		return fmt.Errorf("%w: work item %d has uncommitted changes in %s; commit or discard them first",
			store.ErrConflict, f.ID, dir)
	}
	return err
}

// getWorkItemWorktree returns the path of the work item's worktree, or an empty path when it
// has none yet. It never makes one.
func (s *server) getWorkItemWorktree(w http.ResponseWriter, r *http.Request) {
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
	p, err := s.st.Project(r.Context(), f.ProjectID)
	if err != nil {
		writeError(w, err)
		return
	}
	dir := worktree.Path(s.dataDir, p.ID, f.TreeKey())
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		dir = ""
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": dir})
}

// ensureWorkItemWorktree makes the work item's worktree, the same way the first wake of one of
// its Spaces would, and returns its path. A worktree or branch that already exists is
// reused.
func (s *server) ensureWorkItemWorktree(w http.ResponseWriter, r *http.Request) {
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
	p, err := s.st.Project(r.Context(), f.ProjectID)
	if err != nil {
		writeError(w, err)
		return
	}
	dir := worktree.Path(s.dataDir, p.ID, f.TreeKey())
	if err := space.EnsureWorktree(r.Context(), s.trees, p, dir, f.TreeKey()); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": dir})
}

// getWorkItemDiff returns the work item worktree's changes, uncommitted and untracked files
// included, against the commit its branch and the project's base branch last shared, or against
// the worktree's own HEAD with ?scope=current. A work item with no worktree yet has no changes.
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
	p, err := s.st.Project(r.Context(), f.ProjectID)
	if err != nil {
		writeError(w, err)
		return
	}
	dir := worktree.Path(s.dataDir, p.ID, f.TreeKey())
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		writeJSON(w, http.StatusOK, []worktree.FileDiff{})
		return
	}
	base := "HEAD"
	if r.URL.Query().Get("scope") != "current" {
		base, err = worktree.Base(r.Context(), p.Path, dir, p.BaseBranch)
		if err != nil {
			writeError(w, err)
			return
		}
	}
	diffs, err := worktree.Diff(r.Context(), dir, base)
	if err != nil {
		writeError(w, err)
		return
	}
	if diffs == nil {
		diffs = []worktree.FileDiff{}
	}
	writeJSON(w, http.StatusOK, diffs)
}

func (s *server) getBrief(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	b, err := s.st.Brief(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (s *server) putBrief(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in store.Brief
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	in.WorkItemID = id
	b, err := s.st.PutBrief(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

func (s *server) listBriefVersions(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	bs, err := s.st.BriefVersions(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	if bs == nil {
		bs = []store.Brief{}
	}
	writeJSON(w, http.StatusOK, bs)
}

func (s *server) getRoleNotes(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	n, err := s.st.RoleNotes(r.Context(), id, r.PathValue("role"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (s *server) putRoleNotes(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in store.RoleNotes
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	in.WorkItemID, in.Role = id, r.PathValue("role")
	n, err := s.st.PutRoleNotes(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (s *server) deleteRoleNotes(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.st.DeleteRoleNotes(r.Context(), id, r.PathValue("role")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
