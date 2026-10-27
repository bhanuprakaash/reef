package server

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

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

// openStarted gives each top-level item the queue moved to in progress that has no open Space
// one, led by its Team's lead, and puts the item in the lead's first prompt. A failure is
// noted in the Space. A sent-back item already has a Space and is left to it.
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
			}
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
	note := "Started from the queue."
	if err := s.startUserTurn(ctx, sp.ID, userTurn{kind: _message, text: queuePrompt(w, brief.Body)}); err != nil {
		note = fmt.Sprintf("Started from the queue, but the first turn did not run: %v. "+
			"Send the lead a message to run it.", err)
	}
	_, err = s.st.CreateMessage(ctx, store.Message{SpaceID: sp.ID, Author: store.ReefAuthor, Body: note})
	return err
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
