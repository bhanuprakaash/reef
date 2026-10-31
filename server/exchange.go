package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/bhanuprakaash/reef/store"
)

func (s *server) exchangeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /exchanges/{id}", s.getExchange)
	mux.HandleFunc("/exchanges/{id}", methodNotAllowed)
	mux.HandleFunc("POST /exchanges/{id}/retry", s.retryExchange)
	mux.HandleFunc("/exchanges/{id}/retry", methodNotAllowed)
	mux.HandleFunc("POST /exchanges/{id}/cancel", s.cancelExchange)
	mux.HandleFunc("/exchanges/{id}/cancel", methodNotAllowed)
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

// retryExchange moves a failed exchange back to waiting and delivers it again.
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
	dm, err := s.st.CancelExchange(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	s.notify(dm.SpaceID, "direct")
	s.getExchange(w, r)
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
	w := directWake(&dm)
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
	return fmt.Errorf("space %d has a turn running that cannot take the exchange: %w", spaceID, store.ErrConflict)
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
	if slices.ContainsFunc(l.queued, func(q wake) bool { return q.direct != nil && q.direct.ID == w.direct.ID }) {
		return true, nil
	}
	l.queued = append(l.queued, w)
	return true, nil
}

// withWaitingExchanges adds to held the waiting exchanges of Space id that it does not hold yet.
func (s *server) withWaitingExchanges(ctx context.Context, id int64, held []wake) ([]wake, error) {
	waiting, err := s.st.WaitingExchanges(ctx, id)
	if err != nil {
		return nil, err
	}
	for i := range waiting {
		if !slices.ContainsFunc(held, func(h wake) bool { return h.direct != nil && h.direct.ID == waiting[i].ID }) {
			held = append(held, directWake(&waiting[i]))
		}
	}
	return held, nil
}
