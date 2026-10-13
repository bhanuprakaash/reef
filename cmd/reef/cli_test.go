package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

func reef(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := run(context.Background(), append(args, "--data", dir), strings.NewReader(""), &out, &syncBuffer{})
	return out.String(), err
}

func mustReef(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := reef(t, dir, args...)
	if err != nil {
		t.Fatalf("reef %s: %v", strings.Join(args, " "), err)
	}
	return out
}

func TestCLI_Flow(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(t.TempDir())
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cwd, err = store.ResolvePath(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := mustReef(t, dir, "project", "add", "--name", "shop"),
		"1  shop  "+cwd+"\n"; got != want {
		t.Fatalf("project add = %q, want %q", got, want)
	}

	steps := []struct {
		name string
		give []string
		want string
	}{
		{"feature add", []string{"feature", "add", "login", "Login", "--goal", "sign in"},
			"1  login  Login\n"},
		{"decide with flag after args", []string{"decide", "login", "use sessions", "--why", "simple"},
			"1  use sessions\n"},
		{"task add on feature", []string{"task", "add", "login", "write tests"},
			"1  todo  write tests\n"},
		{"task add on project", []string{"task", "add", "buy milk"}, "2  todo  buy milk\n"},
		{"task list feature", []string{"task", "list", "login"}, "1  todo  write tests\n"},
		{"task list project", []string{"task", "list"},
			"1  todo  write tests\n2  todo  buy milk\n"},
		{"task list status", []string{"task", "list", "--status", "done"}, ""},
	}
	for _, tt := range steps {
		if got := mustReef(t, dir, tt.give...); got != tt.want {
			t.Errorf("%s = %q, want %q", tt.name, got, tt.want)
		}
	}

	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	ds, err := st.Decisions(context.Background(), 1)
	if err != nil || len(ds) != 1 || ds[0].Author != "user" || ds[0].Why != "simple" {
		t.Errorf("decisions = %+v, %v, want one by user with why %q", ds, err, "simple")
	}

	if _, err := reef(t, dir, "brief", "login"); err == nil ||
		err.Error() != "feature login has no brief yet" {
		t.Errorf("brief error = %v, want %q", err, "feature login has no brief yet")
	}
	if _, err := st.PutBrief(context.Background(),
		store.Brief{FeatureID: 1, Body: "sign in with sessions", Author: "user"}); err != nil {
		t.Fatal(err)
	}
	if got := mustReef(t, dir, "brief", "login"); got != "sign in with sessions\n" {
		t.Errorf("brief = %q, want %q", got, "sign in with sessions\n")
	}
}

func TestCLI_Errors(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(t.TempDir())

	if _, err := reef(t, dir, "feature", "add", "login", "Login"); err == nil ||
		!strings.HasPrefix(err.Error(), "no reef project for ") ||
		!strings.HasSuffix(err.Error(), "; run reef project add") {
		t.Errorf("feature add without project: error = %v", err)
	}
	mustReef(t, dir, "project", "add")
	if _, err := reef(t, dir, "project", "add"); err == nil ||
		!strings.HasSuffix(err.Error(), " is already project 1") {
		t.Errorf("project add twice: error = %v, want already project 1", err)
	}

	tests := []struct {
		name string
		give []string
		want string
	}{
		{"bad slug", []string{"feature", "add", "Bad_Slug", "x"},
			`invalid: feature slug "Bad_Slug" must match ^[a-z0-9][a-z0-9-]*$`},
		{"digit slug", []string{"feature", "add", "2024", "x"},
			`invalid: feature slug "2024" must contain a letter or -`},
		{"unknown feature", []string{"decide", "nope", "x"}, `feature "nope": not found`},
		{"bad status", []string{"task", "list", "--status", "bogus"},
			`invalid: unknown status "bogus"`},
		{"missing args", []string{"decide", "login"},
			`usage: reef decide <feature> "<body>" [--why W] [--data DIR]`},
		{"unknown command", []string{"nope"}, _usage},
	}
	for _, tt := range tests {
		_, err := reef(t, dir, tt.give...)
		if err == nil || err.Error() != tt.want {
			t.Errorf("%s: error = %v, want %q", tt.name, err, tt.want)
		}
	}
}

func TestCLI_SlugInTwoProjects(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a", "b"} {
		t.Chdir(t.TempDir())
		mustReef(t, dir, "project", "add", "--name", name)
		mustReef(t, dir, "feature", "add", "login", "Login")
	}

	want := `invalid: feature slug "login" is in projects 1, 2; use the feature id`
	if _, err := reef(t, dir, "brief", "login"); err == nil || err.Error() != want {
		t.Errorf("brief login: error = %v, want %q", err, want)
	}
	if _, err := reef(t, dir, "brief", "2"); err == nil ||
		err.Error() != "feature 2 has no brief yet" {
		t.Errorf("brief 2: error = %v, want no brief yet", err)
	}
}

func TestCLI_SymlinkedCWDFindsProject(t *testing.T) {
	dir := t.TempDir()
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "project")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	target, err := store.ResolvePath(target)
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(link)
	if got, want := mustReef(t, dir, "project", "add", "--name", "shop"),
		"1  shop  "+target+"\n"; got != want {
		t.Fatalf("project add from symlinked folder = %q, want %q", got, want)
	}
	if got := mustReef(t, dir, "feature", "add", "login", "Login"); got != "1  login  Login\n" {
		t.Fatalf("feature add from symlinked folder = %q, want project feature", got)
	}
}

func TestCLI_DoubleDashEndsFlags(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(t.TempDir())
	mustReef(t, dir, "project", "add")
	mustReef(t, dir, "feature", "add", "login", "Login")

	var out bytes.Buffer
	err := run(context.Background(),
		[]string{"task", "add", "login", "--data", dir, "--", "--why not"},
		strings.NewReader(""), &out, &syncBuffer{})
	if err != nil {
		t.Fatal(err)
	}
	if want := "1  todo  --why not\n"; out.String() != want {
		t.Errorf("task add = %q, want %q", out.String(), want)
	}
}
