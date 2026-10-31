package server

import (
	"database/sql"
	"testing"
	"time"
)

var _reviewStart = time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)

func (e *liveEnv) setClock(at time.Time) { e.clock.Store(at.Unix()) }

func reviewMarker(t *testing.T, e *liveEnv, id int64) (reviewed int64, started sql.NullInt64) {
	t.Helper()
	err := e.st.DB().QueryRow(
		"SELECT memory_reviewed_at, memory_review_started_at FROM bot WHERE id = ?", id).
		Scan(&reviewed, &started)
	if err != nil {
		t.Fatal(err)
	}
	return reviewed, started
}

func waitReview(t *testing.T, e *liveEnv, id int64) (reviewed int64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		reviewed, started := reviewMarker(t, e, id)
		if !started.Valid {
			return reviewed
		}
		if time.Now().After(deadline) {
			t.Fatal("review did not end")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (e *liveEnv) sendAndWait(t *testing.T, text string) {
	t.Helper()
	if status, body := e.send(t, text); status != 202 {
		t.Fatalf("send = %d %s, want 202", status, body)
	}
	e.waitIdle(t, 1)
}

func TestMemoryReview_StartsOnceAfterSevenDays(t *testing.T) {
	env := newLiveEnv(t)
	botID := *testServerBot(t, env.st, "lead")

	env.setClock(_reviewStart)
	env.sendAndWait(t, "hello")
	if got, _ := reviewMarker(t, env, botID); got != _reviewStart.Unix() {
		t.Fatalf("marker after first activity = %d, want %d", got, _reviewStart.Unix())
	}

	env.setClock(_reviewStart.Add(6 * 24 * time.Hour))
	env.sendAndWait(t, "hello")

	due := _reviewStart.Add(7 * 24 * time.Hour)
	env.setClock(due)
	env.fake.mu.Lock()
	env.fake.reviewHold = make(chan struct{})
	hold := env.fake.reviewHold
	env.fake.mu.Unlock()
	env.sendAndWait(t, "hello")
	if got := <-env.fake.reviewed; got != botID {
		t.Fatalf("review of Bot %d, want %d", got, botID)
	}
	env.sendAndWait(t, "hello again while it runs")
	close(hold)
	if got := waitReview(t, env, botID); got != due.Unix() {
		t.Errorf("marker after review = %d, want %d", got, due.Unix())
	}
	env.fake.mu.Lock()
	reviews := len(env.fake.reviews)
	env.fake.mu.Unlock()
	if reviews != 1 {
		t.Errorf("reviews = %d, want 1", reviews)
	}

	env.restart(t)
	if got, _ := reviewMarker(t, env, botID); got != due.Unix() {
		t.Errorf("marker after restart = %d, want %d", got, due.Unix())
	}
}

func TestMemoryReview_FailureKeepsMarker(t *testing.T) {
	env := newLiveEnv(t)
	botID := *testServerBot(t, env.st, "lead")

	env.setClock(_reviewStart)
	env.sendAndWait(t, "hello")
	due := _reviewStart.Add(8 * 24 * time.Hour)
	env.setClock(due)
	env.fake.mu.Lock()
	env.fake.reviewFail = true
	env.fake.mu.Unlock()
	env.sendAndWait(t, "hello")
	<-env.fake.reviewed
	if got := waitReview(t, env, botID); got != _reviewStart.Unix() {
		t.Fatalf("marker after failed review = %d, want %d", got, _reviewStart.Unix())
	}

	env.fake.mu.Lock()
	env.fake.reviewFail = false
	env.fake.mu.Unlock()
	env.sendAndWait(t, "hello")
	<-env.fake.reviewed
	if got := waitReview(t, env, botID); got != due.Unix() {
		t.Errorf("marker after retry = %d, want %d", got, due.Unix())
	}
}
