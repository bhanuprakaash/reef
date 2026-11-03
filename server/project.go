package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/bhanuprakaash/reef/memory"
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

const _projectDeletedReason = "project deleted: "

func projectDeletedNote(name string) string {
	return fmt.Sprintf("Project %s was deleted. Its repository, workspaces and unfinished changes stay as they are. "+
		"Remove it from this Work item's repositories, then continue the Space.", name)
}

// deleteProject deletes the project in steps, each safe to run again. It first records that
// deletion began, so no work is admitted against the Project from then on, then stops the
// turns that run against it, pauses the Work items that still need it, retires its own memory
// and last records it deleted. The repository folder, worktrees, workspace rows, checkpoint refs,
// branches and every conversation stay. A crash between the steps is finished at the next start.
func (s *server) deleteProject(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if _, err := s.st.MarkProjectDeleting(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	if err := s.finishProjectDelete(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// resumeProjectDeletes finishes each deletion a restart cut off. A Project that cannot be
// finished stays deleting, so nothing is admitted against it, and is tried again at the next
// start or delete.
func (s *server) resumeProjectDeletes() {
	ps, err := s.st.DeletingProjects(s.ctx)
	if err != nil {
		log.Printf("list deleting projects: %v", err)
		return
	}
	for _, p := range ps {
		if err := s.finishProjectDelete(s.ctx, p.ID); err != nil {
			log.Printf("finish deleting project %d: %v", p.ID, err)
		}
	}
}

func (s *server) finishProjectDelete(ctx context.Context, id int64) error {
	p, err := s.st.Project(ctx, id)
	if err != nil {
		return err
	}
	items, err := s.st.ItemsTargeting(ctx, id)
	if err != nil {
		return err
	}
	spaces, err := s.projectSpaces(ctx, id, items)
	if err != nil {
		return err
	}
	lift := s.stopForDelete(spaces...)
	defer lift()
	for _, f := range items {
		if err := s.holdItem(ctx, f, p.Name); err != nil {
			return err
		}
	}
	if err := s.ms.RetireScope(ctx, memory.Ref{Kind: memory.ScopeProject, ID: strconv.FormatInt(id, 10)}); err != nil {
		return err
	}
	_, err = s.st.MarkProjectDeleted(ctx, id)
	return err
}

// projectSpaces lists the Spaces whose turns run against project id: the Spaces of the items and
// their sub-items, and the Spaces holding a workspace in it.
func (s *server) projectSpaces(ctx context.Context, id int64, items []store.WorkItem) ([]int64, error) {
	var ids []int64
	for _, f := range items {
		tree := []int64{f.ID}
		subs, err := s.st.WorkItems(ctx, store.WorkItemFilter{ParentID: &f.ID})
		if err != nil {
			return nil, err
		}
		for _, sub := range subs {
			tree = append(tree, sub.ID)
		}
		for _, wid := range tree {
			cs, err := s.st.Spaces(ctx, store.SpaceFilter{WorkItemID: &wid})
			if err != nil {
				return nil, err
			}
			for _, c := range cs {
				ids = append(ids, c.ID)
			}
		}
	}
	ws, err := s.st.ProjectWorkspaces(ctx, id)
	if err != nil {
		return nil, err
	}
	for _, w := range ws {
		if w.SpaceID != nil {
			ids = append(ids, *w.SpaceID)
		}
	}
	slices.Sort(ids)
	return slices.Compact(ids), nil
}

// holdItem keeps item f from going on without the deleted Project, until the user adjusts its
// repositories: its open Space is paused with a note, which puts the item in Needs you, and a
// queued item with no Space goes back to the backlog, which withdraws its approval. Its target
// and workspace rows stay as the record.
func (s *server) holdItem(ctx context.Context, f store.WorkItem, name string) error {
	reason := _projectDeletedReason + name
	sp, err := s.st.WorkItemActiveSpace(ctx, f.ID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		if f.Status == "queued" {
			_, err = s.st.MoveWorkItem(ctx, f.ID, "backlog", store.ByUser)
			return err
		}
		return nil
	case err != nil:
		return err
	case sp.Paused == reason:
		return nil
	case sp.Paused != "":
		_, err = s.st.CreateMessage(ctx, store.Message{SpaceID: sp.ID, Author: store.ReefAuthor, Body: projectDeletedNote(name)})
		return err
	}
	return s.pauseWithNote(ctx, sp.ID, reason, projectDeletedNote(name))
}
