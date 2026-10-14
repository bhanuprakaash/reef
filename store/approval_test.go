package store

import (
	"context"
	"errors"
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
	ch, err := s.CreateChannel(ctx, Channel{ProjectID: 1, Name: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	msg, err := s.CreateMessage(ctx, Message{ChannelID: ch.ID, Author: "lead", Body: "@architect plan"})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range []Approval{
		{Kind: ApprovalHold, Text: "first"},
		{Text: "gate"},
		{Kind: ApprovalHold, Text: "second"},
	} {
		a.ChannelID, a.From, a.To, a.MessageID = ch.ID, "lead", "architect", msg.ID
		if _, err := s.CreateApproval(ctx, a); err != nil {
			t.Fatal(err)
		}
	}

	texts := func(as []Approval) []string {
		var out []string
		for _, a := range as {
			out = append(out, a.Text)
		}
		return out
	}
	if as, err := s.PendingApprovals(ctx, ch.ID); err != nil || !reflect.DeepEqual(texts(as), []string{"gate"}) {
		t.Errorf("PendingApprovals = %+v, %v, want only the gate", as, err)
	}
	if _, err := s.Approval(ctx, ch.ID, 1); !errors.Is(err, ErrNotFound) {
		t.Errorf("Approval of a hold = %v, want ErrNotFound", err)
	}
	if as, err := s.ClaimHolds(ctx, ch.ID); err != nil || !reflect.DeepEqual(texts(as), []string{"first", "second"}) {
		t.Errorf("ClaimHolds = %+v, %v, want first, second", as, err)
	}
	if as, err := s.ClaimHolds(ctx, ch.ID); err != nil || len(as) != 0 {
		t.Errorf("second ClaimHolds = %+v, %v, want none", as, err)
	}

	if _, err := s.CreateApproval(ctx, Approval{ChannelID: ch.ID, From: "lead", To: "architect",
		MessageID: msg.ID, Text: "third", Kind: ApprovalHold}); err != nil {
		t.Fatal(err)
	}
	if err := s.PauseChannel(ctx, ch.ID, "capped"); err != nil {
		t.Fatal(err)
	}
	if err := s.ContinueChannel(ctx, ch.ID, 1000000); err != nil {
		t.Fatal(err)
	}
	if as, err := s.ClaimHolds(ctx, ch.ID); err != nil || len(as) != 0 {
		t.Errorf("ClaimHolds after continue = %+v, %v, want none: the pause that held them is gone", as, err)
	}
}
