package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bhanuprakaash/reef/agent"
)

func TestChannel_NilTeamUsesSoloTeam(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	project, err := s.CreateProject(ctx, Project{Name: "shop", Path: "/tmp/shop"})
	if err != nil {
		t.Fatal(err)
	}
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO channel (project_id, name, created_at, team_id) VALUES (?, ?, ?, NULL)",
		project.ID, "chat", time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := s.Channel(ctx, id)
	if err != nil || ch.Team != SoloTeam {
		t.Errorf("Channel(%d) = %+v, %v, want team %q", id, ch, err, SoloTeam)
	}
	channels, err := s.Channels(ctx, ChannelFilter{ProjectID: &project.ID})
	if err != nil || len(channels) != 1 || channels[0].Team != SoloTeam {
		t.Errorf("Channels() = %+v, %v, want one channel with team %q", channels, err, SoloTeam)
	}
}

func TestChannelTokens_ExcludesCacheReads(t *testing.T) {
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

	if got, err := s.ChannelTokens(ctx, 1); err != nil || got != 542 {
		t.Errorf("ChannelTokens(1) = %d, %v, want 542", got, err)
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
	if err := s.ContinueChannel(ctx, ch.ID, 1000500); err != nil {
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

func TestCheckPromote_ChangesNothing(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.CreateProject(ctx, Project{Name: "shop", Path: "/tmp/shop"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateChannel(ctx, Channel{ProjectID: 1, Name: "chat"}); err != nil {
		t.Fatal(err)
	}

	if err := s.CheckPromote(ctx, 1, Feature{Slug: "login", Name: "Login"}); err != nil {
		t.Fatalf("CheckPromote = %v, want nil", err)
	}
	if got, err := s.Channel(ctx, 1); err != nil || got.FeatureID != nil {
		t.Errorf("channel after check = %+v, %v, want no feature", got, err)
	}
	if fs, err := s.Features(ctx, nil); err != nil || len(fs) != 0 {
		t.Errorf("features after check = %+v, %v, want none", fs, err)
	}

	if _, err := s.PromoteChannel(ctx, 1, Feature{Slug: "login", Name: "Login"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateChannel(ctx, Channel{ProjectID: 1, Name: "second"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CheckPromote(ctx, 2, Feature{Slug: "login", Name: "Login"}); !errors.Is(err, ErrConflict) {
		t.Errorf("CheckPromote with a taken slug = %v, want ErrConflict", err)
	}
	if err := s.CheckPromote(ctx, 1, Feature{ID: 1}); !errors.Is(err, ErrInvalid) {
		t.Errorf("CheckPromote on a channel with a feature = %v, want ErrInvalid", err)
	}
}

func TestChannel_NameSteps(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	p, err := s.CreateProject(ctx, Project{Name: "shop", Path: "/tmp/shop"})
	if err != nil {
		t.Fatal(err)
	}
	mk := func(name string) int64 {
		t.Helper()
		c, err := s.CreateChannel(ctx, Channel{ProjectID: p.ID, Name: name, Backend: "claude"})
		if err != nil {
			t.Fatal(err)
		}
		return c.ID
	}

	typed := mk("typed")
	if ok, err := s.ClaimName(ctx, typed); err != nil || ok {
		t.Errorf("ClaimName on a user-named channel = %v, %v, want false", ok, err)
	}

	pending := mk("claude 02:32 PM")
	if err := s.MarkNamePending(ctx, pending); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.ClaimName(ctx, pending); err != nil || !ok {
		t.Fatalf("first ClaimName = %v, %v, want true", ok, err)
	}
	if ok, err := s.ClaimName(ctx, pending); err != nil || ok {
		t.Errorf("second ClaimName = %v, %v, want false", ok, err)
	}
	if ok, err := s.FinishName(ctx, pending, "login redirect"); err != nil || !ok {
		t.Fatalf("FinishName = %v, %v, want true", ok, err)
	}
	if c, err := s.Channel(ctx, pending); err != nil || c.Name != "login redirect" {
		t.Errorf("name = %q, %v, want login redirect", c.Name, err)
	}
	if ok, err := s.FinishName(ctx, pending, "again"); err != nil || ok {
		t.Errorf("second FinishName = %v, %v, want false", ok, err)
	}

	failed := mk("claude 02:33 PM")
	if err := s.MarkNamePending(ctx, failed); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimName(ctx, failed); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.FinishName(ctx, failed, ""); err != nil || !ok {
		t.Fatalf("FinishName with no name = %v, %v, want true", ok, err)
	}
	if c, err := s.Channel(ctx, failed); err != nil || c.Name != "claude 02:33 PM" {
		t.Errorf("name = %q, %v, want the default name kept", c.Name, err)
	}

	renamed := mk("claude 02:34 PM")
	if err := s.MarkNamePending(ctx, renamed); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimName(ctx, renamed); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateChannel(ctx, Channel{ID: renamed, Name: "mine"}); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.FinishName(ctx, renamed, "proposed"); err != nil || ok {
		t.Errorf("FinishName after a rename = %v, %v, want false", ok, err)
	}
	if c, err := s.Channel(ctx, renamed); err != nil || c.Name != "mine" {
		t.Errorf("name = %q, %v, want mine", c.Name, err)
	}

	same := mk("claude 02:35 PM")
	if err := s.MarkNamePending(ctx, same); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateChannel(ctx, Channel{ID: same, Name: "claude 02:35 PM", PermissionMode: "acceptEdits"}); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.ClaimName(ctx, same); err != nil || !ok {
		t.Errorf("ClaimName after an update that kept the name = %v, %v, want true", ok, err)
	}
}
