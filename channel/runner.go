package channel

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/store"
)

const (
	_userAuthor = "user"
	_toolCap    = 25
)

// ErrCapped is returned by Run when a turn exceeds the tool call cap.
var ErrCapped = errors.New("capped")

// Runner records one role's exchanges with a started agent in one channel: the
// user messages, the agent's replies and a turn for each exchange.
type Runner struct {
	// OnPrompt, when set, sees every prompt before it is sent to the agent.
	OnPrompt func(prompt string)

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

// Say stores body as the user message.
func (r *Runner) Say(ctx context.Context, body string) error {
	_, err := r.st.CreateMessage(ctx, store.Message{
		ChannelID: r.channelID, Author: _userAuthor, Body: body,
	})
	return err
}

// Run runs one turn with text. A cancelled ctx wraps ctx.Err(); a turn over the tool
// call cap pauses the channel and wraps ErrCapped.
func (r *Runner) Run(ctx context.Context, text string, onEvent func(agent.Event)) error {
	// The turn is recorded even when ctx is cancelled.
	wctx := context.WithoutCancel(ctx)
	turn, err := r.st.CreateTurn(wctx, store.Turn{
		ChannelID: r.channelID, Role: r.role, Backend: r.backend, SessionID: r.sessionID,
	})
	if err != nil {
		return err
	}

	prompt := text
	if r.preamble != "" {
		prompt = r.preamble + "\n\n" + text
	}
	if r.OnPrompt != nil {
		r.OnPrompt(prompt)
	}
	if err := r.ag.Send(ctx, prompt); err != nil {
		return errors.Join(err, r.fail(wctx, turn, err.Error(), ""))
	}
	r.preamble = ""

	var reply strings.Builder
	calls := 0
	for {
		select {
		case <-ctx.Done():
			return r.abort(wctx, turn, ctx.Err(), "interrupted", reply.String())
		case ev, ok := <-r.ag.Events():
			if !ok {
				const msg = "agent exited"
				return errors.Join(errors.New(msg), r.fail(wctx, turn, msg, reply.String()))
			}
			onEvent(ev)
			switch ev.Kind {
			case agent.KindSession:
				r.sessionID, turn.SessionID = ev.SessionID, ev.SessionID
				if _, err := r.st.SetTurnSession(wctx, turn.ID, ev.SessionID); err != nil {
					return r.abort(wctx, turn, err, err.Error(), reply.String())
				}
			case agent.KindText:
				reply.WriteString(ev.Text)
			case agent.KindToolCall:
				if calls++; calls > r.toolCap {
					capped := fmt.Errorf("%w: more than %d tool calls", ErrCapped, r.toolCap)
					return errors.Join(r.abort(wctx, turn, capped, capped.Error(), reply.String()),
						r.st.PauseChannel(wctx, r.channelID, capped.Error()))
				}
			case agent.KindUsage:
				if ev.Usage != nil {
					turn.Usage.Add(*ev.Usage)
				}
			case agent.KindError:
				return errors.Join(fmt.Errorf("agent: %s", ev.Text),
					r.fail(wctx, turn, ev.Text, reply.String()))
			case agent.KindTurnEnd:
				if err := r.keepReply(wctx, reply.String()); err != nil {
					return r.abort(wctx, turn, err, err.Error(), "")
				}
				turn.Status = "done"
				if _, err := r.st.EndTurn(wctx, turn); err != nil {
					return r.abort(wctx, turn, err, err.Error(), "")
				}
				return nil
			}
		}
	}
}

// abort interrupts the agent and stores turn as failed with reason, keeping the reply
// streamed so far. It returns cause joined with the errors of those steps. Usage the
// agent left in Events while stopping counts to the turn.
func (r *Runner) abort(
	ctx context.Context, turn store.Turn, cause error, reason, reply string,
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
	return errors.Join(cause, err, r.fail(ctx, turn, reason, reply))
}

func (r *Runner) fail(ctx context.Context, turn store.Turn, reason, reply string) error {
	keepErr := r.keepReply(ctx, reply)
	turn.Status, turn.Error = "failed", reason
	_, err := r.st.EndTurn(ctx, turn)
	return errors.Join(keepErr, err)
}

// keepReply stores a non-empty reply as the role's message.
func (r *Runner) keepReply(ctx context.Context, reply string) error {
	if reply == "" {
		return nil
	}
	_, err := r.st.CreateMessage(ctx, store.Message{
		ChannelID: r.channelID, Author: r.role, Body: reply,
	})
	return err
}
