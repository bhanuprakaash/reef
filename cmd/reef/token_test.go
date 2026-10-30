package main

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

func TestCLI_Token(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	plain := t.TempDir()
	spaced := filepath.Join(t.TempDir(), "Application Support", "Reef")
	tests := []struct {
		name     string
		give     string
		wantData string
	}{
		{"plain path", plain, plain},
		{"path with a space", spaced, "'" + spaced + "'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			mustReef(t, tt.give, "project", "add", "--name", "shop")
			mustReef(t, tt.give, "work-item", "add", "Login")

			out := mustReef(t, tt.give, "token", "1", "--role", "lead")

			m := regexp.MustCompile(`REEF_TOKEN=([0-9a-f]{64}) `).FindStringSubmatch(out)
			if m == nil {
				t.Fatalf("output %q has no REEF_TOKEN of 64 hex digits", out)
			}
			want := "claude mcp add reef --env REEF_TOKEN=" + m[1] + " -- " + exe +
				" mcp --data " + tt.wantData + "\n"
			if out != want {
				t.Errorf("output = %q, want %q", out, want)
			}

			st, err := store.Open(tt.give)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = st.Close() }()
			tok, err := st.Token(context.Background(), m[1])
			if err != nil || tok.Role != "lead" || tok.SpaceID != 1 {
				t.Errorf("token = %+v, %v, want role lead on Space 1", tok, err)
			}
			ch, err := st.Space(context.Background(), 1)
			if err != nil || ch.Name != "lead (claude code)" || ch.Role != "lead" ||
				ch.Backend != "claude" {
				t.Errorf("space = %+v, %v, want lead role and claude backend", ch, err)
			}
		})
	}
}

func TestCLI_TokenAttachesToTheActiveSpace(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	t.Chdir(t.TempDir())
	mustReef(t, dir, "project", "add")
	mustReef(t, dir, "work-item", "add", "Login")
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	parent := int64(1)
	if _, err := st.CreateWorkItem(ctx, store.WorkItem{ProjectID: 1, ParentID: &parent, Title: "Form"}); err != nil {
		t.Fatal(err)
	}

	mustReef(t, dir, "token", "1", "--role", "lead")
	for _, name := range []string{"developer", "tester"} {
		id := testCLIBot(t, st, name)
		if _, err := st.AddSpaceMember(ctx, 1, *id); err != nil {
			t.Fatal(err)
		}
	}
	mustReef(t, dir, "token", "1", "--role", "developer")
	out := mustReef(t, dir, "token", "1", "--role", "tester")

	m := regexp.MustCompile(`REEF_TOKEN=([0-9a-f]{64}) `).FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("output %q has no REEF_TOKEN of 64 hex digits", out)
	}
	tok, err := st.Token(ctx, m[1])
	if err != nil || tok.Role != "tester" || tok.SpaceID != 1 {
		t.Errorf("token = %+v, %v, want role tester on Space 1", tok, err)
	}
	if chs, err := st.Spaces(ctx, store.SpaceFilter{}); err != nil || len(chs) != 1 {
		t.Errorf("spaces = %+v, %v, want one", chs, err)
	}
	_, err = reef(t, dir, "token", "2", "--role", "tester")
	if err == nil || err.Error() != "invalid: work item 2 is a sub-item of work item 1: open its parent" {
		t.Errorf("token of a sub-item error = %v, want it refused", err)
	}
}
