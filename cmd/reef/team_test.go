package main

import (
	"context"
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

func TestTeam_ApplyAndList(t *testing.T) {
	dir := t.TempDir()
	file := writeTeam(t, `{"name":"core","roles":[
		{"name":"architect","backend":"codex"},
		{"name":"lead","lead":true,"backend":"claude","persona":"old"}]}`)
	if got, want := mustReef(t, dir, "team", "apply", file),
		"applied team core: lead*(claude) architect(codex)\n"; got != want {
		t.Errorf("apply = %q, want %q", got, want)
	}

	file = writeTeam(t, `{"name":"core","roles":[
		{"name":"lead","lead":true,"backend":"claude","persona":"new"}]}`)
	mustReef(t, dir, "team", "apply", file)
	if got, want := mustReef(t, dir, "team", "list"),
		"core  lead*(claude)\nsolo  lead*(claude)\n"; got != want {
		t.Errorf("list = %q, want %q", got, want)
	}
}

func TestTeam_ApplyKeepsFace(t *testing.T) {
	dir := t.TempDir()
	mustReef(t, dir, "team", "apply", writeTeam(t, `{"name":"core","roles":[
		{"name":"lead","lead":true,"backend":"claude","face":"otter"},
		{"name":"architect","backend":"codex"}]}`))

	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	teams, err := st.Teams(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var faces []string
	for _, tm := range teams {
		if tm.Name == "core" {
			for _, r := range tm.Roles {
				faces = append(faces, r.Name+"="+r.Face)
			}
		}
	}
	if got, want := strings.Join(faces, " "), "lead=otter architect="; got != want {
		t.Errorf("faces = %q, want %q", got, want)
	}
}

func TestTeam_ApplyErrors(t *testing.T) {
	tests := []struct {
		name string
		give string
		want string
	}{
		{"no lead", `{"name":"a","roles":[{"name":"x","backend":"claude"}]}`,
			"invalid: team a needs exactly one lead role, has 0"},
		{"bad name", `{"name":"a b","roles":[{"name":"x","lead":true,"backend":"claude"}]}`,
			`invalid: team name "a b" must be letters, digits, _ or -`},
		{"two leads", `{"name":"a","roles":[{"name":"x","lead":true,"backend":"claude"},` +
			`{"name":"y","lead":true,"backend":"claude"}]}`,
			"invalid: team a needs exactly one lead role, has 2"},
		{"bad backend", `{"name":"a","roles":[{"name":"x","lead":true,"backend":"gpt"}]}`,
			"role x: invalid: backend must be claude or codex"},
		{"gate on unknown role", `{"name":"a","roles":[{"name":"x","lead":true,"backend":"claude"}],` +
			`"gates":[{"from":"x","to":"y","on":true}]}`,
			"gate x to y names a role team a does not have"},
		{"unknown face", `{"name":"a","roles":[{"name":"x","lead":true,"backend":"claude","face":"dragon"}]}`,
			`role x: face "dragon" must be one of jellyfish, whale, penguin, shark, octopus, turtle, ` +
				"seahorse, otter, clownfish, manta-ray, pebble"},
		{"read-only claude role with Bash", `{"name":"a","roles":[{"name":"x","lead":true,"backend":"claude"},` +
			`{"name":"tester","backend":"claude","tools":["Bash"]}]}`,
			"role tester has no write, so it cannot have unrestricted Bash; " +
				"list command patterns such as Bash(go test:*)"},
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
