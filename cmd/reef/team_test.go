package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestTeam_ApplyErrors(t *testing.T) {
	tests := []struct {
		name string
		give string
		want string
	}{
		{"no lead", `{"name":"a","roles":[{"name":"x","backend":"claude"}]}`,
			"invalid: team a needs exactly one lead role, has 0"},
		{"two leads", `{"name":"a","roles":[{"name":"x","lead":true,"backend":"claude"},` +
			`{"name":"y","lead":true,"backend":"claude"}]}`,
			"invalid: team a needs exactly one lead role, has 2"},
		{"bad backend", `{"name":"a","roles":[{"name":"x","lead":true,"backend":"gpt"}]}`,
			"role x: invalid: backend must be claude or codex"},
		{"gate on unknown role", `{"name":"a","roles":[{"name":"x","lead":true,"backend":"claude"}],` +
			`"gates":[{"from":"x","to":"y","on":true}]}`,
			"gate x to y names a role team a does not have"},
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
