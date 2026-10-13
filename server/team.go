package server

import "net/http"

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
	writeJSON(w, http.StatusOK, ts)
}

func (s *server) getTeam(w http.ResponseWriter, r *http.Request) {
	t, err := s.st.Team(r.Context(), r.PathValue("name"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}
