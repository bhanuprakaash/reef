// Package server serves Reef's REST API over a Store.
package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/bhanuprakaash/reef/store"
)

type server struct {
	st *store.Store
}

// New returns the HTTP handler for the whole API.
func New(st *store.Store) http.Handler {
	s := &server{st: st}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/health", methodNotAllowed)
	s.projectRoutes(mux)
	s.featureRoutes(mux)
	s.channelRoutes(mux)
	s.taskRoutes(mux)
	return mux
}

// methodNotAllowed is registered without a method for each path. The mux
// prefers method patterns, so this only sees the other methods, and it
// answers in JSON where the mux's own 405 is plain text.
func methodNotAllowed(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
}

func readJSON(r *http.Request, v any) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return fmt.Errorf("%w: bad JSON: %v", store.ErrInvalid, err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}

func writeError(w http.ResponseWriter, err error) {
	status, msg := http.StatusInternalServerError, "internal error"
	switch {
	case errors.Is(err, store.ErrInvalid):
		status, msg = http.StatusBadRequest, err.Error()
	case errors.Is(err, store.ErrNotFound):
		status, msg = http.StatusNotFound, err.Error()
	case errors.Is(err, store.ErrConflict):
		status, msg = http.StatusConflict, err.Error()
	default:
		log.Print(err)
	}
	writeJSON(w, status, map[string]string{"error": msg})
}

func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: id %q is not a number", store.ErrInvalid, r.PathValue("id"))
	}
	return id, nil
}
