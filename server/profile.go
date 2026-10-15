package server

import (
	"net/http"

	"github.com/bhanuprakaash/reef/store"
)

func (s *server) profileRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /profile", s.getProfile)
	mux.HandleFunc("PUT /profile", s.putProfile)
	mux.HandleFunc("/profile", methodNotAllowed)
}

func (s *server) getProfile(w http.ResponseWriter, r *http.Request) {
	p, err := s.st.Profile(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *server) putProfile(w http.ResponseWriter, r *http.Request) {
	var in store.Profile
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	p, err := s.st.PutProfile(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}
