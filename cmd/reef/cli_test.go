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
		{"work-item add", []string{"work-item", "add", "Login", "--description", "sign in"},
			"1  feature  Login\n"},
		{"work-item add typed", []string{"work-item", "add", "Crash on start", "--type", "bug"},
			"2  bug  Crash on start\n"},
		{"decide with flag after args", []string{"decide", "1", "use sessions", "--why", "simple"},
			"1  use sessions\n"},
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

	if _, err := reef(t, dir, "brief", "1"); err == nil ||
		err.Error() != "work item 1 has no brief yet" {
		t.Errorf("brief error = %v, want %q", err, "work item 1 has no brief yet")
	}
	if _, err := st.PutBrief(context.Background(),
		store.Brief{WorkItemID: 1, Body: "sign in with sessions", Author: "user"}); err != nil {
		t.Fatal(err)
	}
	if got := mustReef(t, dir, "brief", "1"); got != "sign in with sessions\n" {
		t.Errorf("brief = %q, want %q", got, "sign in with sessions\n")
	}
}

func TestCLI_Context_ReadsMemoryItems(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(t.TempDir())
	mustReef(t, dir, "project", "add", "--name", "shop")
	mustReef(t, dir, "work-item", "add", "Login")
	mustReef(t, dir, "decide", "1", "use sessions")
	mustReef(t, dir, "memory", "add", "fact", "Go 1.24", "--scope", "project:1")
	mustReef(t, dir, "memory", "add", "lesson", "keep it small", "--scope", "bot:developer")
	mustReef(t, dir, "memory", "add", "lesson", "tester only", "--scope", "bot:tester")

	got := mustReef(t, dir, "context", "1", "--role", "developer")
	for _, want := range []string{
		"### Decisions\ndecision 1: use sessions (user, ",
		"### Project\nfact 2: Go 1.24 (user, ",
		"1 of 1; recall(..., scope=project)\n",
		"### Your items\nlesson 3: keep it small (user, ",
		"1 of 1; recall(..., scope=bot)\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("reef context =\n%s\nwant it to contain %q", got, want)
		}
	}
	for _, gone := range []string{"tester only", "### Findings", "### Team"} {
		if strings.Contains(got, gone) {
			t.Errorf("reef context contains %q:\n%s", gone, got)
		}
	}
}

func TestCLI_DecideWritesAnAcceptedWorkItemDecisionByTheUser(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(t.TempDir())
	mustReef(t, dir, "project", "add", "--name", "shop")
	mustReef(t, dir, "work-item", "add", "Login")

	if got, want := mustReef(t, dir, "decide", "1", "use sessions", "--why", "simple"),
		"1  use sessions\n"; got != want {
		t.Errorf("decide = %q, want %q", got, want)
	}
	if got, want := mustReef(t, dir, "memory", "show", "1"),
		"1  accepted  decision  work_item:1  use sessions\nwhy  simple\nauthor  user  user\n"; got != want {
		t.Errorf("memory show 1 = %q, want %q", got, want)
	}
	if got, want := mustReef(t, dir, "decide", "1", "use sessions."),
		"2  use sessions.\n1  similar  use sessions\n"; got != want {
		t.Errorf("decide of a lookalike = %q, want %q", got, want)
	}
}

func TestCLI_Errors(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(t.TempDir())

	if _, err := reef(t, dir, "work-item", "add", "Login"); err == nil ||
		!strings.HasPrefix(err.Error(), "no reef project for ") ||
		!strings.HasSuffix(err.Error(), "; run reef project add") {
		t.Errorf("work-item add without project: error = %v", err)
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
		{"bad type", []string{"work-item", "add", "x", "--type", "epic"},
			`invalid: unknown type "epic"`},
		{"not an id", []string{"decide", "nope", "x"}, `work item "nope" is not an id`},
		{"unknown work item", []string{"decide", "9", "x"}, `work item 9: not found`},
		{"missing args", []string{"decide", "1"},
			`usage: reef decide <work-item> "<body>" [--why W] [--data DIR]`},
		{"unknown command", []string{"nope"}, _usage},
	}
	for _, tt := range tests {
		_, err := reef(t, dir, tt.give...)
		if err == nil || err.Error() != tt.want {
			t.Errorf("%s: error = %v, want %q", tt.name, err, tt.want)
		}
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
	if got := mustReef(t, dir, "work-item", "add", "Login"); got != "1  feature  Login\n" {
		t.Fatalf("work-item add from symlinked folder = %q, want project work item", got)
	}
}

func TestCLI_ProjectStoredWithSymlinkedPathIsFound(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	link := filepath.Join(t.TempDir(), "project")
	if err := os.Symlink(t.TempDir(), link); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateProject(ctx, store.Project{Name: "shop", Path: link}); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	t.Chdir(link)
	if got := mustReef(t, dir, "work-item", "add", "Login"); got != "1  feature  Login\n" {
		t.Errorf("work-item add = %q, want the work item of project shop", got)
	}
	if err := os.Mkdir(filepath.Join(link, "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Join(link, "api"))
	if got := mustReef(t, dir, "work-item", "add", "Billing"); got != "2  feature  Billing\n" {
		t.Errorf("work-item add from a subfolder = %q, want the work item of project shop", got)
	}
}

func TestCLI_DoubleDashEndsFlags(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(t.TempDir())
	mustReef(t, dir, "project", "add")
	mustReef(t, dir, "work-item", "add", "Login")

	var out bytes.Buffer
	err := run(context.Background(),
		[]string{"decide", "1", "--data", dir, "--", "--why not"},
		strings.NewReader(""), &out, &syncBuffer{})
	if err != nil {
		t.Fatal(err)
	}
	if want := "1  --why not\n"; out.String() != want {
		t.Errorf("decide = %q, want %q", out.String(), want)
	}
}
