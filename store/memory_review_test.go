package store

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestClaimMemoryReview(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	bot, err := s.CreateBot(ctx, Bot{Name: "ana", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	const week, hour = 7 * 24 * time.Hour, time.Hour
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	claim := func(at time.Time) bool {
		t.Helper()
		ok, err := s.ClaimMemoryReview(ctx, bot.ID, at, week, hour)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}

	if claim(t0) || claim(t0.Add(week-time.Second)) {
		t.Fatal("claimed before seven days from the first activity")
	}
	var won atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if claim(t0.Add(week)) {
				won.Add(1)
			}
		}()
	}
	wg.Wait()
	if won.Load() != 1 {
		t.Fatalf("claims won = %d, want 1", won.Load())
	}

	if err := s.FinishMemoryReview(ctx, bot.ID, t0.Add(week), false); err != nil {
		t.Fatal(err)
	}
	if !claim(t0.Add(week + time.Minute)) {
		t.Error("claim after a failed review = false, want true")
	}
	if claim(t0.Add(week + 2*time.Minute)) {
		t.Error("claim while another is held = true, want false")
	}
	if !claim(t0.Add(week + 2*hour)) {
		t.Error("claim after the held one went stale = false, want true")
	}
	if err := s.FinishMemoryReview(ctx, bot.ID, t0.Add(week+2*hour), true); err != nil {
		t.Fatal(err)
	}
	if claim(t0.Add(2*week + hour)) {
		t.Error("claim a week less an hour after a good review = true, want false")
	}
	if !claim(t0.Add(2*week + 2*hour)) {
		t.Error("claim a week after a good review = false, want true")
	}
}
