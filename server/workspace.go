package server

import (
	"net/http"

	"github.com/bhanuprakaash/reef/space"
)

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
	top := f.ID
	if f.ParentID != nil {
		top = *f.ParentID
	}
	ws, err := s.st.WorkItemWorkspaces(r.Context(), top)
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
