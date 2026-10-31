package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestTeam_Responsibility(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	created, err := testCreateTeam(t, s, ctx, Team{
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
	updated, err := testApplyTeam(t, s, ctx, created)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Responsibility != "Owns releases and rollback." {
		t.Errorf("updated responsibility = %q, want %q", updated.Responsibility,
			"Owns releases and rollback.")
	}
}

func TestCreateTeam_CopiesBotDefaults(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	bot, err := s.CreateBot(ctx, Bot{
		Name: "sam", Backend: "claude", Tools: []string{"Read"}, PermissionMode: "default",
	})
	if err != nil {
		t.Fatal(err)
	}

	team, err := testCreateTeam(t, s, ctx, Team{Name: "release", Roles: []Role{{
		BotID: bot.ID, Name: "lead", Lead: true, Backend: "claude", Write: true,
		PermissionMode: "acceptEdits", Tools: []string{"Edit"},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	got := team.Roles[0]
	if got.BotID != bot.ID || got.Write || got.PermissionMode != "default" ||
		!reflect.DeepEqual(got.Tools, []string{"Read"}) {
		t.Errorf("created membership = %+v, want bot %d with defaults Read, default, no write",
			got, bot.ID)
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
			_, err := testApplyTeam(t, s, ctx, Team{Name: SoloTeam, Roles: tt.give})
			if !errors.Is(err, tt.want) {
				t.Errorf("ApplyTeam solo error = %v, want %v", err, tt.want)
			}
		})
	}

	seeded, err := s.Team(ctx, SoloTeam)
	if err != nil {
		t.Fatal(err)
	}
	got, err := testApplyTeam(t, s, ctx, seeded)
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
			_, err := testApplyTeam(t, s, ctx, Team{Name: "t", Roles: []Role{
				{Name: "lead", Lead: true, Backend: "claude"}, {Name: tt.give, Backend: "claude"},
			}})
			if err == nil || err.Error() != tt.want {
				t.Errorf("ApplyTeam error = %v, want %q", err, tt.want)
			}
		})
	}
	if _, err := testApplyTeam(t, s, ctx, Team{Name: "t", Roles: []Role{
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
			_, err := testApplyTeam(t, s, ctx, Team{Name: tt.give, Roles: []Role{
				{Name: "lead", Lead: true, Backend: "claude"},
			}})
			if err == nil || err.Error() != tt.want {
				t.Errorf("ApplyTeam error = %v, want %q", err, tt.want)
			}
		})
	}
	if _, err := testApplyTeam(t, s, ctx, Team{Name: "Team_2-b", Roles: []Role{
		{Name: "lead", Lead: true, Backend: "claude"},
	}}); err != nil {
		t.Errorf("ApplyTeam with name Team_2-b error = %v, want nil", err)
	}
}

func TestApplyTeam_ReferencesExistingBotsOnly(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	lead, err := s.CreateBot(ctx, Bot{Name: "boss", Backend: "claude", Description: "Plans.", Write: true})
	if err != nil {
		t.Fatal(err)
	}
	dev, err := s.CreateBot(ctx, Bot{Name: "dev", Backend: "codex"})
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.ApplyTeam(ctx, TeamSpec{Name: "crew", Roles: []Member{{BotID: lead.ID, Lead: true}, {BotID: dev.ID}}})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, r := range got.Roles {
		names = append(names, r.Name)
	}
	if want := []string{"boss", "dev"}; !reflect.DeepEqual(names, want) {
		t.Errorf("roles = %v, want %v", names, want)
	}
	if r := got.Roles[0]; r.Description != "Plans." || !r.Write {
		t.Errorf("lead = %+v, want the Bot's description and write", r)
	}

	tests := []struct {
		name string
		give []Member
	}{
		{"unknown bot", []Member{{BotID: lead.ID, Lead: true}, {BotID: 999}}},
		{"no bot id", []Member{{BotID: lead.ID, Lead: true}, {}}},
		{"bot twice", []Member{{BotID: lead.ID, Lead: true}, {BotID: lead.ID}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := s.ApplyTeam(ctx, TeamSpec{Name: "crew", Roles: tt.give}); !errors.Is(err, ErrInvalid) {
				t.Errorf("ApplyTeam error = %v, want ErrInvalid", err)
			}
		})
	}
	if bots, err := s.Bots(ctx); err != nil || len(bots) != 3 {
		t.Errorf("Bots = %d, %v, want 3: the solo lead, boss and dev", len(bots), err)
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
			_, err := testApplyTeam(t, s, ctx, Team{Name: "t", Roles: []Role{
				{Name: "lead", Lead: true, Backend: "claude"},
				{Name: "tester", Write: tt.write, Backend: tt.backend, Tools: tt.tools},
			}})
			if tt.want == "" && err != nil || tt.want != "" && (err == nil || err.Error() != tt.want) {
				t.Errorf("ApplyTeam error = %v, want %q", err, tt.want)
			}
		})
	}
}
