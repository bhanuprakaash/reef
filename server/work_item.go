package server

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/bhanuprakaash/reef/memory"
	"github.com/bhanuprakaash/reef/space"
	"github.com/bhanuprakaash/reef/store"
)

func (s *server) workItemRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /work-items", s.listWorkItems)
	mux.HandleFunc("POST /work-items", s.createWorkItem)
	mux.HandleFunc("/work-items", methodNotAllowed)
	mux.HandleFunc("GET /work-items/{id}", s.getWorkItem)
	mux.HandleFunc("PUT /work-items/{id}", s.updateWorkItem)
	mux.HandleFunc("PATCH /work-items/{id}", s.moveWorkItem)
	mux.HandleFunc("DELETE /work-items/{id}", s.deleteWorkItem)
	mux.HandleFunc("/work-items/{id}", methodNotAllowed)
	mux.HandleFunc("PUT /work-items/{id}/queue-position", s.moveInQueue)
	mux.HandleFunc("/work-items/{id}/queue-position", methodNotAllowed)
	mux.HandleFunc("POST /work-items/{id}/approve", s.approveWorkItem)
	mux.HandleFunc("/work-items/{id}/approve", methodNotAllowed)
	mux.HandleFunc("PUT /work-items/{id}/participants", s.putParticipants)
	mux.HandleFunc("/work-items/{id}/participants", methodNotAllowed)
	mux.HandleFunc("POST /work-items/{id}/owner", s.transferOwner)
	mux.HandleFunc("/work-items/{id}/owner", methodNotAllowed)
	mux.HandleFunc("GET /work-items/{id}/targets", s.listTargets)
	mux.HandleFunc("POST /work-items/{id}/targets", s.addTarget)
	mux.HandleFunc("DELETE /work-items/{id}/targets/{projectId}", s.deleteTarget)
	mux.HandleFunc("/work-items/{id}/targets", methodNotAllowed)
	mux.HandleFunc("GET /work-items/{id}/brief", s.getBrief)
	mux.HandleFunc("PUT /work-items/{id}/brief", s.putBrief)
	mux.HandleFunc("/work-items/{id}/brief", methodNotAllowed)
	mux.HandleFunc("GET /work-items/{id}/brief/versions", s.listBriefVersions)
	mux.HandleFunc("/work-items/{id}/brief/versions", methodNotAllowed)
	mux.HandleFunc("GET /work-items/{id}/notes/{role}", s.getRoleNotes)
	mux.HandleFunc("PUT /work-items/{id}/notes/{role}", s.putRoleNotes)
	mux.HandleFunc("DELETE /work-items/{id}/notes/{role}", s.deleteRoleNotes)
	mux.HandleFunc("/work-items/{id}/notes/{role}", methodNotAllowed)
	mux.HandleFunc("GET /work-items/{id}/workspaces", s.listWorkItemWorkspaces)
	mux.HandleFunc("POST /work-items/{id}/workspaces", s.ensureWorkItemWorkspaces)
	mux.HandleFunc("/work-items/{id}/workspaces", methodNotAllowed)
	mux.HandleFunc("GET /work-items/{id}/diff", s.getWorkItemDiff)
	mux.HandleFunc("/work-items/{id}/diff", methodNotAllowed)
	mux.HandleFunc("GET /work-items/{id}/proof", s.getWorkItemProof)
	mux.HandleFunc("/work-items/{id}/proof", methodNotAllowed)
	mux.HandleFunc("GET /work-items/{id}/proof/screenshots/{n}", s.getProofScreenshot)
	mux.HandleFunc("/work-items/{id}/proof/screenshots/{n}", methodNotAllowed)
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
	in.Status, in.QueuePosition = "", nil
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

// moveWorkItem sets the status of the work item as the user; a move the user may not make is 400.
// note is the reason for sending an item in review back to the queue.
func (s *server) moveWorkItem(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in struct {
		Status string `json:"status"`
		Note   string `json:"note"`
	}
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	item, err := s.st.WorkItem(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	if in.Status == "dropped" && item.ParentID == nil {
		lift, err := s.stopSpaces(r.Context(), store.SpaceFilter{WorkItemID: &id})
		if err != nil {
			writeError(w, err)
			return
		}
		defer lift()
	}
	var f store.WorkItem
	if item.Status == "review" && item.ParentID == nil {
		f, err = s.decideReview(r.Context(), item, in.Status, in.Note)
	} else {
		f, err = s.st.MoveWorkItem(r.Context(), id, in.Status, store.ByUser)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	s.openStarted(r.Context())
	writeJSON(w, http.StatusOK, f)
}

// decideReview moves the top-level item in review as the user. Done approves the wrap-up the
// lead proposed when it submitted, or closes the Space when there is none. Queued sends the item
// back with note, which reaches the lead when the queue restarts the item. A wrap-up still
// running is 409, so the user decides on what the lead finished. An item with no proof has no
// Space to wrap up or wake: it is only moved.
func (s *server) decideReview(ctx context.Context, item store.WorkItem, to, note string) (store.WorkItem, error) {
	if to != "done" && to != "queued" {
		return s.st.MoveWorkItem(ctx, item.ID, to, store.ByUser)
	}
	proof, err := s.st.Proof(ctx, item.ID)
	if errors.Is(err, store.ErrNotFound) {
		return s.st.MoveWorkItem(ctx, item.ID, to, store.ByUser)
	}
	if err != nil {
		return store.WorkItem{}, err
	}
	if l := s.lookup(proof.SpaceID); l != nil {
		l.mu.Lock()
		busy := l.state != _idle
		l.mu.Unlock()
		if busy {
			return store.WorkItem{}, fmt.Errorf("space %d has a turn running, wait for the wrap-up: %w",
				proof.SpaceID, store.ErrConflict)
		}
	}
	if to == "queued" {
		return s.st.SendBack(ctx, item.ID, note)
	}
	pending, err := s.st.PendingProposals(ctx, &item.ID, nil)
	if err != nil {
		return store.WorkItem{}, err
	}
	if i := slices.IndexFunc(pending, func(p store.Proposal) bool {
		return p.Kind == store.ProposalWrap && p.SpaceID == proof.SpaceID
	}); i >= 0 {
		if _, err := s.approve(ctx, pending[i].ID); err != nil {
			return store.WorkItem{}, err
		}
	} else {
		if err := s.st.CloseSpace(ctx, proof.SpaceID); err != nil {
			return store.WorkItem{}, err
		}
		if l := s.lookup(proof.SpaceID); l != nil {
			s.stopLive(l)
		}
	}
	return s.st.MoveWorkItem(ctx, item.ID, to, store.ByUser)
}

// approveWorkItem is the user authorizing a backlog item with the owner and participants in the
// body (the proposal's where left out) and queuing it. key names this approval: the same key again
// answers 200 with the item as it is, another key for an approved item is 409.
func (s *server) approveWorkItem(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in struct {
		OwnerBotID        *int64  `json:"ownerBotId"`
		ParticipantBotIDs []int64 `json:"participantBotIds"`
		TargetProjectIDs  []int64 `json:"targetProjectIds"`
		Key               string  `json:"key"`
	}
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	f, err := s.st.ApproveWorkItem(r.Context(), id, in.OwnerBotID, in.ParticipantBotIDs, in.TargetProjectIDs, in.Key)
	if err != nil {
		writeError(w, err)
		return
	}
	s.openStarted(r.Context())
	writeJSON(w, http.StatusOK, f)
}

// putParticipants sets the owner and participants of a backlog work item; any other item is 400.
func (s *server) putParticipants(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in struct {
		OwnerBotID        *int64  `json:"ownerBotId"`
		ParticipantBotIDs []int64 `json:"participantBotIds"`
	}
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	f, err := s.st.SetWorkItemParticipants(r.Context(), id, in.OwnerBotID, in.ParticipantBotIDs)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (s *server) listTargets(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	ts, err := s.st.WorkItemTargets(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ts)
}

// addTarget lets the user add a Project's repository to a top-level work item. The item's Space
// gets a message naming it. A sub-item or an unknown Project is 400, a repeated Project 409.
func (s *server) addTarget(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in struct {
		ProjectID int64 `json:"projectId"`
	}
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	t, err := s.st.AddWorkItemTarget(r.Context(), id, in.ProjectID, store.ByUser)
	if err != nil {
		writeError(w, err)
		return
	}
	if s.begin() {
		go func() {
			defer s.wg.Done()
			s.openAddedTarget(s.ctx, id, in.ProjectID)
		}()
	}
	writeJSON(w, http.StatusCreated, t)
}

// deleteTarget takes a Project's repository off a backlog work item. A started item or a Project
// with a workspace is 409; the home Project and a Project that is not a target are 400.
func (s *server) deleteTarget(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	projectID, err := strconv.ParseInt(r.PathValue("projectId"), 10, 64)
	if err != nil {
		writeError(w, fmt.Errorf("%w: project id", store.ErrInvalid))
		return
	}
	if err := s.st.RemoveWorkItemTarget(r.Context(), id, projectID); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// moveInQueue puts the queued work item at a position in the queue; a position outside
// the queue, or an item that is not queued, is 400.
func (s *server) moveInQueue(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in struct {
		Position int64 `json:"position"`
	}
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	f, err := s.st.MoveInQueue(r.Context(), id, in.Position)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, f)
}

// deleteWorkItem deletes the work item, its sub-items, its worktree and the checkpoint refs of
// its turns, and retires the memory items of the item and its sub-items. The branch is kept. A running turn of its Spaces is stopped first, as for a Space delete.
// A worktree with uncommitted changes is 409 and nothing is removed; so is a Space with a
// turn running elsewhere, or open in reef chat. The worktree goes first: it is the step that
// can refuse or fail, and the next wake makes it again. The memory items are retired before the
// row goes, so a failure leaves the item to delete again. The refs go last, after the rows that
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
	if err := s.spacesInUse(r.Context(), store.SpaceFilter{WorkItemID: &id}); err != nil {
		writeError(w, err)
		return
	}
	refs, err := s.checkpointRefs(r.Context(), store.SpaceFilter{WorkItemID: &id})
	if err != nil {
		writeError(w, err)
		return
	}
	if f.ParentID == nil {
		ws, err := s.st.WorkItemWorkspaces(r.Context(), f.ID)
		if err == nil {
			err = s.removeWorkspaces(r.Context(), ws)
		}
		if err == nil {
			err = os.RemoveAll(space.WorkRoot(s.dataDir, f.TreeKey()))
		}
		if err != nil {
			writeError(w, err)
			return
		}
	}
	subs, err := s.st.WorkItems(r.Context(), store.WorkItemFilter{ParentID: &id})
	if err != nil {
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
	if err := s.st.DeleteWorkItem(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	s.openStarted(r.Context())
	if err := s.deleteRefs(context.WithoutCancel(r.Context()), refs); err != nil {
		writeError(w, err)
		return
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

// getWorkItemProof returns what the lead submitted the work item with; 404 before it submitted.
func (s *server) getWorkItemProof(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	p, err := s.st.Proof(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// getProofScreenshot serves screenshot n (from 0) of the work item's proof. Only an image file
// under the data folder is served, whatever path the lead submitted.
func (s *server) getProofScreenshot(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	p, err := s.st.Proof(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	n, err := strconv.Atoi(r.PathValue("n"))
	if err != nil || n < 0 || n >= len(p.Screenshots) {
		writeError(w, fmt.Errorf("screenshot %s of work item %d: %w", r.PathValue("n"), id, store.ErrNotFound))
		return
	}
	path := p.Screenshots[n]
	rel, err := filepath.Rel(s.dataDir, path)
	if err != nil || !filepath.IsAbs(path) || !filepath.IsLocal(rel) ||
		!strings.HasPrefix(mime.TypeByExtension(filepath.Ext(path)), "image/") {
		writeError(w, fmt.Errorf("screenshot %s of work item %d is not an image under the data folder: %w",
			r.PathValue("n"), id, store.ErrNotFound))
		return
	}
	http.ServeFile(w, r, path)
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
