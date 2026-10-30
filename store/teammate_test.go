package store

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestTeammate_MembershipUsesProfileRules(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.ApplyTeam(ctx, Team{Name: "core",
		Roles: []Role{{Name: "boss", Lead: true, Backend: "claude"}}}); err != nil {
		t.Fatal(err)
	}
	sam, err := s.CreateTeammate(ctx, Teammate{Name: "sam", Backend: "claude", Write: true,
		Tools: []string{"Read"}, PermissionMode: "acceptEdits"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddMember(ctx, "core", sam.ID, false); err != nil {
		t.Fatal(err)
	}

	sam.Tools = []string{"Read", "Grep"}
	sam.Write = false
	if _, err := s.UpdateTeammate(ctx, sam); err != nil {
		t.Fatal(err)
	}

	team, err := s.Team(ctx, "core")
	if err != nil {
		t.Fatal(err)
	}
	got := team.Roles[1]
	if got.Name != "sam" || !reflect.DeepEqual(got.Tools, []string{"Read", "Grep"}) || got.Write ||
		got.PermissionMode != "acceptEdits" {
		t.Errorf("member sam = %+v, want tools [Read Grep], no write, acceptEdits", got)
	}
	back, err := s.Teammate(ctx, sam.ID)
	if err != nil || !reflect.DeepEqual(back.Tools, []string{"Read", "Grep"}) || back.Write {
		t.Errorf("Teammate sam = %+v, %v, want the edited defaults", back, err)
	}
}

func TestTeammate_Refusals(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.ApplyTeam(ctx, Team{Name: "core",
		Roles: []Role{{Name: "boss", Lead: true, Backend: "claude"}}}); err != nil {
		t.Fatal(err)
	}
	sam, err := s.CreateTeammate(ctx, Teammate{Name: "sam", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	ann, err := s.CreateTeammate(ctx, Teammate{Name: "ann", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddMember(ctx, "core", sam.ID, false); err != nil {
		t.Fatal(err)
	}
	core, err := s.Team(ctx, "core")
	if err != nil {
		t.Fatal(err)
	}
	boss := core.Roles[0].TeammateID

	tests := []struct {
		name string
		do   func() error
		want error
		msg  string
	}{
		{"delete a member", func() error { return s.DeleteTeammate(ctx, sam.ID) },
			ErrInvalid, "invalid: teammate sam is in team core; remove it from the team first"},
		{"second lead on edit", func() error {
			_, err := s.EditMember(ctx, "core", sam.ID, Membership{Lead: true})
			return err
		}, ErrInvalid, "invalid: the team already has a lead, boss"},
		{"second lead on add", func() error {
			_, err := s.AddMember(ctx, "core", ann.ID, true)
			return err
		}, ErrInvalid, "invalid: the team already has a lead, boss"},
		{"remove the lead", func() error { return s.RemoveMember(ctx, "core", boss) },
			ErrInvalid, "invalid: team core needs exactly one lead, has 0"},
		{"un-lead the lead", func() error {
			_, err := s.EditMember(ctx, "core", boss, Membership{Lead: false})
			return err
		}, ErrInvalid, "invalid: team core needs exactly one lead, has 0"},
		{"name taken", func() error {
			_, err := s.CreateTeammate(ctx, Teammate{Name: "sam", Backend: "claude"})
			return err
		}, ErrConflict, `conflict: a teammate named "sam" already exists`},
		{"already a member", func() error {
			_, err := s.AddMember(ctx, "core", sam.ID, false)
			return err
		}, ErrConflict, "conflict: team core already has teammate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.do()
			if !errors.Is(err, tt.want) || !strings.HasPrefix(err.Error(), tt.msg) {
				t.Errorf("error = %v, want %v starting %q", err, tt.want, tt.msg)
			}
		})
	}

	if core, err := s.Team(ctx, "core"); err != nil || len(core.Roles) != 2 || !core.Roles[0].Lead {
		t.Errorf("core after the refusals = %+v, %v, want boss leading with sam", core, err)
	}
	if err := s.RemoveMember(ctx, "core", sam.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteTeammate(ctx, sam.ID); err != nil {
		t.Errorf("DeleteTeammate after leaving the team = %v, want nil", err)
	}
}
