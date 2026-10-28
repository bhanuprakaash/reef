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
)

// Runner records one role's exchanges with a started agent in one Space: the
// user messages, the agent's replies and a turn for each exchange.
type Runner struct {
	// OnPrompt, when set, sees every prompt before it is sent to the agent.
	OnPrompt func(prompt string)
	// CheckpointDir, when set, is a git checkout snapshotted before every turn. A turn whose
	// snapshot fails is stored failed and never sent to the agent. A checkout with no commit
	// has nothing to snapshot: its turns run with no checkpoint.
	CheckpointDir string
	// DataDir is the data folder of this Reef copy. It is part of every checkpoint ref, so
	// two copies sharing a repo never overwrite each other's.
	DataDir string
	// NoWake, when set, keeps no wake on the turns it stores, so they cannot be retried.
	NoWake bool
	// Scopes, when set, are the scopes of the wake the next turn sends, each with its newest
	// message, and are stored on the turn so a later turn of the session skips them.
	Scopes store.Scopes
	// WakeMessage, when set, is the public message that woke the next turn, stored on it.
	WakeMessage *int64
	// FirstPrompt, when set, builds the first prompt, again after the backend compacts the
	// session. It is built after the turn's start is stamped, so what is written after it is
	// in the prompt or the next delta.
	FirstPrompt func(ctx context.Context) (string, error)
	// Memory is read for the delta of a Space.
	Memory *memory.Store
	// OnNote, when set, is called after the watch posts a message, so a window shows it.
	OnNote func()

	st        *store.Store
	ag        agent.Agent
	spaceID   int64
	role      string
	backend   string
	preamble  string
	sessionID string
	model     string
	direct    *store.DirectMessage

	sessionGone     bool
	firstPromptSent bool

	// watching is closed to stop the watch that Watch started; watched is closed when it has.
	// The rest is what the watch read that the next turn needs: endWatch applies the session,
	// and the next turn counts the usage and reports the error if its prompt cannot be sent.
	watching    chan struct{}
	watched     chan struct{}
	idleUsage   agent.Usage
	idleSession agent.Event
	idleErr     string
}

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
		SpaceID: r.spaceID, Role: r.role, Backend: r.backend, SessionID: r.sessionID,
		WakeText: r.wake(text), WakeParent: parent, DirectMessageID: r.direct.IDRef(),
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

	if r.CheckpointDir != "" {
		ref, err := r.checkpointRef(turn.ID)
		if err == nil {
			err = worktree.Checkpoint(wctx, r.CheckpointDir, ref)
		}
		if err == nil {
			_, err = r.st.SetTurnCheckpoint(wctx, turn.ID, ref)
		}
		if err != nil && !errors.Is(err, worktree.ErrNoCommit) {
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
	r.preamble, r.firstPromptSent = "", true
	if _, err := r.st.SetTurnSent(wctx, turn.ID, prompt); err != nil {
		return r.abort(wctx, turn, err, err.Error(), "", parent)
	}

	var reply strings.Builder
	toolRows := map[string]int64{}
	postCalls := map[string]bool{}
	posted := false
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
			case agent.KindToolResult:
				posted = posted || (postCalls[ev.ToolID] && !ev.IsError)
				if id, ok := toolRows[ev.ToolID]; ok {
					if err := r.st.SetToolCallResult(wctx, id, ev.Output); err != nil {
						return r.abort(wctx, turn, err, err.Error(), reply.String(), parent)
					}
				}
			case agent.KindUsage:
				if ev.Usage != nil {
					turn.Usage.Add(*ev.Usage)
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
				if (posted && r.direct == nil) || (r.direct != nil && r.direct.ReplyToID != nil) {
					turn.Reply = reply.String()
				} else if err := r.keepReply(wctx, reply.String(), parent, _replyDone); err != nil {
					return r.abort(wctx, turn, err, err.Error(), "", parent)
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

// Watch reads the agent's events while no turn runs and posts the answer of each background
// task as the role's message, so one that ends between turns is not held for the next. Run
// ends the watch before it sends, so a turn is the only reader of Events while it runs.
func (r *Runner) Watch() {
	r.endWatch()
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
				case ev.Kind == agent.KindCompacted:
					r.firstPromptSent = false
					err := r.st.SetTurnCompacted(context.Background(), r.spaceID, r.role, r.sessionID)
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

func (r *Runner) keepBackground(ctx context.Context, text string) error {
	_, err := r.st.CreateMessage(ctx, store.Message{SpaceID: r.spaceID, Author: r.role, Body: text})
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
	if ev.CostUSD != nil {
		before, err := r.st.SessionCostBefore(ctx, turn.SessionID, turn.ID)
		if err != nil {
			return err
		}
		own := max(*ev.CostUSD-before, 0)
		turn.CostUSD, turn.SessionCostUSD = &own, ev.CostUSD
	}
	if ev.SessionUsage != nil {
		before, err := r.st.SessionUsageBefore(ctx, turn.SessionID, turn.ID)
		if err != nil {
			return err
		}
		own := *ev.SessionUsage
		own.Sub(before)
		turn.Usage, turn.SessionUsage = own, ev.SessionUsage
	}
	return nil
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
	since, ok, err := r.st.LastSentAt(ctx, ch.ID, r.role, r.sessionID)
	if err != nil || !ok {
		return "", err
	}
	return changedSince(ctx, r.st, r.Memory, ch, r.role, since)
}

// checkpointRef is refs/reef/checkpoints/<12 hex of sha256 of the absolute data dir>/<turn id>.
// Turns made before the data dir was in the name keep their stored ref, which still restores.
func (r *Runner) checkpointRef(turnID int64) (string, error) {
	dir, err := filepath.Abs(r.DataDir)
	if err != nil {
		return "", fmt.Errorf("checkpoint ref: %w", err)
	}
	sum := sha256.Sum256([]byte(dir))
	return fmt.Sprintf("refs/reef/checkpoints/%x/%d", sum[:6], turnID), nil
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
			_, err := r.st.CreateDirectResponse(ctx, r.direct.ID, r.role, reply)
			return err
		}
		_, err := r.st.CreateFailedDirectResponse(ctx, r.direct.ID, r.role, reply)
		return err
	}
	if reply == "" {
		return nil
	}
	_, err := r.st.CreateMessage(ctx, store.Message{
		SpaceID: r.spaceID, ParentID: parent, Author: r.role, Body: reply,
	})
	return err
}
