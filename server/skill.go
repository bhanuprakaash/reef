package server

import (
	"fmt"
	"net/http"
	"os"

	"github.com/bhanuprakaash/reef/skill"
	"github.com/bhanuprakaash/reef/store"
)

func (s *server) skillRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /skills", s.listSkills)
	mux.HandleFunc("/skills", methodNotAllowed)
}

func (s *server) listSkills(w http.ResponseWriter, r *http.Request) {
	id, err := queryID(r, "project")
	if err == nil && id == nil {
		err = fmt.Errorf("%w: project is required", store.ErrInvalid)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	p, err := s.st.Project(r.Context(), *id)
	if err != nil {
		writeError(w, err)
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		writeError(w, fmt.Errorf("find home folder: %w", err))
		return
	}
	claude, err := skill.Claude(home, p.Path)
	if err != nil {
		writeError(w, err)
		return
	}
	codex, err := skill.Codex(home, p.Path)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string][]skill.Skill{"claude": claude, "codex": codex})
}
