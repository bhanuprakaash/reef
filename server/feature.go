package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

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
// A worktree with uncommitted changes is 409 and nothing is removed. The worktree goes
// first: it is the step that can refuse or fail, and the next wake makes it again. The refs go
// last, after the rows that name them.
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
	repo, refs, err := s.removeFeatureTree(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.st.DeleteFeature(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	if err := worktree.DeleteRefs(context.WithoutCancel(r.Context()), repo, refs); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// removeFeatureTree removes the worktree of feature id. It returns the project's repo and the
// checkpoint refs of the feature's turns, for the caller to delete once the feature is.
func (s *server) removeFeatureTree(
	ctx context.Context, id int64,
) (repo string, refs []string, err error) {
	f, err := s.st.Feature(ctx, id)
	if err != nil {
		return "", nil, err
	}
	p, err := s.st.Project(ctx, f.ProjectID)
	if err != nil {
		return "", nil, err
	}
	cs, err := s.st.Channels(ctx, store.ChannelFilter{FeatureID: &id})
	if err != nil {
		return "", nil, err
	}
	for _, c := range cs {
		ts, err := s.st.Turns(ctx, store.TurnFilter{ChannelID: &c.ID})
		if err != nil {
			return "", nil, err
		}
		for _, t := range ts {
			if t.Checkpoint != nil {
				refs = append(refs, *t.Checkpoint)
			}
		}
	}
	dir := worktree.Path(s.dataDir, p.ID, f.Slug)
	err = worktree.Remove(ctx, p.Path, dir)
	if errors.Is(err, worktree.ErrDirty) {
		err = fmt.Errorf("%w: feature %s has uncommitted changes in %s; commit or discard them first",
			store.ErrConflict, f.Slug, dir)
	}
	return p.Path, refs, err
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
