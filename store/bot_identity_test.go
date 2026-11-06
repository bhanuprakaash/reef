package store

import (
	"context"
	"slices"
	"testing"
	"time"
)

func TestStoredBotIdentitySurvivesRename(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateProject(ctx, Project{Name: "shop", Path: "/tmp/shop"}); err != nil {
		t.Fatal(err)
	}
	space, err := s.CreateSpace(ctx, Space{ProjectID: 1, Name: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	sam, err := s.CreateBot(ctx, Bot{Name: "sam", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AddSpaceMember(ctx, space.ID, sam.ID); err != nil {
		t.Fatal(err)
	}
	ann, err := s.CreateBot(ctx, Bot{Name: "ann", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AddSpaceMember(ctx, space.ID, ann.ID); err != nil {
		t.Fatal(err)
	}
	msg, err := s.CreateMessage(ctx, Message{SpaceID: space.ID, Author: "sam", BotID: &sam.ID, Body: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	reply, err := s.CreateMessage(ctx, Message{SpaceID: space.ID, Author: "user", ParentID: &msg.ID, Body: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := s.CreateTurn(ctx, Turn{SpaceID: space.ID, BotID: &sam.ID, Role: "sam", Backend: "claude", SessionID: "session"})
	if err != nil {
		t.Fatal(err)
	}
	token, err := s.CreateToken(ctx, space.ID, sam.ID)
	if err != nil {
		t.Fatal(err)
	}
	dm, err := s.CreateDirectMessage(ctx, DirectMessage{SpaceID: space.ID, From: "sam", To: "ann", FromBotID: &sam.ID, ToBotID: &ann.ID, Body: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	sam.Name = "sally"
	if _, err = s.UpdateBot(ctx, sam); err != nil {
		t.Fatal(err)
	}
	other, err := s.CreateBot(ctx, Bot{Name: "sam", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AddSpaceMember(ctx, space.ID, other.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateDirectMessage(ctx, DirectMessage{SpaceID: space.ID, From: "sam", To: "sam", FromBotID: &sam.ID, ToBotID: &other.ID, Body: "same snapshot name"}); err != nil {
		t.Fatalf("distinct Bots with matching snapshots: %v", err)
	}
	roster, err := s.SpaceTeam(ctx, space.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(roster.Roles, func(r Role) bool { return r.Name == "sally" && r.BotID == sam.ID }) {
		t.Fatalf("renamed roster = %+v", roster.Roles)
	}
	id, err := s.SpaceMemberBotID(ctx, space.ID, "sally")
	if err != nil || id != sam.ID {
		t.Fatalf("renamed member = %d, %v", id, err)
	}
	ann.Name = "anna"
	if _, err = s.UpdateBot(ctx, ann); err != nil {
		t.Fatal(err)
	}
	response, err := s.CreateDirectResponse(ctx, dm.ID, &ann.ID, "anna", "hi")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	messages, err := s.Messages(ctx, space.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].Author != "sam" || messages[0].BotID == nil || *messages[0].BotID != sam.ID || messages[1].BotID != nil {
		t.Fatalf("messages = %+v", messages)
	}
	byBot, err := s.MessagesByBot(ctx, space.ID, sam.ID, time.Time{})
	if err != nil || len(byBot) != 1 || byBot[0].ID != msg.ID {
		t.Fatalf("messages by Bot = %+v, %v", byBot, err)
	}
	owner, err := s.ThreadOwnerBotID(ctx, reply.ID)
	if err != nil || owner == nil || *owner != sam.ID {
		t.Fatalf("thread owner = %v, %v", owner, err)
	}
	turns, err := s.Turns(ctx, TurnFilter{SpaceID: &space.ID, BotID: &sam.ID})
	if err != nil || len(turns) != 1 || turns[0].ID != turn.ID || turns[0].Role != "sam" || turns[0].SessionID != "session" {
		t.Fatalf("turns = %+v, %v", turns, err)
	}
	if _, err := s.CreateToken(ctx, space.ID, 99999); err == nil {
		t.Fatal("token for a non-member was accepted")
	}
	gotToken, err := s.Token(ctx, token.Token)
	if err != nil || gotToken.Role != "sam" || gotToken.BotID == nil || *gotToken.BotID != sam.ID {
		t.Fatalf("token = %+v, %v", gotToken, err)
	}
	directs, err := s.DirectMessagesForBot(ctx, space.ID, sam.ID, 0)
	if err != nil || len(directs) != 3 {
		t.Fatalf("directs = %+v, %v", directs, err)
	}
	if directs[0].From != "sam" || directs[0].To != "ann" || *directs[0].FromBotID != sam.ID || *directs[0].ToBotID != ann.ID || directs[2].ID != response.ID || directs[2].From != "anna" || directs[2].To != "sam" || *directs[2].FromBotID != ann.ID || *directs[2].ToBotID != sam.ID {
		t.Fatalf("directs = %+v", directs)
	}
	if _, err := s.CreateDirectResponse(ctx, dm.ID, &sam.ID, "sally", "wrong"); err == nil {
		t.Fatal("sender answered recipient's request")
	}
}
