package space

import (
	"context"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

func TestWakeAs_GuidanceTheBotRanIsNotListedAgain(t *testing.T) {
	ctx := context.Background()
	e := newWakeEnv(t)
	dev := *testBotID(t, e.st, "developer")
	key := "g"
	g, _, err := e.st.CreateExchange(ctx, store.DirectMessage{
		SpaceID: e.ch.ID, OriginSpaceID: &e.ch.ID, From: "user", To: "developer", ToBotID: &dev,
		Body: "use sessions", Kind: store.ExchangeGuidance, RequestKey: &key,
	}, "To developer: use sessions")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.s.RunDirectBot(ctx, dev, "guidance", g, discard); err != nil {
		t.Fatal(err)
	}
	e.say("user", "next", nil)
	want := "[Space 1, message 2]\nnext"
	if got := e.wake("developer", want, nil, store.Scope{}); got != want {
		t.Errorf("wake =\n%q\nwant\n%q: the guidance was its own wake", got, want)
	}
}
