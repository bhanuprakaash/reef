package store

import (
	"context"
	"errors"
	"testing"
)

func TestConsultationGuidanceIsUnseenByTheAskerOnce(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.CreateProject(ctx, Project{Name: "reef", Path: "/tmp/reef"}); err != nil {
		t.Fatal(err)
	}
	origin, err := s.CreateSpace(ctx, Space{ProjectID: 1, Name: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	dest, err := s.CreateSpace(ctx, Space{ProjectID: 1, Name: "consultation"})
	if err != nil {
		t.Fatal(err)
	}
	bots := identityTestBots(t, s)
	if _, _, err := s.CreateExchange(ctx, DirectMessage{
		SpaceID: dest.ID, OriginSpaceID: &origin.ID, From: "lead", To: "architect",
		FromBotID: bots["lead"], ToBotID: bots["architect"], Body: "plan?",
	}, "Asked architect"); err != nil {
		t.Fatal(err)
	}
	g, _, err := s.CreateExchange(ctx, DirectMessage{
		SpaceID: dest.ID, OriginSpaceID: &dest.ID, From: "user", To: "architect",
		ToBotID: bots["architect"], Body: "use sessions", Kind: ExchangeGuidance,
	}, "To architect: use sessions")
	if err != nil {
		t.Fatal(err)
	}
	unseen := func() []Exchange {
		t.Helper()
		got, err := s.UnseenConsultationGuidance(ctx, origin.ID, *bots["lead"])
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := unseen(); len(got) != 0 {
		t.Fatalf("before the guided turn = %+v, want none", got)
	}

	if _, err := s.CreateTurn(ctx, Turn{
		SpaceID: dest.ID, Role: "architect", BotID: bots["architect"], Backend: "claude", DirectMessageID: &g.ID,
	}); err != nil {
		t.Fatal(err)
	}
	if got := unseen(); len(got) != 0 {
		t.Fatalf("before the reply = %+v, want none", got)
	}
	if _, err := s.CreateDirectResponse(ctx, g.ID, bots["architect"], "architect", "sessions it is"); err != nil {
		t.Fatal(err)
	}
	if got := unseen(); len(got) != 1 || got[0].Request.Body != "use sessions" || got[0].Response.Body != "sessions it is" {
		t.Fatalf("after the reply = %+v, want the guidance with the reply", got)
	}

	if _, err := s.CreateTurn(ctx, Turn{
		SpaceID: origin.ID, Role: "lead", BotID: bots["lead"], Backend: "claude",
	}); err != nil {
		t.Fatal(err)
	}
	if got := unseen(); len(got) != 0 {
		t.Fatalf("after the asker's next turn = %+v, want none", got)
	}
}

func TestExchangeStartsOnceAndCrashAfterReplyKeepsIt(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.CreateProject(ctx, Project{Name: "reef", Path: "/tmp/reef"}); err != nil {
		t.Fatal(err)
	}
	origin, err := s.CreateSpace(ctx, Space{ProjectID: 1, Name: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	dest, err := s.CreateSpace(ctx, Space{ProjectID: 1, Name: "work"})
	if err != nil {
		t.Fatal(err)
	}
	bots := identityTestBots(t, s)
	dm, created, err := s.CreateExchange(ctx, DirectMessage{
		SpaceID: dest.ID, OriginSpaceID: &origin.ID, From: "lead", To: "architect",
		FromBotID: bots["lead"], ToBotID: bots["architect"], Body: "review",
	}, "Asked architect")
	if err != nil || !created || dm.State != DirectWaiting || dm.Kind != ExchangeRequest || dm.OriginMessageID == nil {
		t.Fatalf("exchange = %+v, %v, %v, want waiting request with an origin message", dm, created, err)
	}
	turn := Turn{SpaceID: dest.ID, Role: "architect", BotID: bots["architect"], Backend: "claude", DirectMessageID: &dm.ID}
	if _, err := s.CreateTurn(ctx, turn); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateTurn(ctx, turn); !errors.Is(err, ErrNotWaiting) {
		t.Fatalf("second turn for the exchange = %v, want ErrNotWaiting", err)
	}
	if _, err := s.RetryExchange(ctx, dm.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("retry of a delivered exchange = %v, want conflict", err)
	}

	if _, err := s.CreateDirectResponse(ctx, dm.ID, bots["architect"], "architect", "looks fine"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FailRunningTurns(ctx, nil); err != nil {
		t.Fatal(err)
	}
	ex, err := s.Exchange(ctx, dm.ID)
	if err != nil || ex.Request.State != DirectAnswered || ex.Response.Status != DirectDone || ex.Response.Body != "looks fine" {
		t.Fatalf("exchange after a crash following its reply = %+v, %v, want answered with the reply", ex, err)
	}
	msgs, err := s.Messages(ctx, origin.ID, 0)
	if err != nil || len(msgs) != 2 || msgs[1].Body != "looks fine" || *msgs[1].ExchangeID != dm.ID {
		t.Fatalf("origin messages = %+v, %v, want the note and the reply linked to the exchange", msgs, err)
	}
	if _, err := s.RetryExchange(ctx, dm.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("retry of an answered exchange = %v, want conflict", err)
	}
}
