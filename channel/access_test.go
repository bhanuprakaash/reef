package channel

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
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
			[]string{"Edit", "Write", "MultiEdit", "NotebookEdit", "Skill"}},
		{"reviewer", "read-only", []string{"mcp__reef__*"}, nil},
		{"developer", "", []string{"Read", "Task", "mcp__reef__*"}, []string{"Skill"}},
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

func TestSession_RunAsDeniesUnpickedSkills(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, name := range []string{"how", "why"} {
		dir := filepath.Join(home, ".claude", "skills", name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+name+"\n---\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	plugins := filepath.Join(home, ".claude", "plugins")
	if err := os.MkdirAll(plugins, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plugins, "installed_plugins.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	log.SetOutput(io.Discard)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	st, f := openStore(t)
	if _, err := st.ApplyTeam(ctx, store.Team{Name: "crew", Roles: []store.Role{
		{Name: "lead", Lead: true, Backend: "claude"},
		{Name: "tester", Backend: "claude", Tools: []string{"Read"}, Skills: []string{"how"}},
		{Name: "reviewer", Backend: "codex", Skills: []string{"how"}},
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

	for role, want := range map[string]struct{ tools, deny []string }{
		"tester": {
			[]string{"Read", "Skill(how)", "mcp__reef__*"},
			[]string{"Edit", "Write", "MultiEdit", "NotebookEdit", "Skill(why)"},
		},
		"lead": {
			[]string{"mcp__reef__*"},
			[]string{"Edit", "Write", "MultiEdit", "NotebookEdit", "Skill"},
		},
		"reviewer": {[]string{"mcp__reef__*"}, nil},
	} {
		next = role
		if err := s.RunAs(ctx, role, "go", nil, discard); err != nil {
			t.Fatalf("%s: %v", role, err)
		}
		if got := ags[role].cfg; !reflect.DeepEqual(got.Tools, want.tools) ||
			!reflect.DeepEqual(got.DisallowedTools, want.deny) {
			t.Errorf("%s tools = %q, denied = %q, want %q and %q",
				role, got.Tools, got.DisallowedTools, want.tools, want.deny)
		}
	}
}

func TestAccess(t *testing.T) {
	tests := []struct {
		name      string
		give      store.Role
		onDisk    []string
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
			nil,
			"default", []string{"Read", "Bash(go test:*)"},
			[]string{"Edit", "Write", "MultiEdit", "NotebookEdit", "Skill"},
		},
		{
			"claude without write drops unrestricted Bash, even from a team saved before the check",
			store.Role{
				Backend: "claude",
				Tools:   []string{"Read", "Bash", "Bash(*)", "Bash(:*)", "Bash(go test:*)"},
			},
			nil,
			"default", []string{"Read", "Bash(go test:*)"},
			[]string{"Edit", "Write", "MultiEdit", "NotebookEdit", "Skill"},
		},
		{
			"claude with write keeps its mode and gains Task",
			store.Role{
				Backend: "claude", Write: true, PermissionMode: "acceptEdits",
				Tools: []string{"Read", "Edit"},
			},
			nil,
			"acceptEdits", []string{"Read", "Edit", "Task"}, []string{"Skill"},
		},
		{
			"claude without write allows its picked skills and denies the other skills on disk",
			store.Role{Backend: "claude", Tools: []string{"Read"}, Skills: []string{"how"}},
			[]string{"how", "why", "slack:standup"},
			"default", []string{"Read", "Skill(how)"},
			[]string{"Edit", "Write", "MultiEdit", "NotebookEdit", "Skill(why)", "Skill(slack:standup)"},
		},
		{
			"claude with write allows its picked skills after Task",
			store.Role{Backend: "claude", Write: true, Tools: []string{"Read"}, Skills: []string{"how", "why"}},
			[]string{"how", "why", "teach"},
			"", []string{"Read", "Task", "Skill(how)", "Skill(why)"}, []string{"Skill(teach)"},
		},
		{
			"claude denies no skill by name when every skill on disk is picked",
			store.Role{Backend: "claude", Write: true, Skills: []string{"how"}},
			[]string{"how"},
			"", []string{"Task", "Skill(how)"}, nil,
		},
		{
			"claude with no picked skills denies the Skill tool",
			store.Role{Backend: "claude", Write: true},
			[]string{"how", "why"},
			"", []string{"Task"}, []string{"Skill"},
		},
		{
			"codex gets no skill rules",
			store.Role{Backend: "codex", Write: true, Skills: []string{"how"}},
			[]string{"how", "why"},
			"workspace-write", nil, nil,
		},
		{
			"codex without write is read-only",
			store.Role{Backend: "codex", PermissionMode: "workspace-write"},
			nil,
			"read-only", nil, nil,
		},
		{
			"codex with write defaults to workspace-write",
			store.Role{Backend: "codex", Write: true},
			nil,
			"workspace-write", nil, nil,
		},
		{
			"codex with write keeps a narrower mode",
			store.Role{Backend: "codex", Write: true, PermissionMode: "read-only"},
			nil,
			"read-only", nil, nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode, tools, deny := access(tt.give, tt.onDisk)
			if mode != tt.wantMode || !reflect.DeepEqual(tools, tt.wantTools) ||
				!reflect.DeepEqual(deny, tt.wantDeny) {
				t.Errorf("access = %q %q %q, want %q %q %q",
					mode, tools, deny, tt.wantMode, tt.wantTools, tt.wantDeny)
			}
		})
	}
}
