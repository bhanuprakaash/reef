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
			mustReef(t, tt.give, "feature", "add", "login", "Login")

			out := mustReef(t, tt.give, "token", "login", "--role", "architect")

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
			if err != nil || tok.Role != "architect" || tok.ChannelID != 1 {
				t.Errorf("token = %+v, %v, want role architect on channel 1", tok, err)
			}
			ch, err := st.Channel(context.Background(), 1)
			if err != nil || ch.Name != "architect (claude code)" {
				t.Errorf("channel = %+v, %v, want name %q", ch, err, "architect (claude code)")
			}
		})
	}
}
