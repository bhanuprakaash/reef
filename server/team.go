package server

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/bhanuprakaash/reef/store"
)

// teamView is a team as REST shows it. A role's MCP servers are left out: their args and
// url may carry keys.
type teamView struct {
	Name  string       `json:"name"`
	Roles []roleView   `json:"roles"`
	Gates []store.Gate `json:"gates,omitempty"`
}

type roleView struct {
	Name           string   `json:"name"`
	Lead           bool     `json:"lead"`
	Write          bool     `json:"write"`
	Backend        string   `json:"backend"`
	PermissionMode string   `json:"permissionMode"`
	Persona        string   `json:"persona"`
	Skills         string   `json:"skills"`
	Tools          []string `json:"tools"`
}

func (s *server) teamRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /teams", s.listTeams)
	mux.HandleFunc("/teams", methodNotAllowed)
	mux.HandleFunc("GET /teams/{name}", s.getTeam)
	mux.HandleFunc("PUT /teams/{name}", s.putTeam)
	mux.HandleFunc("/teams/{name}", methodNotAllowed)
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

// putTeam applies the body like `reef team apply`. A role whose body omits mcpServers keeps
// the server's stored ones, since the window never sees them. With a `from` query param it
// instead copies the team named by `from`: `name` must not exist yet, and a role's stored
// mcpServers come from `from`'s role of the same name.
func (s *server) putTeam(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	from := r.URL.Query().Get("from")
	var t store.Team
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
		existing, err := s.st.Team(r.Context(), name)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			writeError(w, err)
			return
		}
		fillMCPServers(t, existing)
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
	fillMCPServers(t, source)
	applied, err := s.st.CreateTeam(r.Context(), t)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, viewTeam(applied))
}

// fillMCPServers gives each role of t that omits mcpServers the servers of existing's role of
// the same name, since the window never sees them.
func fillMCPServers(t store.Team, existing store.Team) {
	for i, role := range t.Roles {
		if role.MCPServers != nil {
			continue
		}
		for _, er := range existing.Roles {
			if er.Name == role.Name {
				t.Roles[i].MCPServers = er.MCPServers
				break
			}
		}
	}
}

func viewTeam(t store.Team) teamView {
	roles := make([]roleView, len(t.Roles))
	for i, r := range t.Roles {
		roles[i] = roleView{
			Name: r.Name, Lead: r.Lead, Write: r.Write, Backend: r.Backend, PermissionMode: r.PermissionMode,
			Persona: r.Persona, Skills: r.Skills, Tools: r.Tools,
		}
	}
	return teamView{Name: t.Name, Roles: roles, Gates: t.Gates}
}
