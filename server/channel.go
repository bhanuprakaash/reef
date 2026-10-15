package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/bhanuprakaash/reef/store"
)

func (s *server) channelRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /channels", s.listChannels)
	mux.HandleFunc("POST /channels", s.createChannel)
	mux.HandleFunc("/channels", methodNotAllowed)
	mux.HandleFunc("GET /channels/{id}", s.getChannel)
	mux.HandleFunc("PUT /channels/{id}", s.updateChannel)
	mux.HandleFunc("DELETE /channels/{id}", s.deleteChannel)
	mux.HandleFunc("/channels/{id}", methodNotAllowed)
	mux.HandleFunc("GET /channels/{id}/inspector", s.getInspector)
	mux.HandleFunc("/channels/{id}/inspector", methodNotAllowed)
	mux.HandleFunc("GET /messages", s.listMessages)
	mux.HandleFunc("POST /messages", s.createMessage)
	mux.HandleFunc("/messages", methodNotAllowed)
	mux.HandleFunc("/messages/{id}", methodNotAllowed)
}

func (s *server) listChannels(w http.ResponseWriter, r *http.Request) {
	var f store.ChannelFilter
	var err error
	if f.ProjectID, err = queryID(r, "project"); err != nil {
		writeError(w, err)
		return
	}
	if f.FeatureID, err = queryID(r, "feature"); err != nil {
		writeError(w, err)
		return
	}
	cs, err := s.st.ChannelPreviews(r.Context(), f)
	if err != nil {
		writeError(w, err)
		return
	}
	if cs == nil {
		cs = []store.ChannelPreview{}
	}
	writeJSON(w, http.StatusOK, cs)
}

func (s *server) createChannel(w http.ResponseWriter, r *http.Request) {
	var in store.Channel
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	if in.Role == "" {
		in.Role = "lead"
	}
	if in.Role != "lead" {
		writeError(w, fmt.Errorf("%w: role must be lead", store.ErrInvalid))
		return
	}
	if in.Team == "" {
		in.Team = store.SoloTeam
	}
	team, err := s.st.Team(r.Context(), in.Team)
	if errors.Is(err, store.ErrNotFound) {
		err = fmt.Errorf("%w: unknown team %s", store.ErrInvalid, in.Team)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	lead := team.Roles[0]
	in.Role = lead.Name
	if in.Backend == "" {
		in.Backend = lead.Backend
	}
	if in.PermissionMode == "" && in.Backend == lead.Backend {
		in.PermissionMode = lead.PermissionMode
	}
	if err := store.ValidateAgent(in.Backend, in.PermissionMode); err != nil {
		writeError(w, err)
		return
	}
	c, err := s.st.CreateChannel(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (s *server) getChannel(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	c, err := s.st.Channel(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *server) updateChannel(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in struct {
		Name           string  `json:"name"`
		PermissionMode *string `json:"permissionMode"`
	}
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	current, err := s.st.Channel(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	if in.PermissionMode != nil {
		current.PermissionMode = *in.PermissionMode
		if err := store.ValidateAgent(current.Backend, current.PermissionMode); err != nil {
			writeError(w, err)
			return
		}
	}
	current.Name = in.Name
	c, err := s.st.UpdateChannel(r.Context(), current)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *server) deleteChannel(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	defer s.stopForDelete(id)()
	if err := s.st.DeleteChannel(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// stopChannels stops the agent of every channel f selects, as stopForDelete does. The
// caller calls the returned func once the store delete is done.
func (s *server) stopChannels(ctx context.Context, f store.ChannelFilter) (func(), error) {
	cs, err := s.st.Channels(ctx, f)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, len(cs))
	for i, c := range cs {
		ids[i] = c.ID
	}
	return s.stopForDelete(ids...), nil
}

// stopForDelete marks the channels as being deleted, so a send to them is refused with
// 404, stops their agents and drops their idle entries. The caller calls the returned
// func once the store delete is done, which lifts the marks.
func (s *server) stopForDelete(ids ...int64) (lift func()) {
	s.mu.Lock()
	for _, id := range ids {
		s.deleting[id]++
	}
	s.mu.Unlock()
	for _, id := range ids {
		if l := s.lookup(id); l != nil {
			s.stopLive(l)
			s.dropIdle(id, l, true)
		}
	}
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, id := range ids {
			if s.deleting[id]--; s.deleting[id] == 0 {
				delete(s.deleting, id)
			}
		}
	}
}

func (s *server) listMessages(w http.ResponseWriter, r *http.Request) {
	channelID, err := queryID(r, "channel")
	if err != nil {
		writeError(w, err)
		return
	}
	if channelID == nil {
		writeError(w, fmt.Errorf("%w: channel is required", store.ErrInvalid))
		return
	}
	after, err := queryID(r, "after")
	if err != nil {
		writeError(w, err)
		return
	}
	var afterID int64
	if after != nil {
		afterID = *after
	}
	ms, err := s.st.Messages(r.Context(), *channelID, afterID)
	if err != nil {
		writeError(w, err)
		return
	}
	if ms == nil {
		ms = []store.Message{}
	}
	writeJSON(w, http.StatusOK, ms)
}

func (s *server) createMessage(w http.ResponseWriter, r *http.Request) {
	var in store.Message
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	m, err := s.st.CreateMessage(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, m)
}
