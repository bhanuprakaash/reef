package server

import (
	"fmt"
	"net/http"

	"github.com/bhanuprakaash/reef/store"
)

func (s *server) connectionRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /connections", s.listConnections)
	mux.HandleFunc("POST /connections", s.createConnection)
	mux.HandleFunc("/connections", methodNotAllowed)
	mux.HandleFunc("PUT /connections/{id}", s.putConnection)
	mux.HandleFunc("DELETE /connections/{id}", s.deleteConnection)
	mux.HandleFunc("/connections/{id}", methodNotAllowed)
}

func (s *server) listConnections(w http.ResponseWriter, r *http.Request) {
	cs, err := s.st.Connections(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, nonNil(cs))
}

func (s *server) createConnection(w http.ResponseWriter, r *http.Request) {
	var in store.Connection
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	in.ID = 0
	c, err := s.st.CreateConnection(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (s *server) putConnection(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in store.Connection
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	if in.ID != 0 && in.ID != id {
		writeError(w, fmt.Errorf("%w: body id %d does not match %d", store.ErrInvalid, in.ID, id))
		return
	}
	in.ID = id
	c, err := s.st.UpdateConnection(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *server) deleteConnection(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.st.DeleteConnection(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
