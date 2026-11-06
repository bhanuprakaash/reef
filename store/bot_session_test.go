package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestBotSession_AdoptKeepsSettingsAcrossBotEdits(t *testing.T) {
	ctx := context.Background()
	s := spaceMemberStore(t)
	sam, err := s.CreateBot(ctx, Bot{
		Name: "sam", Backend: "claude", Model: "haiku", Description: "You code.", Skills: []string{"claude:sketch"},
		Tools: []string{"Read"}, Write: true, PermissionMode: "acceptEdits",
	})
	if err != nil {
		t.Fatal(err)
	}
	chat, err := s.CreateBotSpace(ctx,
		Space{ProjectID: 1, Name: "chat", Kind: KindConversation, Role: "sam"}, sam.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ActiveBotSession(ctx, chat.ID, sam.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("active session before adopting = %v, want ErrNotFound", err)
	}

	first, err := s.AdoptBotSession(ctx, chat.ID, sam.ID)
	if err != nil {
		t.Fatal(err)
	}
	sam.Model, sam.Description, sam.Tools, sam.PermissionMode = "opus", "You review.", []string{"Grep"}, "default"
	if _, err := s.UpdateBot(ctx, sam); err != nil {
		t.Fatal(err)
	}
	got, err := s.ActiveBotSession(ctx, chat.ID, sam.ID)
	if err != nil || got.ID != first.ID || got.Backend != "claude" || got.Model != "haiku" ||
		got.Description != "You code." || !reflect.DeepEqual(got.Skills, []AdoptedSkill{{Ref: "claude:sketch"}}) ||
		!reflect.DeepEqual(got.Tools, []string{"Read"}) || !got.Write || got.PermissionMode != "acceptEdits" ||
		got.Status != BotSessionActive {
		t.Fatalf("active session = %+v, %v, want the settings adopted before the edit", got, err)
	}

	if _, err := s.SetRefreshPending(ctx, chat.ID, sam.ID); err != nil {
		t.Fatal(err)
	}
	second, err := s.AdoptBotSession(ctx, chat.ID, sam.ID)
	var pending int
	if err := s.db.QueryRowContext(ctx, "SELECT refresh_pending FROM bot_session WHERE id = ?", first.ID).Scan(&pending); err != nil || pending != 0 {
		t.Errorf("replaced session refresh_pending = %d, %v, want 0", pending, err)
	}
	if err != nil || second.ID == first.ID || second.Model != "opus" || second.Description != "You review." ||
		!reflect.DeepEqual(second.Tools, []string{"Grep"}) || second.PermissionMode != "default" {
		t.Fatalf("second session = %+v, %v, want the edited settings", second, err)
	}
	if err := s.EndBotSession(ctx, first.ID, BotSessionRevoked); !errors.Is(err, ErrNotFound) {
		t.Errorf("end of a replaced session = %v, want ErrNotFound", err)
	}
	if err := s.EndBotSession(ctx, second.ID, BotSessionRevoked); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ActiveBotSession(ctx, chat.ID, sam.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("active session after revoking = %v, want ErrNotFound", err)
	}
}
