package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"

	"github.com/bhanuprakaash/reef/memory"
	"github.com/bhanuprakaash/reef/space"
	"github.com/bhanuprakaash/reef/store"
	"github.com/bhanuprakaash/reef/worktree"
)

func (s *server) spaceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /spaces", s.listSpaces)
	mux.HandleFunc("POST /spaces", s.createSpace)
	mux.HandleFunc("/spaces", methodNotAllowed)
	mux.HandleFunc("GET /spaces/{id}", s.getSpace)
	mux.HandleFunc("PUT /spaces/{id}", s.updateSpace)
	mux.HandleFunc("DELETE /spaces/{id}", s.deleteSpace)
	mux.HandleFunc("/spaces/{id}", methodNotAllowed)
	mux.HandleFunc("POST /spaces/{id}/attach", s.attachSpace)
	mux.HandleFunc("/spaces/{id}/attach", methodNotAllowed)
	mux.HandleFunc("GET /spaces/{id}/members", s.listSpaceMembers)
	mux.HandleFunc("POST /spaces/{id}/members", s.addSpaceMember)
	mux.HandleFunc("/spaces/{id}/members", methodNotAllowed)
	mux.HandleFunc("PUT /spaces/{id}/coordinator", s.setSpaceCoordinator)
	mux.HandleFunc("/spaces/{id}/coordinator", methodNotAllowed)
	mux.HandleFunc("GET /spaces/{id}/projects", s.listSpaceProjects)
	mux.HandleFunc("/spaces/{id}/projects", methodNotAllowed)
	mux.HandleFunc("PUT /spaces/{id}/projects/{projectId}", s.addSpaceProject)
	mux.HandleFunc("DELETE /spaces/{id}/projects/{projectId}", s.removeSpaceProject)
	mux.HandleFunc("/spaces/{id}/projects/{projectId}", methodNotAllowed)
	mux.HandleFunc("POST /spaces/{id}/reopen", s.reopenSpace)
	mux.HandleFunc("/spaces/{id}/reopen", methodNotAllowed)
	mux.HandleFunc("GET /spaces/{id}/inspector", s.getInspector)
	mux.HandleFunc("/spaces/{id}/inspector", methodNotAllowed)
	mux.HandleFunc("GET /spaces/{id}/direct-messages", s.listDirectMessages)
	mux.HandleFunc("/spaces/{id}/direct-messages", methodNotAllowed)
	mux.HandleFunc("GET /search", s.search)
	mux.HandleFunc("/search", methodNotAllowed)
	mux.HandleFunc("GET /messages", s.listMessages)
	mux.HandleFunc("POST /messages", s.createMessage)
	mux.HandleFunc("/messages", methodNotAllowed)
	mux.HandleFunc("/messages/{id}", methodNotAllowed)
}

func (s *server) listDirectMessages(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	after, err := queryID(r, "after")
	if err != nil {
		writeError(w, err)
		return
	}
	var afterID int64
	if after != nil {
		afterID = *after
	}
	msgs, err := s.st.DirectMessages(r.Context(), id, afterID)
	if err != nil {
		writeError(w, err)
		return
	}
	if msgs == nil {
		msgs = []store.DirectMessage{}
	}
	writeJSON(w, http.StatusOK, msgs)
}

func (s *server) search(w http.ResponseWriter, r *http.Request) {
	workItemID, err := queryID(r, "workItem")
	if err != nil {
		writeError(w, err)
		return
	}
	if workItemID == nil {
		writeError(w, fmt.Errorf("%w: work item is required", store.ErrInvalid))
		return
	}
	q := r.URL.Query().Get("q")
	if q == "" {
		writeError(w, fmt.Errorf("%w: q is required", store.ErrInvalid))
		return
	}
	limit := int64(_defaultSearchLimit)
	n, err := queryID(r, "limit")
	if err != nil {
		writeError(w, err)
		return
	}
	if n != nil && *n > 0 {
		limit = *n
	}
	hits, err := s.st.SearchHistory(r.Context(), *workItemID, q, int(limit), nil)
	if err != nil {
		writeError(w, err)
		return
	}
	if hits == nil {
		hits = []store.MessageHit{}
	}
	writeJSON(w, http.StatusOK, hits)
}

func (s *server) listSpaces(w http.ResponseWriter, r *http.Request) {
	var f store.SpaceFilter
	var err error
	if f.ProjectID, err = queryID(r, "project"); err != nil {
		writeError(w, err)
		return
	}
	if f.WorkItemID, err = queryID(r, "workItem"); err != nil {
		writeError(w, err)
		return
	}
	cs, err := s.st.SpacePreviews(r.Context(), f)
	if err != nil {
		writeError(w, err)
		return
	}
	if cs == nil {
		cs = []store.SpacePreview{}
	}
	writeJSON(w, http.StatusOK, cs)
}

func (s *server) createSpace(w http.ResponseWriter, r *http.Request) {
	var in struct {
		store.Space
		BotID         int64   `json:"botId"`
		BotIDs        []int64 `json:"botIds"`
		CoordinatorID int64   `json:"coordinatorBotId"`
	}
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	if in.WorkItemID != nil {
		if in.BotID != 0 {
			writeError(w, fmt.Errorf("%w: the Space of a work item is made by a Team, not a Bot", store.ErrInvalid))
			return
		}
		if err := s.useItemTeam(r.Context(), &in.Space); err != nil {
			writeError(w, err)
			return
		}
	}
	if in.BotID != 0 {
		s.createBotSpace(w, r, in.Space, in.BotID)
		return
	}
	if in.Role == "" {
		in.Role = "lead"
	}
	if in.Role != "lead" {
		writeError(w, fmt.Errorf("%w: role must be lead", store.ErrInvalid))
		return
	}
	if in.Team == "" {
		in.Team = store.SoloTeam
	}
	autoName := isDefaultName(in.Name, in.Team)
	if len(in.BotIDs) > 0 {
		c, err := s.st.CreateSpaceOf(r.Context(), in.Space, in.BotIDs, in.CoordinatorID)
		if err != nil {
			writeError(w, err)
			return
		}
		if autoName {
			if err := s.st.MarkNamePending(r.Context(), c.ID); err != nil {
				writeError(w, err)
				return
			}
		}
		writeJSON(w, http.StatusCreated, c)
		return
	}
	team, err := s.st.Team(r.Context(), in.Team)
	if errors.Is(err, store.ErrNotFound) {
		err = fmt.Errorf("%w: unknown team %s", store.ErrInvalid, in.Team)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	lead := team.Roles[0]
	in.Role = lead.Name
	c, err := s.st.CreateSpace(r.Context(), in.Space)
	if err != nil {
		writeError(w, err)
		return
	}
	if autoName {
		if err := s.st.MarkNamePending(r.Context(), c.ID); err != nil {
			writeError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusCreated, c)
}

// useItemTeam sets the Team of a new Space of a work item to the item's Team, and refuses another
// one. An item with no Team, or an unknown item (the store refuses that), leaves the Team as given.
func (s *server) useItemTeam(ctx context.Context, in *store.Space) error {
	item, err := s.st.WorkItem(ctx, *in.WorkItemID)
	if errors.Is(err, store.ErrNotFound) || (err == nil && item.TeamID == nil) {
		return nil
	}
	if err != nil {
		return err
	}
	team, err := s.st.TeamByID(ctx, *item.TeamID)
	if err != nil {
		return err
	}
	if in.Team != "" && in.Team != team.Name {
		return fmt.Errorf("%w: work item %d belongs to Team %s, not %s", store.ErrInvalid, item.ID, team.Name, in.Team)
	}
	in.Team = team.Name
	return nil
}

// createBotSpace creates a Space with no Team and one member, the Bot, which leads it
// and runs on its profile.
func (s *server) createBotSpace(
	w http.ResponseWriter, r *http.Request, in store.Space, botID int64,
) {
	if in.Team != "" {
		writeError(w, fmt.Errorf("%w: give botId or team, not both", store.ErrInvalid))
		return
	}
	t, err := s.st.Bot(r.Context(), botID)
	if errors.Is(err, store.ErrNotFound) {
		err = fmt.Errorf("%w: unknown bot %d", store.ErrInvalid, botID)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	autoName := isDefaultName(in.Name, t.Name)
	in.Role = t.Name
	c, err := s.st.CreateBotSpace(r.Context(), in, botID)
	if err != nil {
		writeError(w, err)
		return
	}
	if autoName {
		if err := s.st.MarkNamePending(r.Context(), c.ID); err != nil {
			writeError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusCreated, c)
}

// isDefaultName reports whether name is what the new-space form sends when its name field
// is empty: the team (or the Bot's name), a space and the time as hours and
// minutes, with AM or PM in a 12-hour locale.
func isDefaultName(name, prefix string) bool {
	return regexp.MustCompile(`^` + regexp.QuoteMeta(prefix) +
		` \d{1,2}:\d{2}([\s\x{202f}\x{a0}]?[AaPp][Mm])?$`).MatchString(name)
}

func (s *server) getSpace(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	c, err := s.st.Space(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	used, err := s.st.SpaceTokens(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		store.Space
		TokensUsed int64 `json:"tokensUsed"`
	}{c, used})
}

func (s *server) updateSpace(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	current, err := s.st.Space(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	current.Name = in.Name
	c, err := s.st.UpdateSpace(r.Context(), current)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *server) deleteSpace(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	defer s.stopForDelete(id)()
	if err := s.st.DeleteSpace(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	if err := os.RemoveAll(space.ChatDir(s.dataDir, id)); err != nil {
		log.Printf("remove folder of Space %d: %v", id, err)
	}
	w.WriteHeader(http.StatusNoContent)
}

// attachSpace gives a free Space a work item and moves its space items to the work item's.
// The Space must be idle: its agents are stopped, so their next turn runs in the item's worktree,
// and the Space gets a note saying where that is.
func (s *server) attachSpace(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in struct {
		WorkItemID *int64 `json:"workItemId"`
	}
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	if in.WorkItemID == nil {
		writeError(w, fmt.Errorf("%w: workItemId is required", store.ErrInvalid))
		return
	}
	c, ch, err := s.claim(r.Context(), id, _noTurn)
	if err != nil {
		writeError(w, err)
		return
	}
	attached, err := s.attach(r.Context(), ch, *in.WorkItemID)
	if err = c.release(err); err != nil {
		writeError(w, err)
		return
	}
	if l := s.lookup(id); l != nil {
		s.stopLive(l)
	}
	writeJSON(w, http.StatusOK, attached)
}

func (s *server) attach(ctx context.Context, ch store.Space, workItemID int64) (store.Space, error) {
	f, err := s.st.WorkItem(ctx, workItemID)
	if errors.Is(err, store.ErrNotFound) {
		return store.Space{}, fmt.Errorf("%w: unknown work item %d", store.ErrInvalid, workItemID)
	}
	if err != nil {
		return store.Space{}, err
	}
	p, err := s.st.Project(ctx, f.ProjectID)
	if err != nil {
		return store.Space{}, err
	}
	if !worktree.HasCommit(ctx, p.Path) {
		return store.Space{}, fmt.Errorf("%w: project %s is not a git repo with a commit", store.ErrInvalid, p.Path)
	}
	attached, err := s.st.AttachSpace(ctx, ch.ID, workItemID)
	if err != nil {
		return store.Space{}, err
	}
	from := memory.Ref{Kind: memory.ScopeSpace, ID: strconv.FormatInt(ch.ID, 10)}
	to := memory.Ref{Kind: memory.ScopeWorkItem, ID: strconv.FormatInt(workItemID, 10)}
	if _, err := s.ms.Rescope(ctx, from, to); err != nil {
		return store.Space{}, err
	}
	body := fmt.Sprintf("This Space is now attached to work item %q (%d). It works in %s on %s.",
		f.Title, f.ID, worktree.Path(s.dataDir, p.ID, f.TreeKey()), worktree.Branch(f.TreeKey()))
	if dirty, err := worktree.Dirty(ctx, p.Path); err != nil {
		log.Printf("check %s for changes: %v", p.Path, err)
	} else if dirty {
		body += " Uncommitted changes stay on the main checkout."
	}
	_, err = s.st.CreateMessage(ctx, store.Message{SpaceID: ch.ID, Author: store.ReefAuthor, Body: body})
	return attached, err
}

func (s *server) listSpaceProjects(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	s.writeSpaceProjects(w, r, id)
}

func (s *server) addSpaceProject(w http.ResponseWriter, r *http.Request) {
	s.editSpaceProject(w, r, s.st.AddSpaceProject)
}

func (s *server) removeSpaceProject(w http.ResponseWriter, r *http.Request) {
	s.editSpaceProject(w, r, s.st.RemoveSpaceProject)
}

func (s *server) editSpaceProject(
	w http.ResponseWriter, r *http.Request, edit func(ctx context.Context, id, projectID int64, by *int64) error,
) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	projectID, err := strconv.ParseInt(r.PathValue("projectId"), 10, 64)
	if err != nil {
		writeError(w, fmt.Errorf("%w: project id %q is not a number", store.ErrInvalid, r.PathValue("projectId")))
		return
	}
	if err := edit(r.Context(), id, projectID, nil); err != nil {
		writeError(w, err)
		return
	}
	s.writeSpaceProjects(w, r, id)
}

func (s *server) writeSpaceProjects(w http.ResponseWriter, r *http.Request, id int64) {
	ps, err := s.st.SpaceProjects(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	if ps == nil {
		ps = []store.Project{}
	}
	writeJSON(w, http.StatusOK, ps)
}

func (s *server) listSpaceMembers(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	t, err := s.st.SpaceTeam(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, viewTeam(t).Roles)
}

func (s *server) addSpaceMember(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in struct {
		BotID int64  `json:"botId"`
		Crew  string `json:"crew"`
	}
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	if in.Crew != "" {
		roles, err := s.st.AddSpaceCrew(r.Context(), id, in.Crew)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, viewTeam(store.Team{Roles: roles}).Roles)
		return
	}
	if ch, err := s.st.Space(r.Context(), id); err == nil && ch.Kind == store.KindConversation {
		if err := s.moveConversation(r.Context(), ch, in.BotID); err != nil {
			writeError(w, err)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		return
	}
	role, err := s.st.AddSpaceMember(r.Context(), id, in.BotID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, role)
}

func (s *server) setSpaceCoordinator(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in struct {
		BotID int64 `json:"botId"`
	}
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	c, _, err := s.claim(r.Context(), id, _noTurn)
	if err != nil {
		writeError(w, err)
		return
	}
	roles, err := s.st.SetSpaceCoordinator(r.Context(), id, in.BotID)
	if err = c.release(err); err != nil {
		writeError(w, err)
		return
	}
	if l := s.lookup(id); l != nil {
		s.stopLive(l)
	}
	writeJSON(w, http.StatusOK, viewTeam(store.Team{Roles: roles}).Roles)
}

func (s *server) reopenSpace(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.st.ReopenSpace(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// stopSpaces stops the agent of every Space f selects, as stopForDelete does. The
// caller calls the returned func once the store delete is done.
func (s *server) stopSpaces(ctx context.Context, f store.SpaceFilter) (func(), error) {
	cs, err := s.st.Spaces(ctx, f)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, len(cs))
	for i, c := range cs {
		ids[i] = c.ID
	}
	return s.stopForDelete(ids...), nil
}

// stopForDelete marks the Spaces as being deleted, so a send to them is refused with
// 404, stops their agents and drops their idle entries. The caller calls the returned
// func once the store delete is done, which lifts the marks.
func (s *server) stopForDelete(ids ...int64) (lift func()) {
	s.mu.Lock()
	for _, id := range ids {
		s.deleting[id]++
	}
	s.mu.Unlock()
	for _, id := range ids {
		if l := s.lookup(id); l != nil {
			s.stopLive(l)
			s.dropIdle(id, l, true)
		}
	}
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, id := range ids {
			if s.deleting[id]--; s.deleting[id] == 0 {
				delete(s.deleting, id)
			}
		}
	}
}

func (s *server) listMessages(w http.ResponseWriter, r *http.Request) {
	spaceID, err := queryID(r, "space")
	if err != nil {
		writeError(w, err)
		return
	}
	if spaceID == nil {
		writeError(w, fmt.Errorf("%w: Space is required", store.ErrInvalid))
		return
	}
	after, err := queryID(r, "after")
	if err != nil {
		writeError(w, err)
		return
	}
	var afterID int64
	if after != nil {
		afterID = *after
	}
	before, err := queryID(r, "before")
	if err != nil {
		writeError(w, err)
		return
	}
	limit, err := queryID(r, "limit")
	if err != nil {
		writeError(w, err)
		return
	}
	var ms []store.Message
	if before != nil || limit != nil {
		n := int64(_defaultPage)
		if limit != nil && *limit > 0 {
			n = min(*limit, _maxPage)
		}
		var beforeID int64
		if before != nil {
			beforeID = *before
		}
		ms, err = s.st.MessagesPage(r.Context(), *spaceID, beforeID, int(n))
	} else {
		ms, err = s.st.Messages(r.Context(), *spaceID, afterID)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	if ms == nil {
		ms = []store.Message{}
	}
	writeJSON(w, http.StatusOK, ms)
}

func (s *server) createMessage(w http.ResponseWriter, r *http.Request) {
	var in store.Message
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	m, err := s.st.CreateMessage(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, m)
}
