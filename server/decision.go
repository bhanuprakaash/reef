package server

import (
	"fmt"
	"net/http"

	"github.com/bhanuprakaash/reef/store"
)

func (s *server) decisionRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /decisions", s.listDecisions)
	mux.HandleFunc("POST /decisions", s.createDecision)
	mux.HandleFunc("/decisions", methodNotAllowed)
	mux.HandleFunc("GET /decisions/{id}", s.getDecision)
	mux.HandleFunc("/decisions/{id}", methodNotAllowed)
}

func (s *server) listDecisions(w http.ResponseWriter, r *http.Request) {
	featureID, err := queryID(r, "feature")
	if err != nil {
		writeError(w, err)
		return
	}
	if featureID == nil {
		writeError(w, fmt.Errorf("%w: feature is required", store.ErrInvalid))
		return
	}
	ds, err := s.st.Decisions(r.Context(), *featureID)
	if err != nil {
		writeError(w, err)
		return
	}
	if ds == nil {
		ds = []store.Decision{}
	}
	writeJSON(w, http.StatusOK, ds)
}

func (s *server) createDecision(w http.ResponseWriter, r *http.Request) {
	var in store.Decision
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	d, err := s.st.CreateDecision(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, d)
}

func (s *server) getDecision(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	d, err := s.st.Decision(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}
