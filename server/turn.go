package server

import (
	"net/http"

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
	if f.ChannelID, err = queryID(r, "channel"); err != nil {
		writeError(w, err)
		return
	}
	f.Status = r.URL.Query().Get("status")
	ts, err := s.st.Turns(r.Context(), f)
	if err != nil {
		writeError(w, err)
		return
	}
	if ts == nil {
		ts = []store.Turn{}
	}
	writeJSON(w, http.StatusOK, ts)
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
