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

func TestDiff_EmptyNewFile(t *testing.T) {
	repo := newRepo(t)
	dir := filepath.Join(t.TempDir(), "demo")
	if err := Ensure(context.Background(), repo, dir, "feature/demo"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "empty.txt"), nil, 0o644); err != nil {
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
	if d.Path != "empty.txt" || d.Status != "added" {
		t.Fatalf("file = %+v, want empty.txt added", d)
	}
	if d.Hunks == nil || len(d.Hunks) != 0 {
		t.Fatalf("hunks = %+v, want non-nil empty", d.Hunks)
	}
}

func TestDiff_BinaryFile(t *testing.T) {
	repo := newRepo(t)
	dir := filepath.Join(t.TempDir(), "demo")
	if err := Ensure(context.Background(), repo, dir, "feature/demo"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "binary.bin"), []byte{0x00, 0x01, 0x02, 0xff}, 0o644); err != nil {
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
	if d.Path != "binary.bin" || d.Status != "added" {
		t.Fatalf("file = %+v, want binary.bin added", d)
	}
	if d.Hunks == nil || len(d.Hunks) != 0 {
		t.Fatalf("hunks = %+v, want non-nil empty", d.Hunks)
	}
}

func TestDiff_RenameAsDeleteAndAdd(t *testing.T) {
	repo := newRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "old.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, repo, "add", "old.txt")
	run(t, repo, "commit", "-m", "add old")

	dir := filepath.Join(t.TempDir(), "demo")
	if err := Ensure(context.Background(), repo, dir, "feature/demo"); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(dir, "old.txt"), filepath.Join(dir, "new.txt")); err != nil {
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
	if len(diffs) != 2 {
		t.Fatalf("len(diffs) = %d, want 2: %+v", len(diffs), diffs)
	}
	add, del := diffs[0], diffs[1]
	if del.Path != "old.txt" || del.Status != "deleted" {
		t.Fatalf("diffs[0] = %+v, want old.txt deleted", del)
	}
	if add.Path != "new.txt" || add.Status != "added" {
		t.Fatalf("diffs[1] = %+v, want new.txt added", add)
	}
}

func TestDiff_NonASCIIName(t *testing.T) {
	repo := newRepo(t)
	dir := filepath.Join(t.TempDir(), "demo")
	if err := Ensure(context.Background(), repo, dir, "feature/demo"); err != nil {
		t.Fatal(err)
	}
	name := "déjà.txt"
	if err := os.WriteFile(filepath.Join(dir, name), []byte("hi\n"), 0o644); err != nil {
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
	if d.Path != name || d.Status != "added" {
		t.Fatalf("file = %+v, want %q added", d, name)
	}
}

func TestDiff_RemovedCommentLine(t *testing.T) {
	repo := newRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "sql.txt"), []byte("line1\n-- a comment\nline3\nline4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, repo, "add", "sql.txt")
	run(t, repo, "commit", "-m", "add sql")

	dir := filepath.Join(t.TempDir(), "demo")
	if err := Ensure(context.Background(), repo, dir, "feature/demo"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sql.txt"), []byte("line1\nline3\n"), 0o644); err != nil {
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
	if d.Path != "sql.txt" || d.Status != "modified" {
		t.Fatalf("file = %+v, want sql.txt modified", d)
	}
	if len(d.Hunks) != 1 {
		t.Fatalf("hunks = %+v, want 1", d.Hunks)
	}
	want := []Line{
		{Kind: "ctx", Old: 1, New: 1, Text: "line1"},
		{Kind: "del", Old: 2, Text: "-- a comment"},
		{Kind: "ctx", Old: 3, New: 2, Text: "line3"},
		{Kind: "del", Old: 4, Text: "line4"},
	}
	if len(d.Hunks[0].Lines) != len(want) {
		t.Fatalf("lines = %+v, want %+v", d.Hunks[0].Lines, want)
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
