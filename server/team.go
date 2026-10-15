package server

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/store"
)

// teamView is a team as REST shows it. A role's MCP servers show only a name and a label:
// their args, env and url may carry keys.
type teamView struct {
	Name  string       `json:"name"`
	Roles []roleView   `json:"roles"`
	Gates []store.Gate `json:"gates,omitempty"`
}

type roleView struct {
	Name           string    `json:"name"`
	Lead           bool      `json:"lead"`
	Write          bool      `json:"write"`
	Backend        string    `json:"backend"`
	PermissionMode string    `json:"permissionMode"`
	Persona        string    `json:"persona"`
	Skills         string    `json:"skills"`
	Face           string    `json:"face,omitempty"`
	Model          string    `json:"model,omitempty"`
	Tools          []string  `json:"tools"`
	MCP            []mcpView `json:"mcp"`
}

// mcpView names one of a role's MCP servers as the agent sees it (mcp1, mcp2, ...) and labels it
// by its command, or the host of its url.
type mcpView struct {
	Name  string `json:"name"`
	Label string `json:"label"`
}

// putRole is a role in a PUT body. Its mcpRemove lists the servers to drop, as GET showed them.
type putRole struct {
	store.Role
	MCPRemove []mcpView `json:"mcpRemove"`
}

type putBody struct {
	Name  string       `json:"name"`
	Roles []putRole    `json:"roles"`
	Gates []store.Gate `json:"gates,omitempty"`
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
// the server's stored ones, since the window never sees them, except those its `mcpRemove`
// lists; a listed server that no longer matches by name and label is a conflict. With a `from` query param it instead copies the team
// named by `from`: `name` must not exist yet, and a role's stored mcpServers come from
// `from`'s role of the same name.
func (s *server) putTeam(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	from := r.URL.Query().Get("from")
	var body putBody
	if err := readJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	t := store.Team{Name: body.Name, Roles: make([]store.Role, len(body.Roles)), Gates: body.Gates}
	for i, pr := range body.Roles {
		t.Roles[i] = pr.Role
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
// the same name, since the window never sees them, minus the ones the role's body lists in
// mcpRemove.
func fillMCPServers(t store.Team, body putBody, existing store.Team) error {
	for i, role := range t.Roles {
		if role.MCPServers != nil {
			continue
		}
		for _, er := range existing.Roles {
			if er.Name != role.Name {
				continue
			}
			kept, err := removeMCP(er.MCPServers, body.Roles[i].MCPRemove)
			if err != nil {
				return err
			}
			t.Roles[i].MCPServers = kept
			break
		}
	}
	return nil
}

// removeMCP drops the servers that remove names. Each must match a stored server by both name
// and label, so a team changed by `reef team apply` since the window loaded it is a conflict,
// never a removal of the wrong server.
func removeMCP(servers []agent.MCPServer, remove []mcpView) ([]agent.MCPServer, error) {
	if len(remove) == 0 {
		return servers, nil
	}
	views := viewMCP(servers)
	for _, r := range remove {
		if !slices.Contains(views, r) {
			return nil, fmt.Errorf("%w: MCP servers changed since you loaded the team; reload", store.ErrConflict)
		}
	}
	var kept []agent.MCPServer
	for i, srv := range servers {
		if !slices.Contains(remove, views[i]) {
			kept = append(kept, srv)
		}
	}
	return kept, nil
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
		roles[i] = roleView{
			Name: r.Name, Lead: r.Lead, Write: r.Write, Backend: r.Backend, PermissionMode: r.PermissionMode,
			Persona: r.Persona, Skills: r.Skills, Face: r.Face, Model: r.Model, Tools: r.Tools, MCP: viewMCP(r.MCPServers),
		}
	}
	return teamView{Name: t.Name, Roles: roles, Gates: t.Gates}
}

func mcpName(i int) string { return "mcp" + strconv.Itoa(i+1) }
