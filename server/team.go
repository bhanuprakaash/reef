package server

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"

	"github.com/bhanuprakaash/reef/store"
)

// teamView is a team as REST shows it.
type teamView struct {
	ID             int64      `json:"id"`
	Name           string     `json:"name"`
	Responsibility string     `json:"responsibility"`
	Roles          []roleView `json:"roles"`
}

type roleView struct {
	BotID          int64        `json:"botId,omitempty"`
	Name           string       `json:"name"`
	Lead           bool         `json:"lead"`
	Write          bool         `json:"write"`
	Backend        string       `json:"backend"`
	PermissionMode string       `json:"permissionMode"`
	Description    string       `json:"description"`
	Skills         []string     `json:"skills"`
	Face           string       `json:"face,omitempty"`
	Colour         string       `json:"colour,omitempty"`
	Model          string       `json:"model,omitempty"`
	Provider       string       `json:"provider,omitempty"`
	Tools          []string     `json:"tools"`
	Connections    []int64      `json:"connections"`
	Session        *sessionView `json:"session,omitempty"`
}

// sessionView is what a Bot's active session in a Space runs with. Stale says the Bot's own
// settings have changed since. Connections are names only: the session's env and urls stay out.
type sessionView struct {
	Backend        string             `json:"backend"`
	Model          string             `json:"model,omitempty"`
	PermissionMode string             `json:"permissionMode"`
	Write          bool               `json:"write"`
	Tools          []string           `json:"tools"`
	Skills         []sessionSkillView `json:"skills"`
	Connections    []string           `json:"connections"`
	RefreshPending bool               `json:"refreshPending"`
	Stale          bool               `json:"stale"`
}

type sessionSkillView struct {
	Ref        string `json:"ref"`
	Compatible bool   `json:"compatible"`
	Reason     string `json:"reason,omitempty"`
}

// mcpView names a connection as the agent sees its MCP server and labels it by its command, or
// the host of its url.
type mcpView struct {
	Name  string `json:"name"`
	Label string `json:"label"`
}

func (s *server) teamRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /teams", s.listTeams)
	mux.HandleFunc("POST /teams", s.createTeam)
	mux.HandleFunc("/teams", methodNotAllowed)
	mux.HandleFunc("GET /teams/{name}", s.getTeam)
	mux.HandleFunc("PUT /teams/{name}", s.putTeam)
	mux.HandleFunc("DELETE /teams/{name}", s.deleteTeam)
	mux.HandleFunc("/teams/{name}", methodNotAllowed)
	mux.HandleFunc("POST /teams/{name}/members", s.addMember)
	mux.HandleFunc("/teams/{name}/members", methodNotAllowed)
	mux.HandleFunc("PUT /teams/{name}/members/{id}", s.editMember)
	mux.HandleFunc("DELETE /teams/{name}/members/{id}", s.removeMember)
	mux.HandleFunc("/teams/{name}/members/{id}", methodNotAllowed)
}

func (s *server) createTeam(w http.ResponseWriter, r *http.Request) {
	var t store.TeamSpec
	if err := readJSON(r, &t); err != nil {
		writeError(w, err)
		return
	}
	if from := r.URL.Query().Get("from"); from != "" {
		if _, err := s.st.Team(r.Context(), from); err != nil {
			writeError(w, err)
			return
		}
	}
	created, err := s.st.CreateTeam(r.Context(), t)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, viewTeam(created))
}

func (s *server) deleteTeam(w http.ResponseWriter, r *http.Request) {
	if err := s.st.DeleteTeam(r.Context(), r.PathValue("name")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) listTeams(w http.ResponseWriter, r *http.Request) {
	ts, err := s.st.Teams(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	views := make([]teamView, len(ts))
	for i, t := range ts {
		views[i] = viewTeam(t)
	}
	writeJSON(w, http.StatusOK, views)
}

func (s *server) getTeam(w http.ResponseWriter, r *http.Request) {
	t, err := s.st.Team(r.Context(), r.PathValue("name"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, viewTeam(t))
}

func (s *server) putTeam(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	from := r.URL.Query().Get("from")
	var t store.TeamSpec
	if err := readJSON(r, &t); err != nil {
		writeError(w, err)
		return
	}
	if t.Name == "" {
		t.Name = name
	} else if t.Name != name {
		writeError(w, fmt.Errorf("%w: body name %q does not match %q", store.ErrInvalid, t.Name, name))
		return
	}
	if from == "" {
		applied, err := s.st.ApplyTeam(r.Context(), t)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, viewTeam(applied))
		return
	}
	if _, err := s.st.Team(r.Context(), from); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, fmt.Errorf("%w: team %q not found", store.ErrNotFound, from))
		} else {
			writeError(w, err)
		}
		return
	}
	applied, err := s.st.CreateTeam(r.Context(), t)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, viewTeam(applied))
}

func viewMCP(connections []store.Connection) []mcpView {
	views := make([]mcpView, len(connections))
	for i, c := range connections {
		label := c.Command
		if u, err := url.Parse(c.URL); label == "" && err == nil {
			label = u.Host
		}
		views[i] = mcpView{Name: c.Name, Label: label}
	}
	return views
}

func viewTeam(t store.Team) teamView {
	roles := make([]roleView, len(t.Roles))
	for i, r := range t.Roles {
		roles[i] = viewRole(r)
	}
	return teamView{ID: t.ID, Name: t.Name, Responsibility: t.Responsibility, Roles: roles}
}

func viewRole(r store.Role) roleView {
	return roleView{
		BotID: r.BotID, Name: r.Name, Lead: r.Lead, Write: r.Write, Backend: r.Backend,
		PermissionMode: r.PermissionMode, Description: r.Description, Skills: r.Skills, Face: r.Face,
		Colour: r.Colour, Model: r.Model, Provider: r.Provider, Tools: r.Tools,
		Connections: connectionIDs(r.Connections),
	}
}

func viewSession(bs store.BotSession, live store.Role, pending bool) *sessionView {
	v := &sessionView{
		Backend: bs.Backend, Model: bs.Model, PermissionMode: bs.PermissionMode, Write: bs.Write,
		Tools: nonNil(bs.Tools), Skills: make([]sessionSkillView, len(bs.Skills)),
		Connections: make([]string, len(bs.Connections)), RefreshPending: pending,
	}
	for i, k := range bs.Skills {
		v.Skills[i] = sessionSkillView{Ref: k.Ref, Compatible: k.Compatible, Reason: k.Reason}
	}
	for i, c := range bs.Connections {
		v.Connections[i] = c.Name
	}
	adopted := bs.Apply(live)
	v.Stale = bs.Backend != live.Backend || bs.Provider != live.Provider || bs.Model != live.Model ||
		bs.Description != live.Description || bs.Write != live.Write ||
		bs.PermissionMode != live.PermissionMode || !slices.Equal(bs.Tools, live.Tools) ||
		!slices.Equal(adopted.Skills, live.Skills) ||
		!slices.Equal(connectionIDs(bs.Connections), connectionIDs(live.Connections))
	return v
}

func connectionIDs(cs []store.Connection) []int64 {
	ids := make([]int64, len(cs))
	for i, c := range cs {
		ids[i] = c.ID
	}
	return ids
}
