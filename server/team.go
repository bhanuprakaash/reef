package server

import (
	"net/http"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/store"
)

func (s *server) teamRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /teams", s.listTeams)
	mux.HandleFunc("/teams", methodNotAllowed)
	mux.HandleFunc("GET /teams/{name}", s.getTeam)
	mux.HandleFunc("/teams/{name}", methodNotAllowed)
}

func (s *server) listTeams(w http.ResponseWriter, r *http.Request) {
	ts, err := s.st.Teams(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	for i := range ts {
		ts[i] = withoutEnv(ts[i])
	}
	writeJSON(w, http.StatusOK, ts)
}

func (s *server) getTeam(w http.ResponseWriter, r *http.Request) {
	t, err := s.st.Team(r.Context(), r.PathValue("name"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, withoutEnv(t))
}

// withoutEnv copies t with every MCP server's env removed: it may hold API keys.
func withoutEnv(t store.Team) store.Team {
	roles := make([]store.Role, len(t.Roles))
	for i, r := range t.Roles {
		servers := make([]agent.MCPServer, len(r.MCPServers))
		for j, m := range r.MCPServers {
			m.Env = nil
			servers[j] = m
		}
		r.MCPServers = servers
		roles[i] = r
	}
	t.Roles = roles
	return t
}
