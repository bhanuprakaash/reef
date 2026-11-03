package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/bhanuprakaash/reef/store"
)

// _reefSendBack starts the note of a Work item Reef sent back from review, so its owner is not
// told the user did it.
const _reefSendBack = "Reef sent this back from review: "

// repoInterval is how often reef serve reads the status of every open request,
// REEF_REPO_INTERVAL or 60s. A zero or negative value, which would panic time.NewTicker, also
// defaults.
func repoInterval() time.Duration {
	if v := os.Getenv("REEF_REPO_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return 60 * time.Second
}

// startRepoTracking reads the status of every request not yet merged once right away, then
// every repoInterval until the server closes. It reads the stored requests and the provider
// CLIs only, so it needs no Bot session, and a restart picks up where the rows left off.
func (s *server) startRepoTracking() {
	if !s.begin() {
		return
	}
	go func() {
		defer s.wg.Done()
		s.trackRepoRequests(s.ctx)
		t := time.NewTicker(repoInterval())
		defer t.Stop()
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-t.C:
				s.trackRepoRequests(s.ctx)
			}
		}
	}()
}

func (s *server) trackRepoRequests(ctx context.Context) {
	rs, err := s.st.TrackedRequests(ctx)
	if err != nil {
		log.Printf("track requests: %v", err)
		return
	}
	for _, r := range rs {
		if ctx.Err() != nil {
			return
		}
		if err := s.trackRepoRequest(ctx, r); err != nil {
			log.Printf("track request %d: %v", r.ID, err)
		}
	}
}

// trackRepoRequest stores the provider's answer for r, or why there is none, which the next
// read retries. A newly failed check of an open request is routed once: the failure is marked
// handled only when it was delivered or is deliberately left, so a delivery that could not run,
// such as to a paused Space, is tried again on the next read.
func (s *server) trackRepoRequest(ctx context.Context, r store.RepoRequest) error {
	status, err := repoProviderFor(r.Provider).Status(ctx, repoRemote{Provider: r.Provider, Host: r.Host, Path: r.Path}, r.Number)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return s.st.RecordRepoError(ctx, r.ID, err.Error())
	}
	r, err = s.st.RecordRepoStatus(ctx, r.ID, status.State, status.Review, status.Checks)
	if err != nil || r.State != store.RequestOpened || r.Failure == "" || r.Failure == r.FailureHandled {
		return err
	}
	handled, err := s.routeFailure(ctx, r)
	if err != nil || !handled {
		return err
	}
	return s.st.HandleRepoFailure(ctx, r.ID, r.Failure)
}

func failureText(r store.RepoRequest) string {
	var names []string
	for _, c := range r.Checks {
		if c.State == "fail" {
			names = append(names, c.Name)
		}
	}
	kind := "pull request #"
	if r.Provider == store.RepoGitLab {
		kind = "merge request !"
	}
	return fmt.Sprintf("%s failed on %s%d in %s: %s", strings.Join(names, ", "), kind, r.Number, r.Path, r.URL)
}

// routeFailure hands the newly failed check of r to whoever owns it, and reports whether it
// did: the lead of the Space for unfinished work, the owner of work in review sent back with
// review-feedback priority, an inbox notice for Done work, which is never restarted. Work not yet
// started, and a Space that is busy, paused or closed, wait for a later read.
func (s *server) routeFailure(ctx context.Context, r store.RepoRequest) (bool, error) {
	text := failureText(r)
	if r.WorkItemID == nil {
		if r.SpaceID == nil {
			return false, nil
		}
		return s.wakeLead(ctx, *r.SpaceID, text)
	}
	item, err := s.st.WorkItem(ctx, *r.WorkItemID)
	if err != nil {
		return false, err
	}
	switch item.Status {
	case "in_progress", "needs_you":
		sp, err := s.st.WorkItemActiveSpace(ctx, item.ID)
		if errors.Is(err, store.ErrNotFound) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return s.wakeLead(ctx, sp.ID, text)
	case "review":
		return s.sendBackForFailure(ctx, item, text)
	case "done":
		note := fmt.Sprintf("%s. Work item %d %q is Done and was not restarted; propose follow-up work if it needs a fix.", text, item.ID, item.Title)
		return true, s.st.AddFYI(ctx, store.KindDelivery, item.ID, note)
	}
	return false, nil
}

// wakeLead posts the failure in Space id from reef and wakes its coordinator, by Bot identity,
// through the Space's normal turn queue. It leaves a paused Space paused and a Space with a turn running alone.
func (s *server) wakeLead(ctx context.Context, id int64, text string) (bool, error) {
	sp, err := s.st.Space(ctx, id)
	if err != nil || sp.Paused != "" || sp.ClosedAt != nil {
		return false, err
	}
	team, err := s.st.SpaceTeam(ctx, id)
	if err != nil || len(team.Roles) == 0 {
		return false, err
	}
	chain, sp, err := s.claim(ctx, id, _message)
	if errors.Is(err, store.ErrConflict) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err := s.st.CreateMessage(ctx, store.Message{SpaceID: id, Author: store.ReefAuthor, Body: text}); err != nil {
		return false, chain.release(err)
	}
	s.notify(id, "note")
	lead := team.Roles[0]
	err = chain.resume(ctx, sp, []wake{{from: store.ReefAuthor, role: lead.Name, botID: &lead.BotID, text: text}})
	if errors.Is(err, store.ErrConflict) {
		return false, nil
	}
	return err == nil, err
}

// sendBackForFailure moves item from review back to the queue, first in line, with the failure
// as its note, which wakes the owner within the item's existing scope. A wrap-up still running
// in the Space waits for a later read.
func (s *server) sendBackForFailure(ctx context.Context, item store.WorkItem, text string) (bool, error) {
	proof, err := s.st.Proof(ctx, item.ID)
	if err != nil {
		return false, err
	}
	if l := s.lookup(proof.SpaceID); l != nil {
		l.mu.Lock()
		busy := l.state != _idle
		l.mu.Unlock()
		if busy {
			return false, nil
		}
	}
	if _, err := s.st.SendBack(ctx, item.ID, _reefSendBack+text, store.ByReef); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return false, nil
		}
		return false, err
	}
	note := fmt.Sprintf("%s. Reef sent Work item %d back to its owner.", text, item.ID)
	if _, err := s.st.CreateMessage(ctx, store.Message{SpaceID: proof.SpaceID, Author: store.ReefAuthor, Body: note}); err != nil {
		log.Printf("note send-back in Space %d: %v", proof.SpaceID, err)
	}
	s.notify(proof.SpaceID, "note")
	s.openStarted(ctx)
	return true, nil
}

func (s *server) spaceRequests(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if _, err = s.st.Space(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	rows, err := s.st.SpaceRequests(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}
