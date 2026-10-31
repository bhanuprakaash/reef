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

// leaving is a Bot that leaves its Space once the turn that is running ends, and the member that
// leads in its place when it led.
type leaving struct {
	botID       int64
	coordinator int64
}

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
	pending, err := s.dropMember(r.Context(), id, leaving{botID, coordinator})
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

// dropMember removes the Bot at once when the Space is idle, or queues the removal for the end
// of the turn that is running, which it reports as pending.
func (s *server) dropMember(ctx context.Context, id int64, m leaving) (pending bool, err error) {
	if s.queueLeaving(id, m) {
		return true, nil
	}
	c, _, err := s.claim(ctx, id, _noTurn)
	if errors.Is(err, store.ErrConflict) && s.queueLeaving(id, m) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	err = s.st.RemoveSpaceMember(ctx, id, m.botID, m.coordinator)
	c.l.mu.Lock()
	sess := c.l.sess
	c.l.mu.Unlock()
	if err == nil && sess != nil {
		if derr := sess.Drop(m.botID); derr != nil {
			log.Printf("close removed Bot %d of Space %d: %v", m.botID, id, derr)
		}
	}
	if err = c.release(err); err != nil {
		return false, err
	}
	s.notify(id, "direct")
	return false, nil
}

// queueLeaving queues m for the end of the running turn of Space id, and reports false when the
// Space is idle.
func (s *server) queueLeaving(id int64, m leaving) bool {
	l := s.lookup(id)
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.state == _idle {
		return false
	}
	l.leave = append(l.leave, m)
	return true
}

// applyLeaving removes the Bots that waited for the turn that ended. A removal that no longer
// holds gets a notice in the Space and leaves the Bot a member.
func (s *server) applyLeaving(ctx context.Context, id int64, sess session, leave []leaving) {
	ctx = context.WithoutCancel(ctx)
	for _, m := range leave {
		err := s.st.RemoveSpaceMember(ctx, id, m.botID, m.coordinator)
		if err == nil {
			if derr := sess.Drop(m.botID); derr != nil {
				log.Printf("close removed Bot %d of Space %d: %v", m.botID, id, derr)
			}
			continue
		}
		log.Printf("remove Bot %d from Space %d: %v", m.botID, id, err)
		name := fmt.Sprintf("Bot %d", m.botID)
		if bot, berr := s.st.Bot(ctx, m.botID); berr == nil {
			name = bot.Name
		}
		if _, err := s.st.CreateMessage(ctx, store.Message{
			SpaceID: id, Author: store.ReefAuthor, Body: name + " could not be removed and stays in the Space: " + err.Error(),
		}); err != nil {
			log.Printf("note failed removal in Space %d: %v", id, err)
		}
	}
	s.notify(id, "direct")
}

// keepMembers drops from ws the wakes of Bots that are not in team.
func keepMembers(ws []wake, team store.Team) []wake {
	return slices.DeleteFunc(ws, func(w wake) bool {
		return w.botID != nil && !slices.ContainsFunc(team.Roles, func(r store.Role) bool { return r.BotID == *w.botID })
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
