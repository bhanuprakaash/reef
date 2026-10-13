// Package server serves Reef's REST API over a Store.
package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/bhanuprakaash/reef/store"
)

type server struct {
	st *store.Store
}

// New returns the HTTP handler for the whole API.
func New(st *store.Store) http.Handler {
	s := &server{st: st}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/health", methodNotAllowed)
	mux.HandleFunc("/", notFound)
	s.projectRoutes(mux)
	s.featureRoutes(mux)
	s.channelRoutes(mux)
	s.taskRoutes(mux)
	s.decisionRoutes(mux)
	s.turnRoutes(mux)
	return guard(mux)
}

// guard rejects requests a browser page on another origin could send to the
// localhost API: a Host that is not a loopback name (DNS rebinding) and a body
// that is not JSON (a cross-origin form post cannot set that type).
func guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
		switch strings.Trim(host, "[]") {
		case "127.0.0.1", "localhost", "::1":
		default:
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden host"})
			return
		}
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch:
			mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || mt != "application/json" {
				writeJSON(w, http.StatusUnsupportedMediaType,
					map[string]string{"error": "content type must be application/json"})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// methodNotAllowed is registered without a method for each path. The mux
// prefers method patterns, so this only sees the other methods, and it
// answers in JSON where the mux's own 405 is plain text.
func methodNotAllowed(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
}

// notFound answers every path no route claims, in JSON where the mux's own
// 404 is plain text.
func notFound(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
}

func readJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%w: bad JSON: %v", store.ErrInvalid, err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: bad JSON: unexpected data after the body", store.ErrInvalid)
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

// queryID reads an optional numeric query parameter; nil when absent.
func queryID(r *http.Request, name string) (*int64, error) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return nil, nil
	}
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%w: %s %q is not a number", store.ErrInvalid, name, v)
	}
	return &id, nil
}
