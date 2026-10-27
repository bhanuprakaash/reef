package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bhanuprakaash/reef/agent"
)

func TestSpace_NilTeamIsNoTeam(t *testing.T) {
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
		"INSERT INTO space (project_id, name, created_at, team_id) VALUES (?, ?, ?, NULL)",
		project.ID, "chat", time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := s.Space(ctx, id)
	if err != nil || ch.Team != "" {
		t.Errorf("Space(%d) = %+v, %v, want no team", id, ch, err)
	}
	spaces, err := s.Spaces(ctx, SpaceFilter{ProjectID: &project.ID})
	if err != nil || len(spaces) != 1 || spaces[0].Team != "" {
		t.Errorf("Spaces() = %+v, %v, want one Space with no team", spaces, err)
	}
}

func TestSpaceTokens_ExcludesCacheReads(t *testing.T) {
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
		if _, err := s.CreateSpace(ctx, Space{ProjectID: 1, Name: name}); err != nil {
			t.Fatal(err)
		}
	}
	turns := []struct {
		spaceID int64
		usage   agent.Usage
	}{
		{1, agent.Usage{InputTokens: 10, OutputTokens: 5, CacheReadTokens: 1000}},
		{1, agent.Usage{InputTokens: 20, OutputTokens: 7, CacheWriteTokens: 500}},
		{2, agent.Usage{InputTokens: 99, OutputTokens: 99}},
	}
	for _, u := range turns {
		tr, err := s.CreateTurn(ctx, Turn{SpaceID: u.spaceID, Role: "lead", Backend: "claude"})
		if err != nil {
			t.Fatal(err)
		}
		tr.Status, tr.Usage = "done", u.usage
		if _, err := s.EndTurn(ctx, tr); err != nil {
			t.Fatal(err)
		}
	}

	if got, err := s.SpaceTokens(ctx, 1); err != nil || got != 542 {
		t.Errorf("SpaceTokens(1) = %d, %v, want 542", got, err)
	}
}

func TestPauseAndContinueSpace(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.CreateProject(ctx, Project{Name: "shop", Path: "/tmp/shop"}); err != nil {
		t.Fatal(err)
	}
	ch, err := s.CreateSpace(ctx, Space{ProjectID: 1, Name: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.Space(ctx, ch.ID); err != nil || got.Paused != "" || got.TokenBudget != 1000000 {
		t.Fatalf("new Space = %+v, %v, want not paused with budget 1000000", got, err)
	}

	if err := s.PauseSpace(ctx, ch.ID, "capped"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Space(ctx, ch.ID); got.Paused != "capped" {
		t.Errorf("Paused = %q, want capped", got.Paused)
	}
	if err := s.ContinueSpace(ctx, ch.ID, 1000500); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Space(ctx, ch.ID); got.Paused != "" || got.TokenBudget != 1000500 {
		t.Errorf("after continue = paused %q budget %d, want not paused, 1000500",
			got.Paused, got.TokenBudget)
	}
	if err := s.PauseSpace(ctx, ch.ID, ""); !errors.Is(err, ErrInvalid) {
		t.Errorf("PauseSpace with empty reason = %v, want ErrInvalid", err)
	}
	if err := s.PauseSpace(ctx, 99, "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("PauseSpace on unknown Space = %v, want ErrNotFound", err)
	}
}

func TestSpace_NameSteps(t *testing.T) {
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
		c, err := s.CreateSpace(ctx, Space{ProjectID: p.ID, Name: name, Backend: "claude"})
		if err != nil {
			t.Fatal(err)
		}
		return c.ID
	}

	typed := mk("typed")
	if ok, err := s.ClaimName(ctx, typed); err != nil || ok {
		t.Errorf("ClaimName on a user-named Space = %v, %v, want false", ok, err)
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
	if c, err := s.Space(ctx, pending); err != nil || c.Name != "login redirect" {
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
	if c, err := s.Space(ctx, failed); err != nil || c.Name != "claude 02:33 PM" {
		t.Errorf("name = %q, %v, want the default name kept", c.Name, err)
	}

	renamed := mk("claude 02:34 PM")
	if err := s.MarkNamePending(ctx, renamed); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimName(ctx, renamed); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateSpace(ctx, Space{ID: renamed, Name: "mine"}); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.FinishName(ctx, renamed, "proposed"); err != nil || ok {
		t.Errorf("FinishName after a rename = %v, %v, want false", ok, err)
	}
	if c, err := s.Space(ctx, renamed); err != nil || c.Name != "mine" {
		t.Errorf("name = %q, %v, want mine", c.Name, err)
	}

	same := mk("claude 02:35 PM")
	if err := s.MarkNamePending(ctx, same); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateSpace(ctx, Space{ID: same, Name: "claude 02:35 PM", PermissionMode: "acceptEdits"}); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.ClaimName(ctx, same); err != nil || !ok {
		t.Errorf("ClaimName after an update that kept the name = %v, %v, want true", ok, err)
	}
}
