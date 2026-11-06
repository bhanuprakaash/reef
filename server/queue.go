package server

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"slices"
	"strings"

	"github.com/bhanuprakaash/reef/space"
	"github.com/bhanuprakaash/reef/store"
)

const (
	_noRepositoryReason = "no repository of this Work item is ready"
	_firstTurnReason    = "the first turn did not start"
)

// startQueue starts every queued item that has an owner and opens what it started, in the
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
// process, which the server cannot hear, so this is where it learns of one: the queue may have
// started an item meanwhile, so it opens now, and the lead wraps up the Space. The wrap-up is claimed
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

// admission is one started item's first turn, or its send-back wake, claimed under admitMu and
// run outside it: it can fetch from a slow remote.
type admission struct {
	spaceID int64
	run     func(ctx context.Context)
}

// openStarted gives each top-level item the queue moved to in progress that has no open Space
// one, led by its owner with every participant a member, and tells the lead the item. A failure is noted in the Space. A
// sent-back item already has a Space: its lead is woken with the note. A Space made by hand
// that has no message yet is started with the item. Each item is claimed at once and started in
// the background, so a caller never waits on git.
func (s *server) openStarted(ctx context.Context) {
	for _, a := range s.claimStarted(ctx) {
		if !s.begin() {
			s.endAdmission(a.spaceID)
			continue
		}
		go func() {
			defer s.wg.Done()
			defer s.endAdmission(a.spaceID)
			a.run(s.ctx)
		}()
	}
}

func (s *server) endAdmission(id int64) {
	s.admitMu.Lock()
	defer s.admitMu.Unlock()
	delete(s.starting, id)
}

func (s *server) claimStarted(ctx context.Context) []admission {
	s.admitMu.Lock()
	defer s.admitMu.Unlock()
	items, err := s.st.WorkItems(ctx, store.WorkItemFilter{Status: "in_progress"})
	if err != nil {
		log.Printf("list started work items: %v", err)
		return nil
	}
	var claimed []admission
	for _, w := range items {
		if w.ParentID != nil || w.OwnerBotID == nil || w.ApprovedAt == nil || w.ClosedAt != nil {
			continue
		}
		sp, err := s.st.WorkItemActiveSpace(ctx, w.ID)
		if errors.Is(err, store.ErrNotFound) {
			sp, err = s.openSpace(ctx, w)
			if err != nil {
				if !errors.Is(err, store.ErrConflict) {
					log.Printf("open work item %d: %v", w.ID, err)
				}
				continue
			}
			s.starting[sp.ID] = true
			claimed = append(claimed, admission{sp.ID, func(ctx context.Context) { s.startLeadLogged(ctx, sp.ID, w) }})
			continue
		}
		if err != nil {
			log.Printf("read Space of work item %d: %v", w.ID, err)
			continue
		}
		if a, ok := s.claimAttached(ctx, w, sp); ok {
			s.starting[sp.ID] = true
			claimed = append(claimed, a)
		}
	}
	return claimed
}

func (s *server) openSpace(ctx context.Context, w store.WorkItem) (store.Space, error) {
	owner, err := s.st.Bot(ctx, *w.OwnerBotID)
	if err != nil {
		return store.Space{}, err
	}
	return s.st.CreateSpaceOf(ctx, store.Space{
		ProjectID: w.ProjectID, WorkItemID: &w.ID, Name: w.Title, Role: owner.Name,
	}, w.ParticipantBotIDs, owner.ID)
}

func (s *server) startLeadLogged(ctx context.Context, id int64, w store.WorkItem) {
	if err := s.startLead(ctx, id, w); err != nil {
		log.Printf("start Space %d with work item %d: %v", id, w.ID, err)
	}
}

// startLead tells the lead of Space id the item the queue started, in a user message that
// starts its turn. The repositories are made first, here and nowhere else on the way to the first
// turn; one that is blocked waits and the others go on. When none is ready, or a turn cannot
// start, the Space gets a note and is paused, which puts the item in Needs you; the other
// started items go on. Continue, once the cause is fixed, starts the lead.
func (s *server) startLead(ctx context.Context, id int64, w store.WorkItem) error {
	brief, err := s.st.Brief(ctx, w.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	if name, err := s.st.ItemInactiveTarget(ctx, w.ID); err != nil || name != "" {
		if err != nil {
			return err
		}
		return s.pauseWithNote(ctx, id, _projectDeletedReason+name, projectDeletedNote(name))
	}
	ws, err := space.EnsureWorkspaces(ctx, s.st, s.trees, s.dataDir, w)
	if err != nil && ctx.Err() != nil {
		return err
	}
	if !slices.ContainsFunc(ws, func(x store.Workspace) bool { return x.State == store.WorkspaceReady }) {
		lines := []string{"Not started: no repository of this Work item is ready."}
		for _, x := range ws {
			p, perr := s.st.Project(ctx, x.ProjectID)
			if perr != nil {
				return perr
			}
			lines = append(lines, fmt.Sprintf("%s: %s", p.Name, x.Note))
		}
		if err != nil {
			lines = append(lines, err.Error())
		}
		return s.pauseWithNote(ctx, id, _noRepositoryReason, strings.Join(lines, "\n")+"\nFix it, then continue the Space.")
	}
	if err != nil {
		note := "A repository could not be set up, the others go on: " + err.Error()
		if _, merr := s.st.CreateMessage(ctx, store.Message{SpaceID: id, Author: store.ReefAuthor, Body: note}); merr != nil {
			return merr
		}
	}
	err = s.startUserTurn(ctx, id, userTurn{kind: _message, text: queuePrompt(w, brief.Body)})
	if err == nil {
		return nil
	}
	note := fmt.Sprintf("Started from the queue, but the first turn did not run: %v. "+
		"Fix it, then continue the Space.", err)
	if errors.Is(err, store.ErrConflict) {
		_, err = s.st.CreateMessage(ctx, store.Message{SpaceID: id, Author: store.ReefAuthor, Body: note})
		return err
	}
	return s.pauseWithNote(ctx, id, _firstTurnReason, note)
}

// pauseWithNote says why in Space id, from reef, and pauses it, which puts its item in Needs you.
func (s *server) pauseWithNote(ctx context.Context, id int64, reason, note string) error {
	if _, err := s.st.CreateMessage(ctx, store.Message{SpaceID: id, Author: store.ReefAuthor, Body: note}); err != nil {
		return err
	}
	return s.st.PauseSpace(ctx, id, reason)
}

// claimAttached claims the wake of the Space the item already has, unless it is paused: the
// user's note when the item was sent back, and the item when the Space has had no first turn yet.
func (s *server) claimAttached(ctx context.Context, w store.WorkItem, sp store.Space) (admission, bool) {
	if sp.Paused != "" || s.starting[sp.ID] {
		return admission{}, false
	}
	spaceID, note, ok, err := s.st.ClaimSendBack(ctx, w.ID)
	if err != nil {
		log.Printf("claim send-back note of work item %d: %v", w.ID, err)
		return admission{}, false
	}
	if ok {
		return admission{spaceID, func(ctx context.Context) { s.wakeSentBack(ctx, spaceID, note) }}, true
	}
	pending, err := s.st.FirstTurnPending(ctx, sp.ID)
	if err != nil {
		log.Printf("%v", err)
		return admission{}, false
	}
	if !pending {
		return admission{}, false
	}
	return admission{sp.ID, func(ctx context.Context) { s.startLeadLogged(ctx, sp.ID, w) }}, true
}

// wakeSentBack tells the lead of the Space a sent-back item restarted in why the user sent it
// back, in a user message that starts its turn. A turn that cannot start is noted in the Space
// with the user's note.
func (s *server) wakeSentBack(ctx context.Context, spaceID int64, note string) {
	text := "Sent back from review by the user:\n" + note
	if strings.HasPrefix(note, _reefSendBack) {
		text = note
	}
	err := s.startUserTurn(ctx, spaceID, userTurn{kind: _message, text: text})
	if err == nil {
		return
	}
	text = fmt.Sprintf("%s\n\nThe lead's turn did not run: %v. Send the lead a message to run it.", text, err)
	if _, err := s.st.CreateMessage(ctx, store.Message{SpaceID: spaceID, Author: store.ReefAuthor, Body: text}); err != nil {
		log.Printf("note failed send-back turn in Space %d: %v", spaceID, err)
	}
}

// openAddedTarget makes the workspace of a repository added to item id while its Space runs, and
// tells the Space its folder; the Bots there have only the folders listed when they started.
func (s *server) openAddedTarget(ctx context.Context, id, projectID int64) {
	if _, err := s.st.WorkItemActiveSpace(ctx, id); err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			log.Printf("read Space of work item %d: %v", id, err)
		}
		return
	}
	item, err := s.st.WorkItem(ctx, id)
	if err != nil {
		log.Printf("read work item %d: %v", id, err)
		return
	}
	ws, err := space.EnsureWorkspaces(ctx, s.st, s.trees, s.dataDir, item)
	if err != nil {
		log.Printf("make workspaces of work item %d: %v", id, err)
	}
	for _, x := range ws {
		if x.ProjectID != projectID || x.State != store.WorkspaceReady {
			continue
		}
		p, err := s.st.Project(ctx, projectID)
		if err != nil {
			log.Printf("read project %d: %v", projectID, err)
			return
		}
		sp, err := s.st.WorkItemActiveSpace(ctx, id)
		if err != nil {
			log.Printf("read Space of work item %d: %v", id, err)
			return
		}
		body := fmt.Sprintf("Repository %s is ready: folder %s/ under your working folder, on branch %s, base %s.",
			p.Name, filepath.Base(x.Dir), x.Branch, cmp.Or(x.Base, x.BaseRef))
		if _, err := s.st.CreateMessage(ctx, store.Message{SpaceID: sp.ID, Author: store.ReefAuthor, Body: body}); err != nil {
			log.Printf("note workspace of work item %d: %v", id, err)
		}
	}
}

// queuePrompt is what the lead is told when the queue starts the item.
func queuePrompt(w store.WorkItem, brief string) string {
	return fmt.Sprintf("Work item %d: %s\nDescription: %s\nBrief: %s", w.ID, w.Title,
		cmp.Or(w.Description, "none"), cmp.Or(brief, "none"))
}
