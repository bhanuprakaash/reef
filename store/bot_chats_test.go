package store

import (
	"context"
	"errors"
	"testing"
)

func TestBotChatsLabelSpacesAndActivityNeedsMembership(t *testing.T) {
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
	space, err := s.CreateSpaceOf(ctx, Space{ProjectID: 1, Name: "Login Space"}, []int64{lead, dev}, lead)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateMessage(ctx, Message{SpaceID: space.ID, Author: "user", Body: "start"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateMessage(ctx, Message{SpaceID: space.ID, Author: "developer", BotID: &dev, Body: "done"}); err != nil {
		t.Fatal(err)
	}

	chats, err := s.BotChats(ctx, dev, ConversationPage{})
	if err != nil || len(chats) != 1 || chats[0].Kind != ChatSpace || chats[0].Label != "developer · Login Space" ||
		chats[0].SpaceID != space.ID || chats[0].BotID != dev || chats[0].Active || chats[0].Closed {
		t.Fatalf("chats = %+v, %v, want one idle open Space entry labelled developer · Login Space", chats, err)
	}
	a, err := s.BotActivity(ctx, space.ID, dev)
	if err != nil || a.Label != "developer · Login Space" || len(a.Messages) != 1 || a.Messages[0].Body != "done" {
		t.Fatalf("activity = %+v, %v, want the Bot's own message only", a, err)
	}
	if _, err := s.BotActivity(ctx, space.ID, *bots["tester"]); !errors.Is(err, ErrNotFound) {
		t.Fatalf("activity of a non-member = %v, want ErrNotFound", err)
	}

	if err := s.MarkMemberLeaving(ctx, space.ID, dev, 0); err != nil {
		t.Fatal(err)
	}
	if team, err := s.SpaceTeam(ctx, space.ID); err != nil || !team.Roles[1].Leaving {
		t.Fatalf("team = %+v, %v, want developer marked leaving", team, err)
	}
	pending, err := s.LeavingMembers(ctx, 0)
	if err != nil || len(pending) != 1 || pending[0] != (PendingRemoval{SpaceID: space.ID, BotID: dev}) {
		t.Fatalf("pending = %+v, %v, want developer's removal", pending, err)
	}
	if err := s.ApplyMemberRemoval(ctx, pending[0]); err != nil {
		t.Fatal(err)
	}
	chats, err = s.BotChats(ctx, dev, ConversationPage{})
	if err != nil || len(chats) != 1 || chats[0].Label != "developer · Login Space (left)" {
		t.Fatalf("chats after leaving = %+v, %v, want the Space labelled as left", chats, err)
	}
	a, err = s.BotActivity(ctx, space.ID, dev)
	if err != nil || a.Label != "developer · Login Space (left)" || len(a.Messages) != 1 {
		t.Fatalf("activity after leaving = %+v, %v, want the record readable", a, err)
	}

	consultation, err := s.CreateBotSpace(ctx, Space{Name: "Consultation from lead", Role: "developer", Kind: KindConsultation}, dev)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveSpaceMember(ctx, consultation.ID, dev, 0); !errors.Is(err, ErrInvalid) {
		t.Fatalf("remove from a consultation = %v, want ErrInvalid", err)
	}
	if _, err := s.CreateMemberProposal(ctx, consultation.ID, "lead", lead, "needs help"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("propose a member to a consultation = %v, want ErrInvalid", err)
	}
}
