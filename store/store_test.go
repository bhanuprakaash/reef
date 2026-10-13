package store

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestOpen(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Application Support", "Reef")
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 6 {
		t.Errorf("user_version = %d, want 6", version)
	}

	rows, err := s.db.Query("SELECT name FROM sqlite_master WHERE type = 'table' " +
		"AND name NOT LIKE 'sqlite_%' ORDER BY name")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, name)
	}
	want := []string{
		"brief", "channel", "decision", "feature", "message",
		"project", "proposal", "role", "role_notes", "task", "team", "token", "turn",
	}
	if !reflect.DeepEqual(tables, want) {
		t.Errorf("tables = %v, want %v", tables, want)
	}

	if _, err := s.db.Exec(
		"INSERT INTO project (id, name, path, created_at) VALUES (1, 'reef', '/tmp/reef', '2026-10-13T00:00:00Z')",
	); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 6 {
		t.Errorf("user_version after reopen = %d, want 6", version)
	}
	var name string
	if err := s.db.QueryRow("SELECT name FROM project WHERE id = 1").Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "reef" {
		t.Errorf("project name after reopen = %q, want %q", name, "reef")
	}
}

func TestOpenOddDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a#b?c%20d")
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := os.Stat(filepath.Join(dir, "reef.db")); err != nil {
		t.Errorf("reef.db not in the data dir: %v", err)
	}
}

func TestOpenNewerDatabase(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("PRAGMA user_version = 99"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = Open(dir)
	if err == nil || !strings.Contains(err.Error(), "database is newer than this reef") {
		t.Errorf("Open = %v, want a newer-database error", err)
	}
}

func TestOpenConcurrent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Reef")
	const n = 8
	start := make(chan struct{})
	stores := make([]*Store, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			stores[i], errs[i] = Open(dir)
		}()
	}
	close(start)
	wg.Wait()
	for i := range n {
		if errs[i] != nil {
			t.Fatalf("Open %d: %v", i, errs[i])
		}
		defer func() { _ = stores[i].Close() }()
	}
	var version int
	if err := stores[0].db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 6 {
		t.Errorf("user_version = %d, want 6", version)
	}
}

func TestForeignKeys(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	const feature = "INSERT INTO feature (project_id, slug, name, created_at) " +
		"VALUES (?, 'a', 'A', '2026-10-13T00:00:00Z')"
	if _, err := s.db.Exec(feature, 99); err == nil {
		t.Error("insert feature with unknown project_id succeeded, want error")
	}

	if _, err := s.db.Exec(
		"INSERT INTO project (id, name, path, created_at) VALUES (1, 'reef', '/tmp/reef', '2026-10-13T00:00:00Z')",
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(feature, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("DELETE FROM project WHERE id = 1"); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.db.QueryRow("SELECT count(*) FROM feature").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("features after project delete = %d, want 0", n)
	}
}

func TestIDsNotReused(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	ctx := context.Background()
	p, err := s.CreateProject(ctx, Project{Name: "a", Path: "/a"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteProject(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	p, err = s.CreateProject(ctx, Project{Name: "b", Path: "/b"})
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != 2 {
		t.Errorf("id after deleting project 1 = %d, want 2", p.ID)
	}
}

func TestWriteErrNamesMissingParent(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	ctx := context.Background()
	p, err := s.CreateProject(ctx, Project{Name: "a", Path: "/a"})
	if err != nil {
		t.Fatal(err)
	}
	feature := int64(7)
	_, err = s.db.Exec(
		"INSERT INTO task (project_id, feature_id, title, created_at, updated_at) "+
			"VALUES (?, ?, 't', '2026-10-13T00:00:00Z', '2026-10-13T00:00:00Z')", p.ID, feature)
	err = s.writeErr(ctx, "insert task", err, ErrInvalid,
		parent{table: "project", id: &p.ID}, parent{table: "feature", id: &feature})
	if want := "invalid: unknown feature 7"; err == nil || err.Error() != want {
		t.Errorf("writeErr = %v, want %q", err, want)
	}
}
