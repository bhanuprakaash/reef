package store

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestBot_MembershipUsesProfileRules(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := testApplyTeam(t, s, ctx, Team{Name: "core",
		Roles: []Role{{Name: "boss", Lead: true, Backend: "claude"}}}); err != nil {
		t.Fatal(err)
	}
	sam, err := s.CreateBot(ctx, Bot{Name: "sam", Backend: "claude", Write: true,
		Tools: []string{"Read"}, PermissionMode: "acceptEdits"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddMember(ctx, "core", sam.ID, false); err != nil {
		t.Fatal(err)
	}

	sam.Tools = []string{"Read", "Grep"}
	sam.Write = false
	if _, err := s.UpdateBot(ctx, sam); err != nil {
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
	back, err := s.Bot(ctx, sam.ID)
	if err != nil || !reflect.DeepEqual(back.Tools, []string{"Read", "Grep"}) || back.Write {
		t.Errorf("Bot sam = %+v, %v, want the edited defaults", back, err)
	}
}

func TestBot_Refusals(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := testApplyTeam(t, s, ctx, Team{Name: "core",
		Roles: []Role{{Name: "boss", Lead: true, Backend: "claude"}}}); err != nil {
		t.Fatal(err)
	}
	sam, err := s.CreateBot(ctx, Bot{Name: "sam", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	ann, err := s.CreateBot(ctx, Bot{Name: "ann", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddMember(ctx, "core", sam.ID, false); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		do   func() error
		want error
		msg  string
	}{
		{"delete a member", func() error { return s.DeleteBot(ctx, sam.ID) },
			ErrInvalid, "invalid: bot sam is in team core; remove it from the team first"},
		{"second lead on edit", func() error {
			_, err := s.EditMember(ctx, "core", sam.ID, Membership{Lead: true})
			return err
		}, ErrInvalid, "invalid: the team already has a lead, boss"},
		{"second lead on add", func() error {
			_, err := s.AddMember(ctx, "core", ann.ID, true)
			return err
		}, ErrInvalid, "invalid: the team already has a lead, boss"},
		{"name taken", func() error {
			_, err := s.CreateBot(ctx, Bot{Name: "sam", Backend: "claude"})
			return err
		}, ErrConflict, `conflict: a bot named "sam" already exists`},
		{"already a member", func() error {
			_, err := s.AddMember(ctx, "core", sam.ID, false)
			return err
		}, ErrConflict, "conflict: team core already has bot"},
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
	if err := s.DeleteBot(ctx, sam.ID); err != nil {
		t.Errorf("DeleteBot after leaving the team = %v, want nil", err)
	}
}

// testApplyTeam creates a Bot for each role without a BotID, named after the role, then
// applies the team of them. testCreateTeam does the same and creates the team.
func testApplyTeam(t *testing.T, s *Store, ctx context.Context, team Team) (Team, error) {
	t.Helper()
	spec, err := testTeamSpec(t, s, ctx, team)
	if err != nil {
		return Team{}, err
	}
	return s.ApplyTeam(ctx, spec)
}

func testCreateTeam(t *testing.T, s *Store, ctx context.Context, team Team) (Team, error) {
	t.Helper()
	spec, err := testTeamSpec(t, s, ctx, team)
	if err != nil {
		return Team{}, err
	}
	return s.CreateTeam(ctx, spec)
}

func testTeamSpec(t *testing.T, s *Store, ctx context.Context, team Team) (TeamSpec, error) {
	t.Helper()
	spec := TeamSpec{Name: team.Name, Responsibility: team.Responsibility}
	for _, r := range team.Roles {
		if r.BotID == 0 {
			b, err := s.CreateBot(ctx, Bot{Name: r.Name, Description: r.Description, Skills: r.Skills,
				Backend: r.Backend, Model: r.Model, Provider: r.Provider, Face: r.Face, Colour: r.Colour,
				Tools: r.Tools, Connections: connectionIDs(r.Connections), Write: r.Write, PermissionMode: r.PermissionMode})
			if errors.Is(err, ErrConflict) {
				bots, lerr := s.Bots(ctx)
				if lerr != nil {
					return TeamSpec{}, lerr
				}
				for _, existing := range bots {
					if existing.Name == r.Name {
						b, err = existing, nil
					}
				}
			}
			if err != nil {
				return TeamSpec{}, err
			}
			r.BotID = b.ID
		}
		spec.Roles = append(spec.Roles, Member{BotID: r.BotID, Lead: r.Lead})
	}
	return spec, nil
}

func connectionIDs(cs []Connection) []int64 {
	ids := make([]int64, len(cs))
	for i, c := range cs {
		ids[i] = c.ID
	}
	return ids
}
