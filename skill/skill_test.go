package skill

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func skillFile(name, description string) string {
	return "---\nname: " + name + "\ndescription: " + description + "\n---\n# " + name + "\n"
}

func TestClaude(t *testing.T) {
	home, dir := t.TempDir(), t.TempDir()
	write(t, filepath.Join(home, ".claude/skills/how/SKILL.md"), skillFile("how", "Explain flow."))
	write(t, filepath.Join(home, ".claude/skills/why/SKILL.md"), skillFile("why", `"Say \"why\" and cite."`))
	write(t, filepath.Join(home, ".claude/skills/nofront/SKILL.md"), "# no frontmatter\n")
	write(t, filepath.Join(home, ".claude/skills/folded/SKILL.md"),
		"---\nname: folded\ndescription: >\n  First line\n  second line.\nother: x\n---\nbody\n")
	write(t, filepath.Join(home, ".claude/skills/empty-dir/readme.md"), "not a skill")
	write(t, filepath.Join(home, ".claude/skills/.DS_Store"), "stray file")
	write(t, filepath.Join(dir, ".claude/skills/repo-only/SKILL.md"), skillFile("repo-only", "From the repo."))
	write(t, filepath.Join(dir, ".claude/skills/how/SKILL.md"), skillFile("how", "Repo copy."))
	write(t, filepath.Join(home, "elsewhere/linked/SKILL.md"), skillFile("linked", "Behind a symlink."))
	if err := os.Symlink(filepath.Join(home, "elsewhere/linked"), filepath.Join(home, ".claude/skills/linked")); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(home, ".agents/skills/codex-only/SKILL.md"), skillFile("codex-only", "Not for claude."))

	got := Claude(home, dir)
	want := []Skill{
		{"folded", "First line second line."},
		{"how", "Explain flow."},
		{"linked", "Behind a symlink."},
		{"nofront", ""},
		{"repo-only", "From the repo."},
		{"why", `Say "why" and cite.`},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Claude = %+v, want %+v", got, want)
	}
}

func TestClaude_Plugins(t *testing.T) {
	home, dir := t.TempDir(), t.TempDir()
	user := filepath.Join(home, ".claude/plugins/cache/m/slack/1.0")
	local := filepath.Join(home, ".claude/plugins/cache/m/mine/2.0")
	other := filepath.Join(home, ".claude/plugins/cache/m/theirs/3.0")
	write(t, filepath.Join(user, "skills/standup/SKILL.md"), skillFile("standup", "Daily."))
	write(t, filepath.Join(local, "skills/tidy/SKILL.md"), skillFile("tidy", "Tidy up."))
	write(t, filepath.Join(other, "skills/secret/SKILL.md"), skillFile("secret", "Elsewhere."))
	write(t, filepath.Join(home, ".claude/plugins/installed_plugins.json"), `{"version":2,"plugins":{
		"slack@m":[{"scope":"user","installPath":"`+user+`"}],
		"mine@m":[{"scope":"local","projectPath":"`+dir+`","installPath":"`+local+`"}],
		"theirs@m":[{"scope":"local","projectPath":"/somewhere/else","installPath":"`+other+`"}]}}`)

	got := Claude(home, dir)
	want := []Skill{{"mine:tidy", "Tidy up."}, {"slack:standup", "Daily."}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Claude = %+v, want %+v", got, want)
	}

	other2 := t.TempDir()
	got = Claude(home, other2)
	if want := []Skill{{"slack:standup", "Daily."}}; !reflect.DeepEqual(got, want) {
		t.Errorf("Claude in another folder = %+v, want %+v", got, want)
	}
}

func TestClaude_Nothing(t *testing.T) {
	if got := Claude(t.TempDir(), t.TempDir()); got == nil || len(got) != 0 {
		t.Errorf("Claude = %#v, want an empty list", got)
	}
}

func TestClaude_BadPluginList(t *testing.T) {
	home, dir := t.TempDir(), t.TempDir()
	write(t, filepath.Join(home, ".claude/skills/how/SKILL.md"), skillFile("how", "Explain."))
	write(t, filepath.Join(dir, ".claude/skills/repo-skill/SKILL.md"), skillFile("repo-skill", "Repo."))
	write(t, filepath.Join(home, ".claude/plugins/installed_plugins.json"), "{")
	var logged strings.Builder
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	got := Claude(home, dir)
	want := []Skill{{"how", "Explain."}, {"repo-skill", "Repo."}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Claude = %+v, want %+v", got, want)
	}
	if !strings.Contains(logged.String(), "installed_plugins.json") {
		t.Errorf("log = %q, want a line naming installed_plugins.json", logged.String())
	}
}

func TestClaude_UnreadableSkill(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, ".claude/skills/how/SKILL.md"), skillFile("how", "Explain."))
	locked := filepath.Join(home, ".claude/skills/locked/SKILL.md")
	write(t, locked, skillFile("locked", "Nobody can read me."))
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o644) })
	log.SetOutput(io.Discard)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	if got, want := Claude(home, t.TempDir()), []Skill{{"how", "Explain."}}; !reflect.DeepEqual(got, want) {
		t.Errorf("Claude = %+v, want %+v", got, want)
	}
}

func TestClaude_InvalidNames(t *testing.T) {
	home, dir := t.TempDir(), t.TempDir()
	write(t, filepath.Join(home, ".claude/skills/how/SKILL.md"), skillFile("how", "Explain."))
	write(t, filepath.Join(home, ".claude/skills/spaced/SKILL.md"), skillFile("my skill", "Has a space."))
	write(t, filepath.Join(home, ".claude/skills/Bash(rm:*)/SKILL.md"), "# folder name only\n")
	dotted := filepath.Join(home, ".claude/plugins/cache/m/my.plugin/1.0")
	write(t, filepath.Join(dotted, "skills/tidy/SKILL.md"), skillFile("tidy", "Dotted plugin."))
	write(t, filepath.Join(home, ".claude/plugins/installed_plugins.json"),
		`{"plugins":{"my.plugin@m":[{"scope":"user","installPath":"`+dotted+`"}]}}`)
	log.SetOutput(io.Discard)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	if got, want := Claude(home, dir), []Skill{{"how", "Explain."}}; !reflect.DeepEqual(got, want) {
		t.Errorf("Claude = %+v, want %+v", got, want)
	}
}

func TestValidName(t *testing.T) {
	for name, want := range map[string]bool{
		"how": true, "plugin:skill": true, "a_b-1": true, "V2": true,
		"": false, "my skill": false, "v1.2": false, "a/b": false, "Bash(rm:*)": false, " how": false,
	} {
		if got := ValidName(name); got != want {
			t.Errorf("ValidName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestCodex(t *testing.T) {
	home, dir := t.TempDir(), t.TempDir()
	write(t, filepath.Join(home, ".agents/skills/reef-probe/SKILL.md"), skillFile("reef-probe", "Probe."))
	write(t, filepath.Join(dir, ".agents/skills/repo-skill/SKILL.md"), skillFile("repo-skill", "Repo."))
	write(t, filepath.Join(home, ".claude/skills/how/SKILL.md"), skillFile("how", "Claude only."))
	write(t, filepath.Join(dir, ".claude/skills/why/SKILL.md"), skillFile("why", "Claude only."))

	got := Codex(home, dir)
	want := []Skill{{"reef-probe", "Probe."}, {"repo-skill", "Repo."}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Codex = %+v, want %+v", got, want)
	}
}
