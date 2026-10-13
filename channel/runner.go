package channel

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/store"
)

const _userAuthor = "user"

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
}

// NewRunner returns a Runner for ag, which must already be started or resumed.
// A non-empty preamble is put before the first message sent.
func NewRunner(
	st *store.Store, ag agent.Agent, channelID int64, role, backend, preamble string,
) *Runner {
	return &Runner{
		st: st, ag: ag, channelID: channelID, role: role, backend: backend, preamble: preamble,
	}
}

// Send stores text as a user message, runs one turn and calls onEvent for each
// event of the agent. A finished turn is stored done with the agent's reply as
// a message. An agent error ends the turn failed and is returned. When ctx is
// cancelled the agent is interrupted, the turn is stored failed as interrupted
// and the error wraps ctx.Err().
func (r *Runner) Send(ctx context.Context, text string, onEvent func(agent.Event)) error {
	return r.SendAs(ctx, text, text, onEvent)
}

// SendAs is Send with a different text for the agent: the user message stored is
// stored, the agent gets text.
func (r *Runner) SendAs(
	ctx context.Context, stored, text string, onEvent func(agent.Event),
) error {
	if _, err := r.st.CreateMessage(ctx, store.Message{
		ChannelID: r.channelID, Author: _userAuthor, Body: stored,
	}); err != nil {
		return err
	}
	turn, err := r.st.CreateTurn(ctx, store.Turn{
		ChannelID: r.channelID, Role: r.role, Backend: r.backend, SessionID: r.sessionID,
	})
	if err != nil {
		return err
	}
	// From here the turn is recorded even when ctx is cancelled.
	wctx := context.WithoutCancel(ctx)

	prompt := text
	if r.preamble != "" {
		prompt = r.preamble + "\n\n" + text
	}
	if r.OnPrompt != nil {
		r.OnPrompt(prompt)
	}
	if err := r.ag.Send(ctx, prompt); err != nil {
		return errors.Join(err, r.fail(wctx, turn, err.Error()))
	}
	r.preamble = ""

	var reply strings.Builder
	for {
		select {
		case <-ctx.Done():
			return r.abort(wctx, turn, ctx.Err(), "interrupted")
		case ev, ok := <-r.ag.Events():
			if !ok {
				const msg = "agent exited"
				return errors.Join(errors.New(msg), r.fail(wctx, turn, msg))
			}
			onEvent(ev)
			switch ev.Kind {
			case agent.KindSession:
				r.sessionID, turn.SessionID = ev.SessionID, ev.SessionID
				if _, err := r.st.SetTurnSession(wctx, turn.ID, ev.SessionID); err != nil {
					return r.abort(wctx, turn, err, err.Error())
				}
			case agent.KindText:
				reply.WriteString(ev.Text)
			case agent.KindUsage:
				if ev.Usage != nil {
					turn.Usage.Add(*ev.Usage)
				}
			case agent.KindError:
				return errors.Join(fmt.Errorf("agent: %s", ev.Text), r.fail(wctx, turn, ev.Text))
			case agent.KindTurnEnd:
				if reply.Len() > 0 {
					if _, err := r.st.CreateMessage(wctx, store.Message{
						ChannelID: r.channelID, Author: r.role, Body: reply.String(),
					}); err != nil {
						return r.abort(wctx, turn, err, err.Error())
					}
				}
				turn.Status = "done"
				if _, err := r.st.EndTurn(wctx, turn); err != nil {
					return r.abort(wctx, turn, err, err.Error())
				}
				return nil
			}
		}
	}
}

// abort interrupts the agent and stores turn as failed with reason. It returns
// cause joined with the errors of those two steps.
func (r *Runner) abort(ctx context.Context, turn store.Turn, cause error, reason string) error {
	return errors.Join(cause, r.ag.Interrupt(), r.fail(ctx, turn, reason))
}

func (r *Runner) fail(ctx context.Context, turn store.Turn, reason string) error {
	turn.Status, turn.Error = "failed", reason
	_, err := r.st.EndTurn(ctx, turn)
	return err
}
