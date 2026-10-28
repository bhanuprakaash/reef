package space

import (
	"context"
	"fmt"

	"github.com/bhanuprakaash/reef/store"
)

// CheckCaps refuses to run a turn on a paused Space, with an error that wraps
// store.ErrConflict and holds the reason. A Space with no pause that has used its token
// budget is paused first: CheckCaps sets ch.Paused and pausedNow is true.
func CheckCaps(
	ctx context.Context, st *store.Store, ch *store.Space,
) (pausedNow bool, err error) {
	if ch.Paused == "" {
		used, err := st.SpaceTokens(ctx, ch.ID)
		if err != nil {
			return false, err
		}
		if used < ch.TokenBudget {
			return false, nil
		}
		reason := fmt.Sprintf("token budget of %d used: %d tokens", ch.TokenBudget, used)
		if err := st.PauseSpace(ctx, ch.ID, reason); err != nil {
			return false, err
		}
		ch.Paused, pausedNow = reason, true
	}
	return pausedNow, fmt.Errorf("space %d is paused: %s: %w", ch.ID, ch.Paused, store.ErrConflict)
}

// Continue clears the pause of a paused Space. A Space at or over its token budget
// gets a budget of the tokens it used plus store.TokenBudget. A Space that is not paused is an error wrapping
// store.ErrConflict.
func Continue(ctx context.Context, st *store.Store, id int64) error {
	ch, err := st.Space(ctx, id)
	if err != nil {
		return err
	}
	if ch.Paused == "" {
		return fmt.Errorf("space %d is not paused: %w", id, store.ErrConflict)
	}
	budget, err := ContinueBudget(ctx, st, ch)
	if err != nil {
		return err
	}
	return st.ContinueSpace(ctx, id, budget)
}

// ContinueBudget is the token budget ch has once it continues: its own, or the tokens it
// used plus store.TokenBudget when it has used its own.
func ContinueBudget(ctx context.Context, st *store.Store, ch store.Space) (int64, error) {
	used, err := st.SpaceTokens(ctx, ch.ID)
	if err != nil {
		return 0, err
	}
	if used >= ch.TokenBudget {
		return used + store.TokenBudget, nil
	}
	return ch.TokenBudget, nil
}
