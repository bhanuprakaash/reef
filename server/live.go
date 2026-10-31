package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/space"
	"github.com/bhanuprakaash/reef/store"
)

const _keepAlive = 15 * time.Second

const _nameTimeout = time.Minute

var errStopping = errors.New("server is stopping")

const _hopReasonPrefix = "hop cap:"

// talkTool reports whether a tool call keeps a turn talk-only: Reef's own MCP tools, except the
// browser ones, and ToolSearch, which claude calls only to load those tools. Any other tool call
// is work and resets the hop count.
func talkTool(name string) bool {
	return name == "ToolSearch" ||
		strings.HasPrefix(name, "mcp__reef__") && !strings.HasPrefix(name, "mcp__reef__browser_")
}

const _restartReason = store.RestartReason

var _hopReason = fmt.Sprintf("%s %d agent hops without a user message", _hopReasonPrefix, store.HopCap)

type session interface {
	Say(ctx context.Context, body string, parent *int64) (store.Message, error)
	RunBot(
		ctx context.Context, botID int64, text string, parent, messageID *int64, onEvent func(agent.Event),
	) error
	WakeBot(ctx context.Context, botID int64, routed string, scopes store.Scopes, parent, messageID *int64,
		onEvent func(agent.Event)) error
	RunDirectBot(ctx context.Context, botID int64, text string, dm store.DirectMessage,
		onEvent func(agent.Event)) error
	Wrap(ctx context.Context, onEvent func(agent.Event)) (store.Proposal, error)
	Refresh(ctx context.Context, botID int64) error
	Drop(botID int64) error
	SetOnNote(f func())
	Close() error
}

var _ session = (*space.Session)(nil)

type frame struct {
	event string
	data  []byte
}

type turnState struct {
	Role            string `json:"role,omitempty"`
	BotID           *int64 `json:"botId,omitempty"`
	Status          string `json:"status"`
	Error           string `json:"error,omitempty"`
	Note            string `json:"note,omitempty"`
	DirectMessageID *int64 `json:"directMessageId,omitempty"`
}

type pauseState struct {
	Reason string `json:"reason"`
}

type turnKind int

const (
	_noTurn turnKind = iota
	_message
	_wrap
	_move
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
	kind   turnKind
	cancel context.CancelFunc
	done   chan struct{}
	frames []frame
	// ready is closed when the starting chain is running or has ended.
	ready chan struct{}
	// hops counts the turns agent messages woke since the last user message or the last turn
	// that called a tool other than Reef's own; held is the queue of wakes a database failure
	// kept from running; retry is the routing read that failed.
	hops  int
	held  []wake
	retry *retry
	// shed is the part of held that a stopped or failed chain left unrun; the next chain notes
	// it, unless continue runs it.
	shed []wake
	// drop is the revoked Bots whose agent closes before the next turn, as a turn is running.
	drop []int64
	// leave is the Bots that leave the Space when the turn that is running ends.
	leave []leaving
	// turnBot and turnCancel are the Bot of the turn that is running and the cancel of that turn
	// alone; revoked is set when that turn was cancelled because its Bot was revoked, and the
	// chain goes on.
	turnBot    *int64
	turnCancel context.CancelFunc
	revoked    bool
	// queued is the wakes that arrived while a chain starts or runs; the chain runs them after
	// its current turn, or gives them back when it ends first.
	queued []wake
	// dropped counts the frames of earlier chains of turns, cut from the front of frames.
	dropped   int
	activeGen uint64
	wake      chan struct{}
	subs      int
	changed   func()
}

// retry is a routing read to do again: the messages role wrote after the message with id
// after, and the direct messages after the one with id afterDirect.
type retry struct {
	role        string
	botID       *int64
	after       int64
	afterDirect int64
}

// userTurn is a turn that starts with a user message: text, a reply in the thread of parent
// when it is set. A message turn runs for the role the message is routed to, and its reply
// goes in the same thread; a wrap turn runs for the lead; a move turn runs for the lead too, and
// moves the conversation to a Space with the Bot to.
type userTurn struct {
	kind   turnKind
	text   string
	parent *int64
	to     store.Bot
}

// chain is the claim on an idle Space to run a chain of turns. It ends with start, which
// runs the chain, or release, which frees the Space. A stop that arrives before start
// cancels the chain.
type chain struct {
	s              *server
	id             int64
	l              *live
	ready          chan struct{}
	gen            uint64
	ownsActiveTurn bool
	// hint is the active-turn kind this chain last stored. It clears only that value, so a
	// teardown that outruns a newer chain cannot blank the newer chain's hint.
	hint   string
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	// unpause is set when start clears the Space's pause and sets its budget to budget.
	unpause bool
	budget  int64
}

// directFail is a wake a chain ends without running, whose sender is told that delivery
// did not complete.
type directFail struct {
	id   int64
	role string
	body string
}

func (s *server) liveRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /spaces/{id}/send", s.send)
	mux.HandleFunc("/spaces/{id}/send", methodNotAllowed)
	mux.HandleFunc("GET /spaces/{id}/events", s.events)
	mux.HandleFunc("/spaces/{id}/events", methodNotAllowed)
	mux.HandleFunc("POST /spaces/{id}/stop", s.stop)
	mux.HandleFunc("/spaces/{id}/stop", methodNotAllowed)
	mux.HandleFunc("POST /spaces/{id}/continue", s.continueSpace)
	mux.HandleFunc("/spaces/{id}/continue", methodNotAllowed)
	mux.HandleFunc("POST /spaces/{id}/bots/{botId}/refresh", s.refreshBot)
	mux.HandleFunc("/spaces/{id}/bots/{botId}/refresh", methodNotAllowed)
}

// subscribe returns the Space's entry, creating it, and counts a subscriber on it. The
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
		l = &live{state: _idle, wake: make(chan struct{}), changed: s.inboxChanged}
		s.lives[id] = l
	}
	return l
}

func (s *server) lookup(id int64) *live {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lives[id]
}

// notify tells an open stream on the Space that something changed outside a turn, such as a
// github poll note ("note") or a failed direct response recorded after the chain's last turn
// frame ("direct"), so the window refreshes without a reload. A Space nobody has opened yet
// has no entry and is a no-op.
func (s *server) notify(id int64, event string) {
	l := s.lookup(id)
	if l == nil {
		return
	}
	l.mu.Lock()
	l.publishLocked(event, struct{}{})
	l.mu.Unlock()
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
	to, moves, err := s.moveTarget(r.Context(), id, in.Text)
	if err != nil {
		writeError(w, err)
		return
	}
	switch {
	case in.Text == space.WrapCommand:
		t.kind = _wrap
	case moves:
		t.kind, t.to = _move, to
	}
	if err := s.startUserTurn(r.Context(), id, t); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// claim moves the Space from idle to starting and returns the claim and the Space as
// stored. It creates the Space's entry, so a stop that arrives at any point finds it, and
// refuses a Space that is being deleted. kind is the kind of chain the caller will run, or
// _noTurn when it runs none: only a send that finds a message chain starting waits for it.
func (s *server) claim(ctx context.Context, id int64, kind turnKind) (*chain, store.Space, error) {
	if !s.begin() {
		return nil, store.Space{}, errStopping
	}
	turnCtx, cancel := context.WithCancel(s.ctx)
	c := &chain{
		s: s, id: id, ctx: turnCtx, cancel: cancel, done: make(chan struct{}),
		ready: make(chan struct{}),
	}
	if err := s.markStarting(c, kind); err != nil {
		cancel()
		s.wg.Done()
		return nil, store.Space{}, err
	}
	if err := s.st.SetActiveTurn(context.Background(), c.id, store.ActiveTurnStarting); err != nil {
		return nil, store.Space{}, c.release(err)
	}
	c.hint = store.ActiveTurnStarting
	ch, err := s.st.Space(ctx, id)
	if err != nil {
		return nil, store.Space{}, c.release(err)
	}
	return c, ch, nil
}

// markStarting sets c.l to the Space's entry, creating it, and marks it as starting c, a
// chain of kind.
func (s *server) markStarting(c *chain, kind turnKind) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deleting[c.id] > 0 {
		return fmt.Errorf("space %d is being deleted: %w", c.id, store.ErrNotFound)
	}
	l := s.entryLocked(c.id)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.state != _idle {
		return fmt.Errorf("space %d has a turn running: %w", c.id, store.ErrConflict)
	}
	l.state, l.kind, l.cancel, l.done, l.ready = _starting, kind, c.cancel, c.done, c.ready
	l.activeGen++
	c.gen = l.activeGen
	c.ownsActiveTurn = true
	c.l = l
	return nil
}

// dropIdle removes l, the entry of Space id, when it is idle, has no agent and keeps no
// state a later chain needs. A subscribed entry stays unless the Space is deleted.
func (s *server) dropIdle(id int64, l *live, deleted bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l.mu.Lock()
	defer l.mu.Unlock()
	if s.lives[id] != l || l.state != _idle || l.sess != nil {
		return
	}
	if deleted || (l.subs == 0 && l.hops == 0 && len(l.held) == 0 && l.retry == nil && len(l.drop) == 0 && len(l.leave) == 0) {
		delete(s.lives, id)
	}
}

// startUserTurn stores the user message of t, then starts the turn in the background. A
// hop cap pause does not stop it: the message clears the pause and drops the held wakes.
// Any other pause refuses it.
func (s *server) startUserTurn(ctx context.Context, id int64, t userTurn) error {
	if t.kind == _message {
		if queued, err := s.queueMessage(ctx, id, t); queued || err != nil {
			return err
		}
	}
	c, ch, err := s.claim(ctx, id, t.kind)
	if err != nil {
		return err
	}
	if strings.HasPrefix(ch.Paused, _hopReasonPrefix) {
		c.lift(&ch, ch.TokenBudget)
	}
	role, err := s.checkSpace(ctx, c.l, t.kind, ch)
	if err != nil {
		return c.release(err)
	}
	sess, err := c.session()
	if err != nil {
		return c.release(err)
	}
	msg, err := sess.Say(ctx, t.text, t.parent)
	if err != nil {
		return c.release(err)
	}
	team, err := s.st.SpaceTeam(ctx, id)
	if err != nil {
		return c.release(err)
	}
	if len(team.Roles) == 0 {
		return c.release(fmt.Errorf("Space %d has no lead: %w", id, store.ErrConflict))
	}
	if t.kind == _message {
		s.nameSpace(ctx, id, team.Roles[0].Backend, t.text)
	}
	leadID := team.Roles[0].BotID
	queue := []wake{{from: _userAuthor, role: role, botID: &leadID, text: t.text, parent: t.parent}}
	switch t.kind {
	case _message:
		if queue, err = s.route(ctx, id, []store.Message{msg}, nil); err != nil {
			return c.release(err)
		}
	case _move:
		queue[0].text, queue[0].messageID, queue[0].moveTo = movePrompt(t.to.Name, t.text), msg.ID, t.to.ID
	}
	return c.start(ctx, sess, t.kind, queue, msg.ID)
}

// nameSpace starts the one attempt to name a Space created with a default name, from its
// first user message. The name lands only if no rename ended the attempt first; a failed
// attempt keeps the name the Space has.
func (s *server) nameSpace(ctx context.Context, id int64, backend, text string) {
	ok, err := s.st.ClaimName(ctx, id)
	if err != nil {
		log.Printf("claim name of Space %d: %v", id, err)
		return
	}
	if !ok || !s.begin() {
		return
	}
	go func() {
		defer s.wg.Done()
		nctx, cancel := context.WithTimeout(s.ctx, _nameTimeout)
		defer cancel()
		name, err := s.name(nctx, backend, text)
		if err != nil {
			log.Printf("name Space %d: %v", id, err)
			name = ""
		}
		applied, err := s.st.FinishName(context.Background(), id, name)
		if err != nil {
			log.Printf("name Space %d: %v", id, err)
			return
		}
		if applied && name != "" {
			s.notify(id, "name")
		}
	}()
}

// queueMessage stores the message of t and queues its wakes behind the turn a running message
// chain is on, with the checks of a turn. A message chain that is starting is waited for, so
// the message is stored after the one that chain starts with. It reports false, having changed
// nothing, when the Space is not running a message chain.
func (s *server) queueMessage(ctx context.Context, id int64, t userTurn) (bool, error) {
	l := s.lookup(id)
	if l == nil {
		return false, nil
	}
	if err := l.awaitStart(ctx); err != nil || !l.running() {
		return false, err
	}
	ch, err := s.st.Space(ctx, id)
	if err != nil {
		return false, err
	}
	if _, err := s.checkSpace(ctx, l, t.kind, ch); err != nil {
		return false, err
	}
	l.mu.Lock()
	if l.state == _starting {
		l.mu.Unlock()
		return s.queueMessage(ctx, id, t)
	}
	defer l.mu.Unlock()
	if !l.messagingLocked() {
		return false, nil
	}
	msg, err := s.st.CreateMessage(ctx, store.Message{
		SpaceID: id, ParentID: t.parent, Author: _userAuthor, Body: t.text,
	})
	if err != nil {
		return false, err
	}
	ws, err := s.route(ctx, id, []store.Message{msg}, nil)
	if err != nil {
		return false, err
	}
	l.hops = 0
	l.queued = append(l.queued, ws...)
	return true, nil
}

// runOne runs the turn of w and returns the frame that ends it, whether it went well and whether
// a failure closed the session. A turn cancelled by its Bot's revoke leaves the session open.
func (s *server) runOne(
	ctx context.Context, id int64, l *live, sess session, kind turnKind, w wake,
) (end turnState, ok, closed bool) {
	onEvent := func(ev agent.Event) {
		l.mu.Lock()
		if ev.Kind == agent.KindToolCall && !talkTool(ev.ToolName) {
			l.hops = 0
		}
		l.publishLocked("agent", ev)
		l.mu.Unlock()
	}
	var err error
	if w.botID == nil {
		err = fmt.Errorf("%w: wake has no Bot ID", store.ErrInvalid)
	} else {
		bot, lookupErr := s.st.Bot(ctx, *w.botID)
		err = lookupErr
		if err == nil {
			w.role = bot.Name
		}
	}
	if err == nil {
		err = s.applyPending(ctx, id, l, sess)
	}
	if err == nil {
		switch kind {
		case _wrap:
			_, err = sess.Wrap(ctx, onEvent)
		case _move:
			if err = sess.RunBot(ctx, *w.botID, w.text, nil, w.wakeMessage(), onEvent); err == nil {
				err = s.finishMove(ctx, id, w)
			}
		default:
			if w.direct != nil {
				err = sess.RunDirectBot(ctx, *w.botID, w.text, *w.direct, onEvent)
			} else if len(w.scopes) > 0 {
				err = sess.WakeBot(ctx, *w.botID, w.text, w.scopes, w.parent, w.wakeMessage(), onEvent)
			} else {
				err = sess.RunBot(ctx, *w.botID, w.text, w.parent, w.wakeMessage(), onEvent)
			}
		}
	}
	if errors.Is(err, store.ErrNotWaiting) {
		return turnState{Role: w.role, BotID: w.botID, Status: "done", Note: err.Error()}, true, false
	}
	end = turnState{Role: w.role, BotID: w.botID, Status: "done", DirectMessageID: w.direct.IDRef()}
	if errors.Is(err, space.ErrNoProposal) {
		err = nil
		end.Note = space.ErrNoProposal.Error()
	}
	if err != nil {
		end = turnState{Role: w.role, BotID: w.botID, Status: "failed", Error: err.Error(), DirectMessageID: w.direct.IDRef()}
		if ctx.Err() != nil {
			end.Error = "interrupted"
		}
		if w.direct != nil {
			request, state := w.direct.ID, store.DirectFailed
			if w.direct.ReplyToID != nil {
				request = *w.direct.ReplyToID
			}
			if end.Error == "interrupted" {
				state = store.DirectStopped
			}
			if err := s.st.SetDirectRequestState(context.WithoutCancel(ctx), request, state); err != nil {
				log.Print(err)
			}
		}
		log.Printf("turn of %s in Space %d failed: %s", w.role, id, end.Error)
		l.mu.Lock()
		revoked := l.revoked
		l.mu.Unlock()
		if !revoked {
			closed = true
			if err := sess.Close(); err != nil {
				log.Printf("close session: %v", err)
			}
		}
	}
	return end, err == nil, closed
}

// checkSpace refuses a turn the Space may not run: any turn on a closed or moved Space or one
// whose work item is dropped, a wrap on a Space with no work item or while a wrap-up waits for approval, and any turn while the
// Space is paused or over its token budget. It returns the Space's lead role, the role a wrap
// runs for.
func (s *server) checkSpace(
	ctx context.Context, l *live, kind turnKind, ch store.Space,
) (string, error) {
	if err := ch.Writable(); err != nil {
		return "", err
	}
	if ch.WorkItemID != nil {
		w, err := s.st.WorkItem(ctx, *ch.WorkItemID)
		if err != nil {
			return "", err
		}
		if w.Status == "dropped" {
			return "", fmt.Errorf("work item %d is dropped: %w", w.ID, store.ErrConflict)
		}
	}
	if kind == _wrap && ch.WorkItemID == nil {
		return "", fmt.Errorf("%w: Space %d has no work item, attach it to one before wrapping up",
			store.ErrInvalid, ch.ID)
	}
	if kind == _wrap {
		p, err := s.st.PendingProposal(ctx, ch.ID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return "", err
		}
		if err == nil && p.Kind == store.ProposalWrap {
			return "", fmt.Errorf("space %d has a wrap-up waiting for approval: %w", ch.ID, store.ErrConflict)
		}
	}
	pausedNow, err := space.CheckCaps(ctx, s.st, &ch)
	if pausedNow {
		l.mu.Lock()
		l.publishLocked("pause", pauseState{Reason: ch.Paused})
		l.mu.Unlock()
	}
	if err != nil {
		return "", err
	}
	team, err := s.st.SpaceTeam(ctx, ch.ID)
	if err != nil {
		return "", err
	}
	if len(team.Roles) == 0 {
		return "", fmt.Errorf("%w: Space has no lead", store.ErrInvalid)
	}
	return team.Roles[0].Name, nil
}

// hold stores ws as the holds of a pause on Space id. When one cannot be stored it returns
// the wakes not stored so far, and the error.
func (s *server) hold(ctx context.Context, id int64, ws []wake) ([]wake, error) {
	for i, w := range ws {
		_, err := s.st.CreateHold(ctx, holdForWake(w, id, w.text))
		if err != nil {
			return ws[i:], fmt.Errorf("hold wake for %s: %w", w.role, err)
		}
	}
	return nil, nil
}

// holdForWake maps a wake to the row that holds it. A direct wake stores the
// direct message id instead of a public message id, so the two are never confused.
func holdForWake(w wake, spaceID int64, text string) store.Hold {
	h := store.Hold{
		SpaceID: spaceID, From: w.from, To: w.role, FromBotID: w.fromBotID, ToBotID: w.botID, MessageID: w.messageID,
		Text: text, Parent: w.parent, Scopes: w.scopes,
	}
	if w.direct != nil {
		h.MessageID = 0
		idv := w.direct.ID
		h.DirectMessageID = &idv
	}
	return h
}

func (s *server) wakeOf(ctx context.Context, h store.Hold) (wake, error) {
	w := wake{
		from: h.From, fromBotID: h.FromBotID, botID: h.ToBotID, messageID: h.MessageID, role: h.To, text: h.Text, parent: h.Parent,
		hold: h.ID, scopes: h.Scopes,
	}
	if h.DirectMessageID != nil {
		dm, err := s.st.DirectMessage(ctx, *h.DirectMessageID)
		if err != nil {
			return wake{}, fmt.Errorf("load direct message %d: %w", *h.DirectMessageID, err)
		}
		dmCopy := dm
		w.direct = &dmCopy
		w.messageID = dm.ID
	}
	return w, nil
}

// failDirect records that a held direct wake will not run, so the message list tells it from a
// delivery that failed.
func (s *server) failDirect(ctx context.Context, spaceID int64, h store.Hold, body string) {
	ctx = context.WithoutCancel(ctx)
	if err := recordFailedDirect(ctx, s.st, *h.DirectMessageID, h.To, body); err != nil {
		log.Printf("%s direct message %d: %v", body, *h.DirectMessageID, err)
		return
	}
	s.notify(spaceID, "direct")
}

// capLocked pauses the Space because the hop cap held a wake, and returns the error of
// the refused turn.
func (s *server) capLocked(ctx context.Context, id int64, l *live) error {
	if err := s.st.PauseSpace(ctx, id, _hopReason); err != nil {
		return err
	}
	l.publishLocked("pause", pauseState{Reason: _hopReason})
	return fmt.Errorf("space %d is paused: %s: %w", id, _hopReason, store.ErrConflict)
}

func (s *server) pauseLocked(ctx context.Context, id int64, l *live, reason string) {
	if err := s.st.PauseSpace(ctx, id, reason); err != nil {
		log.Printf("pause Space %d: %v", id, err)
		return
	}
	l.publishLocked("pause", pauseState{Reason: reason})
}

// continueSpace clears the pause, and runs the wakes the pause held as a new chain. A
// Space with a turn running is 409; if the held wakes cannot start the Space stays
// paused and keeps them.
func (s *server) continueSpace(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	c, ch, err := s.claim(r.Context(), id, _noTurn)
	if err != nil {
		writeError(w, err)
		return
	}
	if ch.Paused == "" {
		writeError(w, c.release(fmt.Errorf("space %d is not paused: %w", id, store.ErrConflict)))
		return
	}
	l := c.l
	l.mu.Lock()
	l.hops = 0
	l.mu.Unlock()
	held, err := s.retryHeld(r.Context(), id, l)
	var holds []store.Hold
	if err == nil {
		holds, err = s.st.ClaimHolds(r.Context(), id)
	}
	var wakeErr error
	for _, h := range holds {
		w, err := s.wakeOf(r.Context(), h)
		if err != nil {
			wakeErr = err
			if rerr := s.st.ReopenHold(context.WithoutCancel(r.Context()), h.ID); rerr != nil {
				wakeErr = errors.Join(wakeErr, rerr)
			}
			break
		}
		held = append(held, w)
	}
	if err == nil && wakeErr == nil {
		held, err = s.withWaitingExchanges(r.Context(), id, held)
	}
	if wakeErr != nil {
		for _, h := range holds {
			if rerr := s.st.ReopenHold(context.WithoutCancel(r.Context()), h.ID); rerr != nil {
				wakeErr = errors.Join(wakeErr, rerr)
			}
		}
		err = c.release(wakeErr)
		writeError(w, err)
		return
	}
	if err != nil {
		err = c.release(err)
	} else if len(held) == 0 {
		err = space.Continue(r.Context(), s.st, id)
		if err == nil {
			l.mu.Lock()
			l.publishLocked("pause", pauseState{})
			l.mu.Unlock()
		}
		err = c.release(err)
	} else if budget, berr := space.ContinueBudget(r.Context(), s.st, ch); berr != nil {
		err = c.release(berr)
	} else {
		c.lift(&ch, budget)
		err = c.resume(r.Context(), ch, held)
	}
	if err != nil {
		for _, h := range holds {
			if rerr := s.st.ReopenHold(context.WithoutCancel(r.Context()), h.ID); rerr != nil {
				err = errors.Join(err, rerr)
			}
		}
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// retryHeld does the routing read that failed, if any, and returns the wakes to run. After
// an error the Space keeps what it could not do.
func (s *server) retryHeld(ctx context.Context, id int64, l *live) ([]wake, error) {
	l.mu.Lock()
	held, again := slices.Clone(l.held), l.retry
	l.mu.Unlock()
	if again != nil {
		ws, _, err := s.wakes(ctx, id, again.after, again.botID)
		if err != nil {
			return nil, fmt.Errorf("route messages: %w", err)
		}
		direct, _, err := s.directWakes(ctx, id, again.afterDirect, again.botID)
		if err != nil {
			return nil, fmt.Errorf("route direct messages: %w", err)
		}
		held = append(held, inStoredOrder(ws, direct)...)
	}
	l.mu.Lock()
	l.held, l.retry, l.shed = held, nil, nil
	l.mu.Unlock()
	return held, nil
}

// failSent gives every request role sent after direct message after, during the turn that
// just ended unrun by a stop or a failure, a failed response with body "stopped" or "failed".
func (s *server) failSent(ctx context.Context, spaceID int64, botID *int64, after int64, stopped bool) {
	body := "failed"
	if stopped {
		body = "stopped"
	}
	msgs, err := s.st.DirectMessages(ctx, spaceID, after)
	if err != nil {
		log.Printf("list direct messages of Space %d: %v", spaceID, err)
		return
	}
	for _, m := range msgs {
		if botID == nil || m.FromBotID == nil || *m.FromBotID != *botID || m.ReplyToID != nil ||
			m.OriginSpaceID != nil {
			continue
		}
		if err := recordFailedDirect(ctx, s.st, m.ID, m.To, body); err != nil {
			log.Printf("direct response for %d: %v", m.ID, err)
			continue
		}
		s.notify(spaceID, "direct")
	}
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

// refreshBot has a Bot of the Space adopt its current settings and start a new backend context at
// its next turn. A model or backend in the body is written to the Bot first. A turn that is running
// finishes first: the refresh is kept and applied before the next turn starts, and the reply says
// pending.
func (s *server) refreshBot(w http.ResponseWriter, r *http.Request) {
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
		Model   string `json:"model"`
		Backend string `json:"backend"`
	}
	if r.ContentLength != 0 {
		if err := readJSON(r, &in); err != nil {
			writeError(w, err)
			return
		}
	}
	team, err := s.st.SpaceTeam(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	if !slices.ContainsFunc(team.Roles, func(m store.Role) bool { return m.BotID == botID }) {
		writeError(w, fmt.Errorf("%w: Bot %d is not a member of Space %d", store.ErrNotFound, botID, id))
		return
	}
	if in.Model != "" || in.Backend != "" {
		bot, err := s.st.Bot(r.Context(), botID)
		if err != nil {
			writeError(w, err)
			return
		}
		if in.Model != "" {
			bot.Model = in.Model
		}
		if in.Backend != "" {
			bot.Backend = in.Backend
		}
		if _, err := s.st.UpdateBot(r.Context(), bot); err != nil {
			writeError(w, err)
			return
		}
	}
	c, _, err := s.claim(r.Context(), id, _noTurn)
	if errors.Is(err, store.ErrConflict) {
		pending, err := s.st.SetRefreshPending(r.Context(), id, botID)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]bool{"pending": pending})
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	c.l.mu.Lock()
	sess := c.l.sess
	c.l.mu.Unlock()
	if sess != nil {
		err = sess.Refresh(r.Context(), botID)
	} else {
		_, err = space.AdoptBotSession(r.Context(), s.st, s.dataDir, id, botID)
	}
	if err = c.release(err); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"pending": false})
}

// applyPending closes the agents of the Bots revoked during the turn that ended, and refreshes the
// Bots whose refresh waited for it, also after a restart. A refresh that fails stops waiting and
// gets one notice in the Space, and refreshing again retries it; it fails no turn.
func (s *server) applyPending(ctx context.Context, id int64, l *live, sess session) error {
	l.mu.Lock()
	drop := l.drop
	l.drop = nil
	l.mu.Unlock()
	for _, botID := range drop {
		if err := sess.Drop(botID); err != nil {
			log.Printf("close revoked Bot %d of Space %d: %v", botID, id, err)
		}
	}
	bots, err := s.st.RefreshPendingBots(ctx, id)
	if err != nil {
		return err
	}
	for _, botID := range bots {
		err := sess.Refresh(ctx, botID)
		if err == nil {
			continue
		}
		log.Printf("refresh Bot %d of Space %d: %v", botID, id, err)
		if cerr := s.st.ClearRefreshPending(context.WithoutCancel(ctx), id, botID); cerr != nil {
			return cerr
		}
		name := fmt.Sprintf("Bot %d", botID)
		if bot, berr := s.st.Bot(ctx, botID); berr == nil {
			name = bot.Name
		}
		if _, err := s.st.CreateMessage(context.WithoutCancel(ctx), store.Message{
			SpaceID: id, Author: store.ReefAuthor, Body: name + " could not refresh and keeps its earlier settings until refreshed again: " + err.Error(),
		}); err != nil {
			log.Printf("note refresh failure in Space %d: %v", id, err)
			continue
		}
		s.notify(id, "note")
	}
	return nil
}

// dropBot stops the revoked Bot in the Space and no other. Its running turn is interrupted when
// the turn is its own; its agent closes at once when the Space is idle, else before the next turn.
func (s *server) dropBot(ctx context.Context, spaceID int64, l *live, botID int64) {
	l.mu.Lock()
	if l.state != _idle {
		if l.turnBot != nil && *l.turnBot == botID {
			l.revoked = true
			l.turnCancel()
		} else {
			l.drop = append(l.drop, botID)
		}
		l.mu.Unlock()
		return
	}
	l.mu.Unlock()
	c, _, err := s.claim(ctx, spaceID, _noTurn)
	if errors.Is(err, store.ErrConflict) {
		l.mu.Lock()
		l.drop = append(l.drop, botID)
		l.mu.Unlock()
		return
	}
	if err != nil {
		log.Printf("close revoked Bot %d of Space %d: %v", botID, spaceID, err)
		return
	}
	c.l.mu.Lock()
	sess := c.l.sess
	c.l.mu.Unlock()
	var cerr error
	if sess != nil {
		cerr = sess.Drop(botID)
	}
	if err := c.release(cerr); err != nil {
		log.Printf("close revoked Bot %d of Space %d: %v", botID, spaceID, err)
	}
}

// stopLive cancels the Space's turn and waits for it to end, then closes the agent.
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

// events sends the Space's state first, so a subscriber that connects or reconnects
// at any time ends up in step: the running turn and its events so far, or an idle turn,
// then the pause when the Space is paused.
func (s *server) events(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	ch, err := s.st.Space(r.Context(), id)
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
	s.holdQueued()
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

// holdQueued stores the wakes the stopped chains left in memory as holds, and pauses their
// Spaces so continue runs them after a restart. A Space already paused keeps its reason.
// A routing read that failed is done again first, and what it wakes is held too; when it fails
// again, a message from reef tells the user which messages woke nobody.
func (s *server) holdQueued() {
	ctx := context.Background()
	s.mu.Lock()
	lives := maps.Clone(s.lives)
	s.mu.Unlock()
	for id, l := range lives {
		l.mu.Lock()
		again := l.retry
		l.mu.Unlock()
		if again != nil {
			if _, err := s.retryHeld(ctx, id, l); err != nil {
				log.Printf("space %d: %v", id, err)
				s.noteUnrouted(ctx, id, again.role, err)
			}
		}
		l.mu.Lock()
		held := slices.Clone(l.held)
		l.mu.Unlock()
		if len(held) == 0 {
			continue
		}
		if _, err := s.hold(ctx, id, held); err != nil {
			log.Printf("space %d: %v", id, err)
			continue
		}
		ch, err := s.st.Space(ctx, id)
		if err == nil && ch.Paused == "" {
			err = s.st.PauseSpace(ctx, id, _restartReason)
		}
		if err != nil {
			log.Printf("pause Space %d: %v", id, err)
		}
	}
}

// noteUnrouted posts a message from reef in Space id saying that the messages role wrote last
// were not routed, so what they asked for did not run.
func (s *server) noteUnrouted(ctx context.Context, id int64, role string, cause error) {
	body := fmt.Sprintf("The app stopped before the last messages of %s were routed: %v. "+
		"Anyone they asked was not woken; send a message to ask again.", role, cause)
	if _, err := s.st.CreateMessage(ctx, store.Message{
		SpaceID: id, Author: store.ReefAuthor, Body: body,
	}); err != nil {
		log.Printf("note unrouted messages in Space %d: %v", id, err)
	}
}

// noteDropped posts a message from reef in Space id naming the roles whose wakes a stopped
// or failed turn left unrun, which a new chain does not run.
func (s *server) noteDropped(ctx context.Context, id int64, ws []wake) {
	var roles []string
	for _, w := range ws {
		if !slices.Contains(roles, w.role) {
			roles = append(roles, w.role)
		}
	}
	body := fmt.Sprintf("Wakes for %s did not run: the turn before them was stopped or failed. "+
		"Send a message to ask again.", strings.Join(roles, ", "))
	if _, err := s.st.CreateMessage(ctx, store.Message{
		SpaceID: id, Author: store.ReefAuthor, Body: body,
	}); err != nil {
		log.Printf("note dropped wakes in Space %d: %v", id, err)
	}
}

func (c *chain) release(err error) error {
	stopped := c.ctx.Err() != nil
	c.cancel()
	var directFailed []directFail
	var holds []int64
	if c.l != nil {
		c.l.mu.Lock()
		directFailed, holds = c.shedLocked(nil, stopped)
		// Only the chain that still owns the live state may retire it. An older chain
		// whose teardown ran late must not clear a newer chain's starting/running state.
		mine := c.gen == c.l.activeGen
		if mine {
			c.l.state, c.l.cancel = _idle, nil
			c.l.activeGen = 0
		}
		c.l.mu.Unlock()
		c.finalize(mine, directFailed, holds)
		close(c.done)
		close(c.ready)
		c.s.dropIdle(c.id, c.l, false)
	} else {
		close(c.done)
	}
	c.s.wg.Done()
	return err
}

// finalize performs the store work of ending a chain outside the Space lock: failed
// direct responses for the wakes it did not run, reopened holds, and clearing the
// active-turn hint when this chain still owns the live state.
func (c *chain) finalize(mine bool, directFailed []directFail, holds []int64) {
	for _, f := range directFailed {
		if err := recordFailedDirect(context.WithoutCancel(c.ctx), c.s.st, f.id, f.role, f.body); err != nil {
			log.Printf("direct response for %d: %v", f.id, err)
		}
	}
	if len(directFailed) > 0 {
		c.s.notify(c.id, "direct")
	}
	for _, id := range holds {
		if err := c.s.st.ReopenHold(context.WithoutCancel(c.ctx), id); err != nil {
			log.Printf("reopen hold %d of Space %d: %v", id, c.id, err)
		}
	}
	if mine && c.ownsActiveTurn {
		c.ownsActiveTurn = false
		if err := c.s.st.ClearActiveTurnIf(context.WithoutCancel(c.ctx), c.id, c.hint); err != nil {
			log.Printf("clear active turn %d: %v", c.id, err)
		}
	}
}

// shedLocked restores the state of the wakes a chain ends without running, including the
// wake of the leftover queue: a claimed hold goes back to pending, a direct wake records a
// failed response so its sender sees that delivery did not complete (body "stopped" when
// the chain was stopped), and a public wake is kept as held. It returns the deferred store
// work for the caller to perform outside the Space lock.
func (c *chain) shedLocked(left []wake, stopped bool) (directFailed []directFail, holds []int64) {
	body := ""
	if stopped {
		body = "stopped"
	}
	kept := len(c.l.held)
	closing := c.s.ctx.Err() != nil
	for _, w := range c.l.queued {
		if w.hold != 0 {
			holds = append(holds, w.hold)
			continue
		}
		if w.direct != nil && closing && w.direct.OriginSpaceID != nil {
			continue
		}
		if w.direct != nil {
			directFailed = append(directFailed, directFail{w.direct.ID, w.role, body})
			continue
		}
		c.l.held = append(c.l.held, w)
	}
	c.l.queued = nil
	for _, w := range left {
		if w.hold != 0 {
			holds = append(holds, w.hold)
			continue
		}
		if w.direct != nil && closing && w.direct.OriginSpaceID != nil {
			continue
		}
		if w.direct != nil {
			directFailed = append(directFailed, directFail{w.direct.ID, w.role, body})
			continue
		}
		c.l.held = append(c.l.held, w)
	}
	c.l.shed = append(c.l.shed, c.l.held[kept:]...)
	return
}

// lift clears the pause in ch and sets its budget, and has start do the same in the store.
func (c *chain) lift(ch *store.Space, budget int64) {
	ch.Paused, ch.TokenBudget = "", budget
	c.unpause, c.budget = true, budget
}

// session returns the Space's agent session, opening it when it has none. A chain that
// was stopped gets a conflict, and no agent is opened for it.
func (c *chain) session() (session, error) {
	stopped := fmt.Errorf("space %d was stopped: %w", c.id, store.ErrConflict)
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
		sess.SetOnNote(func() { c.s.notify(c.id, "note") })
		c.l.mu.Lock()
		c.l.sess = sess
		c.l.mu.Unlock()
	}
	if c.ctx.Err() != nil {
		return nil, stopped
	}
	return sess, nil
}

// start moves the Space to running and runs queue in the background. A first wake that
// would pass the hop cap pauses the Space instead.
func (c *chain) start(
	ctx context.Context, sess session, kind turnKind, queue []wake, after int64,
) error {
	kindStr := store.ActiveTurnMessage
	if kind == _wrap {
		kindStr = store.ActiveTurnWrap
	}
	if c.ownsActiveTurn {
		if err := c.s.st.SetActiveTurn(ctx, c.id, kindStr); err != nil {
			log.Printf("set active turn %d: %v", c.id, err)
		} else {
			c.hint = kindStr
		}
	}
	if queue[0].botID == nil {
		return c.release(fmt.Errorf("%w: wake has no Bot ID", store.ErrInvalid))
	}
	bot, err := c.s.st.Bot(ctx, *queue[0].botID)
	if err != nil {
		return c.release(err)
	}
	queue[0].role = bot.Name
	afterDirect, err := c.s.st.LastDirectMessageID(ctx, c.id)
	if err != nil {
		return c.release(err)
	}
	l := c.l
	l.mu.Lock()
	err = nil
	var dropped []store.Hold
	if c.unpause {
		if dropped, err = c.s.st.PendingHolds(ctx, c.id); err == nil {
			err = c.s.st.ContinueSpace(ctx, c.id, c.budget)
		}
	}
	if err == nil && !l.countHop(queue[0]) {
		err = c.s.capLocked(ctx, c.id, l)
	}
	if err != nil {
		l.mu.Unlock()
		return c.release(err)
	}
	if len(l.shed) > 0 {
		c.s.noteDropped(ctx, c.id, l.shed)
	}
	l.state, l.kind, l.held, l.retry, l.shed = _running, kind, nil, nil, nil
	l.dropped += len(l.frames)
	l.frames = nil
	if c.unpause {
		l.publishLocked("pause", pauseState{})
	}
	l.publishLocked("turn", turnState{
		Role: queue[0].role, BotID: queue[0].botID, Status: "running", DirectMessageID: queue[0].direct.IDRef()})
	l.mu.Unlock()
	for _, h := range dropped {
		if h.DirectMessageID != nil {
			c.s.failDirect(ctx, c.id, h, "dropped")
		}
	}
	close(c.ready)
	go c.runTurn(sess, kind, queue, after, afterDirect)
	return nil
}

// resume runs queue as a new chain, after the checks of a turn on ch, with no user message.
func (c *chain) resume(ctx context.Context, ch store.Space, queue []wake) error {
	if _, err := c.s.checkSpace(ctx, c.l, _message, ch); err != nil {
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
// leaves the Space idle once none is left. The queue is the one place a wake is added;
// a wrap turn talks to the user and wakes nobody.
// A turn that fails, or a Space that is paused, ends the chain.
func (c *chain) runTurn(sess session, kind turnKind, queue []wake, after, afterDirect int64) {
	s, l := c.s, c.l
	defer s.wg.Done()
	defer s.inboxChanged()
	defer s.startSubmitted(c.id)
	// finish ends the chain owning the live state, and hands the wakes of remaining, the
	// queue left before the next turn, to shedLocked. Callers pass the leftover queue when
	// the chain dies outside a turn (failed or stopped turn), and nil when a stall or cap
	// error already moved the queue to the Space's held ones.
	finish := func(remaining []wake) {
		directFailed, holds := c.shedLocked(remaining, c.ctx.Err() != nil)
		mine := c.gen == l.activeGen
		if mine {
			// Only the chain that still owns the live state may retire it; an older
			// chain whose teardown ran late leaves a newer chain's state alone.
			l.cancel()
			l.state, l.cancel = _idle, nil
			l.activeGen = 0
		}
		l.mu.Unlock()
		c.finalize(mine, directFailed, holds)
		close(c.done)
	}
	for {
		var w wake
		w, queue = queue[0], queue[1:]
		turnCtx, cancelTurn := context.WithCancel(c.ctx)
		l.mu.Lock()
		l.turnBot, l.turnCancel = w.botID, cancelTurn
		l.mu.Unlock()
		end, ok, closed := s.runOne(turnCtx, c.id, l, sess, kind, w)
		cancelTurn()
		l.mu.Lock()
		revoked := l.revoked
		l.turnBot, l.turnCancel, l.revoked = nil, nil, false
		l.mu.Unlock()
		if !ok || c.ctx.Err() != nil {
			s.failSent(context.WithoutCancel(c.ctx), c.id, w.botID, afterDirect, c.ctx.Err() != nil)
		} else if w.botID != nil {
			s.reviewMemory(c.id, *w.botID)
		}
		if ok && c.ctx.Err() == nil {
			if kind == _message {
				s.deliverAsked(c.ctx, c.id, w.botID)
			}
			if d := w.direct; d != nil && d.ReplyToID == nil && d.OriginSpaceID != nil && *d.OriginSpaceID != c.id {
				s.wakeAsker(c.ctx, d.ID)
			}
		}
		if revoked && !closed && c.ctx.Err() == nil {
			ok = true
			if err := sess.Drop(*w.botID); err != nil {
				log.Printf("close revoked Bot %d of Space %d: %v", *w.botID, c.id, err)
			}
		}
		var stall error
		var again *retry
		if ok && kind == _message && c.ctx.Err() == nil {
			ws, last, err := s.wakes(c.ctx, c.id, after, w.botID)
			if err != nil {
				stall = fmt.Errorf("route messages: %w", err)
				again = &retry{role: w.role, botID: w.botID, after: after, afterDirect: afterDirect}
			} else {
				direct, directLast, directErr := s.directWakes(c.ctx, c.id, afterDirect, w.botID)
				if directErr != nil {
					stall = fmt.Errorf("route direct messages: %w", directErr)
					again = &retry{role: w.role, botID: w.botID, after: after, afterDirect: afterDirect}
				} else {
					afterDirect = directLast
					queue = append(queue, inStoredOrder(ws, direct)...)
				}
				after = last
			}
		}
		for {
			l.mu.Lock()
			drop := l.drop
			l.drop = nil
			if len(drop) == 0 {
				break
			}
			l.mu.Unlock()
			for _, botID := range drop {
				if err := sess.Drop(botID); err != nil {
					log.Printf("close revoked Bot %d of Space %d: %v", botID, c.id, err)
				}
			}
		}
		leave := l.leave
		l.leave = nil
		l.mu.Unlock()
		if len(leave) > 0 {
			s.applyLeaving(c.ctx, c.id, sess, leave)
		}
		team, teamErr := s.st.SpaceTeam(context.WithoutCancel(c.ctx), c.id)
		l.mu.Lock()
		if teamErr == nil {
			queue, l.queued = keepMembers(queue, team), keepMembers(l.queued, team)
		}
		if !ok {
			l.sess = nil
		}
		l.publishLocked("turn", end)
		if stall != nil && c.ctx.Err() == nil {
			l.held, l.retry = queue, again
			s.pauseLocked(c.ctx, c.id, l, stall.Error())
			finish(nil)
			return
		}
		if !ok || c.ctx.Err() != nil || len(queue)+len(l.queued) == 0 {
			// A turn failed or was stopped: the wakes the chain does not run go back to a
			// waiting or failed state instead of silently dropping in place.
			finish(queue)
			return
		}
		queue, l.queued = append(queue, l.queued...), nil
		l.mu.Unlock()
		kind = _message
		ch, err := s.st.Space(c.ctx, c.id)
		if err == nil {
			_, err = s.checkSpace(c.ctx, l, kind, ch)
		}
		if err == nil {
			if queue[0].botID == nil {
				err = fmt.Errorf("%w: wake has no Bot ID", store.ErrInvalid)
			} else {
				var bot store.Bot
				bot, err = s.st.Bot(c.ctx, *queue[0].botID)
				if err == nil {
					queue[0].role = bot.Name
				}
			}
		}
		l.mu.Lock()
		if err == nil && !l.countHop(queue[0]) {
			err = s.capLocked(c.ctx, c.id, l)
		}
		if err != nil {
			if c.ctx.Err() == nil {
				log.Printf("wake %s in Space %d: %v", queue[0].role, c.id, err)
				l.held = queue
				if !errors.Is(err, store.ErrConflict) {
					s.pauseLocked(c.ctx, c.id, l, err.Error())
				} else {
					var herr error
					if l.held, herr = s.hold(c.ctx, c.id, queue); herr != nil {
						log.Printf("space %d: %v", c.id, herr)
					}
				}
			}
			finish(nil)
			return
		}
		l.publishLocked("turn", turnState{
			Role: queue[0].role, BotID: queue[0].botID, Status: "running", DirectMessageID: queue[0].direct.IDRef()})
		l.mu.Unlock()
	}
}

// running reports whether a message chain is starting or running.
func (l *live) running() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.messagingLocked()
}

// messagingLocked reports whether a message chain is starting or running.
func (l *live) messagingLocked() bool {
	return (l.state == _starting || l.state == _running) && l.kind == _message
}

// awaitStart returns once the chain that is starting is running or has ended; at once when no
// message chain is starting.
func (l *live) awaitStart(ctx context.Context) error {
	l.mu.Lock()
	ready, starting := l.ready, l.state == _starting && l.kind == _message
	l.mu.Unlock()
	if !starting {
		return nil
	}
	select {
	case <-ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// countHop counts w as a hop when a message from an agent woke it, and reports whether w
// may run: a wake over the cap may not. A message from the user resets the count, and so does
// a turn that calls a tool other than Reef's own.
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
	if event != "agent" {
		l.changed()
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

// recordFailedDirect records that the wake of direct message directID will not run. A request
// gets a failed response with body. A response has no response of its own, so its request
// takes the failed state.
func recordFailedDirect(
	ctx context.Context, st *store.Store, directID int64, role, body string,
) error {
	dm, err := st.DirectMessage(ctx, directID)
	if err != nil {
		return err
	}
	if dm.ReplyToID != nil {
		return st.SetDirectRequestState(ctx, *dm.ReplyToID, store.FailedDirectState(body))
	}
	_, err = st.CreateFailedDirectResponse(ctx, directID, dm.ToBotID, role, body)
	return err
}
