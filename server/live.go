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
	"strconv"
	"sync"
	"time"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/channel"
	"github.com/bhanuprakaash/reef/store"
)

const _keepAlive = 15 * time.Second

var errStopping = errors.New("server is stopping")

var _hopReason = fmt.Sprintf("hop cap: %d agent hops without a user message", store.HopCap)

type session interface {
	Say(ctx context.Context, body string, parent *int64) (store.Message, error)
	RunAs(ctx context.Context, role, text string, parent *int64, onEvent func(agent.Event)) error
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
	Role   string `json:"role,omitempty"`
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
	// hops counts the turns agent messages woke since the last user message; held is the
	// queue of wakes a database failure kept from running; retry is the routing read that failed.
	hops  int
	held  []wake
	retry *retry
	// dropped counts the frames of earlier chains of turns, cut from the front of frames.
	dropped int
	wake    chan struct{}
	subs    int
}

// retry is a routing read to do again: the messages role wrote after the message with id
// after.
type retry struct {
	role  string
	after int64
}

// userTurn is a turn that starts with a user message: text, a reply in the thread of parent
// when it is set. A message turn runs for the role the message is routed to, and its reply
// goes in the same thread; wrap and promote turns run for the lead. prepare, when set, runs
// once the agent is open; if it fails no turn starts.
type userTurn struct {
	kind    turnKind
	text    string
	parent  *int64
	prepare func() error
}

// chain is the claim on an idle channel to run a chain of turns. It ends with start, which
// runs the chain, or release, which frees the channel. A stop that arrives before start
// cancels the chain.
type chain struct {
	s      *server
	id     int64
	l      *live
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	// unpause is set when start clears the channel's pause and sets its budget to budget.
	unpause bool
	budget  int64
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
	mux.HandleFunc("GET /channels/{id}/approvals", s.listApprovals)
	mux.HandleFunc("/channels/{id}/approvals", methodNotAllowed)
	mux.HandleFunc("POST /channels/{id}/approvals/{aid}", s.decideApproval)
	mux.HandleFunc("/channels/{id}/approvals/{aid}", methodNotAllowed)
}

// subscribe returns the channel's entry, creating it, and counts a subscriber on it. The
// caller calls unsubscribe when done. Only a turn or a subscriber creates an entry; a
// lookup that must not create one uses lookup.
func (s *server) subscribe(id int64) *live {
	s.mu.Lock()
	defer s.mu.Unlock()
	l := s.entryLocked(id)
	l.mu.Lock()
	l.subs++
	l.mu.Unlock()
	return l
}

func (s *server) unsubscribe(l *live) {
	l.mu.Lock()
	l.subs--
	l.mu.Unlock()
}

func (s *server) entryLocked(id int64) *live {
	l, ok := s.lives[id]
	if !ok {
		l = &live{state: _idle, wake: make(chan struct{})}
		s.lives[id] = l
	}
	return l
}

func (s *server) lookup(id int64) *live {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lives[id]
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
		Text   string `json:"text"`
		Parent *int64 `json:"parent"`
	}
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	if in.Text == "" {
		writeError(w, fmt.Errorf("%w: text is required", store.ErrInvalid))
		return
	}
	t := userTurn{kind: _message, text: in.Text, parent: in.Parent}
	if in.Text == channel.WrapCommand {
		t.kind = _wrap
	}
	if err := s.startUserTurn(r.Context(), id, t); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// claim moves the channel from idle to starting and returns the claim and the channel as
// stored. It creates the channel's entry, so a stop that arrives at any point finds it, and
// refuses a channel that is being deleted.
func (s *server) claim(ctx context.Context, id int64) (*chain, store.Channel, error) {
	if !s.begin() {
		return nil, store.Channel{}, errStopping
	}
	turnCtx, cancel := context.WithCancel(s.ctx)
	c := &chain{s: s, id: id, ctx: turnCtx, cancel: cancel, done: make(chan struct{})}
	if err := s.markStarting(c); err != nil {
		cancel()
		s.wg.Done()
		return nil, store.Channel{}, err
	}
	ch, err := s.st.Channel(ctx, id)
	if err != nil {
		return nil, store.Channel{}, c.release(err)
	}
	return c, ch, nil
}

// markStarting sets c.l to the channel's entry, creating it, and marks it as starting c.
func (s *server) markStarting(c *chain) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deleting[c.id] > 0 {
		return fmt.Errorf("channel %d is being deleted: %w", c.id, store.ErrNotFound)
	}
	l := s.entryLocked(c.id)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.state != _idle {
		return fmt.Errorf("channel %d has a turn running: %w", c.id, store.ErrConflict)
	}
	l.state, l.cancel, l.done = _starting, c.cancel, c.done
	c.l = l
	return nil
}

// dropIdle removes l, the entry of channel id, when it is idle, has no agent and keeps no
// state a later chain needs. A subscribed entry stays unless the channel is deleted.
func (s *server) dropIdle(id int64, l *live, deleted bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l.mu.Lock()
	defer l.mu.Unlock()
	if s.lives[id] != l || l.state != _idle || l.sess != nil {
		return
	}
	if deleted || (l.subs == 0 && l.hops == 0 && len(l.held) == 0 && l.retry == nil) {
		delete(s.lives, id)
	}
}

// startUserTurn stores the user message of t, then starts the turn in the background. A
// hop cap pause does not stop it: the message clears the pause and drops the held wakes.
// Any other pause refuses it.
func (s *server) startUserTurn(ctx context.Context, id int64, t userTurn) error {
	c, ch, err := s.claim(ctx, id)
	if err != nil {
		return err
	}
	if ch.Paused == _hopReason {
		c.lift(&ch, ch.TokenBudget)
	}
	role, err := s.checkChannel(ctx, c.l, t.kind, ch)
	if err != nil {
		return c.release(err)
	}
	sess, err := c.session()
	if err != nil {
		return c.release(err)
	}
	if t.prepare != nil {
		if err := t.prepare(); err != nil {
			return c.release(err)
		}
	}
	msg, err := sess.Say(ctx, t.text, t.parent)
	if err != nil {
		return c.release(err)
	}
	queue := []wake{{from: _userAuthor, role: role, text: t.text, parent: t.parent}}
	if t.kind == _message {
		if queue, err = s.route(ctx, id, []store.Message{msg}, _userAuthor); err != nil {
			return c.release(err)
		}
	}
	return c.start(ctx, sess, t.kind, queue, msg.ID)
}

// runOne runs the turn of w and returns the frame that ends it, the pause it caused, if
// any, and whether it went well.
func (s *server) runOne(
	ctx context.Context, id int64, l *live, sess session, kind turnKind, w wake,
) (end turnState, pause *pauseState, ok bool) {
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
		err = sess.RunAs(ctx, w.role, w.text, w.parent, onEvent)
	}
	end = turnState{Role: w.role, Status: "done"}
	if errors.Is(err, channel.ErrNoProposal) {
		err = nil
		end.Note = channel.ErrNoProposal.Error()
	}
	if err != nil {
		end = turnState{Role: w.role, Status: "failed", Error: err.Error()}
		if ctx.Err() != nil {
			end.Error = "interrupted"
		}
		log.Printf("turn of %s in channel %d failed: %s", w.role, id, end.Error)
		if err := sess.Close(); err != nil {
			log.Printf("close session: %v", err)
		}
	}
	if errors.Is(err, channel.ErrCapped) {
		pause = &pauseState{Reason: err.Error()}
		ch, err := s.st.Channel(context.WithoutCancel(ctx), id)
		if err != nil {
			log.Printf("read channel %d: %v", id, err)
		} else {
			pause.Reason = ch.Paused
		}
	}
	return end, pause, err == nil
}

// checkChannel refuses a turn the channel may not run: a wrap on a quick session, and any
// turn while the channel is paused or over its token budget. It returns the channel's lead
// role, the role a wrap or promote runs for.
func (s *server) checkChannel(
	ctx context.Context, l *live, kind turnKind, ch store.Channel,
) (string, error) {
	if kind == _wrap && ch.FeatureID == nil {
		return "", fmt.Errorf("%w: channel %d is a quick session, promote it before wrapping up",
			store.ErrInvalid, ch.ID)
	}
	pausedNow, err := channel.CheckCaps(ctx, s.st, &ch)
	if pausedNow {
		l.mu.Lock()
		l.publishLocked("pause", pauseState{Reason: ch.Paused})
		l.mu.Unlock()
	}
	return ch.Role, err
}

// gate returns the wakes of ws that may run, and holds the others as pending approvals. When
// an approval cannot be stored it returns the wakes not held so far, and the error.
func (s *server) gate(ctx context.Context, id int64, l *live, ws []wake) ([]wake, error) {
	var open []wake
	for i, w := range ws {
		if !w.gated {
			open = append(open, w)
			continue
		}
		a, err := s.st.CreateApproval(ctx, store.Approval{
			ChannelID: id, From: w.from, To: w.role, MessageID: w.messageID, Text: w.text,
			Parent: w.parent,
		})
		if err != nil {
			return append(open, ws[i:]...), fmt.Errorf("hold wake for %s: %w", w.role, err)
		}
		l.mu.Lock()
		l.publishLocked("approval", a)
		l.mu.Unlock()
	}
	return open, nil
}

// hold stores ws as the holds of a pause on channel id. When one cannot be stored it returns
// the wakes not stored so far, and the error.
func (s *server) hold(ctx context.Context, id int64, ws []wake) ([]wake, error) {
	for i, w := range ws {
		_, err := s.st.CreateApproval(ctx, store.Approval{
			ChannelID: id, From: w.from, To: w.role, MessageID: w.messageID, Text: w.text,
			Parent: w.parent, Kind: store.ApprovalHold,
		})
		if err != nil {
			return ws[i:], fmt.Errorf("hold wake for %s: %w", w.role, err)
		}
	}
	return nil, nil
}

func wakeOf(a store.Approval) wake {
	return wake{from: a.From, messageID: a.MessageID, role: a.To, text: a.Text, parent: a.Parent}
}

// capLocked pauses the channel because the hop cap held a wake, and returns the error of
// the refused turn.
func (s *server) capLocked(ctx context.Context, id int64, l *live) error {
	if err := s.st.PauseChannel(ctx, id, _hopReason); err != nil {
		return err
	}
	l.publishLocked("pause", pauseState{Reason: _hopReason})
	return fmt.Errorf("channel %d is paused: %s: %w", id, _hopReason, store.ErrConflict)
}

// continueChannel clears the pause, and runs the wakes the pause held as a new chain. A
// channel with a turn running is 409; if the held wakes cannot start the channel stays
// paused and keeps them.
func (s *server) continueChannel(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	c, ch, err := s.claim(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	if ch.Paused == "" {
		writeError(w, c.release(fmt.Errorf("channel %d is not paused: %w", id, store.ErrConflict)))
		return
	}
	l := c.l
	l.mu.Lock()
	l.hops = 0
	l.mu.Unlock()
	held, err := s.retryHeld(r.Context(), id, l)
	var holds []store.Approval
	if err == nil {
		holds, err = s.st.ClaimHolds(r.Context(), id)
	}
	for _, a := range holds {
		held = append(held, wakeOf(a))
	}
	if err != nil {
		err = c.release(err)
	} else if len(held) == 0 {
		err = channel.Continue(r.Context(), s.st, id)
		if err == nil {
			l.mu.Lock()
			l.publishLocked("pause", pauseState{})
			l.mu.Unlock()
		}
		err = c.release(err)
	} else if budget, berr := channel.ContinueBudget(r.Context(), s.st, ch); berr != nil {
		err = c.release(berr)
	} else {
		c.lift(&ch, budget)
		err = c.resume(r.Context(), ch, held)
	}
	if err != nil {
		for _, a := range holds {
			if rerr := s.st.ReopenApproval(context.WithoutCancel(r.Context()), a.ID); rerr != nil {
				err = errors.Join(err, rerr)
			}
		}
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// retryHeld does the routing read that failed, if any, and gates the held wakes again. It
// returns the wakes to run. After an error the channel keeps what it could not do.
func (s *server) retryHeld(ctx context.Context, id int64, l *live) ([]wake, error) {
	l.mu.Lock()
	held, again := slices.Clone(l.held), l.retry
	l.mu.Unlock()
	if again != nil {
		ws, _, err := s.wakes(ctx, id, again.after, again.role)
		if err != nil {
			return nil, fmt.Errorf("route messages: %w", err)
		}
		held = append(held, ws...)
	}
	held, err := s.gate(ctx, id, l, held)
	l.mu.Lock()
	l.held, l.retry = held, nil
	l.mu.Unlock()
	return held, err
}

func (s *server) listApprovals(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	as, err := s.st.PendingApprovals(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	if as == nil {
		as = []store.Approval{}
	}
	writeJSON(w, http.StatusOK, as)
}

// decideApproval runs the held wake of a pending approval as a new chain, or drops it.
func (s *server) decideApproval(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	aid, err := strconv.ParseInt(r.PathValue("aid"), 10, 64)
	if err != nil {
		writeError(w, fmt.Errorf("%w: id %q is not a number", store.ErrInvalid, r.PathValue("aid")))
		return
	}
	var in struct {
		Approve *bool `json:"approve"`
	}
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	if in.Approve == nil {
		writeError(w, fmt.Errorf("%w: approve is required", store.ErrInvalid))
		return
	}
	a, err := s.st.Approval(r.Context(), id, aid)
	if err != nil {
		writeError(w, err)
		return
	}
	if a.Status != store.ApprovalPending {
		writeError(w, fmt.Errorf("approval %d is %s: %w", aid, a.Status, store.ErrConflict))
		return
	}
	a.Status = store.ApprovalRejected
	if *in.Approve {
		a.Status = store.ApprovalApproved
	}
	if err := s.st.DecideApproval(r.Context(), aid, a.Status); err != nil {
		writeError(w, err)
		return
	}
	if *in.Approve {
		ctx := context.WithoutCancel(r.Context())
		c, ch, err := s.claim(ctx, id)
		if err == nil {
			err = c.resume(ctx, ch, []wake{wakeOf(a)})
		}
		if err != nil {
			if rerr := s.st.ReopenApproval(ctx, aid); rerr != nil {
				err = errors.Join(err, rerr)
			}
			writeError(w, err)
			return
		}
	}
	if l := s.lookup(id); l != nil {
		l.mu.Lock()
		l.publishLocked("approval", a)
		l.mu.Unlock()
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) stop(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if l := s.lookup(id); l != nil {
		s.stopLive(l)
	}
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
	l := s.subscribe(id)
	defer s.unsubscribe(l)
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
	var sessions []session
	s.mu.Lock()
	for _, l := range s.lives {
		l.mu.Lock()
		if l.sess != nil {
			sessions = append(sessions, l.sess)
			l.sess = nil
		}
		l.mu.Unlock()
	}
	s.mu.Unlock()
	var wg sync.WaitGroup
	for _, sess := range sessions {
		wg.Go(func() {
			if err := sess.Close(); err != nil {
				log.Printf("close session: %v", err)
			}
		})
	}
	wg.Wait()
}

func (c *chain) release(err error) error {
	c.cancel()
	c.l.mu.Lock()
	c.l.state, c.l.cancel = _idle, nil
	close(c.done)
	c.l.mu.Unlock()
	c.s.dropIdle(c.id, c.l, false)
	c.s.wg.Done()
	return err
}

// lift clears the pause in ch and sets its budget, and has start do the same in the store.
func (c *chain) lift(ch *store.Channel, budget int64) {
	ch.Paused, ch.TokenBudget = "", budget
	c.unpause, c.budget = true, budget
}

// session returns the channel's agent session, opening it when it has none. A chain that
// was stopped gets a conflict, and no agent is opened for it.
func (c *chain) session() (session, error) {
	stopped := fmt.Errorf("channel %d was stopped: %w", c.id, store.ErrConflict)
	if c.ctx.Err() != nil {
		return nil, stopped
	}
	c.l.mu.Lock()
	sess := c.l.sess
	c.l.mu.Unlock()
	if sess == nil {
		var err error
		if sess, err = c.s.open(c.ctx, c.id); err != nil {
			if c.ctx.Err() != nil {
				err = stopped
			}
			return nil, err
		}
		c.l.mu.Lock()
		c.l.sess = sess
		c.l.mu.Unlock()
	}
	if c.ctx.Err() != nil {
		return nil, stopped
	}
	return sess, nil
}

// start moves the channel to running and runs queue in the background. A first wake that
// would pass the hop cap pauses the channel instead.
func (c *chain) start(
	ctx context.Context, sess session, kind turnKind, queue []wake, after int64,
) error {
	l := c.l
	l.mu.Lock()
	var err error
	if c.unpause {
		err = c.s.st.ContinueChannel(ctx, c.id, c.budget)
	}
	if err == nil && !l.countHop(queue[0]) {
		err = c.s.capLocked(ctx, c.id, l)
	}
	if err != nil {
		l.mu.Unlock()
		return c.release(err)
	}
	l.state, l.held, l.retry = _running, nil, nil
	l.dropped += len(l.frames)
	l.frames = nil
	if c.unpause {
		l.publishLocked("pause", pauseState{})
	}
	l.publishLocked("turn", turnState{Role: queue[0].role, Status: "running"})
	l.mu.Unlock()
	go c.runTurn(sess, kind, queue, after)
	return nil
}

// resume runs queue as a new chain, after the checks of a turn on ch, with no user message.
func (c *chain) resume(ctx context.Context, ch store.Channel, queue []wake) error {
	if _, err := c.s.checkChannel(ctx, c.l, _message, ch); err != nil {
		return c.release(err)
	}
	sess, err := c.session()
	if err != nil {
		return c.release(err)
	}
	after, err := c.s.st.LastMessageID(ctx, c.id)
	if err != nil {
		return c.release(err)
	}
	return c.start(ctx, sess, _message, queue, after)
}

// runTurn runs the first turn, then the turns its messages wake, one after another, and
// leaves the channel idle once none is left. The queue is the one place a wake is added;
// a wrap or promote turn talks to the user and wakes nobody.
// A turn that fails, or a channel that is paused, ends the chain.
func (c *chain) runTurn(sess session, kind turnKind, queue []wake, after int64) {
	s, l := c.s, c.l
	defer s.wg.Done()
	finishLocked := func() {
		l.cancel()
		l.state, l.cancel = _idle, nil
		close(c.done)
	}
	for {
		var w wake
		w, queue = queue[0], queue[1:]
		end, pause, ok := s.runOne(c.ctx, c.id, l, sess, kind, w)
		var stall error
		var again *retry
		if ok && kind == _message && c.ctx.Err() == nil {
			ws, last, err := s.wakes(c.ctx, c.id, after, w.role)
			if err != nil {
				stall, again = fmt.Errorf("route messages: %w", err), &retry{role: w.role, after: after}
			} else {
				var open []wake
				open, stall = s.gate(c.ctx, c.id, l, ws)
				queue, after = append(queue, open...), last
			}
		}
		l.mu.Lock()
		if !ok {
			l.sess = nil
		}
		l.publishLocked("turn", end)
		if pause != nil {
			l.publishLocked("pause", *pause)
		}
		if stall != nil && c.ctx.Err() == nil {
			l.held, l.retry = queue, again
			reason := stall.Error()
			if err := s.st.PauseChannel(c.ctx, c.id, reason); err != nil {
				log.Printf("pause channel %d: %v", c.id, err)
			} else {
				l.publishLocked("pause", pauseState{Reason: reason})
			}
			finishLocked()
			l.mu.Unlock()
			return
		}
		if !ok || len(queue) == 0 {
			finishLocked()
			l.mu.Unlock()
			return
		}
		l.mu.Unlock()
		kind = _message
		ch, err := s.st.Channel(c.ctx, c.id)
		if err == nil {
			_, err = s.checkChannel(c.ctx, l, kind, ch)
		}
		l.mu.Lock()
		if err == nil && !l.countHop(queue[0]) {
			err = s.capLocked(c.ctx, c.id, l)
		}
		if err != nil {
			if c.ctx.Err() == nil {
				log.Printf("wake %s in channel %d: %v", queue[0].role, c.id, err)
				l.held = queue
				if errors.Is(err, store.ErrConflict) {
					var herr error
					if l.held, herr = s.hold(c.ctx, c.id, queue); herr != nil {
						log.Printf("channel %d: %v", c.id, herr)
					}
				}
			}
			finishLocked()
			l.mu.Unlock()
			return
		}
		l.publishLocked("turn", turnState{Role: queue[0].role, Status: "running"})
		l.mu.Unlock()
	}
}

// countHop counts w as a hop when a message from an agent woke it, and reports whether w
// may run: a wake over the cap may not. A message from the user resets the count.
func (l *live) countHop(w wake) bool {
	if w.from == _userAuthor {
		l.hops = 0
		return true
	}
	if l.hops >= store.HopCap {
		return false
	}
	l.hops++
	return true
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
