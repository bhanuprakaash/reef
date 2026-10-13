package store

import (
	"context"
	"errors"
	"testing"

	"github.com/bhanuprakaash/reef/agent"
)

func TestChannelTokens_ExcludesCache(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.CreateProject(ctx, Project{Name: "shop", Path: "/tmp/shop"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a", "b"} {
		if _, err := s.CreateChannel(ctx, Channel{ProjectID: 1, Name: name}); err != nil {
			t.Fatal(err)
		}
	}
	turns := []struct {
		channelID int64
		usage     agent.Usage
	}{
		{1, agent.Usage{InputTokens: 10, OutputTokens: 5, CacheReadTokens: 1000}},
		{1, agent.Usage{InputTokens: 20, OutputTokens: 7, CacheWriteTokens: 500}},
		{2, agent.Usage{InputTokens: 99, OutputTokens: 99}},
	}
	for _, u := range turns {
		tr, err := s.CreateTurn(ctx, Turn{ChannelID: u.channelID, Role: "lead", Backend: "claude"})
		if err != nil {
			t.Fatal(err)
		}
		tr.Status, tr.Usage = "done", u.usage
		if _, err := s.EndTurn(ctx, tr); err != nil {
			t.Fatal(err)
		}
	}

	if got, err := s.ChannelTokens(ctx, 1); err != nil || got != 42 {
		t.Errorf("ChannelTokens(1) = %d, %v, want 42", got, err)
	}
}

func TestPauseAndContinueChannel(t *testing.T) {
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
	if got, err := s.Channel(ctx, ch.ID); err != nil || got.Paused != "" || got.TokenBudget != 1000000 {
		t.Fatalf("new channel = %+v, %v, want not paused with budget 1000000", got, err)
	}

	if err := s.PauseChannel(ctx, ch.ID, "capped"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Channel(ctx, ch.ID); got.Paused != "capped" {
		t.Errorf("Paused = %q, want capped", got.Paused)
	}
	if err := s.ContinueChannel(ctx, ch.ID, 500); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Channel(ctx, ch.ID); got.Paused != "" || got.TokenBudget != 1000500 {
		t.Errorf("after continue = paused %q budget %d, want not paused, 1000500",
			got.Paused, got.TokenBudget)
	}
	if err := s.PauseChannel(ctx, ch.ID, ""); !errors.Is(err, ErrInvalid) {
		t.Errorf("PauseChannel with empty reason = %v, want ErrInvalid", err)
	}
	if err := s.PauseChannel(ctx, 99, "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("PauseChannel on unknown channel = %v, want ErrNotFound", err)
	}
}
