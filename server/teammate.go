package server

import (
	"fmt"
	"net/http"

	"github.com/bhanuprakaash/reef/store"
)

// teammateView is a Teammate as REST shows it. Its MCP servers show only a name and a label,
// like a role's: their args, env and url may carry keys.
type teammateView struct {
	ID             int64     `json:"id"`
	Name           string    `json:"name"`
	Persona        string    `json:"persona"`
	Skills         []string  `json:"skills"`
	Backend        string    `json:"backend"`
	Model          string    `json:"model,omitempty"`
	Provider       string    `json:"provider,omitempty"`
	Face           string    `json:"face,omitempty"`
	Colour         string    `json:"colour,omitempty"`
	Tools          []string  `json:"tools"`
	MCP            []mcpView `json:"mcp"`
	Write          bool      `json:"write"`
	PermissionMode string    `json:"permissionMode"`
}

type addMemberBody struct {
	TeammateID int64 `json:"teammateId"`
	Lead       bool  `json:"lead"`
}

func (s *server) teammateRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /teammates", s.listTeammates)
	mux.HandleFunc("POST /teammates", s.createTeammate)
	mux.HandleFunc("/teammates", methodNotAllowed)
	mux.HandleFunc("GET /teammates/{id}", s.getTeammate)
	mux.HandleFunc("PUT /teammates/{id}", s.putTeammate)
	mux.HandleFunc("DELETE /teammates/{id}", s.deleteTeammate)
	mux.HandleFunc("/teammates/{id}", methodNotAllowed)
	mux.HandleFunc("GET /teammates/{id}/conversations", s.listConversations)
	mux.HandleFunc("/teammates/{id}/conversations", methodNotAllowed)
}

func (s *server) listConversations(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if _, err := s.st.Teammate(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	cs, err := s.st.TeammateConversations(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	if cs == nil {
		cs = []store.SpacePreview{}
	}
	writeJSON(w, http.StatusOK, cs)
}

func (s *server) listTeammates(w http.ResponseWriter, r *http.Request) {
	ts, err := s.st.Teammates(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	views := make([]teammateView, len(ts))
	for i, t := range ts {
		views[i] = viewTeammate(t)
	}
	writeJSON(w, http.StatusOK, views)
}

func (s *server) createTeammate(w http.ResponseWriter, r *http.Request) {
	var in store.Teammate
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	in.ID = 0
	t, err := s.st.CreateTeammate(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, viewTeammate(t))
}

func (s *server) getTeammate(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	t, err := s.st.Teammate(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, viewTeammate(t))
}

// putTeammate replaces every field of the Teammate. A body that omits mcpServers keeps the
// stored ones, since REST never shows them; `"mcpServers": []` clears them.
func (s *server) putTeammate(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in store.Teammate
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	if in.ID != 0 && in.ID != id {
		writeError(w, fmt.Errorf("%w: body id %d does not match %d", store.ErrInvalid, in.ID, id))
		return
	}
	in.ID = id
	if in.MCPServers == nil {
		cur, err := s.st.Teammate(r.Context(), id)
		if err != nil {
			writeError(w, err)
			return
		}
		in.MCPServers = cur.MCPServers
	}
	t, err := s.st.UpdateTeammate(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, viewTeammate(t))
}

func (s *server) deleteTeammate(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.st.DeleteTeammate(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// addMember puts a Teammate in the team with a copy of its default access rules.
func (s *server) addMember(w http.ResponseWriter, r *http.Request) {
	var in addMemberBody
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	role, err := s.st.AddMember(r.Context(), r.PathValue("name"), in.TeammateID, in.Lead)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, viewRole(role))
}

func (s *server) editMember(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in store.Membership
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	team := r.PathValue("name")
	role, err := s.st.EditMember(r.Context(), team, id, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, viewRole(role))
}

func (s *server) removeMember(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.st.RemoveMember(r.Context(), r.PathValue("name"), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func viewTeammate(t store.Teammate) teammateView {
	return teammateView{
		ID: t.ID, Name: t.Name, Persona: t.Persona, Skills: t.Skills, Backend: t.Backend,
		Model: t.Model, Provider: t.Provider, Face: t.Face, Colour: t.Colour, Tools: t.Tools,
		MCP: viewMCP(t.MCPServers), Write: t.Write, PermissionMode: t.PermissionMode,
	}
}
