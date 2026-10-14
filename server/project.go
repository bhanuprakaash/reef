package server

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/bhanuprakaash/reef/store"
)

func (s *server) projectRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /projects", s.listProjects)
	mux.HandleFunc("POST /projects", s.createProject)
	mux.HandleFunc("/projects", methodNotAllowed)
	mux.HandleFunc("GET /projects/{id}", s.getProject)
	mux.HandleFunc("PUT /projects/{id}", s.updateProject)
	mux.HandleFunc("DELETE /projects/{id}", s.deleteProject)
	mux.HandleFunc("/projects/{id}", methodNotAllowed)
}

// projectDir checks path is an absolute directory and returns it with symbolic
// links resolved, the form the CLI compares its working folder against.
func projectDir(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%w: project path %q: not an absolute path", store.ErrInvalid, path)
	}
	if fi, err := os.Stat(path); err != nil || !fi.IsDir() {
		return "", fmt.Errorf("%w: project path %q: not a directory", store.ErrInvalid, path)
	}
	resolved, err := store.ResolvePath(path)
	if err != nil {
		return "", fmt.Errorf("%w: project path %q: %v", store.ErrInvalid, path, err)
	}
	return resolved, nil
}

func (s *server) listProjects(w http.ResponseWriter, r *http.Request) {
	ps, err := s.st.Projects(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	if ps == nil {
		ps = []store.Project{}
	}
	writeJSON(w, http.StatusOK, ps)
}

func (s *server) createProject(w http.ResponseWriter, r *http.Request) {
	var in store.Project
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	path, err := projectDir(in.Path)
	if err != nil {
		writeError(w, err)
		return
	}
	in.Path = path
	p, err := s.st.CreateProject(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *server) getProject(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	p, err := s.st.Project(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *server) updateProject(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in store.Project
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	in.ID = id
	if in.Path, err = projectDir(in.Path); err != nil {
		writeError(w, err)
		return
	}
	p, err := s.st.UpdateProject(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *server) deleteProject(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	lift, err := s.stopChannels(r.Context(), store.ChannelFilter{ProjectID: &id})
	if err != nil {
		writeError(w, err)
		return
	}
	defer lift()
	if err := s.st.DeleteProject(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
