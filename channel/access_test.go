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
	s, err := open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), mk)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	tests := []struct {
		role      string
		wantMode  string
		wantTools []string
		wantDeny  []string
	}{
		{"tester", "default", []string{"Read", "mcp__reef__*"},
			[]string{"Edit", "Write", "MultiEdit", "NotebookEdit"}},
		{"reviewer", "read-only", []string{"mcp__reef__*"}, nil},
		{"developer", "", []string{"Read", "Task", "mcp__reef__*"}, nil},
	}
	for _, tt := range tests {
		next = tt.role
		if err := s.RunAs(ctx, tt.role, "go", nil, discard); err != nil {
			t.Fatal(err)
		}
		if got := ags[tt.role].cfg; got.PermissionMode != tt.wantMode ||
			!reflect.DeepEqual(got.Tools, tt.wantTools) ||
			!reflect.DeepEqual(got.DisallowedTools, tt.wantDeny) {
			t.Errorf("%s config = %q %q %q, want %q %q %q", tt.role,
				got.PermissionMode, got.Tools, got.DisallowedTools,
				tt.wantMode, tt.wantTools, tt.wantDeny)
		}
	}
}

func TestAccess(t *testing.T) {
	tests := []struct {
		name      string
		give      store.Role
		wantMode  string
		wantTools []string
		wantDeny  []string
	}{
		{
			"claude without write drops edit tools and acceptEdits",
			store.Role{
				Backend: "claude", PermissionMode: "acceptEdits",
				Tools: []string{"Read", "Edit", "Bash(go test:*)", "Write(docs/**)", "MultiEdit", "NotebookEdit"},
			},
			"default", []string{"Read", "Bash(go test:*)"},
			[]string{"Edit", "Write", "MultiEdit", "NotebookEdit"},
		},
		{
			"claude with write keeps its mode and gains Task",
			store.Role{
				Backend: "claude", Write: true, PermissionMode: "acceptEdits",
				Tools: []string{"Read", "Edit"},
			},
			"acceptEdits", []string{"Read", "Edit", "Task"}, nil,
		},
		{
			"codex without write is read-only",
			store.Role{Backend: "codex", PermissionMode: "workspace-write"},
			"read-only", nil, nil,
		},
		{
			"codex with write defaults to workspace-write",
			store.Role{Backend: "codex", Write: true},
			"workspace-write", nil, nil,
		},
		{
			"codex with write keeps a narrower mode",
			store.Role{Backend: "codex", Write: true, PermissionMode: "read-only"},
			"read-only", nil, nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode, tools, deny := access(tt.give)
			if mode != tt.wantMode || !reflect.DeepEqual(tools, tt.wantTools) ||
				!reflect.DeepEqual(deny, tt.wantDeny) {
				t.Errorf("access = %q %q %q, want %q %q %q",
					mode, tools, deny, tt.wantMode, tt.wantTools, tt.wantDeny)
			}
		})
	}
}
