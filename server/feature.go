package server

import (
	"net/http"

	"github.com/bhanuprakaash/reef/store"
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
	if err := s.st.DeleteFeature(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
