package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/channel"
	"github.com/bhanuprakaash/reef/store"
)

const _keepAlive = 15 * time.Second

var errStopping = errors.New("server is stopping")

type session interface {
	Say(ctx context.Context, body string) error
	Run(ctx context.Context, text string, onEvent func(agent.Event)) error
	Wrap(ctx context.Context, onEvent func(agent.Event)) (store.Proposal, error)
	Promote(ctx context.Context, onEvent func(agent.Event)) (store.Proposal, error)
	Close() error
}

var _ session = (*channel.Session)(nil)

type frame struct {
	event string
	data  []byte
}

type turnState struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
	Note   string `json:"note,omitempty"`
}

type pauseState struct {
	Reason string `json:"reason"`
}

type turnKind int

const (
	_message turnKind = iota + 1
	_wrap
	_promote
)

type liveState int

const (
	_idle liveState = iota + 1
	_starting
	_running
	_stopping
)

type live struct {
	mu     sync.Mutex
	sess   session
	state  liveState
	cancel context.CancelFunc
	done   chan struct{}
	frames []frame
	// dropped counts the frames of earlier turns, cut from the front of frames.
	dropped int
	wake    chan struct{}
}

func (s *server) liveRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /channels/{id}/send", s.send)
	mux.HandleFunc("/channels/{id}/send", methodNotAllowed)
	mux.HandleFunc("GET /channels/{id}/events", s.events)
	mux.HandleFunc("/channels/{id}/events", methodNotAllowed)
	mux.HandleFunc("POST /channels/{id}/stop", s.stop)
	mux.HandleFunc("/channels/{id}/stop", methodNotAllowed)
	mux.HandleFunc("POST /channels/{id}/continue", s.continueChannel)
	mux.HandleFunc("/channels/{id}/continue", methodNotAllowed)
}

func (s *server) live(id int64) *live {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.lives[id]
	if !ok {
		l = &live{state: _idle, wake: make(chan struct{})}
		s.lives[id] = l
	}
	return l
}

func (s *server) begin() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s.wg.Add(1)
	return true
}

func (s *server) send(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in struct {
		Text string `json:"text"`
	}
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	if in.Text == "" {
		writeError(w, fmt.Errorf("%w: text is required", store.ErrInvalid))
		return
	}
	kind := _message
	if in.Text == channel.WrapCommand {
		kind = _wrap
	}
	if err := s.startTurn(r.Context(), id, kind, in.Text, nil); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// startTurn stores text as the user message, then starts the turn in the background. It
// runs prepare, when set, once the agent is open; if it fails no turn starts. A stop that
// arrives before the turn runs cancels it.
func (s *server) startTurn(
	ctx context.Context, id int64, kind turnKind, text string, prepare func() error,
) error {
	if !s.begin() {
		return errStopping
	}
	l := s.live(id)
	turnCtx, cancel := context.WithCancel(s.ctx)
	done := make(chan struct{})
	l.mu.Lock()
	if l.state != _idle {
		l.mu.Unlock()
		cancel()
		s.wg.Done()
		return fmt.Errorf("channel %d has a turn running: %w", id, store.ErrConflict)
	}
	l.state, l.cancel, l.done = _starting, cancel, done
	sess := l.sess
	l.mu.Unlock()

	giveUp := func(err error) error {
		cancel()
		l.mu.Lock()
		l.state, l.cancel = _idle, nil
		close(done)
		l.mu.Unlock()
		s.wg.Done()
		return err
	}
	if err := s.checkTurn(ctx, id, l, kind); err != nil {
		return giveUp(err)
	}
	if sess == nil {
		var err error
		if sess, err = s.open(turnCtx, id); err != nil {
			if turnCtx.Err() != nil {
				err = fmt.Errorf("channel %d was stopped: %w", id, store.ErrConflict)
			}
			return giveUp(err)
		}
		l.mu.Lock()
		l.sess = sess
		l.mu.Unlock()
	}
	if turnCtx.Err() != nil {
		return giveUp(fmt.Errorf("channel %d was stopped: %w", id, store.ErrConflict))
	}
	if prepare != nil {
		if err := prepare(); err != nil {
			return giveUp(err)
		}
	}
	if err := sess.Say(ctx, text); err != nil {
		return giveUp(err)
	}
	l.mu.Lock()
	l.state = _running
	l.dropped += len(l.frames)
	l.frames = nil
	l.publishLocked("turn", turnState{Status: "running"})
	l.mu.Unlock()
	go s.runTurn(turnCtx, id, l, sess, kind, text, done)
	return nil
}

func (s *server) runTurn(
	ctx context.Context, id int64, l *live, sess session, kind turnKind, text string,
	done chan struct{},
) {
	defer s.wg.Done()
	onEvent := func(ev agent.Event) {
		l.mu.Lock()
		l.publishLocked("agent", ev)
		l.mu.Unlock()
	}
	var err error
	switch kind {
	case _wrap:
		_, err = sess.Wrap(ctx, onEvent)
	case _promote:
		_, err = sess.Promote(ctx, onEvent)
	default:
		err = sess.Run(ctx, text, onEvent)
	}
	end := turnState{Status: "done"}
	if errors.Is(err, channel.ErrNoProposal) {
		err = nil
		end.Note = channel.ErrNoProposal.Error()
	}
	if err != nil {
		end = turnState{Status: "failed", Error: err.Error()}
		if ctx.Err() != nil {
			end.Error = "interrupted"
		}
		if err := sess.Close(); err != nil {
			log.Printf("close session: %v", err)
		}
	}
	capped := errors.Is(err, channel.ErrCapped)
	var pauseReason string
	if capped {
		pauseReason = err.Error()
		ch, err := s.st.Channel(context.WithoutCancel(ctx), id)
		if err != nil {
			log.Printf("read channel %d: %v", id, err)
		} else {
			pauseReason = ch.Paused
		}
	}
	l.mu.Lock()
	if err != nil {
		l.sess = nil
	}
	l.publishLocked("turn", end)
	if capped {
		l.publishLocked("pause", pauseState{Reason: pauseReason})
	}
	l.cancel()
	l.state, l.cancel = _idle, nil
	close(done)
	l.mu.Unlock()
}

// checkTurn refuses a turn the channel may not run: a wrap on a quick session, and any
// turn while the channel is paused or over its token budget.
func (s *server) checkTurn(ctx context.Context, id int64, l *live, kind turnKind) error {
	ch, err := s.st.Channel(ctx, id)
	if err != nil {
		return err
	}
	if kind == _wrap && ch.FeatureID == nil {
		return fmt.Errorf("%w: channel %d is a quick session, promote it before wrapping up",
			store.ErrInvalid, id)
	}
	if ch.Paused == "" {
		used, err := s.st.ChannelTokens(ctx, id)
		if err != nil {
			return err
		}
		if used < ch.TokenBudget {
			return nil
		}
		ch.Paused = fmt.Sprintf("token budget of %d used: %d tokens", ch.TokenBudget, used)
		if err := s.st.PauseChannel(ctx, id, ch.Paused); err != nil {
			return err
		}
		l.mu.Lock()
		l.publishLocked("pause", pauseState{Reason: ch.Paused})
		l.mu.Unlock()
	}
	return fmt.Errorf("channel %d is paused: %s: %w", id, ch.Paused, store.ErrConflict)
}

func (s *server) continueChannel(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	ch, err := s.st.Channel(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	if ch.Paused == "" {
		writeError(w, fmt.Errorf("channel %d is not paused: %w", id, store.ErrConflict))
		return
	}
	used, err := s.st.ChannelTokens(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	var extra int64
	if used >= ch.TokenBudget {
		extra = store.TokenBudget
	}
	if err := s.st.ContinueChannel(r.Context(), id, extra); err != nil {
		writeError(w, err)
		return
	}
	l := s.live(id)
	l.mu.Lock()
	l.publishLocked("pause", pauseState{})
	l.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) stop(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	s.stopLive(s.live(id))
	w.WriteHeader(http.StatusNoContent)
}

// stopLive cancels the channel's turn and waits for it to end, then closes the agent.
func (s *server) stopLive(l *live) {
	l.mu.Lock()
	cancel, done := l.cancel, l.done
	l.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
	l.mu.Lock()
	sess := l.sess
	if sess == nil || l.state != _idle {
		l.mu.Unlock()
		return
	}
	l.sess, l.state = nil, _stopping
	l.mu.Unlock()
	if err := sess.Close(); err != nil {
		log.Printf("close session: %v", err)
	}
	l.mu.Lock()
	l.state = _idle
	l.mu.Unlock()
}

// events sends the channel's state first, so a subscriber that connects or reconnects
// at any time ends up in step: the running turn and its events so far, or an idle turn,
// then the pause when the channel is paused.
func (s *server) events(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	ch, err := s.st.Channel(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	var first []frame
	add := func(event string, v any) {
		if f, ok := newFrame(event, v); ok {
			first = append(first, f)
		}
	}
	l := s.live(id)
	l.mu.Lock()
	next := l.dropped + len(l.frames)
	if l.state == _running {
		next = l.dropped
	} else {
		add("turn", turnState{Status: "idle"})
	}
	l.mu.Unlock()
	if ch.Paused != "" {
		add("pause", pauseState{Reason: ch.Paused})
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	rc := http.NewResponseController(w)
	if err := writeFrames(w, first); err != nil || rc.Flush() != nil {
		return
	}
	tick := time.NewTicker(_keepAlive)
	defer tick.Stop()
	for {
		quitting := false
		select {
		case <-s.quit:
			quitting = true
		default:
		}
		l.mu.Lock()
		next = max(next, l.dropped)
		pending := slices.Clone(l.frames[next-l.dropped:])
		wake := l.wake
		l.mu.Unlock()
		if err := writeFrames(w, pending); err != nil {
			return
		}
		next += len(pending)
		if len(pending) > 0 && rc.Flush() != nil {
			return
		}
		if quitting {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-s.quit:
		case <-wake:
		case <-tick.C:
			if _, err := io.WriteString(w, ": keepalive\n\n"); err != nil || rc.Flush() != nil {
				return
			}
		}
	}
}

func writeFrames(w io.Writer, frames []frame) error {
	for _, f := range frames {
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", f.event, f.data); err != nil {
			return err
		}
	}
	return nil
}

func newFrame(event string, v any) (frame, bool) {
	data, err := json.Marshal(v)
	if err != nil {
		log.Printf("encode %s event: %v", event, err)
		return frame{}, false
	}
	return frame{event: event, data: data}, true
}

// publishLocked never waits for a subscriber.
func (l *live) publishLocked(event string, v any) {
	f, ok := newFrame(event, v)
	if !ok {
		return
	}
	l.frames = append(l.frames, f)
	close(l.wake)
	l.wake = make(chan struct{})
}

func (s *server) close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.mu.Unlock()
	s.cancel()
	s.wg.Wait()
	close(s.quit)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, l := range s.lives {
		l.mu.Lock()
		sess := l.sess
		l.sess = nil
		l.mu.Unlock()
		if sess == nil {
			continue
		}
		if err := sess.Close(); err != nil {
			log.Printf("close session: %v", err)
		}
	}
}
