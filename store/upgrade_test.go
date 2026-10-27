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
	if version != 67 {
		t.Errorf("user_version = %d, want 67", version)
	}

	at := time.Date(2026, time.January, 2, 3, 4, 5, 123456789, time.UTC)
	if got, err := s.Project(ctx, 1); err != nil ||
		!reflect.DeepEqual(got, Project{ID: 1, Name: "shop", Path: "/work/shop", CreatedAt: at}) {
		t.Errorf("project = %+v, %v, want shop kept", got, err)
	}
	if _, err := s.Team(ctx, "solo"); err != nil {
		t.Errorf("solo team: %v", err)
	}
	for _, table := range []string{
		"work_item", "brief", "role_notes", "space", "message", "turn", "decision",
	} {
		var n int
		if err := s.db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("%s rows after the upgrade = %d, want 0", table, n)
		}
	}
	w, err := s.CreateWorkItem(ctx, WorkItem{ProjectID: 1, Title: "next"})
	if err != nil || w.ID != 2 {
		t.Errorf("first work item after the upgrade = %+v, %v, want id 2", w, err)
	}
}

func TestOpen_UpgradesVersion8Database(t *testing.T) {
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
	if version != 67 {
		t.Errorf("user_version = %d, want 67", version)
	}
	var approvals int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM approval").Scan(&approvals); err != nil {
		t.Fatal(err)
	}
	if approvals != 0 {
		t.Errorf("approval rows = %d, want old rows removed", approvals)
	}
	var tables int
	if err := s.db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'gate'",
	).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables != 0 {
		t.Errorf("gate tables = %d, want none", tables)
	}
}

func TestOpen_ProposalItemsMigrationRejectsPendingWrapsOnly(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "reef.db"))
	if err != nil {
		t.Fatal(err)
	}
	for n := 1; n <= 37; n++ {
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
		"INSERT INTO proposal (channel_id, role, kind, brief, notes, decisions, status, created_at) " +
			"VALUES (1, 'lead', 'wrap', 'wrap', '', '[]', 'pending', '2026-01-02T03:04:05Z')",
		"INSERT INTO proposal (channel_id, role, kind, brief, notes, decisions, status, created_at) " +
			"VALUES (1, 'lead', 'brief', 'brief', '', '[]', 'pending', '2026-01-02T03:04:06Z')",
		"INSERT INTO proposal (channel_id, role, kind, brief, notes, decisions, status, created_at, decided_at) " +
			"VALUES (1, 'lead', 'wrap', 'old wrap', '', '[]', 'approved', '2026-01-02T03:04:07Z', '2026-01-02T03:04:08Z')",
		"PRAGMA user_version = 37",
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

	wrap, err := s.Proposal(ctx, 1)
	if err != nil || wrap.Status != ProposalRejected || wrap.DecidedAt == nil ||
		time.Since(*wrap.DecidedAt) > time.Minute || time.Until(*wrap.DecidedAt) > time.Minute {
		t.Errorf("pending wrap = %+v, %v, want rejected just now", wrap, err)
	}
	brief, err := s.Proposal(ctx, 2)
	if err != nil || brief.Status != ProposalPending || brief.DecidedAt != nil {
		t.Errorf("pending brief = %+v, %v, want still pending", brief, err)
	}
	approved, err := s.Proposal(ctx, 3)
	want := time.Date(2026, time.January, 2, 3, 4, 8, 0, time.UTC)
	if err != nil || approved.Status != ProposalApproved || approved.DecidedAt == nil ||
		!approved.DecidedAt.Equal(want) {
		t.Errorf("approved wrap = %+v, %v, want approved at %v", approved, err, want)
	}
}

func TestOpen_UpgradeLeavesSpaceNamesUserSet(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "reef.db"))
	if err != nil {
		t.Fatal(err)
	}
	for n := 1; n <= 32; n++ {
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
		"PRAGMA user_version = 32",
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

	var pending int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM space WHERE name_auto <> 0").Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending != 0 {
		t.Errorf("spaces the provider may rename = %d, want 0", pending)
	}
	if ok, err := s.ClaimName(context.Background(), 1); err != nil || ok {
		t.Errorf("ClaimName on an upgraded Space = %v, %v, want false", ok, err)
	}
}
