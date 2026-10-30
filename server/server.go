// Package server serves Reef's REST API over a Store.
package server

import (
	"context"
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
	"sync"

	"github.com/bhanuprakaash/reef/memory"
	"github.com/bhanuprakaash/reef/space"
	"github.com/bhanuprakaash/reef/store"
	"github.com/bhanuprakaash/reef/worktree"
)

type server struct {
	st      *store.Store
	ms      *memory.Store
	dataDir string
	trees   *space.Trees
	open    func(ctx context.Context, spaceID int64) (session, error)
	// name proposes a Space name from its first message, with the Space's backend.
	name func(ctx context.Context, backend, message string) (string, error)

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	quit   chan struct{}

	mu       sync.Mutex
	closed   bool
	lives    map[int64]*live
	deleting map[int64]int

	approveMu sync.Mutex

	inboxMu   sync.Mutex
	inboxWake chan struct{}

	// ghMu guards ghNoGhLogged and ghWorkItemErrLogged: the first github poll runs on its
	// own goroutine and can overlap a caller that polls the server right after construction.
	ghMu sync.Mutex
	// ghNoGhLogged is set once the github poll has logged that gh is not on PATH, so it
	// logs that once, not every poll.
	ghNoGhLogged bool
	// ghWorkItemErrLogged is the last gh error logged for a work item, so the poll logs a
	// cause once and logs again only when it changes.
	ghWorkItemErrLogged map[int64]string
}

// Handler serves the API. Close it when done.
type Handler struct {
	h http.Handler
	s *server
}

// New returns the API over st.
func New(st *store.Store, ms *memory.Store, dataDir, exe string) *Handler {
	trees := space.NewTrees(dataDir)
	return newHandler(st, ms, dataDir, trees, func(ctx context.Context, spaceID int64) (session, error) {
		sess, err := space.Open(ctx, st, ms, dataDir, exe, spaceID, trees, space.NewAgent)
		if err != nil {
			return nil, err
		}
		return sess, nil
	})
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.h.ServeHTTP(w, r) }

// Close waits for running turns to be stored. A send after it gets 503.
func (h *Handler) Close() { h.s.close() }

func newHandler(
	st *store.Store, ms *memory.Store, dataDir string, trees *space.Trees,
	open func(ctx context.Context, spaceID int64) (session, error),
) *Handler {
	ctx, cancel := context.WithCancel(context.Background())
	s := &server{
		st: st, ms: ms, dataDir: dataDir, trees: trees, open: open, name: space.ProposeName,
		ctx: ctx, cancel: cancel,
		quit: make(chan struct{}), lives: map[int64]*live{}, deleting: map[int64]int{},
		inboxWake: make(chan struct{}), ghWorkItemErrLogged: map[int64]string{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/health", methodNotAllowed)
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, _ *http.Request) {
		running := map[int64]bool{}
		s.markRunning(running)
		writeJSON(w, http.StatusOK, map[string]int{"running": len(running)})
	})
	mux.HandleFunc("/status", methodNotAllowed)
	mux.HandleFunc("/", notFound)
	s.projectRoutes(mux)
	s.profileRoutes(mux)
	s.workItemRoutes(mux)
	s.spaceRoutes(mux)
	s.teamRoutes(mux)
	s.botRoutes(mux)
	s.providerRoutes(mux)
	s.skillRoutes(mux)
	s.liveRoutes(mux)
	s.turnOpRoutes(mux)
	s.proposalRoutes(mux)
	s.decisionRoutes(mux)
	s.memoryRoutes(mux)
	s.turnRoutes(mux)
	s.inboxRoutes(mux)
	s.startGitHubPoll()
	s.startQueue()
	return &Handler{h: guard(s.notifyInbox(mux)), s: s}
}

// notifyInbox tells the inbox streams after each request that may write: it can add, clear or
// delete an item that no Space event reports, such as dismissing a waiting item.
func (s *server) notifyInbox(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			s.inboxChanged()
		}
	})
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
	case errors.Is(err, errStopping):
		status, msg = http.StatusServiceUnavailable, err.Error()
	case errors.Is(err, store.ErrInvalid), errors.Is(err, memory.ErrInvalid):
		status, msg = http.StatusBadRequest, err.Error()
	case errors.Is(err, store.ErrNotFound), errors.Is(err, memory.ErrNotFound):
		status, msg = http.StatusNotFound, err.Error()
	case errors.Is(err, store.ErrConflict), errors.Is(err, worktree.ErrNoRepo):
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
