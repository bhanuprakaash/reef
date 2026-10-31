package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strconv"

	"github.com/bhanuprakaash/reef/store"
)

func (s *server) memberRoutes(mux *http.ServeMux) {
	mux.HandleFunc("DELETE /spaces/{id}/members/{botId}", s.removeSpaceMember)
	mux.HandleFunc("/spaces/{id}/members/{botId}", methodNotAllowed)
	mux.HandleFunc("GET /spaces/{id}/member-proposals", s.listMemberProposals)
	mux.HandleFunc("/spaces/{id}/member-proposals", methodNotAllowed)
	mux.HandleFunc("POST /member-proposals/{id}/approve", s.approveMemberProposal)
	mux.HandleFunc("/member-proposals/{id}/approve", methodNotAllowed)
	mux.HandleFunc("POST /member-proposals/{id}/reject", s.rejectMemberProposal)
	mux.HandleFunc("/member-proposals/{id}/reject", methodNotAllowed)
}

// joinSpace puts the Bot in Space id and posts the notice that it joined. The Bot's first turn
// there starts from a handover of where the Space stands, and the members learn of it with their
// next wake.
func (s *server) joinSpace(ctx context.Context, id, botID int64) (store.Role, error) {
	role, err := s.st.AddSpaceMember(ctx, id, botID)
	if err != nil {
		return store.Role{}, err
	}
	if c, err := s.st.Space(ctx, id); err == nil && c.WorkItemID != nil {
		if err := s.st.AddWorkItemParticipant(ctx, *c.WorkItemID, botID); err != nil {
			return store.Role{}, err
		}
	}
	s.noteJoined(ctx, id, role.Name)
	return role, nil
}

func (s *server) noteJoined(ctx context.Context, id int64, name string) {
	if _, err := s.st.CreateMessage(ctx, store.Message{
		SpaceID: id, Author: store.ReefAuthor, Body: name + store.JoinedSuffix,
	}); err != nil {
		log.Printf("note that %s joined Space %d: %v", name, id, err)
		return
	}
	s.notify(id, "note")
}

// removeSpaceMember takes a Bot out of a Space. The answer is 200 once it is out, or 202 with
// pending true when a turn is running: the Bot then leaves when that turn ends.
func (s *server) removeSpaceMember(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	botID, err := strconv.ParseInt(r.PathValue("botId"), 10, 64)
	if err != nil {
		writeError(w, fmt.Errorf("%w: bot id %q is not a number", store.ErrInvalid, r.PathValue("botId")))
		return
	}
	var in struct {
		Coordinator int64 `json:"coordinator"`
		Replacement int64 `json:"replacement"`
	}
	if r.ContentLength != 0 {
		if err := readJSON(r, &in); err != nil {
			writeError(w, err)
			return
		}
	}
	ch, err := s.st.Space(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	if ch.Kind == store.KindConversation {
		writeError(w, fmt.Errorf("%w: Space %d is a direct conversation with one Bot", store.ErrInvalid, id))
		return
	}
	team, err := s.st.SpaceTeam(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	i := slices.IndexFunc(team.Roles, func(m store.Role) bool { return m.BotID == botID })
	if i < 0 {
		writeError(w, fmt.Errorf("%w: Bot %d is not a member of Space %d", store.ErrNotFound, botID, id))
		return
	}
	var remaining []int64
	for _, m := range team.Roles {
		if m.BotID != botID {
			remaining = append(remaining, m.BotID)
		}
	}
	adds := in.Replacement != 0 && in.Replacement != botID && !slices.Contains(remaining, in.Replacement)
	if adds {
		remaining = append(remaining, in.Replacement)
	}
	if len(remaining) == 0 {
		writeError(w, fmt.Errorf("%w: %s is the last Bot of Space %d; name a replacement",
			store.ErrConflict, team.Roles[i].Name, id))
		return
	}
	coordinator := in.Coordinator
	if team.Roles[i].Lead {
		if coordinator == 0 && adds && len(remaining) == 1 {
			coordinator = remaining[0]
		}
		if !slices.Contains(remaining, coordinator) {
			writeError(w, fmt.Errorf("%w: %s coordinates Space %d; name a remaining member as coordinator",
				store.ErrConflict, team.Roles[i].Name, id))
			return
		}
	}
	if adds {
		if _, err := s.joinSpace(r.Context(), id, in.Replacement); err != nil {
			writeError(w, err)
			return
		}
	}
	pending, err := s.dropMember(r.Context(), id, store.PendingRemoval{SpaceID: id, BotID: botID, Coordinator: coordinator})
	if err != nil {
		writeError(w, err)
		return
	}
	if pending {
		writeJSON(w, http.StatusAccepted, map[string]bool{"pending": true})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"pending": false})
}

// dropMember marks the Bot as leaving, then removes it at once when the Space is idle. When a
// chain holds the Space it reports the removal as pending: that chain applies it when it ends.
func (s *server) dropMember(ctx context.Context, id int64, m store.PendingRemoval) (pending bool, err error) {
	if err := s.st.MarkMemberLeaving(ctx, id, m.BotID, m.Coordinator); err != nil {
		return false, err
	}
	c, _, err := s.claim(ctx, id, _noTurn)
	if errors.Is(err, store.ErrConflict) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	c.l.mu.Lock()
	sess := c.l.sess
	c.l.mu.Unlock()
	err = s.applyLeaving(ctx, id, sess)
	if team, terr := s.st.SpaceTeam(ctx, id); terr == nil {
		c.l.mu.Lock()
		c.l.held, c.l.queued = keepMembers(c.l.held, team), keepMembers(c.l.queued, team)
		c.l.mu.Unlock()
	}
	if err = c.release(err); err != nil {
		return false, err
	}
	return false, nil
}

// settleLeaving applies the removals still pending in Space id when a chain ends. When another
// chain already holds the Space, that chain applies them when it ends.
func (s *server) settleLeaving(ctx context.Context, id int64) {
	ctx = context.WithoutCancel(ctx)
	pending, err := s.st.LeavingMembers(ctx, id)
	if err != nil {
		log.Printf("list leaving members of Space %d: %v", id, err)
		return
	}
	if len(pending) == 0 {
		return
	}
	c, _, err := s.claim(ctx, id, _noTurn)
	if err != nil {
		return
	}
	c.l.mu.Lock()
	sess := c.l.sess
	c.l.mu.Unlock()
	c.settling = true
	_ = s.applyLeaving(ctx, id, sess)
	_ = c.release(nil)
}

// applyLeaving removes the Bots marked as leaving Space id and closes their agents in sess, when
// the Space has one. A removal that no longer holds gets a notice in the Space and leaves the Bot
// a member; the errors are returned.
func (s *server) applyLeaving(ctx context.Context, id int64, sess session) error {
	ctx = context.WithoutCancel(ctx)
	pending, err := s.st.LeavingMembers(ctx, id)
	if err != nil {
		return err
	}
	if len(pending) == 0 {
		return nil
	}
	var errs []error
	for _, m := range pending {
		err := s.st.ApplyMemberRemoval(ctx, m)
		if err == nil {
			if sess != nil {
				if derr := sess.Drop(m.BotID); derr != nil {
					log.Printf("close removed Bot %d of Space %d: %v", m.BotID, id, derr)
				}
			}
			continue
		}
		log.Printf("remove Bot %d from Space %d: %v", m.BotID, id, err)
		errs = append(errs, err)
		name := fmt.Sprintf("Bot %d", m.BotID)
		if bot, berr := s.st.Bot(ctx, m.BotID); berr == nil {
			name = bot.Name
		}
		if _, err := s.st.CreateMessage(ctx, store.Message{
			SpaceID: id, Author: store.ReefAuthor, Body: name + " could not be removed and stays in the Space: " + err.Error(),
		}); err != nil {
			log.Printf("note failed removal in Space %d: %v", id, err)
		}
	}
	s.notify(id, "direct")
	return errors.Join(errs...)
}

// keepMembers drops from ws the wakes of Bots that are not in team or are leaving it.
func keepMembers(ws []wake, team store.Team) []wake {
	return slices.DeleteFunc(ws, func(w wake) bool {
		return w.botID != nil && !slices.ContainsFunc(team.Roles, func(r store.Role) bool {
			return r.BotID == *w.botID && !r.Leaving
		})
	})
}

func (s *server) listMemberProposals(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	ps, err := s.st.PendingMemberProposals(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	if ps == nil {
		ps = []store.MemberProposal{}
	}
	writeJSON(w, http.StatusOK, ps)
}

// approveMemberProposal adds the proposed Bot as a user-requested addition would.
func (s *server) approveMemberProposal(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	p, err := s.st.MemberProposal(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	if p.Status != "pending" {
		writeError(w, fmt.Errorf("%w: member proposal %d is already %s", store.ErrConflict, id, p.Status))
		return
	}
	if _, err := s.joinSpace(r.Context(), p.SpaceID, p.BotID); err != nil {
		writeError(w, err)
		return
	}
	p, err = s.st.DecideMemberProposal(r.Context(), id, true)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *server) rejectMemberProposal(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	p, err := s.st.DecideMemberProposal(r.Context(), id, false)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}
