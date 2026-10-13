package store

import (
	"path/filepath"
	"reflect"
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
	if version != 1 {
		t.Errorf("user_version = %d, want 1", version)
	}

	rows, err := s.db.Query("SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
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
		"project", "role_notes", "task", "turn",
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
	defer s.Close()
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 1 {
		t.Errorf("user_version after reopen = %d, want 1", version)
	}
	var name string
	if err := s.db.QueryRow("SELECT name FROM project WHERE id = 1").Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "reef" {
		t.Errorf("project name after reopen = %q, want %q", name, "reef")
	}
}

func TestForeignKeys(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

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
