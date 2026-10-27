package server

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/bhanuprakaash/reef/space"
	"github.com/bhanuprakaash/reef/store"
)

// startQueue starts the first queued item of every free Team and opens what it started, in the
// background, so a queued item waits no longer than a restart. It also starts the wrap-up of
// each submitted item the server quit before wrapping up.
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
		ids, err := s.st.UnwrappedSpaces(s.ctx)
		if err != nil {
			log.Printf("list unwrapped Spaces: %v", err)
		}
		for _, id := range ids {
			s.startSubmitted(id)
		}
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
// one, led by its Team's lead, and tells the lead the item. A failure is noted in the Space. A
// sent-back item already has a Space: its lead is woken with the note. A Space made by hand
// that has no message yet is started with the item.
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
		sp, err := s.st.WorkItemActiveSpace(ctx, w.ID)
		if !errors.Is(err, store.ErrNotFound) {
			if err != nil {
				log.Printf("read Space of work item %d: %v", w.ID, err)
				continue
			}
			s.wakeAttached(ctx, w, sp)
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
	sp, err := s.st.CreateSpace(ctx, store.Space{
		ProjectID: w.ProjectID, WorkItemID: &w.ID, Name: w.Title, Role: lead.Name,
		Backend: lead.Backend, PermissionMode: lead.PermissionMode, Team: team.Name,
	})
	if err != nil {
		return err
	}
	return s.startLead(ctx, sp.ID, w)
}

// startLead tells the lead of Space id the item the queue started, in a user message that
// starts its turn. A turn that cannot start is noted in the Space.
func (s *server) startLead(ctx context.Context, id int64, w store.WorkItem) error {
	brief, err := s.st.Brief(ctx, w.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	err = s.startUserTurn(ctx, id, userTurn{kind: _message, text: queuePrompt(w, brief.Body)})
	if err == nil {
		return nil
	}
	note := fmt.Sprintf("Started from the queue, but the first turn did not run: %v. "+
		"Send the lead a message to run it.", err)
	_, err = s.st.CreateMessage(ctx, store.Message{SpaceID: id, Author: store.ReefAuthor, Body: note})
	return err
}

// wakeAttached wakes the Space the item already has: with the user's note when the item was
// sent back, and with the item when the Space has no message yet.
func (s *server) wakeAttached(ctx context.Context, w store.WorkItem, sp store.Space) {
	if s.wakeSentBack(ctx, w.ID) {
		return
	}
	n, err := s.st.CountMessages(ctx, sp.ID)
	if err != nil {
		log.Printf("count messages of Space %d: %v", sp.ID, err)
		return
	}
	if n > 0 {
		return
	}
	if err := s.startLead(ctx, sp.ID, w); err != nil {
		log.Printf("start Space %d with work item %d: %v", sp.ID, w.ID, err)
	}
}

// wakeSentBack tells the lead of the Space a sent-back item restarted in why the user sent it
// back, in a user message that starts its turn. A turn that cannot start is noted in the Space
// with the user's note. It reports false when the item has no note to send.
func (s *server) wakeSentBack(ctx context.Context, id int64) bool {
	spaceID, note, ok, err := s.st.ClaimSendBack(ctx, id)
	if err != nil {
		log.Printf("claim send-back note of work item %d: %v", id, err)
		return true
	}
	if !ok {
		return false
	}
	text := "Sent back from review by the user:\n" + note
	err = s.startUserTurn(ctx, spaceID, userTurn{kind: _message, text: text})
	if err == nil {
		return true
	}
	text = fmt.Sprintf("%s\n\nThe lead's turn did not run: %v. Send the lead a message to run it.", text, err)
	if _, err := s.st.CreateMessage(ctx, store.Message{SpaceID: spaceID, Author: store.ReefAuthor, Body: text}); err != nil {
		log.Printf("note failed send-back turn in Space %d: %v", spaceID, err)
	}
	return true
}

// queuePrompt is what the lead is told when its Team starts the item.
func queuePrompt(w store.WorkItem, brief string) string {
	return fmt.Sprintf("Work item %d: %s\nDescription: %s\nBrief: %s", w.ID, w.Title,
		cmp.Or(w.Description, "none"), cmp.Or(brief, "none"))
}
