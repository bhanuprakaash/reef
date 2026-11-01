package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/store"
)

func (s *server) turnRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /turns", s.listTurns)
	mux.HandleFunc("POST /turns", s.createTurn)
	mux.HandleFunc("/turns", methodNotAllowed)
	mux.HandleFunc("GET /turns/{id}", s.getTurn)
	mux.HandleFunc("PATCH /turns/{id}", s.endTurn)
	mux.HandleFunc("/turns/{id}", methodNotAllowed)
}

func (s *server) listTurns(w http.ResponseWriter, r *http.Request) {
	var f store.TurnFilter
	var err error
	if f.SpaceID, err = queryID(r, "space"); err != nil {
		writeError(w, err)
		return
	}
	f.Status = r.URL.Query().Get("status")
	f.Retryable = r.URL.Query().Get("retryable") == "1"
	ts, err := s.st.Turns(r.Context(), f)
	if err != nil {
		writeError(w, err)
		return
	}
	if r.URL.Query().Get("rollbackable") == "1" {
		if f.SpaceID == nil {
			writeError(w, fmt.Errorf("%w: rollbackable needs a space", store.ErrInvalid))
			return
		}
		if ts, err = s.rollbackable(r.Context(), *f.SpaceID, ts); err != nil {
			writeError(w, err)
			return
		}
	}
	if ts == nil {
		ts = []store.Turn{}
	}
	writeJSON(w, http.StatusOK, ts)
}

// rollbackable keeps the turns of Space id that have checkpoints, all taken in workspaces of
// the work item the Space belongs to now.
func (s *server) rollbackable(ctx context.Context, id int64, ts []store.Turn) ([]store.Turn, error) {
	ch, err := s.st.Space(ctx, id)
	if err != nil {
		return nil, err
	}
	var out []store.Turn
	for _, t := range ts {
		if len(t.Checkpoints) == 0 {
			continue
		}
		if _, err := s.checkpointTrees(ctx, ch, t); err == nil {
			out = append(out, t)
		}
	}
	return out, nil
}

func (s *server) createTurn(w http.ResponseWriter, r *http.Request) {
	var in store.Turn
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	t, err := s.st.CreateTurn(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (s *server) getTurn(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	t, err := s.st.Turn(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *server) endTurn(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in store.Turn
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	in.ID = id
	var t store.Turn
	if in.Status == "" {
		if in.Error != "" || in.Usage != (agent.Usage{}) {
			writeError(w, fmt.Errorf("%w: error and usage need a status", store.ErrInvalid))
			return
		}
		t, err = s.st.SetTurnSession(r.Context(), id, in.SessionID)
	} else {
		t, err = s.st.EndTurn(r.Context(), in)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}
