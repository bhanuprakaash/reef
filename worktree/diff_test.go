package worktree

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDiff_UncommittedAndUntracked(t *testing.T) {
	repo := newRepo(t)
	dir := filepath.Join(t.TempDir(), "demo")
	if err := Ensure(context.Background(), repo, dir, "feature/demo"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	base, err := Base(context.Background(), repo, dir)
	if err != nil {
		t.Fatal(err)
	}
	diffs, err := Diff(context.Background(), dir, base)
	if err != nil {
		t.Fatal(err)
	}
	if len(diffs) != 1 {
		t.Fatalf("len(diffs) = %d, want 1: %+v", len(diffs), diffs)
	}
	d := diffs[0]
	if d.Path != "new.txt" || d.Status != "added" {
		t.Fatalf("file = %+v, want new.txt added", d)
	}
	if len(d.Hunks) != 1 || len(d.Hunks[0].Lines) != 2 {
		t.Fatalf("hunks = %+v", d.Hunks)
	}
	want := []Line{
		{Kind: "add", New: 1, Text: "one"},
		{Kind: "add", New: 2, Text: "two"},
	}
	for i, l := range d.Hunks[0].Lines {
		if l != want[i] {
			t.Errorf("line %d = %+v, want %+v", i, l, want[i])
		}
	}
}

func TestDiff_NoChanges(t *testing.T) {
	repo := newRepo(t)
	dir := filepath.Join(t.TempDir(), "demo")
	if err := Ensure(context.Background(), repo, dir, "feature/demo"); err != nil {
		t.Fatal(err)
	}
	base, err := Base(context.Background(), repo, dir)
	if err != nil {
		t.Fatal(err)
	}
	diffs, err := Diff(context.Background(), dir, base)
	if err != nil {
		t.Fatal(err)
	}
	if len(diffs) != 0 {
		t.Fatalf("diffs = %+v, want none", diffs)
	}
}
