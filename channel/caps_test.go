package channel

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/store"
)

func TestContinue(t *testing.T) {
	tests := []struct {
		name       string
		givePause  string
		giveTokens int64
		wantErr    string
		wantBudget int64
	}{
		{name: "tool cap", givePause: "tool cap", giveTokens: 20, wantBudget: 1000000},
		{name: "over budget", givePause: "budget", giveTokens: 1000000, wantBudget: 2000000},
		{name: "far over budget", givePause: "budget", giveTokens: 3000000, wantBudget: 4000000},
		{
			name: "not paused", giveTokens: 20, wantBudget: 1000000,
			wantErr: "channel 1 is not paused: conflict",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			st, f := openStore(t)
			ch := sessionChannel(t, st, f, false)
			if tt.givePause != "" {
				if err := st.PauseChannel(ctx, ch.ID, tt.givePause); err != nil {
					t.Fatal(err)
				}
			}
			turn, err := st.CreateTurn(ctx, store.Turn{
				ChannelID: ch.ID, Role: "architect", Backend: "claude",
			})
			if err != nil {
				t.Fatal(err)
			}
			turn.Status, turn.Usage = "done", agent.Usage{InputTokens: tt.giveTokens}
			if _, err := st.EndTurn(ctx, turn); err != nil {
				t.Fatal(err)
			}

			err = Continue(ctx, st, ch.ID)
			if tt.wantErr == "" && err != nil ||
				tt.wantErr != "" && (err == nil || err.Error() != tt.wantErr || !errors.Is(err, store.ErrConflict)) {
				t.Fatalf("Continue = %v, want %q", err, tt.wantErr)
			}
			if got, err := st.Channel(ctx, ch.ID); err != nil || got.Paused != "" || got.TokenBudget != tt.wantBudget {
				t.Errorf("channel = paused %q budget %d, %v, want not paused, budget %d",
					got.Paused, got.TokenBudget, err, tt.wantBudget)
			}
		})
	}
}

func TestContinue_ConcurrentCallsDoNotGrowBudgetTwice(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionChannel(t, st, f, false)
	if err := st.PauseChannel(ctx, ch.ID, "budget"); err != nil {
		t.Fatal(err)
	}
	const used = 3000000
	turn, err := st.CreateTurn(ctx, store.Turn{
		ChannelID: ch.ID, Role: "architect", Backend: "claude",
	})
	if err != nil {
		t.Fatal(err)
	}
	turn.Status, turn.Usage = "done", agent.Usage{InputTokens: used}
	if _, err := st.EndTurn(ctx, turn); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- Continue(ctx, st, ch.ID)
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
			continue
		}
		if !errors.Is(err, store.ErrConflict) {
			t.Errorf("Continue = %v, want nil or conflict", err)
		}
	}
	if successes != 1 {
		t.Fatalf("successful Continue calls = %d, want 1", successes)
	}
	got, err := st.Channel(ctx, ch.ID)
	if err != nil || got.TokenBudget != used+store.TokenBudget || got.Paused != "" {
		t.Errorf("channel after concurrent continues = paused %q budget %d, %v, want unpaused budget %d",
			got.Paused, got.TokenBudget, err, used+store.TokenBudget)
	}
}

func TestCheckCaps(t *testing.T) {
	tests := []struct {
		name       string
		givePause  string
		giveTokens int64
		wantErr    string
		wantNow    bool
		wantPaused string
	}{
		{name: "under budget", giveTokens: 999999},
		{
			name: "paused", givePause: "tool cap", giveTokens: 20,
			wantErr: "channel 1 is paused: tool cap: conflict", wantPaused: "tool cap",
		},
		{
			name: "over budget", giveTokens: 1000312, wantNow: true,
			wantErr:    "channel 1 is paused: token budget of 1000000 used: 1000312 tokens: conflict",
			wantPaused: "token budget of 1000000 used: 1000312 tokens",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			st, f := openStore(t)
			ch := sessionChannel(t, st, f, false)
			if tt.givePause != "" {
				if err := st.PauseChannel(ctx, ch.ID, tt.givePause); err != nil {
					t.Fatal(err)
				}
			}
			turn, err := st.CreateTurn(ctx, store.Turn{
				ChannelID: ch.ID, Role: "architect", Backend: "claude",
			})
			if err != nil {
				t.Fatal(err)
			}
			turn.Status = "done"
			turn.Usage = agent.Usage{InputTokens: tt.giveTokens - 12, OutputTokens: 12}
			if _, err := st.EndTurn(ctx, turn); err != nil {
				t.Fatal(err)
			}
			if ch, err = st.Channel(ctx, ch.ID); err != nil {
				t.Fatal(err)
			}

			now, err := CheckCaps(ctx, st, &ch)
			if tt.wantErr == "" {
				if err != nil || now {
					t.Fatalf("CheckCaps = %v, %v, want false, nil", now, err)
				}
			} else if err == nil || err.Error() != tt.wantErr ||
				!errors.Is(err, store.ErrConflict) || now != tt.wantNow {
				t.Fatalf("CheckCaps = %v, %v, want %v, %q wrapping ErrConflict",
					now, err, tt.wantNow, tt.wantErr)
			}
			stored, err := st.Channel(ctx, 1)
			if err != nil || stored.Paused != tt.wantPaused || ch.Paused != tt.wantPaused {
				t.Errorf("paused = %q (stored %q), %v, want %q",
					ch.Paused, stored.Paused, err, tt.wantPaused)
			}
		})
	}
}
