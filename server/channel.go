package server

import (
	"context"
	"fmt"
	"net/http"
	"slices"

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
	cs, err := s.st.Channels(r.Context(), f)
	if err != nil {
		writeError(w, err)
		return
	}
	if cs == nil {
		cs = []store.Channel{}
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
	if err := validateAgent(in); err != nil {
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

func permissionModes(backend string) []string {
	switch backend {
	case "claude":
		return []string{"default", "acceptEdits"}
	case "codex":
		return []string{"read-only", "workspace-write"}
	default:
		return nil
	}
}

func validateAgent(c store.Channel) error {
	modes := permissionModes(c.Backend)
	if modes == nil {
		return fmt.Errorf("%w: backend must be claude or codex", store.ErrInvalid)
	}
	if c.PermissionMode != "" && !slices.Contains(modes, c.PermissionMode) {
		return fmt.Errorf("%w: permissionMode for %s must be empty, %s or %s",
			store.ErrInvalid, c.Backend, modes[0], modes[1])
	}
	return nil
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
		if err := validateAgent(current); err != nil {
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
	s.mu.Lock()
	l := s.lives[id]
	s.mu.Unlock()
	if l != nil {
		s.stopLive(l)
	}
	if err := s.st.DeleteChannel(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	s.mu.Lock()
	delete(s.lives, id)
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// stopChannels stops the live agent of every channel f selects and drops its entry.
func (s *server) stopChannels(ctx context.Context, f store.ChannelFilter) error {
	cs, err := s.st.Channels(ctx, f)
	if err != nil {
		return err
	}
	for _, c := range cs {
		s.mu.Lock()
		l := s.lives[c.ID]
		delete(s.lives, c.ID)
		s.mu.Unlock()
		if l != nil {
			s.stopLive(l)
		}
	}
	return nil
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
