package store

import (
	"context"
	"reflect"
	"testing"
)

func TestHolds(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.CreateProject(ctx, Project{Name: "shop", Path: "/tmp/shop"}); err != nil {
		t.Fatal(err)
	}
	ch, err := s.CreateSpace(ctx, Space{ProjectID: 1, Name: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	msg, err := s.CreateMessage(ctx, Message{SpaceID: ch.ID, Author: "lead", Body: "@architect plan"})
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range []Hold{
		{Text: "first"},
		{Text: "second"},
	} {
		h.SpaceID, h.From, h.To, h.MessageID = ch.ID, "lead", "architect", msg.ID
		if _, err := s.CreateHold(ctx, h); err != nil {
			t.Fatal(err)
		}
	}

	texts := func(hs []Hold) []string {
		var out []string
		for _, h := range hs {
			out = append(out, h.Text)
		}
		return out
	}
	if hs, err := s.ClaimHolds(ctx, ch.ID); err != nil || !reflect.DeepEqual(texts(hs), []string{"first", "second"}) {
		t.Errorf("ClaimHolds = %+v, %v, want first, second", hs, err)
	}
	if as, err := s.ClaimHolds(ctx, ch.ID); err != nil || len(as) != 0 {
		t.Errorf("second ClaimHolds = %+v, %v, want none", as, err)
	}

	if _, err := s.CreateHold(ctx, Hold{SpaceID: ch.ID, From: "lead", To: "architect",
		MessageID: msg.ID, Text: "third"}); err != nil {
		t.Fatal(err)
	}
	if err := s.PauseSpace(ctx, ch.ID, "capped"); err != nil {
		t.Fatal(err)
	}
	if err := s.ContinueSpace(ctx, ch.ID, 1000000); err != nil {
		t.Fatal(err)
	}
	if as, err := s.ClaimHolds(ctx, ch.ID); err != nil || len(as) != 0 {
		t.Errorf("ClaimHolds after continue = %+v, %v, want none: the pause that held them is gone", as, err)
	}
}
