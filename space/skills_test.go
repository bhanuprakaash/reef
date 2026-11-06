package space

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/skill"
	"github.com/bhanuprakaash/reef/store"
)

func writeSkillFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readSkillFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSession_AdoptedSkillsReachBackendsAndStayUntilRefresh(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	t.Setenv("HOME", home)
	probe := filepath.Join(home, ".claude/skills/probe")
	writeSkillFile(t, filepath.Join(probe, "SKILL.md"), "---\nname: probe\ndescription: Probe things.\n---\nSee notes/how.md.\n")
	writeSkillFile(t, filepath.Join(probe, "notes/how.md"), "disk notes")
	writeSkillFile(t, filepath.Join(home, ".claude/skills/plugged/SKILL.md"),
		"---\nname: plugged\ndescription: Needs claude.\n---\nRun ${CLAUDE_PLUGIN_ROOT}/run.sh\n")
	source := filepath.Join(t.TempDir(), "deploy")
	writeSkillFile(t, filepath.Join(source, "SKILL.md"), "---\nname: deploy\ndescription: Ship it.\n---\nRun scripts/ship.sh\n")
	writeSkillFile(t, filepath.Join(source, "scripts/ship.sh"), "echo v1")

	st, f := openStore(t)
	pkg, err := skill.ReadFolder(source)
	if err != nil {
		t.Fatal(err)
	}
	lib, err := st.ImportSkill(ctx, "folder", source, "", "", pkg)
	if err != nil {
		t.Fatal(err)
	}
	ch := sessionSpace(t, st, f, false)
	architect := testBotID(t, st, "architect")
	editBot(t, st, "architect", func(b *store.Bot) {
		b.Backend, b.PermissionMode = "codex", ""
		b.Skills = []string{"claude:probe", "claude:plugged", "reef:deploy", "claude:gone"}
	})
	dataDir := t.TempDir()
	ag := newRecordingAgent()
	s, err := Open(ctx, st, testMemory(t, st), dataDir, "/bin/reef", ch.ID, NewTrees(t.TempDir()), ag.factory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := sendAs(ctx, st, ch.ID, s.Run, "go", discard); err != nil {
		t.Fatal(err)
	}

	bs, err := st.ActiveBotSession(ctx, ch.ID, *architect)
	if err != nil {
		t.Fatal(err)
	}
	root := BotSkillsDir(dataDir, bs.ID)
	if got := readSkillFile(t, filepath.Join(root, "claude/probe/notes/how.md")); got != "disk notes" {
		t.Errorf("copied disk skill file = %q, want disk notes", got)
	}
	if got := readSkillFile(t, filepath.Join(root, "reef/deploy/scripts/ship.sh")); got != "echo v1" {
		t.Errorf("copied library file = %q, want echo v1", got)
	}
	if left, _ := filepath.Glob(filepath.Join(dataDir, "bot-sessions", "stage-*")); len(left) != 0 {
		t.Errorf("staging folders left = %v, want none", left)
	}
	var plugged, gone store.AdoptedSkill
	for _, k := range bs.Skills {
		switch k.Ref {
		case "claude:plugged":
			plugged = k
		case "claude:gone":
			gone = k
		}
	}
	if plugged.Compatible || plugged.Dir != "" || !strings.Contains(plugged.Reason, "CLAUDE_PLUGIN_ROOT") {
		t.Errorf("claude-only skill on codex = %+v, want incompatible with its reason and no copy", plugged)
	}
	if gone.Compatible || gone.Reason != `"gone" was not found on disk` {
		t.Errorf("missing skill = %+v, want incompatible: not found", gone)
	}
	want := "## Your skills\nRead a skill's SKILL.md when the skill applies; its supporting files are relative to it.\n" +
		"- probe: Probe things. (" + filepath.Join(root, "claude/probe/SKILL.md") + ")\n" +
		"- deploy: Ship it. (" + filepath.Join(root, "reef/deploy/SKILL.md") + ")"
	if got := ag.cfg.Instructions; !strings.HasSuffix(got, want) {
		t.Errorf("instructions = %q, want them to end with %q", got, want)
	}
	if got := readSkillFile(t, filepath.Join(probe, "notes/how.md")); got != "disk notes" {
		t.Errorf("disk original = %q, want it unchanged", got)
	}

	writeSkillFile(t, filepath.Join(source, "scripts/ship.sh"), "echo v2")
	pkg, err = skill.ReadFolder(source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpdateSkill(ctx, lib.ID, pkg); err != nil {
		t.Fatal(err)
	}
	if err := sendAs(ctx, st, ch.ID, s.Run, "again", discard); err != nil {
		t.Fatal(err)
	}
	if got := readSkillFile(t, filepath.Join(root, "reef/deploy/scripts/ship.sh")); got != "echo v1" {
		t.Errorf("existing session copy after a library update = %q, want echo v1", got)
	}

	if err := s.Refresh(ctx, *architect); err != nil {
		t.Fatal(err)
	}
	fresh, err := st.ActiveBotSession(ctx, ch.ID, *architect)
	if err != nil {
		t.Fatal(err)
	}
	if got := readSkillFile(t, filepath.Join(BotSkillsDir(dataDir, fresh.ID), "reef/deploy/scripts/ship.sh")); got != "echo v2" {
		t.Errorf("refreshed session copy = %q, want echo v2", got)
	}
	if got := readSkillFile(t, filepath.Join(root, "reef/deploy/scripts/ship.sh")); got != "echo v1" {
		t.Errorf("replaced session copy = %q, want echo v1", got)
	}
}

func TestSession_BotWithSkillsRunsInSpaceWithoutProject(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeSkillFile(t, filepath.Join(home, ".claude/skills/probe/SKILL.md"), "---\nname: probe\ndescription: Probe things.\n---\n")
	st, _ := openStore(t)
	ch, err := st.CreateBotSpace(ctx, store.Space{Name: "free chat", Role: "architect"}, *testBotID(t, st, "architect"))
	if err != nil {
		t.Fatal(err)
	}
	editBot(t, st, "architect", func(b *store.Bot) { b.Skills = []string{"claude:probe"} })
	ag := newRecordingAgent()
	s, err := Open(ctx, st, testMemory(t, st), t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), ag.factory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := sendAs(ctx, st, ch.ID, s.Run, "go", discard); err != nil {
		t.Fatal(err)
	}
	if got := ag.cfg.Instructions; !strings.Contains(got, "- probe: Probe things. (") {
		t.Errorf("instructions = %q, want them to list probe", got)
	}
}

func TestSession_ClaudeGetsSkillsFolder(t *testing.T) {
	ctx := context.Background()
	t.Setenv("HOME", t.TempDir())
	source := filepath.Join(t.TempDir(), "deploy")
	writeSkillFile(t, filepath.Join(source, "SKILL.md"), "---\nname: deploy\ndescription: Ship it.\n---\n")
	st, f := openStore(t)
	pkg, err := skill.ReadFolder(source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.ImportSkill(ctx, "folder", source, "", "", pkg); err != nil {
		t.Fatal(err)
	}
	ch := sessionSpace(t, st, f, false)
	editBot(t, st, "architect", func(b *store.Bot) { b.Skills = []string{"reef:deploy"} })
	dataDir := t.TempDir()
	ag := newRecordingAgent()
	s, err := Open(ctx, st, testMemory(t, st), dataDir, "/bin/reef", ch.ID, NewTrees(t.TempDir()), ag.factory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := sendAs(ctx, st, ch.ID, s.Run, "go", discard); err != nil {
		t.Fatal(err)
	}
	bs, err := st.ActiveBotSession(ctx, ch.ID, *testBotID(t, st, "architect"))
	if err != nil {
		t.Fatal(err)
	}
	if got := ag.cfg.AddDirs[0]; got != BotSkillsDir(dataDir, bs.ID) {
		t.Errorf("AddDirs[0] = %q, want %q", got, BotSkillsDir(dataDir, bs.ID))
	}
}

func TestAdoptBotSession_CopiesExecuteBitAndFailsWholly(t *testing.T) {
	ctx := context.Background()
	source := filepath.Join(t.TempDir(), "deploy")
	writeSkillFile(t, filepath.Join(source, "SKILL.md"), "---\nname: deploy\ndescription: Ship it.\n---\n")
	writeSkillFile(t, filepath.Join(source, "scripts/ship.sh"), "echo ship")
	if err := os.Chmod(filepath.Join(source, "scripts/ship.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	st, f := openStore(t)
	pkg, err := skill.ReadFolder(source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.ImportSkill(ctx, "folder", source, "", "", pkg); err != nil {
		t.Fatal(err)
	}
	ch := sessionSpace(t, st, f, false)
	editBot(t, st, "architect", func(b *store.Bot) { b.Skills = []string{"reef:deploy"} })
	architect := *testBotID(t, st, "architect")

	dataDir := t.TempDir()
	bs, err := AdoptBotSession(ctx, st, dataDir, ch.ID, architect)
	if err != nil {
		t.Fatal(err)
	}
	for file, want := range map[string]os.FileMode{"scripts/ship.sh": 0o700, "SKILL.md": 0o600} {
		info, err := os.Stat(filepath.Join(BotSkillsDir(dataDir, bs.ID), "reef/deploy", file))
		if err != nil || info.Mode().Perm() != want {
			t.Errorf("copied %s = %v, %v, want mode %o", file, info, err, want)
		}
	}

	blocked := t.TempDir()
	writeSkillFile(t, filepath.Join(blocked, "bot-sessions"), "a file where the folder goes")
	if _, err := AdoptBotSession(ctx, st, blocked, ch.ID, architect); err == nil {
		t.Fatal("adopt into an unwritable data folder succeeded, want an error")
	}
	active, err := st.ActiveBotSession(ctx, ch.ID, architect)
	if err != nil || active.ID != bs.ID {
		t.Errorf("active session after a failed adopt = %+v, %v, want session %d", active, err, bs.ID)
	}
}
