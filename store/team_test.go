package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestTeam_Gated(t *testing.T) {
	team := Team{Gates: []Gate{
		{From: "lead", To: "architect", On: true},
		{From: "architect", To: "developer", On: false},
	}}
	tests := []struct {
		name string
		from string
		to   string
		want bool
	}{
		{"listed on", "lead", "architect", true},
		{"listed off beats the default", "architect", "developer", false},
		{"default on", "developer", "tester", true},
		{"default off", "architect", "lead", false},
		{"the user is never gated", "user", "lead", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := team.Gated(tt.from, tt.to); got != tt.want {
				t.Errorf("Gated(%q, %q) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
	if !(Team{}).Gated("architect", "developer") {
		t.Error("a team with no gates should default architect to developer on")
	}
}

func TestApplyTeam_Gates(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	roles := []Role{{Name: "lead", Lead: true, Backend: "claude"}, {Name: "architect", Backend: "claude"}}

	got, err := s.ApplyTeam(ctx, Team{Name: "core", Roles: roles,
		Gates: []Gate{{From: "lead", To: "architect", On: true}}})
	if err != nil || !reflect.DeepEqual(got.Gates, []Gate{{From: "lead", To: "architect", On: true}}) {
		t.Fatalf("ApplyTeam gates = %+v, %v, want lead to architect on", got.Gates, err)
	}
	if got, err = s.ApplyTeam(ctx, Team{Name: "core", Roles: roles}); err != nil || got.Gates != nil {
		t.Errorf("ApplyTeam without gates = %+v, %v, want the old gates dropped", got.Gates, err)
	}
	_, err = s.ApplyTeam(ctx, Team{Name: "core", Roles: roles,
		Gates: []Gate{{From: "lead", To: "tester", On: true}}})
	if !errors.Is(err, ErrInvalid) {
		t.Errorf("ApplyTeam gate on an unknown role: err = %v, want ErrInvalid", err)
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
		{"reserved", "user", `invalid: role name "user" must be letters, digits, _ or -, and not user`},
		{"space", "two words", `invalid: role name "two words" must be letters, digits, _ or -, and not user`},
		{"punctuation", "dev.1", `invalid: role name "dev.1" must be letters, digits, _ or -, and not user`},
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
