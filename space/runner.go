package space

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/memory"
	"github.com/bhanuprakaash/reef/store"
	"github.com/bhanuprakaash/reef/worktree"
)

const (
	_userAuthor = "user"
	// _postTool is how both backends name the reef MCP tool that posts to the Space.
	_postTool = "mcp__reef__post_to_space"

	_stoppedReason = "stopped"
)

// ErrNotParked is returned by Answer when no turn waits on the prompt.
var ErrNotParked = errors.New("no turn waits on the prompt")

// Runner records one role's exchanges with a started agent in one Space: the
// user messages, the agent's replies and a turn for each exchange.
type Runner struct {
	// OnPrompt, when set, sees every prompt before it is sent to the agent.
	OnPrompt func(prompt string)
	// Workspaces, when set, lists the workspaces snapshotted before every turn, one checkpoint
	// each. A turn whose snapshot fails is stored failed and never sent to the agent. A
	// checkout with no commit has nothing to snapshot: it gets no checkpoint.
	Workspaces func(ctx context.Context) ([]store.Workspace, error)
	// DataDir is the data folder of this Reef copy. It is part of every checkpoint ref, so
	// two copies sharing a repo never overwrite each other's.
	DataDir string
	// NoWake, when set, keeps no wake on the turns it stores, so they cannot be retried.
	NoWake bool
	// NoReply, when set, stores no message for the agent's reply, which stays on the turn.
	NoReply bool
	// Scopes, when set, are the scopes of the wake the next turn sends, each with its newest
	// message, and are stored on the turn so a later turn of the session skips them.
	Scopes store.Scopes
	// WakeMessage, when set, is the public message that woke the next turn, stored on it.
	WakeMessage *int64
	// FirstPrompt, when set, builds the first prompt, again after the backend compacts the
	// session. It is built after the turn's start is stamped, so what is written after it is
	// in the prompt or the next delta.
	FirstPrompt func(ctx context.Context) (string, error)
	// Handover, when set, builds the summary that follows the first prompt of a new backend
	// context that replaces an earlier one. It is dropped once a prompt is sent.
	Handover func(ctx context.Context) (string, error)
	// Memory is read for the delta of a Space.
	Memory *memory.Store
	// OnNote, when set, is called after the watch posts a message, so a window shows it.
	OnNote func()
	// OnPermission, when set, sees every permission prompt the Runner stores or resolves.
	OnPermission func(store.PermissionPrompt)
	// Connections are the ids of the Bot's connections that sign in, by server name. The
	// backend's report on one of them is stored as its sign-in state.
	Connections map[string]int64

	st        *store.Store
	ag        agent.Agent
	spaceID   int64
	botID     *int64
	botSessID *int64
	role      string
	backend   string
	preamble  string
	sessionID string
	model     string
	direct    *store.DirectMessage
	// forkedFrom is the session this runner's session was copied from, until the copy has a turn.
	forkedFrom string

	sessionGone     bool
	firstPromptSent bool
	// refreshNote is posted to the Space once the backend reports its first session: a
	// replacement that has not started has not been adopted.
	refreshNote string

	// watching is closed to stop the watch that Watch started; watched is closed when it has.
	// The rest is what the watch read that the next turn needs: endWatch applies the session,
	// and the next turn counts the usage and reports the error if its prompt cannot be sent.
	watching    chan struct{}
	watched     chan struct{}
	idleUsage   agent.Usage
	idleSession agent.Event
	idleErr     string

	// loop is the turn parked on a permission prompt, until Answer or stopParked.
	loop *turnLoop
}

// turnLoop is the state of one turn's event loop, kept while the turn waits for a prompt.
type turnLoop struct {
	turn        store.Turn
	parent      *int64
	direct      *store.DirectMessage
	reply       strings.Builder
	toolRows    map[string]int64
	postCalls   map[string]bool
	posted      bool
	postedAt    int
	workedAfter bool
	// prompt is the waiting prompt; denyRest, when set, denies every later prompt of the turn.
	prompt   *store.PermissionPrompt
	denyRest string
}

// ErrWaiting is returned by Run and Answer when the turn waits for an answer to a permission
// prompt: it is still running, and the agent waits for Answer.
var ErrWaiting = errors.New("turn is waiting for a permission answer")

type replyState uint8

const (
	_replyDone replyState = iota + 1
	_replyFailed
)

// NewRunner returns a Runner for ag, which must already be started or resumed.
// A non-empty preamble is put before the first message sent; FirstPrompt replaces it.
func NewRunner(
	st *store.Store, ag agent.Agent, spaceID int64, role, backend, preamble string,
) *Runner {
	return &Runner{
		st: st, ag: ag, spaceID: spaceID, role: role, backend: backend, preamble: preamble,
	}
}

// Run runs one turn with text. The reply is a reply in the thread of parent when it is
// set. A cancelled ctx wraps ctx.Err(), and a ctx cancelled before the prompt is sent
// stores the turn failed without sending it.
func (r *Runner) Run(
	ctx context.Context, text string, parent *int64, onEvent func(agent.Event),
) error {
	r.endWatch()
	// The turn is recorded even when ctx is cancelled.
	wctx := context.WithoutCancel(ctx)
	turn, err := r.st.CreateTurn(wctx, store.Turn{
		SpaceID: r.spaceID, BotID: r.botID, BotSessionID: r.botSessID, Role: r.role, Backend: r.backend,
		SessionID: r.sessionID, WakeText: r.wake(text), WakeParent: parent, DirectMessageID: r.direct.IDRef(),
		WakeMessageID: r.WakeMessage, Scopes: r.Scopes,
	})
	if err != nil {
		return err
	}
	turn.Model = r.model
	turn.Usage.Add(r.idleUsage)
	r.idleUsage = agent.Usage{}
	idleErr := r.idleErr
	r.idleErr = ""

	if err := ctx.Err(); err != nil {
		return errors.Join(err, r.fail(wctx, turn, "interrupted", "", parent))
	}

	if r.Workspaces != nil {
		if err := r.checkpoint(wctx, turn.ID); err != nil {
			return errors.Join(err, r.fail(wctx, turn, err.Error(), "", parent))
		}
	}

	prompt, err := r.withNotes(wctx, turn, text)
	if err != nil {
		return errors.Join(err, r.fail(wctx, turn, err.Error(), "", parent))
	}
	preamble, delta := r.preamble, ""
	if r.FirstPrompt != nil && !r.firstPromptSent {
		preamble, err = r.FirstPrompt(wctx)
		if err == nil && r.Handover != nil {
			var hand string
			if hand, err = r.Handover(wctx); err == nil {
				preamble += "\n\n" + hand
			}
		}
	} else {
		delta, err = r.delta(wctx)
	}
	if err != nil {
		return errors.Join(err, r.fail(wctx, turn, err.Error(), "", parent))
	}
	if delta != "" {
		prompt = afterHeader(prompt, delta+"\n")
	}
	if preamble != "" {
		prompt = preamble + "\n\n" + prompt
	}
	turn.Prompt = prompt
	if r.OnPrompt != nil {
		r.OnPrompt(prompt)
	}
	if err := r.ag.Send(ctx, prompt); err != nil {
		if idleErr != "" {
			err = fmt.Errorf("%w: %s", err, idleErr)
		}
		r.sessionGone = sessionMissing(err.Error())
		return errors.Join(err, r.fail(wctx, turn, err.Error(), "", parent))
	}
	r.preamble, r.firstPromptSent, r.Handover = "", true, nil
	if _, err := r.st.SetTurnSent(wctx, turn.ID, prompt); err != nil {
		return r.abort(wctx, turn, err, err.Error(), "", parent)
	}

	l := &turnLoop{
		turn: turn, parent: parent, direct: r.direct,
		toolRows: map[string]int64{}, postCalls: map[string]bool{},
	}
	return r.read(ctx, wctx, l, onEvent)
}

// read reads the events of l's turn until it ends, or until a tool call waits for permission.
func (r *Runner) read(
	ctx, wctx context.Context, l *turnLoop, onEvent func(agent.Event),
) error {
	turn, parent, reply := l.turn, l.parent, &l.reply
	toolRows, postCalls := l.toolRows, l.postCalls
	posted, postedAt, workedAfter := l.posted, l.postedAt, l.workedAfter
	for {
		select {
		case <-ctx.Done():
			return r.abort(wctx, turn, ctx.Err(), "interrupted", reply.String(), parent)
		case ev, ok := <-r.ag.Events():
			if !ok {
				const msg = "agent exited"
				return errors.Join(errors.New(msg), r.fail(wctx, turn, msg, reply.String(), parent))
			}
			onEvent(ev)
			switch ev.Kind {
			case agent.KindSession:
				r.keepSession(ev)
				turn.SessionID, turn.Model = r.sessionID, r.model
				if _, err := r.st.SetTurnSession(wctx, turn.ID, ev.SessionID); err != nil {
					return r.abort(wctx, turn, err, err.Error(), reply.String(), parent)
				}
				if r.refreshNote != "" {
					note := store.Message{SpaceID: r.spaceID, Author: store.ReefAuthor, Body: r.refreshNote}
					if _, err := r.st.CreateMessage(wctx, note); err != nil {
						return r.abort(wctx, turn, err, err.Error(), reply.String(), parent)
					}
					r.refreshNote = ""
				}
			case agent.KindMCP:
				r.keepMCP(wctx, ev)
			case agent.KindCompacted:
				turn.Compacted, r.firstPromptSent = true, false
			case agent.KindText:
				reply.WriteString(ev.Text)
			case agent.KindBackground:
				if err := r.keepBackground(wctx, ev.Text); err != nil {
					return r.abort(wctx, turn, err, err.Error(), reply.String(), parent)
				}
			case agent.KindToolCall:
				var parentRow int64
				if ev.ParentToolID != "" {
					parentRow = toolRows[ev.ParentToolID]
				}
				tc, err := r.st.CreateToolCall(
					wctx, turn.ID, parentRow, ev.ToolName, string(ev.Input))
				if err != nil {
					return r.abort(wctx, turn, err, err.Error(), reply.String(), parent)
				}
				toolRows[ev.ToolID] = tc.ID
				postCalls[ev.ToolID] = ev.ToolName == _postTool
				workedAfter = workedAfter || (posted && ev.ToolName != _postTool)
			case agent.KindToolResult:
				if postCalls[ev.ToolID] && !ev.IsError {
					posted, postedAt, workedAfter = true, reply.Len(), false
				}
				if id, ok := toolRows[ev.ToolID]; ok {
					if err := r.st.SetToolCallResult(wctx, id, ev.Output); err != nil {
						return r.abort(wctx, turn, err, err.Error(), reply.String(), parent)
					}
				}
			case agent.KindPermission:
				waiting, err := r.permission(wctx, l, turn.ID, ev)
				if err != nil {
					return r.abort(wctx, turn, err, err.Error(), reply.String(), parent)
				}
				if waiting {
					l.turn, l.posted, l.postedAt, l.workedAfter = turn, posted, postedAt, workedAfter
					r.loop = l
					return ErrWaiting
				}
			case agent.KindUsage:
				if ev.Usage != nil {
					turn.Usage.Add(*ev.Usage)
					turn.ContextTokens = ev.Usage.Context()
					if err := r.st.SetTurnUsage(wctx, turn.ID, turn.Usage); err != nil {
						return r.abort(wctx, turn, err, err.Error(), reply.String(), parent)
					}
				}
			case agent.KindError:
				r.sessionGone = sessionMissing(ev.Text)
				if err := r.ownTotals(wctx, &turn, ev); err != nil {
					return r.abort(wctx, turn, err, err.Error(), reply.String(), parent)
				}
				return errors.Join(fmt.Errorf("agent: %s", ev.Text),
					r.fail(wctx, turn, ev.Text, reply.String(), parent))
			case agent.KindTurnEnd:
				if err := r.ownTotals(wctx, &turn, ev); err != nil {
					return r.abort(wctx, turn, err, err.Error(), reply.String(), parent)
				}
				text, keep := reply.String(), true
				switch {
				case r.NoReply || (r.direct != nil && r.direct.ReplyToID != nil):
					turn.Reply, keep = text, false
				case posted && r.direct == nil:
					turn.Reply, keep = text, false
					if tail := strings.TrimSpace(text[postedAt:]); workedAfter && tail != "" {
						turn.Reply, text, keep = text[:postedAt], tail, true
					}
				}
				if keep {
					if err := r.keepReply(wctx, text, parent, _replyDone); err != nil {
						return r.abort(wctx, turn, err, err.Error(), "", parent)
					}
				}
				turn.Status = "done"
				if _, err := r.st.EndTurn(wctx, turn); err != nil {
					return r.abort(wctx, turn, err, err.Error(), "", parent)
				}
				return nil
			}
		}
	}
}

// permission handles a tool call that waits for permission. It reports true when the turn must
// wait for the user: the prompt is stored waiting and OnPermission has seen it. A turn that had
// a prompt expire denies the rest at once, and a Bot the user allowed after one expired gets that
// allow once.
func (r *Runner) permission(
	ctx context.Context, l *turnLoop, turnID int64, ev agent.Event,
) (bool, error) {
	asker, ok := r.ag.(agent.Asker)
	if !ok {
		return false, errors.New("agent cannot answer permission prompts")
	}
	p := store.PermissionPrompt{
		SpaceID: r.spaceID, BotID: *r.botID, TurnID: turnID, RequestID: ev.RequestID,
		Tool: ev.ToolName, Input: ev.Input, Rule: ev.Rule, Path: ev.Text,
	}
	if l.denyRest != "" {
		p.Status, p.Answer, p.Reason = store.PromptExpired, store.AnswerDeny, l.denyRest
		if _, err := r.keepPrompt(ctx, p); err != nil {
			return false, err
		}
		return false, asker.Answer(ev.RequestID, agent.Answer{Reason: l.denyRest})
	}
	if ev.Rule != "" {
		granted, err := r.st.TakeGrantedPrompt(ctx, r.spaceID, *r.botID, ev.Rule)
		if err != nil {
			return false, err
		}
		if granted {
			return false, asker.Answer(ev.RequestID, agent.Answer{Allow: true})
		}
	}
	p, err := r.keepPrompt(ctx, p)
	l.prompt = &p
	return true, err
}

func (r *Runner) keepPrompt(
	ctx context.Context, p store.PermissionPrompt,
) (store.PermissionPrompt, error) {
	p, err := r.st.CreatePermissionPrompt(ctx, p)
	if err == nil && r.OnPermission != nil {
		r.OnPermission(p)
	}
	return p, err
}

// Waiting returns the prompt the turn waits on, if a turn does.
func (r *Runner) Waiting() (store.PermissionPrompt, bool) {
	if r.loop == nil || r.loop.prompt == nil {
		return store.PermissionPrompt{}, false
	}
	return *r.loop.prompt, true
}

// Answer answers prompt promptID, which the parked turn waits on, with a, and reads the turn on
// to its end or its next prompt, which returns ErrWaiting again. With denyRest the turn denies
// its later prompts at once, with the reason of a.
func (r *Runner) Answer(
	ctx context.Context, promptID int64, a agent.Answer, denyRest bool, onEvent func(agent.Event),
) error {
	l := r.loop
	if l == nil || l.prompt == nil || l.prompt.ID != promptID {
		return fmt.Errorf("prompt %d: %w", promptID, ErrNotParked)
	}
	r.loop, r.direct = nil, l.direct
	defer func() { r.direct = nil }()
	r.endWatch()
	wctx := context.WithoutCancel(ctx)
	requestID := l.prompt.RequestID
	l.prompt = nil
	if denyRest {
		l.denyRest = a.Reason
	}
	if err := r.ag.(agent.Asker).Answer(requestID, a); err != nil {
		return r.abort(wctx, l.turn, err, err.Error(), l.reply.String(), l.parent)
	}
	return r.read(ctx, wctx, l, onEvent)
}

// stopParked ends the turn parked on a prompt: the prompt is denied with reason "stopped" and the
// turn is stored interrupted.
func (r *Runner) stopParked() error {
	l := r.loop
	if l == nil {
		return nil
	}
	r.loop, r.direct = nil, l.direct
	defer func() { r.direct = nil }()
	ctx := context.Background()
	if l.prompt != nil {
		p, err := r.st.ResolvePermissionPrompt(ctx, l.prompt.ID,
			store.PromptWaiting, store.PromptDenied, store.AnswerDeny, _stoppedReason, false)
		if err == nil && r.OnPermission != nil {
			r.OnPermission(p)
		}
		if err != nil && !errors.Is(err, store.ErrConflict) {
			log.Printf("deny prompt %d of %s in Space %d: %v", l.prompt.ID, r.role, r.spaceID, err)
		}
	}
	return r.abort(ctx, l.turn, nil, "interrupted", l.reply.String(), l.parent)
}

// Watch reads the agent's events while no turn runs and posts the answer of each background
// task as the role's message, so one that ends between turns is not held for the next. Run
// ends the watch before it sends, so a turn is the only reader of Events while it runs.
func (r *Runner) Watch() {
	r.endWatch()
	if r.loop != nil {
		return
	}
	stop, done := make(chan struct{}), make(chan struct{})
	r.watching, r.watched = stop, done
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			case ev, ok := <-r.ag.Events():
				if !ok {
					return
				}
				switch {
				case ev.Kind == agent.KindSession:
					r.idleSession = ev
				case ev.Kind == agent.KindMCP:
					r.keepMCP(context.Background(), ev)
				case ev.Kind == agent.KindCompacted:
					r.firstPromptSent = false
					err := r.st.SetTurnCompacted(context.Background(), r.spaceID, *r.botID, r.sessionID)
					if err != nil {
						log.Printf("keep compaction of %s in Space %d: %v", r.role, r.spaceID, err)
					}
				case ev.Kind == agent.KindError:
					r.idleErr = ev.Text
				case ev.Kind == agent.KindUsage && ev.Usage != nil:
					r.idleUsage.Add(*ev.Usage)
				case ev.Kind == agent.KindBackground:
					if err := r.keepBackground(context.Background(), ev.Text); err != nil {
						log.Printf("post background answer of %s in Space %d: %v",
							r.role, r.spaceID, err)
					} else if r.OnNote != nil {
						r.OnNote()
					}
				}
			}
		}
	}()
}

// endWatch stops the watch, if any, and waits until it has.
func (r *Runner) endWatch() {
	if r.watching == nil {
		return
	}
	close(r.watching)
	<-r.watched
	r.watching, r.watched = nil, nil
	if r.idleSession.SessionID != "" {
		r.keepSession(r.idleSession)
		r.idleSession = agent.Event{}
	}
	if r.idleErr != "" {
		r.sessionGone = sessionMissing(r.idleErr)
	}
}

func (r *Runner) keepSession(ev agent.Event) {
	r.sessionID = ev.SessionID
	if ev.Model != "" {
		r.model = ev.Model
	}
}

func (r *Runner) keepMCP(ctx context.Context, ev agent.Event) {
	id, ok := r.Connections[ev.MCPServer]
	if !ok {
		return
	}
	state := store.SignedIn
	if ev.Text == agent.MCPNeedsAuth {
		state = store.NeedsSignIn
	}
	if err := r.st.SetConnectionSignIn(ctx, id, r.backend, state, ""); err != nil {
		log.Printf("keep sign-in of %s for %s in Space %d: %v", ev.MCPServer, r.role, r.spaceID, err)
	}
}

func (r *Runner) keepBackground(ctx context.Context, text string) error {
	_, err := r.st.CreateMessage(ctx, store.Message{SpaceID: r.spaceID, BotID: r.botID, Author: r.role, Body: text})
	return err
}

// RunDirect runs a turn woken by dm. A request keeps the ordinary reply as its one direct
// response. A response keeps its ordinary reply on the turn only; continuing requires send_to_role.
func (r *Runner) RunDirect(
	ctx context.Context, text string, dm store.DirectMessage, onEvent func(agent.Event),
) error {
	r.direct = &dm
	defer func() { r.direct = nil }()
	return r.Run(ctx, text, nil, onEvent)
}

// ownTotals sets the cost and usage of turn from the totals ev reports. They are the running
// totals of the turn's whole session, so the turn's own cost and usage are each total minus the
// one the session's earlier turns had. The totals are stored with the turn for the next one.
func (r *Runner) ownTotals(ctx context.Context, turn *store.Turn, ev agent.Event) error {
	// A forked session's totals may or may not start at its parent's: they do when they still
	// cover the turn's own per-message usage after the parent's is taken off.
	sessionID := turn.SessionID
	if parent := r.forkedFrom; parent != "" {
		r.forkedFrom = ""
		inherits, err := r.inheritsParent(ctx, parent, *turn, ev)
		if err != nil {
			return err
		}
		if inherits {
			sessionID = parent
		}
	}
	if ev.CostUSD != nil {
		before, err := r.st.SessionCostBefore(ctx, sessionID, turn.ID)
		if err != nil {
			return err
		}
		own := max(*ev.CostUSD-before, 0)
		turn.CostUSD, turn.SessionCostUSD = &own, ev.CostUSD
	}
	if ev.SessionUsage != nil {
		before, err := r.st.SessionUsageBefore(ctx, sessionID, turn.ID)
		if err != nil {
			return err
		}
		own := *ev.SessionUsage
		own.Sub(before)
		turn.Usage, turn.SessionUsage = own, ev.SessionUsage
	}
	return nil
}

// inheritsParent reports whether the totals ev reports for the first turn of a forked session
// include those of the session parent it was forked from.
func (r *Runner) inheritsParent(
	ctx context.Context, parent string, turn store.Turn, ev agent.Event,
) (bool, error) {
	if ev.SessionUsage != nil {
		before, err := r.st.SessionUsageBefore(ctx, parent, turn.ID)
		if err != nil {
			return false, err
		}
		return tokens(*ev.SessionUsage)-tokens(before) >= tokens(turn.Usage), nil
	}
	if ev.CostUSD != nil {
		before, err := r.st.SessionCostBefore(ctx, parent, turn.ID)
		if err != nil {
			return false, err
		}
		return *ev.CostUSD >= before, nil
	}
	return false, nil
}

func tokens(u agent.Usage) int64 {
	return u.InputTokens + u.OutputTokens + u.CacheReadTokens + u.CacheWriteTokens
}

func (r *Runner) wake(text string) string {
	if r.NoWake {
		return ""
	}
	return text
}

// delta is what changed in the Space's memory scopes since the session was last sent a prompt.
// It is empty for a session that has been sent none. A turn
// that failed before it sent told the agent nothing, so it does not move the point.
func (r *Runner) delta(ctx context.Context) (string, error) {
	ch, err := r.st.Space(ctx, r.spaceID)
	if err != nil || r.sessionID == "" {
		return "", err
	}
	since, ok, err := r.st.LastSentAt(ctx, ch.ID, *r.botID, r.sessionID)
	if err != nil || !ok {
		return "", err
	}
	return changedSince(ctx, r.st, r.Memory, ch, *r.botID, since)
}

// checkpoint snapshots every workspace Workspaces lists and records a checkpoint of turn id for
// each.
func (r *Runner) checkpoint(ctx context.Context, id int64) error {
	ws, err := r.Workspaces(ctx)
	if err != nil {
		return err
	}
	for _, w := range ws {
		if err := CheckpointWorkspace(ctx, r.st, r.DataDir, id, w); err != nil {
			return err
		}
	}
	return nil
}

func CheckpointWorkspace(
	ctx context.Context, st *store.Store, dataDir string, turnID int64, w store.Workspace,
) error {
	dir, err := filepath.Abs(dataDir)
	if err != nil {
		return fmt.Errorf("checkpoint ref: %w", err)
	}
	sum := sha256.Sum256([]byte(dir))
	ref := fmt.Sprintf("refs/reef/checkpoints/%x/%d/%d", sum[:6], turnID, w.ID)
	if err := worktree.Checkpoint(ctx, w.Dir, ref); err != nil {
		if errors.Is(err, worktree.ErrNoCommit) {
			return nil
		}
		return err
	}
	return st.AddTurnCheckpoint(ctx, turnID, w.ID, ref)
}

// abort interrupts the agent and stores turn as failed with reason, keeping the reply
// streamed so far in the thread of parent. It returns cause joined with the errors of those
// steps. Usage the agent left in Events while stopping counts to the turn, and a compaction
// there is kept like one during the turn.
func (r *Runner) abort(
	ctx context.Context, turn store.Turn, cause error, reason, reply string, parent *int64,
) error {
	err := r.ag.Interrupt()
drain:
	for {
		select {
		case ev, ok := <-r.ag.Events():
			if !ok {
				break drain
			}
			switch {
			case ev.Kind == agent.KindUsage && ev.Usage != nil:
				turn.Usage.Add(*ev.Usage)
				turn.ContextTokens = ev.Usage.Context()
			case ev.Kind == agent.KindCompacted:
				turn.Compacted, r.firstPromptSent = true, false
			}
		default:
			break drain
		}
	}
	return errors.Join(cause, err, r.fail(ctx, turn, reason, reply, parent))
}

func (r *Runner) fail(
	ctx context.Context, turn store.Turn, reason, reply string, parent *int64,
) error {
	keepErr := r.keepReply(ctx, reply, parent, _replyFailed)
	turn.Status, turn.Error = "failed", reason
	_, err := r.st.EndTurn(ctx, turn)
	return errors.Join(keepErr, err)
}

// keepReply stores a non-empty reply as the role's message, in the thread of parent.
func (r *Runner) keepReply(
	ctx context.Context, reply string, parent *int64, state replyState,
) error {
	if r.direct != nil {
		if r.direct.ReplyToID != nil {
			// A response to a direct request never produces another direct reply.
			return nil
		}
		// A successful direct turn with no reply stores no response, and the request
		// has nothing more to wait for. A failed turn, even one with no partial output,
		// leaves a failed response so the sender can see that delivery did not complete.
		if reply == "" && state == _replyDone {
			return r.st.SetDirectRequestState(ctx, r.direct.ID, store.DirectAnswered)
		}
		if state == _replyDone {
			_, err := r.st.CreateDirectResponse(ctx, r.direct.ID, r.botID, r.role, reply)
			return err
		}
		_, err := r.st.CreateFailedDirectResponse(ctx, r.direct.ID, r.botID, r.role, reply)
		return err
	}
	if reply == "" || r.NoReply {
		return nil
	}
	_, err := r.st.CreateMessage(ctx, store.Message{
		SpaceID: r.spaceID, BotID: r.botID, ParentID: parent, Author: r.role, Body: reply,
	})
	return err
}
