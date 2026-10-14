package channel

import (
	"context"
	"fmt"

	"github.com/bhanuprakaash/reef/store"
)

// CheckCaps refuses to run a turn on a paused channel, with an error that wraps
// store.ErrConflict and holds the reason. A channel with no pause that has used its token
// budget is paused first: CheckCaps sets ch.Paused and pausedNow is true.
func CheckCaps(
	ctx context.Context, st *store.Store, ch *store.Channel,
) (pausedNow bool, err error) {
	if ch.Paused == "" {
		used, err := st.ChannelTokens(ctx, ch.ID)
		if err != nil {
			return false, err
		}
		if used < ch.TokenBudget {
			return false, nil
		}
		reason := fmt.Sprintf("token budget of %d used: %d tokens", ch.TokenBudget, used)
		if err := st.PauseChannel(ctx, ch.ID, reason); err != nil {
			return false, err
		}
		ch.Paused, pausedNow = reason, true
	}
	return pausedNow, fmt.Errorf("channel %d is paused: %s: %w", ch.ID, ch.Paused, store.ErrConflict)
}

// Continue clears the pause of a paused channel. A channel at or over its token budget
// also gets another budget. A channel that is not paused is an error wrapping
// store.ErrConflict.
func Continue(ctx context.Context, st *store.Store, id int64) error {
	ch, err := st.Channel(ctx, id)
	if err != nil {
		return err
	}
	if ch.Paused == "" {
		return fmt.Errorf("channel %d is not paused: %w", id, store.ErrConflict)
	}
	used, err := st.ChannelTokens(ctx, id)
	if err != nil {
		return err
	}
	var extra int64
	if used >= ch.TokenBudget {
		extra = store.TokenBudget
	}
	return st.ContinueChannel(ctx, id, extra)
}
