package store

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/bhanuprakaash/reef/agent"
)

// testdata/v1.db was built with only migration 001 applied (user_version 1) and one
// row in every table that migration creates.
func TestOpen_UpgradesVersion1Database(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	fixture, err := os.ReadFile(filepath.Join("testdata", "v1.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "reef.db"), fixture, 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 19 {
		t.Errorf("user_version = %d, want 19", version)
	}

	at := func(h, m, sec, ns int) time.Time {
		return time.Date(2026, time.January, 2, h, m, sec, ns, time.UTC)
	}
	one, ended := int64(1), at(3, 11, 0, 0)
	tests := []struct {
		name string
		give func() (any, error)
		want any
	}{
		{"project", func() (any, error) { return s.Project(ctx, 1) },
			Project{ID: 1, Name: "shop", Path: "/work/shop", CreatedAt: at(3, 4, 5, 123456789)}},
		{"feature", func() (any, error) { return s.Feature(ctx, 1) },
			Feature{ID: 1, ProjectID: 1, Slug: "login", Name: "Login", Goal: "sign in",
				Stage: "design", CreatedAt: at(3, 5, 0, 0)}},
		{"brief", func() (any, error) { return s.Brief(ctx, 1) },
			Brief{ID: 1, FeatureID: 1, Body: "sign in with sessions", Author: "user",
				CreatedAt: at(3, 6, 0, 0)}},
		{"role notes", func() (any, error) { return s.RoleNotes(ctx, 1, "architect") },
			RoleNotes{FeatureID: 1, Role: "architect", Body: "prefer cookies",
				UpdatedAt: at(3, 7, 0, 0)}},
		{"channel", func() (any, error) { return s.Channel(ctx, 1) },
			Channel{ID: 1, ProjectID: 1, FeatureID: &one, Name: "architect chat",
				Role: "architect", Backend: "claude", TokenBudget: 1000000, Team: "solo",
				CreatedAt: at(3, 8, 0, 0)}},
		{"solo team", func() (any, error) { return s.Team(ctx, "solo") },
			Team{Name: "solo", Roles: []Role{{Name: "lead", Lead: true, Write: true, Backend: "claude",
				Tools: []string{}, MCPServers: []agent.MCPServer{}}}}},
		{"messages", func() (any, error) { return s.Messages(ctx, 1, 0) },
			[]Message{{ID: 1, ChannelID: 1, Author: "user", Body: "how do we sign in?",
				CreatedAt: at(3, 9, 0, 0)}}},
		{"turn", func() (any, error) { return s.Turn(ctx, 1) },
			Turn{ID: 1, ChannelID: 1, Role: "architect", Backend: "claude",
				SessionID: "sess-1", Status: "done", Sent: true,
				Usage: agent.Usage{InputTokens: 11, OutputTokens: 22,
					CacheReadTokens: 33, CacheWriteTokens: 44},
				StartedAt: at(3, 10, 0, 0), EndedAt: &ended}},
		{"decision", func() (any, error) { return s.Decision(ctx, 1) },
			Decision{ID: 1, FeatureID: 1, MessageID: &one, Body: "use sessions",
				Why: "simple", Author: "user", CreatedAt: at(3, 12, 0, 0)}},
		{"task", func() (any, error) { return s.Task(ctx, 1) },
			Task{ID: 1, ProjectID: 1, FeatureID: &one, Title: "write tests",
				Body: "cover login", Status: "doing", CreatedAt: at(3, 13, 0, 0),
				UpdatedAt: at(3, 14, 0, 0)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.give()
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestOpen_UpgradesVersion8Database(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "reef.db"))
	if err != nil {
		t.Fatal(err)
	}
	for n := 1; n <= 8; n++ {
		files, err := fs.Glob(_migrations, fmt.Sprintf("migrations/%03d_*.sql", n))
		if err != nil || len(files) != 1 {
			t.Fatalf("migration %d: %v, %v", n, files, err)
		}
		body, err := _migrations.ReadFile(files[0])
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(body)); err != nil {
			t.Fatal(err)
		}
	}
	for _, stmt := range []string{
		"INSERT INTO project (name, path, created_at) VALUES ('shop', '/work/shop', '2026-01-02T03:04:05Z')",
		"INSERT INTO channel (project_id, name, created_at, team_id) " +
			"VALUES (1, 'c', '2026-01-02T03:04:05Z', 1)",
		"INSERT INTO message (channel_id, author, body, created_at) " +
			"VALUES (1, 'lead', '@architect plan', '2026-01-02T03:04:05Z')",
		"INSERT INTO approval (channel_id, from_role, to_role, message_id, text, created_at) " +
			"VALUES (1, 'lead', 'architect', 1, 'plan it', '2026-01-02T03:04:05Z')",
		"PRAGMA user_version = 8",
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 19 {
		t.Errorf("user_version = %d, want 19", version)
	}
	as, err := s.PendingApprovals(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(as) != 1 || as[0].Kind != ApprovalGate || as[0].Text != "plan it" {
		t.Errorf("approvals = %+v, want the one approval with kind gate", as)
	}
}
