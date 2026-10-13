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

// session is the part of channel.Session that the server drives.
type session interface {
	SendAs(ctx context.Context, stored, text string, onEvent func(agent.Event)) error
	Wrap(ctx context.Context, onEvent func(agent.Event)) (store.Proposal, error)
	Promote(ctx context.Context, onEvent func(agent.Event)) (store.Proposal, error)
	Close() error
}

// frame is one server-sent event.
type frame struct {
	event string
	data  []byte
}

type turnState struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type pauseState struct {
	Reason string `json:"reason"`
}

// live is what the server holds for one channel: its session while the agent runs
// and the frames of the current turn for subscribers.
type live struct {
	mu      sync.Mutex
	sess    session
	busy    bool // a send is opening the session or running its turn
	running bool // a turn has started and not ended
	cancel  context.CancelFunc
	done    chan struct{} // closed when the latest turn's goroutine is over
	frames  []frame
	base    int // number of frames dropped before frames[0]
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
		l = &live{wake: make(chan struct{})}
		s.lives[id] = l
	}
	return l
}

// begin takes a place in the server's wait group, or reports that the server is
// closing.
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
	if err := s.startTurn(r.Context(), id, in.Text, nil); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// startTurn starts a turn that stores text as the user message, unless the channel
// has a turn running, is paused or is open elsewhere. prepare, when set, runs once
// the agent is open and the turn is sure to start; if it fails, no turn starts.
func (s *server) startTurn(
	ctx context.Context, id int64, text string, prepare func() error,
) error {
	if !s.begin() {
		return errStopping
	}
	l := s.live(id)
	l.mu.Lock()
	if l.busy {
		l.mu.Unlock()
		s.wg.Done()
		return fmt.Errorf("channel %d has a turn running: %w", id, store.ErrConflict)
	}
	l.busy = true
	sess := l.sess
	l.mu.Unlock()

	release := func() {
		l.mu.Lock()
		l.busy = false
		l.mu.Unlock()
		s.wg.Done()
	}
	if err := s.checkCaps(ctx, id, l); err != nil {
		release()
		return err
	}
	if sess == nil {
		var err error
		if sess, err = s.open(s.ctx, id); err != nil {
			release()
			return err
		}
		l.mu.Lock()
		l.sess = sess
		l.mu.Unlock()
	}
	if prepare != nil {
		if err := prepare(); err != nil {
			release()
			return err
		}
	}
	turnCtx, cancel := context.WithCancel(s.ctx)
	done := make(chan struct{})
	l.mu.Lock()
	l.cancel, l.done, l.running = cancel, done, true
	l.base += len(l.frames)
	l.frames = nil
	l.publishLocked("turn", turnState{Status: "running"})
	l.mu.Unlock()
	go s.runTurn(turnCtx, id, l, sess, text, done)
	return nil
}

func (s *server) runTurn(
	ctx context.Context, id int64, l *live, sess session, text string, done chan struct{},
) {
	defer s.wg.Done()
	onEvent := func(ev agent.Event) {
		l.mu.Lock()
		l.publishLocked("agent", ev)
		l.mu.Unlock()
	}
	var err error
	switch text {
	case channel.WrapCommand:
		_, err = sess.Wrap(ctx, onEvent)
	case channel.PromoteCommand:
		_, err = sess.Promote(ctx, onEvent)
	default:
		err = sess.SendAs(ctx, text, text, onEvent)
	}
	if errors.Is(err, channel.ErrNoProposal) {
		err = nil
	}
	end := turnState{Status: "done"}
	if err != nil {
		end = turnState{Status: "failed", Error: err.Error()}
		if ctx.Err() != nil {
			end.Error = "interrupted"
		}
		if err := sess.Close(); err != nil {
			log.Printf("close session: %v", err)
		}
	}
	var paused string
	if errors.Is(err, channel.ErrCapped) {
		ch, err := s.st.Channel(context.WithoutCancel(ctx), id)
		if err != nil {
			log.Printf("read channel %d: %v", id, err)
		}
		paused = ch.Paused
	}
	l.mu.Lock()
	if err != nil {
		l.sess = nil
	}
	l.publishLocked("turn", end)
	if errors.Is(err, channel.ErrCapped) {
		l.publishLocked("pause", pauseState{Reason: paused})
	}
	l.running, l.busy, l.cancel = false, false, nil
	close(done)
	l.mu.Unlock()
}

// checkCaps refuses a send to a paused channel. A channel that has used its token
// budget is paused first.
func (s *server) checkCaps(ctx context.Context, id int64, l *live) error {
	ch, err := s.st.Channel(ctx, id)
	if err != nil {
		return err
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

// continueChannel clears the pause of a channel. One that has used its token
// budget gets store.TokenBudget more tokens.
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
	l := s.live(id)
	l.mu.Lock()
	cancel, done := l.cancel, l.done
	l.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
	l.mu.Lock()
	sess, idle := l.sess, !l.busy
	if sess != nil && idle {
		l.sess, l.busy = nil, true
	}
	l.mu.Unlock()
	if sess != nil && idle {
		if err := sess.Close(); err != nil {
			log.Printf("close session: %v", err)
		}
		l.mu.Lock()
		l.busy = false
		l.mu.Unlock()
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) events(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if _, err := s.st.Channel(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	l := s.live(id)
	l.mu.Lock()
	next := l.base + len(l.frames)
	if l.running {
		next = l.base
	}
	l.mu.Unlock()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	rc := http.NewResponseController(w)
	if err := rc.Flush(); err != nil {
		return
	}
	tick := time.NewTicker(_keepAlive)
	defer tick.Stop()
	for {
		l.mu.Lock()
		next = max(next, l.base)
		pending := slices.Clone(l.frames[next-l.base:])
		wake := l.wake
		l.mu.Unlock()
		for _, f := range pending {
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", f.event, f.data); err != nil {
				return
			}
		}
		next += len(pending)
		if len(pending) > 0 && rc.Flush() != nil {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-s.quit:
			return
		case <-wake:
		case <-tick.C:
			if _, err := io.WriteString(w, ": keepalive\n\n"); err != nil || rc.Flush() != nil {
				return
			}
		}
	}
}

// publishLocked adds an event to the current turn and wakes the subscribers. It
// never waits for a subscriber.
func (l *live) publishLocked(event string, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		log.Printf("encode %s event: %v", event, err)
		return
	}
	l.frames = append(l.frames, frame{event: event, data: data})
	close(l.wake)
	l.wake = make(chan struct{})
}

// close interrupts every turn, waits until each is stored, ends the event streams
// and closes the sessions.
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
