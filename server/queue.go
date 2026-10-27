package server

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/bhanuprakaash/reef/space"
	"github.com/bhanuprakaash/reef/store"
)

// startQueue starts the first queued item of every free Team and opens what it started, in the
// background, so a queued item waits no longer than a restart.
func (s *server) startQueue() {
	if !s.begin() {
		return
	}
	go func() {
		defer s.wg.Done()
		if err := s.st.StartQueues(s.ctx); err != nil {
			log.Printf("start queues: %v", err)
		}
		s.openStarted(s.ctx)
	}()
}

// startSubmitted runs when a chain ends on Space id. submit_for_review runs in the reef mcp
// process, which the server cannot hear, so this is where it learns of one: the Team is free, so
// the item the queue started opens now, and the lead wraps up the Space. The wrap-up is claimed
// first, so one submit starts one. A wrap-up that cannot start is noted in the Space.
func (s *server) startSubmitted(id int64) {
	if s.ctx.Err() != nil {
		return
	}
	claimed, err := s.st.ClaimWrap(s.ctx, id)
	if err != nil {
		log.Printf("claim wrap-up of Space %d: %v", id, err)
		return
	}
	if !claimed {
		return
	}
	s.openStarted(s.ctx)
	err = s.startUserTurn(s.ctx, id, userTurn{kind: _wrap, text: space.WrapCommand})
	if err == nil {
		return
	}
	note := fmt.Sprintf("Submitted for review, but the wrap-up did not start: %v. Send /wrap to run it.", err)
	if _, err := s.st.CreateMessage(s.ctx, store.Message{SpaceID: id, Author: store.ReefAuthor, Body: note}); err != nil {
		log.Printf("note failed wrap-up in Space %d: %v", id, err)
	}
}

// openStarted gives each top-level item the queue moved to in progress that has no open Space
// one, led by its Team's lead, and puts the item in the lead's first prompt. A failure is
// noted in the Space. A sent-back item already has a Space: its lead is woken with the note.
func (s *server) openStarted(ctx context.Context) {
	items, err := s.st.WorkItems(ctx, store.WorkItemFilter{Status: "in_progress"})
	if err != nil {
		log.Printf("list started work items: %v", err)
		return
	}
	for _, w := range items {
		if w.ParentID != nil || w.TeamID == nil || w.ClosedAt != nil {
			continue
		}
		_, err := s.st.WorkItemActiveSpace(ctx, w.ID)
		if !errors.Is(err, store.ErrNotFound) {
			if err != nil {
				log.Printf("read Space of work item %d: %v", w.ID, err)
				continue
			}
			s.wakeSentBack(ctx, w.ID)
			continue
		}
		if err := s.openItem(ctx, w); err != nil && !errors.Is(err, store.ErrConflict) {
			log.Printf("open work item %d: %v", w.ID, err)
		}
	}
}

func (s *server) openItem(ctx context.Context, w store.WorkItem) error {
	team, err := s.st.TeamByID(ctx, *w.TeamID)
	if err != nil {
		return err
	}
	lead := team.Roles[0]
	brief, err := s.st.Brief(ctx, w.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	sp, err := s.st.CreateSpace(ctx, store.Space{
		ProjectID: w.ProjectID, WorkItemID: &w.ID, Name: w.Title, Role: lead.Name,
		Backend: lead.Backend, PermissionMode: lead.PermissionMode, Team: team.Name,
	})
	if err != nil {
		return err
	}
	err = s.startUserTurn(ctx, sp.ID, userTurn{kind: _message, text: queuePrompt(w, brief.Body)})
	if err == nil {
		return nil
	}
	note := fmt.Sprintf("Started from the queue, but the first turn did not run: %v. "+
		"Send the lead a message to run it.", err)
	_, err = s.st.CreateMessage(ctx, store.Message{SpaceID: sp.ID, Author: store.ReefAuthor, Body: note})
	return err
}

// wakeSentBack tells the lead of the Space a sent-back item restarted in why the user sent it
// back, in a user message that starts its turn. A turn that cannot start is noted in the Space
// with the user's note.
func (s *server) wakeSentBack(ctx context.Context, id int64) {
	spaceID, note, ok, err := s.st.ClaimSendBack(ctx, id)
	if err != nil {
		log.Printf("claim send-back note of work item %d: %v", id, err)
		return
	}
	if !ok {
		return
	}
	text := "Sent back from review by the user:\n" + note
	err = s.startUserTurn(ctx, spaceID, userTurn{kind: _message, text: text})
	if err == nil {
		return
	}
	text = fmt.Sprintf("%s\n\nThe lead's turn did not run: %v. Send the lead a message to run it.", text, err)
	if _, err := s.st.CreateMessage(ctx, store.Message{SpaceID: spaceID, Author: store.ReefAuthor, Body: text}); err != nil {
		log.Printf("note failed send-back turn in Space %d: %v", spaceID, err)
	}
}

// queuePrompt is what the lead is told when its Team starts the item.
func queuePrompt(w store.WorkItem, brief string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Work item %d: %s\nDescription: %s\nAcceptance lines:", w.ID, w.Title,
		cmp.Or(w.Description, "none"))
	for _, l := range w.AcceptanceLines {
		fmt.Fprintf(&b, "\n%d. %s", l.Position, l.Text)
	}
	if len(w.AcceptanceLines) == 0 {
		b.WriteString(" none")
	}
	fmt.Fprintf(&b, "\nBrief: %s", cmp.Or(brief, "none"))
	return b.String()
}
