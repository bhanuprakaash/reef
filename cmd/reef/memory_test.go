package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLI_Memory(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(t.TempDir())
	mustReef(t, dir, "project", "add", "--name", "shop")

	file := filepath.Join(t.TempDir(), "memory.md")
	if err := os.WriteFile(file, []byte("Use Go, not Python.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, want := mustReef(t, dir, "memory", "set", file),
		"saved memory for project shop\n"; got != want {
		t.Fatalf("memory set = %q, want %q", got, want)
	}
	if got, want := mustReef(t, dir, "memory", "show"), "Use Go, not Python.\n\n"; got != want {
		t.Fatalf("memory show = %q, want %q", got, want)
	}

	var out bytes.Buffer
	err := run(context.Background(), []string{"memory", "set", "-", "--data", dir},
		strings.NewReader("from stdin"), &out, &syncBuffer{})
	if err != nil {
		t.Fatalf("memory set -: %v", err)
	}
	if got, want := mustReef(t, dir, "memory", "show"), "from stdin\n"; got != want {
		t.Fatalf("memory show after stdin set = %q, want %q", got, want)
	}

	if _, err := reef(t, dir, "memory", "show", "--project", "9"); err == nil ||
		err.Error() != "project 9: not found" {
		t.Errorf("memory show --project 9: error = %v, want project 9: not found", err)
	}
	if _, err := reef(t, dir, "memory", "show", "--project", "x"); err == nil ||
		err.Error() != `invalid: --project "x" is not a project id` {
		t.Errorf("memory show --project x: error = %v", err)
	}
}
