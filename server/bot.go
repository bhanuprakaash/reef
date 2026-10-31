package server

import (
	"fmt"
	"net/http"

	"github.com/bhanuprakaash/reef/store"
)

// botView is a Bot as REST shows it.
type botView struct {
	ID             int64    `json:"id"`
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Skills         []string `json:"skills"`
	Backend        string   `json:"backend"`
	Model          string   `json:"model,omitempty"`
	Provider       string   `json:"provider,omitempty"`
	Face           string   `json:"face,omitempty"`
	Colour         string   `json:"colour,omitempty"`
	Tools          []string `json:"tools"`
	Connections    []int64  `json:"connections"`
	Write          bool     `json:"write"`
	PermissionMode string   `json:"permissionMode"`
}

type addMemberBody struct {
	BotID int64 `json:"botId"`
	Lead  bool  `json:"lead"`
}

func (s *server) botRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /bots", s.listBots)
	mux.HandleFunc("POST /bots", s.createBot)
	mux.HandleFunc("/bots", methodNotAllowed)
	mux.HandleFunc("GET /bots/{id}", s.getBot)
	mux.HandleFunc("PUT /bots/{id}", s.putBot)
	mux.HandleFunc("DELETE /bots/{id}", s.deleteBot)
	mux.HandleFunc("/bots/{id}", methodNotAllowed)
	mux.HandleFunc("POST /bots/{id}/revoke", s.revokeBot)
	mux.HandleFunc("/bots/{id}/revoke", methodNotAllowed)
	mux.HandleFunc("GET /bots/{id}/conversations", s.listConversations)
	mux.HandleFunc("/bots/{id}/conversations", methodNotAllowed)
}

func (s *server) listConversations(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if _, err := s.st.Bot(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	var page store.ConversationPage
	if page.ProjectID, err = queryID(r, "project"); err != nil {
		writeError(w, err)
		return
	}
	if page.Before, err = queryID(r, "before"); err != nil {
		writeError(w, err)
		return
	}
	page.Resumable = r.URL.Query().Get("resumable") == "1"
	limit, err := queryID(r, "limit")
	if err != nil {
		writeError(w, err)
		return
	}
	if limit != nil && *limit > 0 {
		page.Limit = int(min(*limit, _maxPage))
	}
	cs, err := s.st.BotConversationsPage(r.Context(), id, page)
	if err != nil {
		writeError(w, err)
		return
	}
	if cs == nil {
		cs = []store.SpacePreview{}
	}
	writeJSON(w, http.StatusOK, cs)
}

func (s *server) listBots(w http.ResponseWriter, r *http.Request) {
	ts, err := s.st.Bots(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	views := make([]botView, len(ts))
	for i, t := range ts {
		views[i] = viewBot(t)
	}
	writeJSON(w, http.StatusOK, views)
}

func (s *server) createBot(w http.ResponseWriter, r *http.Request) {
	var in store.Bot
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	in.ID = 0
	t, err := s.st.CreateBot(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, viewBot(t))
}

func (s *server) getBot(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	t, err := s.st.Bot(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, viewBot(t))
}

// putBot replaces every field of the Bot.
func (s *server) putBot(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in store.Bot
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	if in.ID != 0 && in.ID != id {
		writeError(w, fmt.Errorf("%w: body id %d does not match %d", store.ErrInvalid, in.ID, id))
		return
	}
	in.ID = id
	t, err := s.st.UpdateBot(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, viewBot(t))
}

func (s *server) deleteBot(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.st.DeleteBot(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// revokeBot ends every session of the Bot and deletes its tokens. A Space with a live session
// stops the Bot's own turn and closes the Bot's agent, never another Bot's, and every Space the Bot
// had a session in gets a notice. The Bot's next turn in a Space adopts the Bot's current settings
// and starts a new backend context.
func (s *server) revokeBot(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	bot, err := s.st.Bot(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	spaces, tokens, tools, err := s.st.RevokeBot(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	s.stopReview(id)
	body := bot.Name + " was revoked: its sessions stopped. Tool activity that was running may have " +
		"finished or not; revoking does not undo its effects."
	for _, spaceID := range spaces {
		if l := s.lookup(spaceID); l != nil {
			s.dropBot(r.Context(), spaceID, l, id)
		}
		if _, err := s.st.CreateMessage(r.Context(), store.Message{
			SpaceID: spaceID, Author: store.ReefAuthor, Body: body,
		}); err != nil {
			writeError(w, err)
			return
		}
		s.notify(spaceID, "note")
	}
	writeJSON(w, http.StatusOK, map[string]int64{
		"sessions": int64(len(spaces)), "tokens": tokens, "toolCalls": tools,
	})
}

// addMember puts a Bot in the team.
func (s *server) addMember(w http.ResponseWriter, r *http.Request) {
	var in addMemberBody
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	role, err := s.st.AddMember(r.Context(), r.PathValue("name"), in.BotID, in.Lead)
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

func viewBot(t store.Bot) botView {
	return botView{
		ID: t.ID, Name: t.Name, Description: t.Description, Skills: t.Skills, Backend: t.Backend,
		Model: t.Model, Provider: t.Provider, Face: t.Face, Colour: t.Colour, Tools: t.Tools,
		Connections: t.Connections, Write: t.Write, PermissionMode: t.PermissionMode,
	}
}
