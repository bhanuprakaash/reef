package server

import (
	"context"
	"slices"
	"sync"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

func TestWakesOfOneTurnRunInStoredOrder(t *testing.T) {
	env := newLiveEnv(t)
	id, post := env.teamSpace(t,
		store.Role{Name: "lead", Lead: true, Backend: "claude"},
		store.Role{Name: "developer", Backend: "claude"},
		store.Role{Name: "tester", Backend: "claude"})
	var mu sync.Mutex
	var order []string
	env.fake.onRun = func(role, text string, _ *int64) {
		mu.Lock()
		order = append(order, role)
		mu.Unlock()
		if role != "lead" || len(order) > 1 {
			return
		}
		if _, err := env.st.CreateDirectMessage(context.Background(), store.DirectMessage{
			SpaceID: int64(id), From: "lead", To: "developer", FromBotID: testServerBot(t, env.st, "lead"), ToBotID: testServerBot(t, env.st, "developer"), Body: "build it",
		}); err != nil {
			t.Error(err)
		}
		post("lead", "@tester check it", nil)
	}

	env.sendTo(t, id, "go")
	env.waitIdle(t, id)

	mu.Lock()
	defer mu.Unlock()
	if want := []string{"lead", "developer", "tester", "lead"}; !slices.Equal(order, want) {
		t.Errorf("turns ran for %v, want %v: the direct request was stored before the hand-off", order, want)
	}
}
