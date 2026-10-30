package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

func writeTeam(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "team.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func newBots(t *testing.T, dir string, bots ...store.Bot) []int64 {
	t.Helper()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	var ids []int64
	for _, b := range bots {
		created, err := st.CreateBot(context.Background(), b)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, created.ID)
	}
	return ids
}

func TestTeam_ApplyAndList(t *testing.T) {
	dir := t.TempDir()
	ids := newBots(t, dir,
		store.Bot{Name: "architect", Backend: "codex"},
		store.Bot{Name: "boss", Backend: "claude", Description: "Plans."})
	file := writeTeam(t, fmt.Sprintf(`{"name":"core","roles":[
		{"botId":%d},{"botId":%d,"lead":true}]}`, ids[0], ids[1]))
	if got, want := mustReef(t, dir, "team", "apply", file),
		"applied team core: boss*(claude) architect(codex)\n"; got != want {
		t.Errorf("apply = %q, want %q", got, want)
	}

	file = writeTeam(t, fmt.Sprintf(`{"name":"core","roles":[{"botId":%d,"lead":true}]}`, ids[1]))
	mustReef(t, dir, "team", "apply", file)
	if got, want := mustReef(t, dir, "team", "list"),
		"core  boss*(claude)\nsolo  lead*(claude)\n"; got != want {
		t.Errorf("list = %q, want %q", got, want)
	}
}

func TestTeam_ApplyErrors(t *testing.T) {
	tests := []struct {
		name string
		give string
		want string
	}{
		{"no lead", `{"name":"a","roles":[{"botId":1}]}`,
			"invalid: team a needs exactly one lead role, has 0"},
		{"bad name", `{"name":"a b","roles":[{"botId":1,"lead":true}]}`,
			`invalid: team name "a b" must be letters, digits, _ or -`},
		{"two leads", `{"name":"a","roles":[{"botId":1,"lead":true},{"botId":2,"lead":true}]}`,
			"invalid: team a needs exactly one lead role, has 2"},
		{"unknown bot", `{"name":"a","roles":[{"botId":99,"lead":true}]}`,
			"invalid: unknown bot 99"},
		{"no bot id", `{"name":"a","roles":[{"lead":true}]}`,
			"invalid: every member needs a botId"},
		{"member config", `{"name":"a","roles":[{"botId":1,"lead":true,"backend":"claude"}]}`,
			`unknown field "backend"`},
		{"role by name", `{"name":"a","roles":[{"name":"x","lead":true}]}`,
			`unknown field "name"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := reef(t, t.TempDir(), "team", "apply", writeTeam(t, tt.give))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want %q", err, tt.want)
			}
		})
	}
}
