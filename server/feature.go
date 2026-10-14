package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/bhanuprakaash/reef/channel"
	"github.com/bhanuprakaash/reef/store"
	"github.com/bhanuprakaash/reef/worktree"
)

func (s *server) featureRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /features", s.listFeatures)
	mux.HandleFunc("POST /features", s.createFeature)
	mux.HandleFunc("/features", methodNotAllowed)
	mux.HandleFunc("GET /features/{id}", s.getFeature)
	mux.HandleFunc("PUT /features/{id}", s.updateFeature)
	mux.HandleFunc("DELETE /features/{id}", s.deleteFeature)
	mux.HandleFunc("/features/{id}", methodNotAllowed)
	mux.HandleFunc("GET /features/{id}/brief", s.getBrief)
	mux.HandleFunc("PUT /features/{id}/brief", s.putBrief)
	mux.HandleFunc("/features/{id}/brief", methodNotAllowed)
	mux.HandleFunc("GET /features/{id}/brief/versions", s.listBriefVersions)
	mux.HandleFunc("/features/{id}/brief/versions", methodNotAllowed)
	mux.HandleFunc("GET /features/{id}/notes/{role}", s.getRoleNotes)
	mux.HandleFunc("PUT /features/{id}/notes/{role}", s.putRoleNotes)
	mux.HandleFunc("DELETE /features/{id}/notes/{role}", s.deleteRoleNotes)
	mux.HandleFunc("/features/{id}/notes/{role}", methodNotAllowed)
	mux.HandleFunc("POST /features/{id}/worktree", s.ensureFeatureWorktree)
	mux.HandleFunc("/features/{id}/worktree", methodNotAllowed)
}

func (s *server) listFeatures(w http.ResponseWriter, r *http.Request) {
	projectID, err := queryID(r, "project")
	if err != nil {
		writeError(w, err)
		return
	}
	fs, err := s.st.Features(r.Context(), projectID)
	if err != nil {
		writeError(w, err)
		return
	}
	if fs == nil {
		fs = []store.Feature{}
	}
	writeJSON(w, http.StatusOK, fs)
}

func (s *server) createFeature(w http.ResponseWriter, r *http.Request) {
	var in store.Feature
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	f, err := s.st.CreateFeature(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, f)
}

func (s *server) getFeature(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	f, err := s.st.Feature(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (s *server) updateFeature(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in store.Feature
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	in.ID = id
	f, err := s.st.UpdateFeature(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, f)
}

// deleteFeature deletes the feature, its worktree and the checkpoint refs of its turns. The
// branch is kept. A running turn of its channels is stopped first, as for a channel delete.
// A worktree with uncommitted changes is 409 and nothing is removed; so is a channel with a
// turn running elsewhere, or open in reef chat. The worktree goes first: it is the step that
// can refuse or fail, and the next wake makes it again. The refs go last, after the rows that
// name them. A project folder that is gone is not git cleaned up: its worktree folder, if
// any, is just removed.
func (s *server) deleteFeature(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	lift, err := s.stopChannels(r.Context(), store.ChannelFilter{FeatureID: &id})
	if err != nil {
		writeError(w, err)
		return
	}
	defer lift()
	f, err := s.st.Feature(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	p, err := s.st.Project(r.Context(), f.ProjectID)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.channelsInUse(r.Context(), store.ChannelFilter{FeatureID: &id}); err != nil {
		writeError(w, err)
		return
	}
	refs, err := s.checkpointRefs(r.Context(), store.ChannelFilter{FeatureID: &id})
	if err != nil {
		writeError(w, err)
		return
	}
	gone := projectGone(p.Path)
	dir := worktree.Path(s.dataDir, p.ID, f.Slug)
	if gone {
		err = os.RemoveAll(dir)
	} else {
		err = s.removeFeatureTree(r.Context(), p, f)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.st.DeleteFeature(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	if !gone {
		if err := worktree.DeleteRefs(context.WithoutCancel(r.Context()), p.Path, refs); err != nil {
			writeError(w, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// channelsInUse is 409 for the first channel matching f that a `reef chat` has open, or whose
// tree a turn is running on, in this process or another.
func (s *server) channelsInUse(ctx context.Context, f store.ChannelFilter) error {
	cs, err := s.st.Channels(ctx, f)
	if err != nil {
		return err
	}
	for _, c := range cs {
		if channel.InUse(s.dataDir, c.ID) {
			return fmt.Errorf("channel %d is open elsewhere: %w", c.ID, store.ErrConflict)
		}
		dir, _, err := channel.WorkDir(ctx, s.st, s.dataDir, c.ID)
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

// checkpointRefs lists the checkpoint refs of the turns of the channels that match f.
func (s *server) checkpointRefs(ctx context.Context, f store.ChannelFilter) ([]string, error) {
	cs, err := s.st.Channels(ctx, f)
	if err != nil {
		return nil, err
	}
	var refs []string
	for _, c := range cs {
		ts, err := s.st.Turns(ctx, store.TurnFilter{ChannelID: &c.ID})
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

// checkFeatureTree is 409 when the worktree of feature f of project p has uncommitted changes.
func (s *server) checkFeatureTree(ctx context.Context, p store.Project, f store.Feature) error {
	dir := worktree.Path(s.dataDir, p.ID, f.Slug)
	return treeConflict(f, dir, worktree.Clean(ctx, dir))
}

// removeFeatureTree removes the worktree of feature f of project p; 409 when it has
// uncommitted changes.
func (s *server) removeFeatureTree(ctx context.Context, p store.Project, f store.Feature) error {
	dir := worktree.Path(s.dataDir, p.ID, f.Slug)
	return treeConflict(f, dir, worktree.Remove(ctx, p.Path, dir))
}

func treeConflict(f store.Feature, dir string, err error) error {
	if errors.Is(err, worktree.ErrDirty) {
		return fmt.Errorf("%w: feature %s has uncommitted changes in %s; commit or discard them first",
			store.ErrConflict, f.Slug, dir)
	}
	return err
}

// ensureFeatureWorktree makes the feature's worktree, the same way the first wake of one of
// its channels would, and returns its path. A worktree or branch that already exists is
// reused.
func (s *server) ensureFeatureWorktree(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	f, err := s.st.Feature(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	p, err := s.st.Project(r.Context(), f.ProjectID)
	if err != nil {
		writeError(w, err)
		return
	}
	dir := worktree.Path(s.dataDir, p.ID, f.Slug)
	if err := channel.EnsureWorktree(r.Context(), s.trees, p.Path, dir, f.Slug); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": dir})
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
	in.FeatureID = id
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
	in.FeatureID, in.Role = id, r.PathValue("role")
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
