package store

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestTeam_Responsibility(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	created, err := s.CreateTeam(ctx, Team{
		Name:           "release",
		Responsibility: "Ships Reef safely.",
		Roles:          []Role{{Name: "lead", Lead: true, Backend: "claude"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Responsibility != "Ships Reef safely." {
		t.Errorf("created responsibility = %q, want %q", created.Responsibility, "Ships Reef safely.")
	}

	created.Responsibility = "Owns releases and rollback."
	updated, err := s.ApplyTeam(ctx, created)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Responsibility != "Owns releases and rollback." {
		t.Errorf("updated responsibility = %q, want %q", updated.Responsibility,
			"Owns releases and rollback.")
	}
}

func TestCreateTeam_CopiesTeammateDefaults(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	teammate, err := s.CreateTeammate(ctx, Teammate{
		Name: "sam", Backend: "claude", Tools: []string{"Read"}, PermissionMode: "default",
	})
	if err != nil {
		t.Fatal(err)
	}

	team, err := s.CreateTeam(ctx, Team{Name: "release", Roles: []Role{{
		TeammateID: teammate.ID, Name: "lead", Lead: true, Backend: "claude", Write: true,
		PermissionMode: "acceptEdits", Tools: []string{"Edit"},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	got := team.Roles[0]
	if got.TeammateID != teammate.ID || got.Write || got.PermissionMode != "default" ||
		!reflect.DeepEqual(got.Tools, []string{"Read"}) {
		t.Errorf("created membership = %+v, want teammate %d with defaults Read, default, no write",
			got, teammate.ID)
	}
}

func TestTeams_SkipsTeamWithoutRoles(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.db.ExecContext(ctx, "INSERT INTO team (name) VALUES ('empty')"); err != nil {
		t.Fatal(err)
	}

	teams, err := s.Teams(ctx)
	if err != nil || len(teams) != 1 || teams[0].Name != SoloTeam {
		t.Errorf("Teams = %+v, %v, want only solo", teams, err)
	}
}

func TestDeleteTeam_QueuedWorkItem(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	team, err := s.CreateTeam(ctx, Team{Name: "release", Roles: []Role{
		{Name: "lead", Lead: true, Backend: "claude"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	project, err := s.CreateProject(ctx, Project{Name: "reef", Path: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	var teamID int64
	if err := s.db.QueryRowContext(ctx, "SELECT id FROM team WHERE name = ?", team.Name).
		Scan(&teamID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateWorkItem(ctx, WorkItem{
		ProjectID: project.ID,
		Title:     "Ship desktop updater",
		Status:    "queued",
		TeamID:    &teamID,
	}); err != nil {
		t.Fatal(err)
	}

	err = s.DeleteTeam(ctx, "release")
	if !errors.Is(err, ErrConflict) || !strings.Contains(err.Error(), "Ship desktop updater") {
		t.Errorf("DeleteTeam error = %v, want conflict naming Ship desktop updater", err)
	}
}

func TestApplyTeam_Solo(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	tests := []struct {
		name string
		give []Role
		want error
	}{
		{
			name: "extra role",
			give: []Role{
				{Name: "lead", Lead: true, Backend: "claude"},
				{Name: "architect", Backend: "claude"},
			},
			want: ErrInvalid,
		},
		{
			name: "renamed lead",
			give: []Role{{Name: "boss", Lead: true, Backend: "claude"}},
			want: ErrInvalid,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.ApplyTeam(ctx, Team{Name: SoloTeam, Roles: tt.give})
			if !errors.Is(err, tt.want) {
				t.Errorf("ApplyTeam solo error = %v, want %v", err, tt.want)
			}
		})
	}

	seeded, err := s.Team(ctx, SoloTeam)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.ApplyTeam(ctx, seeded)
	if err != nil {
		t.Fatalf("ApplyTeam seeded solo = %v, want nil", err)
	}
	if len(got.Roles) != 1 || got.Roles[0].Name != "lead" || !got.Roles[0].Lead {
		t.Errorf("ApplyTeam seeded solo = %+v, want its lead role unchanged", got.Roles)
	}
}

func TestApplyTeam_RoleName(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	tests := []struct {
		name string
		give string
		want string
	}{
		{"reserved", "user", `invalid: role name "user" must be letters, digits, _ or -, and not user or reef`},
		{"reef", "reef", `invalid: role name "reef" must be letters, digits, _ or -, and not user or reef`},
		{"space", "two words", `invalid: role name "two words" must be letters, digits, _ or -, and not user or reef`},
		{"punctuation", "dev.1", `invalid: role name "dev.1" must be letters, digits, _ or -, and not user or reef`},
		{"empty", "", "invalid: role name is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.ApplyTeam(ctx, Team{Name: "t", Roles: []Role{
				{Name: "lead", Lead: true, Backend: "claude"}, {Name: tt.give, Backend: "claude"},
			}})
			if err == nil || err.Error() != tt.want {
				t.Errorf("ApplyTeam error = %v, want %q", err, tt.want)
			}
		})
	}
	if _, err := s.ApplyTeam(ctx, Team{Name: "t", Roles: []Role{
		{Name: "lead", Lead: true, Backend: "claude"}, {Name: "Dev_2-b", Backend: "claude"},
	}}); err != nil {
		t.Errorf("ApplyTeam with role Dev_2-b error = %v, want nil", err)
	}
}

func TestApplyTeam_Name(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	tests := []struct {
		name string
		give string
		want string
	}{
		{"space", "my team", `invalid: team name "my team" must be letters, digits, _ or -`},
		{"slash", "a/b", `invalid: team name "a/b" must be letters, digits, _ or -`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.ApplyTeam(ctx, Team{Name: tt.give, Roles: []Role{
				{Name: "lead", Lead: true, Backend: "claude"},
			}})
			if err == nil || err.Error() != tt.want {
				t.Errorf("ApplyTeam error = %v, want %q", err, tt.want)
			}
		})
	}
	if _, err := s.ApplyTeam(ctx, Team{Name: "Team_2-b", Roles: []Role{
		{Name: "lead", Lead: true, Backend: "claude"},
	}}); err != nil {
		t.Errorf("ApplyTeam with name Team_2-b error = %v, want nil", err)
	}
}

func TestApplyTeam_Write(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	got, err := s.ApplyTeam(ctx, Team{Name: "crew", Roles: []Role{
		{Name: "lead", Lead: true, Backend: "claude"},
		{Name: "developer", Write: true, Backend: "claude"},
		{Name: "tester", Backend: "codex"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	var writes []bool
	for _, r := range got.Roles {
		writes = append(writes, r.Write)
	}
	if want := []bool{false, true, false}; !reflect.DeepEqual(writes, want) {
		t.Errorf("write of lead, developer, tester = %v, want %v", writes, want)
	}
}

func TestApplyTeam_KeepsExistingTeammate(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	if _, err := s.ApplyTeam(ctx, Team{Name: "crew", Roles: []Role{
		{Name: "lead", Lead: true, Backend: "claude", Persona: "Plans.", Face: "whale", Model: "opus"},
	}}); err != nil {
		t.Fatal(err)
	}
	got, err := s.ApplyTeam(ctx, Team{Name: "crew", Roles: []Role{
		{Name: "lead", Lead: true, Backend: "claude"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if r := got.Roles[0]; r.Persona != "Plans." || r.Face != "whale" || r.Model != "opus" {
		t.Errorf("lead after a second apply = %+v, want persona Plans., face whale, model opus", r)
	}
}

func TestApplyTeam_ReadOnlyClaudeBash(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	const wantErr = "invalid: role tester has no write, so it cannot have unrestricted Bash; " +
		"list command patterns such as Bash(go test:*)"
	tests := []struct {
		name    string
		write   bool
		backend string
		tools   []string
		want    string
	}{
		{"plain", false, "claude", []string{"Read", "Bash"}, wantErr},
		{"star", false, "claude", []string{"Bash(*)"}, wantErr},
		{"colon star", false, "claude", []string{"Bash(:*)"}, wantErr},
		{"pattern", false, "claude", []string{"Bash(go test:*)"}, ""},
		{"write", true, "claude", []string{"Bash"}, ""},
		{"codex", false, "codex", []string{"Bash"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.ApplyTeam(ctx, Team{Name: "t", Roles: []Role{
				{Name: "lead", Lead: true, Backend: "claude"},
				{Name: "tester", Write: tt.write, Backend: tt.backend, Tools: tt.tools},
			}})
			if tt.want == "" && err != nil || tt.want != "" && (err == nil || err.Error() != tt.want) {
				t.Errorf("ApplyTeam error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestSuggestTeam(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	for name, responsibility := range map[string]string{
		"frontend": "React components, CSS and pages in the web UI.",
		"infra":    "Deploys, CI pipelines, servers and cloud infrastructure.",
		"quiet":    "",
	} {
		if _, err := s.CreateTeam(ctx, Team{
			Name: name, Responsibility: responsibility,
			Roles: []Role{{Name: "lead", Lead: true, Backend: "claude"}},
		}); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct{ text, want string }{
		{"The production deploy is failing on every push", "infra"},
		{"Button colours break on the settings page", "frontend"},
		{"Rename the invoice export", SoloTeam},
		{"", SoloTeam},
	}
	for _, tt := range tests {
		got, err := s.SuggestTeam(ctx, tt.text)
		if err != nil || got.Name != tt.want {
			t.Errorf("SuggestTeam(%q) = %q, %v, want %q", tt.text, got.Name, err, tt.want)
		}
	}

	infra, err := s.Team(ctx, "infra")
	if err != nil {
		t.Fatal(err)
	}
	infra.Responsibility = "Billing and invoices."
	if _, err := s.ApplyTeam(ctx, infra); err != nil {
		t.Fatal(err)
	}
	if got, err := s.SuggestTeam(ctx, "The production deploy is failing"); err != nil || got.Name != SoloTeam {
		t.Errorf("after the text changed, SuggestTeam(deploy) = %q, %v, want %q", got.Name, err, SoloTeam)
	}
	if got, err := s.SuggestTeam(ctx, "invoice totals are wrong"); err != nil || got.Name != "infra" {
		t.Errorf("after the text changed, SuggestTeam(invoice) = %q, %v, want infra", got.Name, err)
	}
}
