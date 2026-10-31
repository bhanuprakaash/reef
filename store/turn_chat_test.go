package store

import (
	"context"
	"reflect"
	"testing"
)

func TestFailRunningTurnsInAChatSaysNothingOfTheWorktree(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	dev := *identityTestBots(t, s)["developer"]
	chat, err := s.CreateBotSpace(ctx, Space{Name: "chat", Kind: KindConversation}, dev)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateTurn(ctx, Turn{SpaceID: chat.ID, Role: "developer", Backend: "claude"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FailRunningTurns(ctx, nil); err != nil {
		t.Fatal(err)
	}
	want := []messageRow{{"reef", "Turn 1 by developer failed: server restarted."}}
	if got := messageRows(t, s, chat.ID); !reflect.DeepEqual(got, want) {
		t.Errorf("messages = %v, want %v", got, want)
	}
}
