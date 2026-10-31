package store

import (
	"context"
	"testing"
)

func TestUnseenGuidanceSkipsWhatPredatesTheBotsJoin(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.CreateProject(ctx, Project{Name: "reef", Path: "/tmp/reef"}); err != nil {
		t.Fatal(err)
	}
	bots := identityTestBots(t, s)
	lead, dev, tester := *bots["lead"], *bots["developer"], *bots["tester"]
	space, err := s.CreateSpaceOf(ctx, Space{ProjectID: 1, Name: "Login"}, []int64{lead, dev}, lead)
	if err != nil {
		t.Fatal(err)
	}
	key := "g"
	g, _, err := s.CreateExchange(ctx, DirectMessage{
		SpaceID: space.ID, OriginSpaceID: &space.ID, From: "user", To: "developer", ToBotID: &dev,
		Body: "use sessions", Kind: ExchangeGuidance, RequestKey: &key,
	}, "To developer: use sessions")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateTurn(ctx, Turn{
		SpaceID: space.ID, Role: "developer", BotID: &dev, Backend: "claude", DirectMessageID: &g.ID,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateMessage(ctx, Message{SpaceID: space.ID, Author: ReefAuthor, Body: "tester" + JoinedSuffix}); err != nil {
		t.Fatal(err)
	}

	if got, err := s.UnseenGuidance(ctx, space.ID, tester); err != nil || len(got) != 0 {
		t.Errorf("newcomer's unseen guidance = %+v, %v, want none", got, err)
	}
	if got, err := s.UnseenGuidance(ctx, space.ID, lead); err != nil || len(got) != 1 || got[0].ID != g.ID {
		t.Errorf("lead's unseen guidance = %+v, %v, want the guidance", got, err)
	}
	if known, err := s.GuidanceKnown(ctx, space.ID, lead); err != nil || !known[g.ID] {
		t.Errorf("lead knows %v, %v, want the guidance", known, err)
	}
	if known, err := s.GuidanceKnown(ctx, space.ID, tester); err != nil || len(known) != 0 {
		t.Errorf("newcomer knows %v, %v, want none", known, err)
	}
}

func TestGuidanceKnownCoversWaitingGuidanceAndDoneReplies(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.CreateProject(ctx, Project{Name: "reef", Path: "/tmp/reef"}); err != nil {
		t.Fatal(err)
	}
	bots := identityTestBots(t, s)
	lead, dev := *bots["lead"], *bots["developer"]
	space, err := s.CreateSpaceOf(ctx, Space{ProjectID: 1, Name: "Login"}, []int64{lead, dev}, lead)
	if err != nil {
		t.Fatal(err)
	}
	key := "g"
	g, _, err := s.CreateExchange(ctx, DirectMessage{
		SpaceID: space.ID, OriginSpaceID: &space.ID, From: "user", To: "developer", ToBotID: &dev,
		Body: "use sessions", Kind: ExchangeGuidance, RequestKey: &key,
	}, "To developer: use sessions")
	if err != nil {
		t.Fatal(err)
	}
	if known, err := s.GuidanceKnown(ctx, space.ID, dev); err != nil || !known[g.ID] {
		t.Errorf("developer knows %v, %v, want the waiting guidance, which runs as its own wake", known, err)
	}
	if known, err := s.GuidanceKnown(ctx, space.ID, lead); err != nil || len(known) != 0 {
		t.Errorf("lead knows %v, %v, want none", known, err)
	}

	chat, err := s.CreateBotSpace(ctx, Space{ProjectID: 1, Name: "lead chat", Kind: KindConversation}, lead)
	if err != nil {
		t.Fatal(err)
	}
	ckey := "c"
	ex, _, err := s.Consult(ctx, DirectMessage{
		OriginSpaceID: &chat.ID, From: "lead", To: "developer", FromBotID: &lead, ToBotID: &dev,
		Body: "q", RequestKey: &ckey}, "Consultation from lead", "Asked developer: q")
	if err != nil {
		t.Fatal(err)
	}
	if known, err := s.GuidanceKnown(ctx, chat.ID, lead); err != nil || known[ex.ID] {
		t.Errorf("lead knows %v, %v, want the unanswered question not known", known, err)
	}
	if _, err := s.CreateDirectResponse(ctx, ex.ID, &dev, "developer", "use sessions"); err != nil {
		t.Fatal(err)
	}
	if known, err := s.GuidanceKnown(ctx, chat.ID, lead); err != nil || !known[ex.ID] {
		t.Errorf("lead knows %v, %v, want the answered question: its reply came as a wake", known, err)
	}
}
