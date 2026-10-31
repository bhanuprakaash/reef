package server

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/bhanuprakaash/reef/memory"
	"github.com/bhanuprakaash/reef/space"
	"github.com/bhanuprakaash/reef/store"
	"github.com/bhanuprakaash/reef/worktree"
)

func (s *server) projectRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /projects", s.listProjects)
	mux.HandleFunc("POST /projects", s.createProject)
	mux.HandleFunc("/projects", methodNotAllowed)
	mux.HandleFunc("GET /projects/{id}", s.getProject)
	mux.HandleFunc("PUT /projects/{id}", s.updateProject)
	mux.HandleFunc("DELETE /projects/{id}", s.deleteProject)
	mux.HandleFunc("/projects/{id}", methodNotAllowed)
	mux.HandleFunc("GET /projects/{id}/memory", s.getProjectMemory)
	mux.HandleFunc("PUT /projects/{id}/memory", s.putProjectMemory)
	mux.HandleFunc("/projects/{id}/memory", methodNotAllowed)
}

// projectDir checks path is an absolute directory and returns it with symbolic
// links resolved, the form the CLI compares its working folder against.
func projectDir(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%w: project path %q: not an absolute path", store.ErrInvalid, path)
	}
	if fi, err := os.Stat(path); err != nil || !fi.IsDir() {
		return "", fmt.Errorf("%w: project path %q: not a directory", store.ErrInvalid, path)
	}
	resolved, err := store.ResolvePath(path)
	if err != nil {
		return "", fmt.Errorf("%w: project path %q: %v", store.ErrInvalid, path, err)
	}
	return resolved, nil
}

func (s *server) listProjects(w http.ResponseWriter, r *http.Request) {
	ps, err := s.st.Projects(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	if ps == nil {
		ps = []store.Project{}
	}
	writeJSON(w, http.StatusOK, ps)
}

func (s *server) createProject(w http.ResponseWriter, r *http.Request) {
	var in store.Project
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	path, err := projectDir(in.Path)
	if err != nil {
		writeError(w, err)
		return
	}
	in.Path = path
	p, err := s.st.CreateProject(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *server) getProject(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	p, err := s.st.Project(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *server) updateProject(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in store.Project
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	in.ID = id
	if in.Path, err = projectDir(in.Path); err != nil {
		writeError(w, err)
		return
	}
	if in.BaseBranch != "" && !worktree.ValidBranch(r.Context(), in.BaseBranch) {
		writeError(w, fmt.Errorf("%w: base branch %q: not a branch name", store.ErrInvalid, in.BaseBranch))
		return
	}
	p, err := s.st.UpdateProject(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *server) getProjectMemory(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	m, err := s.st.ProjectMemory(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (s *server) putProjectMemory(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in store.ProjectMemory
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	in.ProjectID = id
	m, err := s.st.PutProjectMemory(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// deleteProject deletes the project, the worktrees of its work items and the checkpoint refs of
// the turns of all its Spaces, quick sessions included. A worktree with uncommitted changes
// is 409 and nothing is removed; so is a Space whose only project this is with a turn running
// elsewhere, or open in reef chat. A Space with no work item stays, readable, with its other
// projects, if any. Branches are kept. A running turn is stopped first, as for a work item delete. A
// project folder that is gone is not git cleaned up: worktree folders, if any, are just
// removed.
func (s *server) deleteProject(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	p, err := s.st.Project(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	lift, err := s.stopSpaces(r.Context(), store.SpaceFilter{OnlyProjectID: &id})
	if err != nil {
		writeError(w, err)
		return
	}
	defer lift()
	if err := s.spacesInUse(r.Context(), store.SpaceFilter{OnlyProjectID: &id}); err != nil {
		writeError(w, err)
		return
	}
	held, unlock, err := s.holdTargetingItems(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	defer unlock()
	fs, err := s.st.WorkItems(r.Context(), store.WorkItemFilter{ProjectID: &id})
	if err != nil {
		writeError(w, err)
		return
	}
	refs, err := s.checkpointRefs(r.Context(), store.SpaceFilter{ProjectID: &id})
	if err != nil {
		writeError(w, err)
		return
	}
	own, err := s.st.ProjectCheckpointRefs(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	refs[id] = append(refs[id], own...)
	ws, err := s.st.ProjectWorkspaces(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	for _, f := range fs {
		more, err := s.st.WorkItemWorkspaces(r.Context(), f.ID)
		if err != nil {
			writeError(w, err)
			return
		}
		ws = append(ws, more...)
	}
	slices.SortFunc(ws, func(a, b store.Workspace) int { return cmp.Compare(a.ID, b.ID) })
	ws = slices.CompactFunc(ws, func(a, b store.Workspace) bool { return a.ID == b.ID })
	if err := s.removeWorkspaces(r.Context(), ws); err != nil {
		writeError(w, err)
		return
	}
	for _, f := range fs {
		if err := os.RemoveAll(space.WorkRoot(s.dataDir, f.TreeKey())); err != nil {
			writeError(w, err)
			return
		}
	}
	for _, x := range ws {
		x.State, x.Note = store.WorkspaceRemoved, "project "+p.Name+" was deleted"
		if err := s.st.SetWorkspace(r.Context(), x); err != nil {
			writeError(w, err)
			return
		}
	}
	if err := s.st.DeleteProject(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	if err := s.pauseTargetingItems(r.Context(), held, p.Name); err != nil {
		writeError(w, err)
		return
	}
	scopes := []memory.Ref{{Kind: memory.ScopeProject, ID: strconv.FormatInt(id, 10)}}
	for _, f := range fs {
		scopes = append(scopes, memory.Ref{Kind: memory.ScopeWorkItem, ID: strconv.FormatInt(f.ID, 10)})
	}
	for _, scope := range scopes {
		if err := s.ms.RetireScope(r.Context(), scope); err != nil {
			writeError(w, err)
			return
		}
	}
	own = refs[id]
	delete(refs, id)
	if !projectGone(p.Path) {
		if err := worktree.DeleteRefs(context.WithoutCancel(r.Context()), p.Path, own); err != nil {
			writeError(w, err)
			return
		}
	}
	if err := s.deleteRefs(context.WithoutCancel(r.Context()), refs); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// holdTargetingItems stops every Space of the other Projects' assignments that have a target in
// project id, and takes the writer lock of each assignment's folder until the returned func is
// called, so no write lands while the workspace is removed. An assignment with a turn running
// that cannot be stopped is 409. Items homed in the Project are deleted with it, not held.
func (s *server) holdTargetingItems(ctx context.Context, id int64) ([]store.WorkItem, func(), error) {
	targeting, err := s.st.ItemsTargeting(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	var held []store.WorkItem
	var spaces []int64
	for _, f := range targeting {
		if f.ProjectID == id {
			continue
		}
		held = append(held, f)
		tree := []int64{f.ID}
		subs, err := s.st.WorkItems(ctx, store.WorkItemFilter{ParentID: &f.ID})
		if err != nil {
			return nil, nil, err
		}
		for _, sub := range subs {
			tree = append(tree, sub.ID)
		}
		for _, wid := range tree {
			cs, err := s.st.Spaces(ctx, store.SpaceFilter{WorkItemID: &wid})
			if err != nil {
				return nil, nil, err
			}
			for _, c := range cs {
				spaces = append(spaces, c.ID)
			}
		}
	}
	lift := s.stopForDelete(spaces...)
	unlocks := []func(){lift}
	release := func() {
		for _, u := range unlocks {
			u()
		}
	}
	for _, f := range held {
		root := space.WorkRoot(s.dataDir, f.TreeKey())
		unlock, ok := s.trees.TryLock(root)
		if !ok {
			release()
			return nil, nil, fmt.Errorf("%s has a turn running: %w", root, store.ErrConflict)
		}
		unlocks = append(unlocks, unlock)
	}
	return held, release, nil
}

// pauseTargetingItems tells the open Spaces of the held items that the Project is gone and pauses
// them, which puts each item in Needs you. The target and workspace rows stay as the record.
func (s *server) pauseTargetingItems(ctx context.Context, held []store.WorkItem, name string) error {
	for _, f := range held {
		sp, err := s.st.WorkItemActiveSpace(ctx, f.ID)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		note := fmt.Sprintf("Project %s was deleted. Its workspace is removed; its branch stays in the repository. "+
			"Decide how this Work item goes on, then continue the Space.", name)
		if sp.Paused != "" {
			_, err = s.st.CreateMessage(ctx, store.Message{SpaceID: sp.ID, Author: store.ReefAuthor, Body: note})
		} else {
			err = s.pauseWithNote(ctx, sp.ID, "project "+name+" was deleted", note)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
