package server

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/store"
)

// teamView is a team as REST shows it. A role's MCP servers show only a name and a label:
// their args, env and url may carry keys.
type teamView struct {
	ID             int64      `json:"id"`
	Name           string     `json:"name"`
	Responsibility string     `json:"responsibility"`
	Roles          []roleView `json:"roles"`
}

type roleView struct {
	TeammateID     int64     `json:"teammateId,omitempty"`
	Name           string    `json:"name"`
	Lead           bool      `json:"lead"`
	Write          bool      `json:"write"`
	Backend        string    `json:"backend"`
	PermissionMode string    `json:"permissionMode"`
	Persona        string    `json:"persona"`
	Skills         []string  `json:"skills"`
	Face           string    `json:"face,omitempty"`
	Colour         string    `json:"colour,omitempty"`
	Model          string    `json:"model,omitempty"`
	Provider       string    `json:"provider,omitempty"`
	Tools          []string  `json:"tools"`
	MCP            []mcpView `json:"mcp"`
}

// mcpView names one of a role's MCP servers as the agent sees it (mcp1, mcp2, ...) and labels it
// by its command, or the host of its url.
type mcpView struct {
	Name  string `json:"name"`
	Label string `json:"label"`
}

type teamRoleBody struct {
	store.Role
}

type teamBody struct {
	Name           string         `json:"name"`
	Responsibility string         `json:"responsibility"`
	Roles          []teamRoleBody `json:"roles"`
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
	var body teamBody
	if err := readJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	t := teamFromBody(body)
	for _, role := range t.Roles {
		if role.TeammateID == 0 {
			writeError(w, fmt.Errorf("%w: role %s needs a teammateId", store.ErrInvalid, role.Name))
			return
		}
	}
	if from := r.URL.Query().Get("from"); from != "" {
		source, err := s.st.Team(r.Context(), from)
		if err != nil {
			writeError(w, err)
			return
		}
		if err := fillMCPServers(t, body, source); err != nil {
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
	var body teamBody
	if err := readJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	t := teamFromBody(body)
	if t.Name == "" {
		t.Name = name
	} else if t.Name != name {
		writeError(w, fmt.Errorf("%w: body name %q does not match %q", store.ErrInvalid, t.Name, name))
		return
	}
	if from == "" {
		existing, err := s.st.Team(r.Context(), name)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			writeError(w, err)
			return
		}
		if err := fillMCPServers(t, body, existing); err != nil {
			writeError(w, err)
			return
		}
		applied, err := s.st.ApplyTeam(r.Context(), t)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, viewTeam(applied))
		return
	}
	source, err := s.st.Team(r.Context(), from)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, fmt.Errorf("%w: team %q not found", store.ErrNotFound, from))
		} else {
			writeError(w, err)
		}
		return
	}
	if err := fillMCPServers(t, body, source); err != nil {
		writeError(w, err)
		return
	}
	applied, err := s.st.CreateTeam(r.Context(), t)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, viewTeam(applied))
}

// fillMCPServers gives each role of t that omits mcpServers the servers of existing's role of
// the same name, since the window never sees them.
func fillMCPServers(t store.Team, body teamBody, existing store.Team) error {
	for i, role := range t.Roles {
		if role.MCPServers != nil {
			continue
		}
		for _, er := range existing.Roles {
			if er.Name != role.Name {
				continue
			}
			t.Roles[i].MCPServers = er.MCPServers
			break
		}
	}
	return nil
}

func viewMCP(servers []agent.MCPServer) []mcpView {
	views := make([]mcpView, len(servers))
	for i, srv := range servers {
		label := srv.Command
		if u, err := url.Parse(srv.URL); label == "" && err == nil {
			label = u.Host
		}
		views[i] = mcpView{Name: mcpName(i), Label: label}
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
		TeammateID: r.TeammateID, Name: r.Name, Lead: r.Lead, Write: r.Write, Backend: r.Backend,
		PermissionMode: r.PermissionMode, Persona: r.Persona, Skills: r.Skills, Face: r.Face,
		Colour: r.Colour, Model: r.Model, Provider: r.Provider, Tools: r.Tools, MCP: viewMCP(r.MCPServers),
	}
}

func mcpName(i int) string { return "mcp" + strconv.Itoa(i+1) }

func teamFromBody(body teamBody) store.Team {
	t := store.Team{
		Name: body.Name, Responsibility: body.Responsibility, Roles: make([]store.Role, len(body.Roles)),
	}
	for i, role := range body.Roles {
		t.Roles[i] = role.Role
	}
	return t
}
