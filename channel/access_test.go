package channel

import (
	"context"
	"reflect"
	"testing"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/store"
)

func TestSession_RunAsConfigFollowsWrite(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	if _, err := st.ApplyTeam(ctx, store.Team{Name: "crew", Roles: []store.Role{
		{Name: "lead", Lead: true, Backend: "claude"},
		{
			Name: "tester", Backend: "claude", PermissionMode: "acceptEdits",
			Tools: []string{"Read", "Edit"},
		},
		{Name: "reviewer", Backend: "codex", PermissionMode: "workspace-write"},
		{Name: "developer", Write: true, Backend: "claude", Tools: []string{"Read"}},
	}}); err != nil {
		t.Fatal(err)
	}
	ch, err := st.CreateChannel(ctx, store.Channel{
		ProjectID: f.ProjectID, FeatureID: &f.ID, Name: "chat", Role: "lead", Backend: "claude",
		Team: "crew",
	})
	if err != nil {
		t.Fatal(err)
	}
	ags := map[string]*recordingAgent{}
	var next string
	mk := func(string) (agent.Agent, error) {
		ag := newRecordingAgent()
		ags[next] = ag
		return ag, nil
	}
	s, err := open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, mk)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	tests := []struct {
		role      string
		wantMode  string
		wantTools []string
	}{
		{"tester", "default", []string{"Read", "mcp__reef__*"}},
		{"reviewer", "read-only", []string{"mcp__reef__*"}},
		{"developer", "", []string{"Read", "Task", "mcp__reef__*"}},
	}
	for _, tt := range tests {
		next = tt.role
		if err := s.RunAs(ctx, tt.role, "go", nil, discard); err != nil {
			t.Fatal(err)
		}
		if got := ags[tt.role].cfg; got.PermissionMode != tt.wantMode ||
			!reflect.DeepEqual(got.Tools, tt.wantTools) {
			t.Errorf("%s config = %q %q, want %q %q",
				tt.role, got.PermissionMode, got.Tools, tt.wantMode, tt.wantTools)
		}
	}
}

func TestAccess(t *testing.T) {
	tests := []struct {
		name      string
		give      store.Role
		wantMode  string
		wantTools []string
	}{
		{
			"claude without write drops edit tools and acceptEdits",
			store.Role{
				Backend: "claude", PermissionMode: "acceptEdits",
				Tools: []string{"Read", "Edit", "Bash(go test:*)", "Write(docs/**)", "MultiEdit", "NotebookEdit"},
			},
			"default", []string{"Read", "Bash(go test:*)"},
		},
		{
			"claude with write keeps its mode and gains Task",
			store.Role{
				Backend: "claude", Write: true, PermissionMode: "acceptEdits",
				Tools: []string{"Read", "Edit"},
			},
			"acceptEdits", []string{"Read", "Edit", "Task"},
		},
		{
			"codex without write is read-only",
			store.Role{Backend: "codex", PermissionMode: "workspace-write"},
			"read-only", nil,
		},
		{
			"codex with write defaults to workspace-write",
			store.Role{Backend: "codex", Write: true},
			"workspace-write", nil,
		},
		{
			"codex with write keeps a narrower mode",
			store.Role{Backend: "codex", Write: true, PermissionMode: "read-only"},
			"read-only", nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode, tools := access(tt.give)
			if mode != tt.wantMode || !reflect.DeepEqual(tools, tt.wantTools) {
				t.Errorf("access = %q %q, want %q %q", mode, tools, tt.wantMode, tt.wantTools)
			}
		})
	}
}
