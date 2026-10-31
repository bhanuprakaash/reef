package server

import (
	"fmt"
	"net/http"

	"github.com/bhanuprakaash/reef/store"
)

const (
	_defaultPage = 50
	_maxPage     = 200
)

func (s *server) navigateRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /pins", s.listPins)
	mux.HandleFunc("/pins", methodNotAllowed)
	mux.HandleFunc("PUT /pins/{kind}/{id}", s.setPin)
	mux.HandleFunc("DELETE /pins/{kind}/{id}", s.deletePin)
	mux.HandleFunc("/pins/{kind}/{id}", methodNotAllowed)
	mux.HandleFunc("GET /find", s.find)
	mux.HandleFunc("/find", methodNotAllowed)
}

func (s *server) listPins(w http.ResponseWriter, r *http.Request) {
	ps, err := s.st.Pins(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	if ps == nil {
		ps = []store.Pin{}
	}
	writeJSON(w, http.StatusOK, ps)
}

func (s *server) setPin(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.st.SetPin(r.Context(), r.PathValue("kind"), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) deletePin(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.st.Unpin(r.Context(), r.PathValue("kind"), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) find(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeError(w, fmt.Errorf("%w: q is required", store.ErrInvalid))
		return
	}
	limit, err := queryID(r, "limit")
	if err != nil {
		writeError(w, err)
		return
	}
	n := int64(_defaultSearchLimit)
	if limit != nil && *limit > 0 {
		n = min(*limit, _maxPage)
	}
	items, err := s.st.Find(r.Context(), q, int(n))
	if err != nil {
		writeError(w, err)
		return
	}
	if items == nil {
		items = []store.FoundItem{}
	}
	writeJSON(w, http.StatusOK, items)
}
