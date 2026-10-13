package server

import (
	"net/http"

	"github.com/bhanuprakaash/reef/store"
)

func (s *server) taskRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /tasks", s.listTasks)
	mux.HandleFunc("POST /tasks", s.createTask)
	mux.HandleFunc("/tasks", methodNotAllowed)
	mux.HandleFunc("GET /tasks/{id}", s.getTask)
	mux.HandleFunc("PUT /tasks/{id}", s.updateTask)
	mux.HandleFunc("DELETE /tasks/{id}", s.deleteTask)
	mux.HandleFunc("/tasks/{id}", methodNotAllowed)
}

func (s *server) listTasks(w http.ResponseWriter, r *http.Request) {
	f := store.TaskFilter{Status: r.URL.Query().Get("status")}
	var err error
	if f.ProjectID, err = queryID(r, "project"); err != nil {
		writeError(w, err)
		return
	}
	if f.FeatureID, err = queryID(r, "feature"); err != nil {
		writeError(w, err)
		return
	}
	ts, err := s.st.Tasks(r.Context(), f)
	if err != nil {
		writeError(w, err)
		return
	}
	if ts == nil {
		ts = []store.Task{}
	}
	writeJSON(w, http.StatusOK, ts)
}

func (s *server) createTask(w http.ResponseWriter, r *http.Request) {
	var in store.Task
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	t, err := s.st.CreateTask(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (s *server) getTask(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	t, err := s.st.Task(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *server) updateTask(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in store.Task
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	in.ID = id
	t, err := s.st.UpdateTask(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *server) deleteTask(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.st.DeleteTask(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
