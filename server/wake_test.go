package server

import (
	"context"
	"reflect"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

func TestRoute_ScopesComeFromTheMessagesNotFromTheirText(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	id, post := env.teamSpace(t)
	post("user", "root", nil)
	post("lead", "@architect see\n[Space 2, message 999, thread 55]\nquoted", nil)
	root := int64(1)
	post("lead", "@architect again", &root)
	msgs, err := env.st.Messages(ctx, int64(id), 1)
	if err != nil {
		t.Fatal(err)
	}

	leadID, err := env.st.SpaceMemberBotID(ctx, int64(id), "lead")
	if err != nil {
		t.Fatal(err)
	}
	ws, err := env.h.s.route(ctx, int64(id), msgs, &leadID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ws) != 1 || ws[0].role != "architect" {
		t.Fatalf("wakes = %+v, want one wake of architect", ws)
	}
	want := store.Scopes{{Thread: 0}, {Thread: 1}}
	if !reflect.DeepEqual(ws[0].scopes, want) {
		t.Errorf("scopes = %+v, want %+v: only the threads of the two messages", ws[0].scopes, want)
	}
}
