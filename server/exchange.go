package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/bhanuprakaash/reef/store"
)

func (s *server) exchangeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /exchanges/{id}", s.getExchange)
	mux.HandleFunc("/exchanges/{id}", methodNotAllowed)
	mux.HandleFunc("POST /exchanges/{id}/retry", s.retryExchange)
	mux.HandleFunc("/exchanges/{id}/retry", methodNotAllowed)
	mux.HandleFunc("POST /exchanges/{id}/cancel", s.cancelExchange)
	mux.HandleFunc("/exchanges/{id}/cancel", methodNotAllowed)
	mux.HandleFunc("POST /spaces/{id}/bots/{botId}/guidance", s.guideBot)
	mux.HandleFunc("/spaces/{id}/bots/{botId}/guidance", methodNotAllowed)
}

func (s *server) getExchange(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	ex, err := s.st.Exchange(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ex)
}

// retryExchange moves a failed or stopped exchange back to waiting and delivers it again.
func (s *server) retryExchange(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	dm, err := s.st.RetryExchange(r.Context(), id)
	if err == nil {
		err = s.deliverExchange(r.Context(), dm.SpaceID, dm.ID)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	s.getExchange(w, r)
}

func (s *server) cancelExchange(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	dm, err := s.st.CancelExchange(r.Context(), id, "")
	if err != nil {
		writeError(w, err)
		return
	}
	s.notify(dm.SpaceID, "direct")
	s.getExchange(w, r)
}

// guideBot records what the user tells a member Bot of Space id, as a message in the Space that
// links the guidance exchange, and delivers it at the next turn boundary of the Space. A paused
// Space keeps it waiting. The same key sent twice makes one exchange.
func (s *server) guideBot(w http.ResponseWriter, r *http.Request) {
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
		Body string `json:"body"`
		Key  string `json:"key"`
	}
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	if in.Key == "" {
		writeError(w, fmt.Errorf("%w: key is required", store.ErrInvalid))
		return
	}
	ctx := r.Context()
	ch, err := s.st.Space(ctx, id)
	if err == nil {
		err = ch.Writable()
	}
	var team store.Team
	if err == nil {
		team, err = s.st.SpaceTeam(ctx, id)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	i := slices.IndexFunc(team.Roles, func(m store.Role) bool { return m.BotID == botID })
	if i < 0 {
		writeError(w, fmt.Errorf("%w: Bot %d is not a member of Space %d", store.ErrNotFound, botID, id))
		return
	}
	key := "guidance:" + in.Key
	dm, created, err := s.st.CreateExchange(ctx, store.DirectMessage{
		SpaceID: id, OriginSpaceID: &id, From: _userAuthor, To: team.Roles[i].Name,
		ToBotID: &botID, Body: in.Body, Kind: store.ExchangeGuidance, RequestKey: &key,
	}, "To "+team.Roles[i].Name+": "+in.Body)
	if err == nil && created {
		s.notify(id, "note")
		err = s.deliverExchange(ctx, id, dm.ID)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	ex, err := s.st.Exchange(ctx, dm.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, ex)
}

// deliverExchange runs waiting exchange id for its recipient in Space spaceID: queued behind the
// turn a running chain is on, or as a new chain when the Space is idle. A paused Space leaves it
// waiting, and continue delivers it. An exchange that is not waiting is left alone, and one the
// Space cannot run, because it is closed or the recipient is not a member, fails with a response
// that says why. A turn is created only from the waiting state, so delivering twice runs once.
func (s *server) deliverExchange(ctx context.Context, spaceID, id int64) error {
	dm, err := s.st.DirectMessage(ctx, id)
	if err != nil {
		return err
	}
	if dm.SpaceID != spaceID || dm.ReplyToID != nil || dm.OriginSpaceID == nil {
		return fmt.Errorf("%w: direct message %d is not an exchange of Space %d", store.ErrInvalid, id, spaceID)
	}
	if dm.State != store.DirectWaiting {
		return nil
	}
	ch, err := s.st.Space(ctx, spaceID)
	if err != nil {
		return err
	}
	if reason := s.undeliverable(ctx, ch, dm); reason != "" {
		if err := recordFailedDirect(ctx, s.st, id, dm.To, reason); err != nil {
			return err
		}
		s.notify(spaceID, "direct")
		return nil
	}
	if ch.Paused != "" {
		return nil
	}
	err = s.runWake(ctx, spaceID, directWake(&dm))
	if errors.Is(err, errBusy) {
		return s.parkBusy(ctx, spaceID)
	}
	return err
}

// runWake queues w behind the turn a running chain of Space spaceID is on, or starts a chain
// for it when the Space is idle.
func (s *server) runWake(ctx context.Context, spaceID int64, w wake) error {
	for range 3 {
		if queued, err := s.queueWake(ctx, spaceID, w); queued || err != nil {
			return err
		}
		c, ch, err := s.claim(ctx, spaceID, _message)
		if errors.Is(err, store.ErrConflict) {
			continue
		}
		if err != nil {
			return err
		}
		return c.resume(ctx, ch, []wake{w})
	}
	return fmt.Errorf("space %d has a turn running that cannot take the wake: %w", spaceID, errBusy)
}

var errBusy = fmt.Errorf("a turn the wake cannot join is running: %w", store.ErrConflict)

// parkBusy pauses Space id, which has a turn running that cannot take a wake, unless it is
// paused already, so continue delivers what waits for it.
func (s *server) parkBusy(ctx context.Context, id int64) error {
	ch, err := s.st.Space(ctx, id)
	if err != nil || ch.Paused != "" {
		return err
	}
	const reason = "a turn that cannot take a wake was running: continue to deliver what waits"
	if err := s.st.PauseSpace(ctx, id, reason); err != nil {
		return err
	}
	if l := s.lookup(id); l != nil {
		l.mu.Lock()
		l.publishLocked("pause", pauseState{Reason: reason})
		l.mu.Unlock()
	}
	return nil
}

// deliverAsked delivers the exchanges that Bot botID asked for in its turn in Space spaceID,
// which began at since: those the tool call made and left waiting.
func (s *server) deliverAsked(ctx context.Context, spaceID int64, botID *int64, since time.Time) {
	for _, ex := range s.askedWaiting(ctx, spaceID, botID, since) {
		if err := s.deliverExchange(ctx, ex.SpaceID, ex.ID); err != nil {
			log.Printf("deliver exchange %d: %v", ex.ID, err)
		}
	}
}

// cancelAsked withdraws the exchanges that Bot botID asked for in its turn in Space spaceID,
// which began at since, when the turn stopped or failed, so a later turn does not deliver them.
func (s *server) cancelAsked(
	ctx context.Context, spaceID int64, botID *int64, since time.Time, how string,
) {
	for _, ex := range s.askedWaiting(ctx, spaceID, botID, since) {
		if _, err := s.st.CancelExchange(ctx, ex.ID, "the asking turn "+how); err != nil {
			log.Printf("cancel exchange %d: %v", ex.ID, err)
			continue
		}
		s.notify(spaceID, "direct")
	}
}

func (s *server) askedWaiting(
	ctx context.Context, spaceID int64, botID *int64, since time.Time,
) []store.DirectMessage {
	exs, err := s.st.SpaceExchanges(ctx, spaceID)
	if err != nil {
		log.Printf("list exchanges of Space %d: %v", spaceID, err)
		return nil
	}
	return slices.DeleteFunc(exs, func(ex store.DirectMessage) bool {
		return ex.State != store.DirectWaiting || ex.SpaceID == spaceID || botID == nil ||
			ex.FromBotID == nil || *ex.FromBotID != *botID || ex.CreatedAt.Before(since)
	})
}

// wakeAsker wakes the Bot that asked exchange id from its origin conversation, origin, with the
// reply stored there, once the exchange has one. The wake is the hold stored with the reply: it
// stays pending while origin is paused, and continue or a restart releases it.
func (s *server) wakeAsker(ctx context.Context, id, origin int64) {
	ch, err := s.st.Space(ctx, origin)
	if err != nil || ch.Paused != "" {
		if err != nil {
			log.Printf("read Space %d: %v", origin, err)
		}
		return
	}
	h, ok, err := s.st.ClaimExchangeHold(ctx, id)
	if err != nil || !ok {
		if err != nil {
			log.Printf("claim the reply wake of exchange %d: %v", id, err)
		}
		return
	}
	w, err := s.wakeOf(ctx, h)
	if err == nil {
		err = s.runWake(ctx, origin, w)
	}
	if err == nil {
		return
	}
	log.Printf("wake %s for the reply to exchange %d: %v", h.To, id, err)
	if rerr := s.st.ReopenHold(ctx, h.ID); rerr != nil {
		log.Printf("reopen hold %d: %v", h.ID, rerr)
	}
	if errors.Is(err, errBusy) {
		if err := s.parkBusy(ctx, origin); err != nil {
			log.Printf("pause Space %d: %v", origin, err)
		}
	}
}

// undeliverable returns why Space ch cannot run exchange dm, or "".
func (s *server) undeliverable(ctx context.Context, ch store.Space, dm store.DirectMessage) string {
	if err := ch.Writable(); err != nil {
		return err.Error()
	}
	team, err := s.st.SpaceTeam(ctx, ch.ID)
	if err != nil {
		return err.Error()
	}
	if !slices.ContainsFunc(team.Roles, func(r store.Role) bool { return r.BotID == *dm.ToBotID }) {
		return fmt.Sprintf("%s is not a member of Space %d", dm.To, ch.ID)
	}
	return ""
}

// queueWake queues w behind the turn a running message chain is on. It reports false, having
// changed nothing, when the Space is not running a message chain.
func (s *server) queueWake(ctx context.Context, id int64, w wake) (bool, error) {
	l := s.lookup(id)
	if l == nil {
		return false, nil
	}
	if err := l.awaitStart(ctx); err != nil || !l.running() {
		return false, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.messagingLocked() {
		return false, nil
	}
	same := func(q wake) bool {
		return (w.direct != nil && q.direct != nil && q.direct.ID == w.direct.ID) ||
			(w.hold != 0 && q.hold == w.hold)
	}
	if slices.ContainsFunc(l.queued, same) || slices.ContainsFunc(l.held, same) {
		return true, nil
	}
	l.queued = append(l.queued, w)
	return true, nil
}

// withWaitingExchanges adds to held the waiting exchanges of Space id that it does not hold yet,
// only those of user guidance when guidanceOnly is set.
func (s *server) withWaitingExchanges(
	ctx context.Context, id int64, held []wake, guidanceOnly bool,
) ([]wake, error) {
	waiting, err := s.st.WaitingExchanges(ctx, id)
	if err != nil {
		return nil, err
	}
	for i := range waiting {
		if guidanceOnly && !waiting[i].IsUserGuidance() {
			continue
		}
		if !slices.ContainsFunc(held, func(h wake) bool { return h.direct != nil && h.direct.ID == waiting[i].ID }) {
			held = append(held, directWake(&waiting[i]))
		}
	}
	return held, nil
}
