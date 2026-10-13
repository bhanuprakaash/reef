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
