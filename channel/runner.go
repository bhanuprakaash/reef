package channel

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/store"
	"github.com/bhanuprakaash/reef/worktree"
)

const (
	_userAuthor = "user"
	_reefAuthor = "reef"
	_toolCap    = 25
)

// ErrCapped is returned by Run when a turn exceeds the tool call cap.
var ErrCapped = errors.New("capped")

// Runner records one role's exchanges with a started agent in one channel: the
// user messages, the agent's replies and a turn for each exchange.
type Runner struct {
	// OnPrompt, when set, sees every prompt before it is sent to the agent.
	OnPrompt func(prompt string)
	// CheckpointDir, when set, is a git checkout snapshotted before every turn. A turn whose
	// snapshot fails is stored failed and never sent to the agent. A checkout with no commit
	// has nothing to snapshot: its turns run with no checkpoint.
	CheckpointDir string

	st        *store.Store
	ag        agent.Agent
	channelID int64
	role      string
	backend   string
	preamble  string
	sessionID string
	toolCap   int
}

// NewRunner returns a Runner for ag, which must already be started or resumed.
// A non-empty preamble is put before the first message sent.
func NewRunner(
	st *store.Store, ag agent.Agent, channelID int64, role, backend, preamble string,
) *Runner {
	return &Runner{
		st: st, ag: ag, channelID: channelID, role: role, backend: backend, preamble: preamble,
		toolCap: _toolCap,
	}
}

// Run runs one turn with text. The reply is a reply in the thread of parent when it is
// set. A cancelled ctx wraps ctx.Err(), and a ctx cancelled before the prompt is sent
// stores the turn failed without sending it; a turn over the tool call cap pauses the
// channel and wraps ErrCapped.
func (r *Runner) Run(
	ctx context.Context, text string, parent *int64, onEvent func(agent.Event),
) error {
	// The turn is recorded even when ctx is cancelled.
	wctx := context.WithoutCancel(ctx)
	turn, err := r.st.CreateTurn(wctx, store.Turn{
		ChannelID: r.channelID, Role: r.role, Backend: r.backend, SessionID: r.sessionID,
		WakeText: text, WakeParent: parent,
	})
	if err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		return errors.Join(err, r.fail(wctx, turn, "interrupted", "", parent))
	}

	if r.CheckpointDir != "" {
		ref := fmt.Sprintf("refs/reef/checkpoints/%d", turn.ID)
		err := worktree.Checkpoint(wctx, r.CheckpointDir, ref)
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
		prompt = r.preamble + "\n\n" + text
	}
	if r.OnPrompt != nil {
		r.OnPrompt(prompt)
	}
	if err := r.ag.Send(ctx, prompt); err != nil {
		return errors.Join(err, r.fail(wctx, turn, err.Error(), "", parent))
	}
	r.preamble = ""

	var reply strings.Builder
	calls := 0
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
				if _, err := r.st.SetTurnSession(wctx, turn.ID, ev.SessionID); err != nil {
					return r.abort(wctx, turn, err, err.Error(), reply.String(), parent)
				}
			case agent.KindText:
				reply.WriteString(ev.Text)
			case agent.KindToolCall:
				if calls++; calls > r.toolCap {
					capped := fmt.Errorf("%w: more than %d tool calls", ErrCapped, r.toolCap)
					return errors.Join(
						r.abort(wctx, turn, capped, capped.Error(), reply.String(), parent),
						r.st.PauseChannel(wctx, r.channelID, capped.Error()))
				}
			case agent.KindUsage:
				if ev.Usage != nil {
					turn.Usage.Add(*ev.Usage)
				}
			case agent.KindError:
				return errors.Join(fmt.Errorf("agent: %s", ev.Text),
					r.fail(wctx, turn, ev.Text, reply.String(), parent))
			case agent.KindTurnEnd:
				if err := r.keepReply(wctx, reply.String(), parent); err != nil {
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
	keepErr := r.keepReply(ctx, reply, parent)
	turn.Status, turn.Error = "failed", reason
	_, err := r.st.EndTurn(ctx, turn)
	return errors.Join(keepErr, err)
}

// keepReply stores a non-empty reply as the role's message, in the thread of parent.
func (r *Runner) keepReply(ctx context.Context, reply string, parent *int64) error {
	if reply == "" {
		return nil
	}
	_, err := r.st.CreateMessage(ctx, store.Message{
		ChannelID: r.channelID, ParentID: parent, Author: r.role, Body: reply,
	})
	return err
}
