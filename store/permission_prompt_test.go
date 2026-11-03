package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestPermissionPrompt_ResolveOnceAndGrant(t *testing.T) {
	ctx := context.Background()
	s := spaceMemberStore(t)
	sam, err := s.CreateBot(ctx, Bot{Name: "sam", Backend: "claude", Tools: []string{"Read"}})
	if err != nil {
		t.Fatal(err)
	}
	chat, err := s.CreateBotSpace(ctx,
		Space{ProjectID: 1, Name: "chat", Kind: KindConversation, Role: "sam"}, sam.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdoptBotSession(ctx, chat.ID, sam.ID); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreatePermissionPrompt(ctx, PermissionPrompt{
		SpaceID: chat.ID, BotID: sam.ID, RequestID: "r1", Tool: "Bash", Rule: "Bash(gzip -l /x)",
		Input: []byte(`{"command":"gzip -l /x"}`),
	})
	if err != nil || p.Status != PromptWaiting {
		t.Fatalf("CreatePermissionPrompt = %+v, %v, want a waiting prompt", p, err)
	}

	if _, err := s.ResolvePermissionPrompt(ctx, p.ID, PromptWaiting, PromptExpired, AnswerDeny, "no answer from the user", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResolvePermissionPrompt(ctx, p.ID, PromptWaiting, PromptAllowed, AnswerOnce, "", false); !errors.Is(err, ErrConflict) {
		t.Fatalf("resolving an expired prompt as waiting = %v, want ErrConflict", err)
	}
	got, err := s.ResolvePermissionPrompt(ctx, p.ID, PromptExpired, PromptAllowed, AnswerOnce, "", true)
	if err != nil || got.Status != PromptAllowed || !got.Granted || got.AnsweredAt == nil {
		t.Fatalf("allowing an expired prompt = %+v, %v, want allowed and granted", got, err)
	}
	for i, want := range []bool{true, false} {
		if took, err := s.TakeGrantedPrompt(ctx, chat.ID, sam.ID, "Bash(gzip -l /x)"); err != nil || took != want {
			t.Errorf("TakeGrantedPrompt #%d = %v, %v, want %v", i+1, took, err, want)
		}
	}

	if err := s.AddBotTool(ctx, chat.ID, sam.ID, "Bash(gzip -l /x)"); err != nil {
		t.Fatal(err)
	}
	bs, err := s.ActiveBotSession(ctx, chat.ID, sam.ID)
	if want := []string{"Read", "Bash(gzip -l /x)"}; err != nil || !reflect.DeepEqual(bs.Tools, want) {
		t.Errorf("session tools = %v, %v, want %v", bs.Tools, err, want)
	}
	bot, err := s.Bot(ctx, sam.ID)
	if want := []string{"Read", "Bash(gzip -l /x)"}; err != nil || !reflect.DeepEqual(bot.Tools, want) {
		t.Errorf("bot tools = %v, %v, want %v", bot.Tools, err, want)
	}
}
