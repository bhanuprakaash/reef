package channel

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/store"
	"github.com/bhanuprakaash/reef/worktree"
)

const _userAuthor = "user"

// Runner records one role's exchanges with a started agent in one channel: the
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

	st        *store.Store
	ag        agent.Agent
	channelID int64
	role      string
	backend   string
	preamble  string
	sessionID string
	model     string
	direct    *store.DirectMessage

	// sessionGone is set when the backend reported the resumed session missing.
	sessionGone bool
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

type replyState uint8

const (
	_replyDone replyState = iota + 1
	_replyFailed
)

// NewRunner returns a Runner for ag, which must already be started or resumed.
// A non-empty preamble is put before the first message sent.
func NewRunner(
	st *store.Store, ag agent.Agent, channelID int64, role, backend, preamble string,
) *Runner {
	return &Runner{
		st: st, ag: ag, channelID: channelID, role: role, backend: backend, preamble: preamble,
	}
}

// Run runs one turn with text. The reply is a reply in the thread of parent when it is
// set. A cancelled ctx wraps ctx.Err(), and a ctx cancelled before the prompt is sent
// stores the turn failed without sending it.
func (r *Runner) Run(
	ctx context.Context, text string, parent *int64, onEvent func(agent.Event),
) error {
	// The turn is recorded even when ctx is cancelled.
	wctx := context.WithoutCancel(ctx)
	turn, err := r.st.CreateTurn(wctx, store.Turn{
		ChannelID: r.channelID, Role: r.role, Backend: r.backend, SessionID: r.sessionID,
		WakeText: r.wake(text), WakeParent: parent, DirectMessageID: r.direct.IDRef(),
	})
	if err != nil {
		return err
	}
	turn.Model = r.model

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
	if r.preamble != "" {
		prompt = r.preamble + "\n\n" + prompt
	}
	turn.Prompt = prompt
	if r.OnPrompt != nil {
		r.OnPrompt(prompt)
	}
	if err := r.ag.Send(ctx, prompt); err != nil {
		return errors.Join(err, r.fail(wctx, turn, err.Error(), "", parent))
	}
	r.preamble = ""
	if _, err := r.st.SetTurnSent(wctx, turn.ID, prompt); err != nil {
		return r.abort(wctx, turn, err, err.Error(), "", parent)
	}

	var reply strings.Builder
	toolRows := map[string]int64{}
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
				r.sessionID, turn.SessionID = ev.SessionID, ev.SessionID
				if ev.Model != "" {
					r.model, turn.Model = ev.Model, ev.Model
				}
				if _, err := r.st.SetTurnSession(wctx, turn.ID, ev.SessionID); err != nil {
					return r.abort(wctx, turn, err, err.Error(), reply.String(), parent)
				}
			case agent.KindText:
				reply.WriteString(ev.Text)
			case agent.KindBackground:
				_, err := r.st.CreateMessage(wctx, store.Message{
					ChannelID: r.channelID, Author: r.role, Body: ev.Text,
				})
				if err != nil {
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
			case agent.KindToolResult:
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
				if turn.CostUSD, err = r.ownCost(wctx, turn.SessionID, ev.CostUSD); err != nil {
					return r.abort(wctx, turn, err, err.Error(), reply.String(), parent)
				}
				return errors.Join(fmt.Errorf("agent: %s", ev.Text),
					r.fail(wctx, turn, ev.Text, reply.String(), parent))
			case agent.KindTurnEnd:
				if turn.CostUSD, err = r.ownCost(wctx, turn.SessionID, ev.CostUSD); err != nil {
					return r.abort(wctx, turn, err, err.Error(), reply.String(), parent)
				}
				if err := r.keepReply(wctx, reply.String(), parent, _replyDone); err != nil {
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

// RunDirect runs a turn woken by dm. A request keeps the ordinary reply as its one direct
// response. A response keeps no ordinary reply; continuing requires send_to_role.
func (r *Runner) RunDirect(
	ctx context.Context, text string, dm store.DirectMessage, onEvent func(agent.Event),
) error {
	r.direct = &dm
	defer func() { r.direct = nil }()
	return r.Run(ctx, text, nil, onEvent)
}

// ownCost is the cost of the turn that reported total, the running cost of its whole session:
// total minus what the session's earlier turns stored. Turns stored before this was done hold
// running totals, so the result is never below zero.
func (r *Runner) ownCost(ctx context.Context, sessionID string, total *float64) (*float64, error) {
	if total == nil {
		return nil, nil
	}
	earlier, err := r.st.SessionCost(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	own := max(*total-earlier, 0)
	return &own, nil
}

func (r *Runner) wake(text string) string {
	if r.NoWake {
		return ""
	}
	return text
}

// abort interrupts the agent and stores turn as failed with reason, keeping the reply
// streamed so far in the thread of parent. It returns cause joined with the errors of those
// steps. Usage the agent left in Events while stopping counts to the turn.
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
			if ev.Kind == agent.KindUsage && ev.Usage != nil {
				turn.Usage.Add(*ev.Usage)
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
		// A successful direct turn with no reply stores no response. A failed turn,
		// even one with no partial output, leaves a failed response so the sender can
		// see that delivery did not complete.
		if reply == "" && state == _replyDone {
			return nil
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
		ChannelID: r.channelID, ParentID: parent, Author: r.role, Body: reply,
	})
	return err
}
