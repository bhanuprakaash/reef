package server

import (
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

func viewTeam(t store.Team) teamView {
	roles := make([]roleView, len(t.Roles))
	for i, r := range t.Roles {
		roles[i] = roleView{
			Name: r.Name, Lead: r.Lead, Backend: r.Backend, PermissionMode: r.PermissionMode,
			Persona: r.Persona, Skills: r.Skills, Tools: r.Tools,
		}
	}
	return teamView{Name: t.Name, Roles: roles, Gates: t.Gates}
}
