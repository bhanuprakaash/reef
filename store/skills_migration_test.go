package store

import (
	"context"
	"testing"
)

func TestMigrateSkillsToList(t *testing.T) {
	tests := []struct {
		name        string
		persona     string
		skills      string
		wantSkills  string
		wantPersona string
	}{
		{"names only", "You lead.", "how, why, teach", `["how","why","teach"]`, "You lead."},
		{"names then prose", "You lead.",
			"how, why, teach, grill, unslop. From ~/.claude/skills/poteto-mode/SKILL.md read only the House rules section.",
			`["how","why","teach","grill","unslop"]`,
			"You lead.\n\nFrom ~/.claude/skills/poteto-mode/SKILL.md read only the House rules section."},
		{"names then prose, empty persona", "", "how. Be brief.", `["how"]`, "Be brief."},
		{"trailing period", "", "how, why.", `["how","why"]`, ""},
		{"dot inside a name", "", "v1.2-skill, how. Be brief.", `[]`, "v1.2-skill, how. Be brief."},
		{"repeated name", "", "how, how", `["how"]`, ""},
		{"odd character", "", "a/b, how", `[]`, "a/b, how"},
		{"bad name among good ones", "You lead.", "how, a*b. Be brief.", `[]`, "You lead.\n\nhow, a*b. Be brief."},
		{"empty names dropped", "", "how,, why", `["how","why"]`, ""},
		{"prose only", "You design.", "Sketch first.", `[]`, "You design.\n\nSketch first."},
		{"prose without a period", "", "Sketch first", `[]`, "Sketch first"},
		{"empty", "You design.", "", `[]`, "You design."},
		{"blank", "", "  \n", `[]`, ""},
		{"name with a quote", "", `a"b, c`, `[]`, `a"b, c`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			s, err := Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = s.Close() }()
			if _, err := s.db.Exec("PRAGMA user_version = 22"); err != nil {
				t.Fatal(err)
			}
			if _, err := s.db.Exec("UPDATE role SET persona = ?, skills = ?", tt.persona, tt.skills); err != nil {
				t.Fatal(err)
			}
			if err := migrate(ctx, s.db); err != nil {
				t.Fatal(err)
			}
			var gotSkills, gotPersona string
			if err := s.db.QueryRow("SELECT skills, persona FROM role").Scan(&gotSkills, &gotPersona); err != nil {
				t.Fatal(err)
			}
			if gotSkills != tt.wantSkills || gotPersona != tt.wantPersona {
				t.Errorf("skills = %q, persona = %q\nwant skills = %q, persona = %q",
					gotSkills, gotPersona, tt.wantSkills, tt.wantPersona)
			}
		})
	}
}
