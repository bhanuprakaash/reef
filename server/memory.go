package server

import (
	"fmt"
	"net/http"

	"github.com/bhanuprakaash/reef/memory"
)

type memoryIn struct {
	Kind  string `json:"kind"`
	Body  string `json:"body"`
	Why   string `json:"why"`
	Scope string `json:"scope"`
}

func (s *server) memoryRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /memory", s.listMemory)
	mux.HandleFunc("POST /memory", s.createMemory)
	mux.HandleFunc("/memory", methodNotAllowed)
	mux.HandleFunc("POST /memory/{id}/retire", s.retireMemory)
	mux.HandleFunc("/memory/{id}/retire", methodNotAllowed)
}

func (s *server) listMemory(w http.ResponseWriter, r *http.Request) {
	scope, err := memory.ParseRef(r.URL.Query().Get("scope"))
	if err != nil {
		writeError(w, fmt.Errorf("scope: %w", err))
		return
	}
	items, _, err := s.ms.Newest(r.Context(), scope, nil, 0)
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]itemOut, 0, len(items))
	for _, it := range items {
		out = append(out, toItemOut(it))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) createMemory(w http.ResponseWriter, r *http.Request) {
	var in memoryIn
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	scope, err := memory.ParseRef(in.Scope)
	if err != nil {
		writeError(w, fmt.Errorf("scope: %w", err))
		return
	}
	res, err := s.ms.Remember(r.Context(),
		memory.Writer{Author: "user", Role: "user", Scope: scope},
		memory.Note{Kind: in.Kind, Body: in.Body, Why: in.Why})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, struct {
		itemOut
		Similar    []similarOut `json:"similar,omitempty"`
		EmbedError string       `json:"embedError,omitempty"`
	}{toItemOut(res.Item), toSimilarOut(res.Similar), embedMessage(res.EmbedErr)})
}

func (s *server) retireMemory(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.ms.Retire(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
