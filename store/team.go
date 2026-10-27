package store

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/skill"
)

// SoloTeam is the lead-only team a Space gets when none is named.
const SoloTeam = "solo"

// Team is a named set of roles, exactly one of them the lead.
type Team struct {
	Name  string `json:"name"`
	Roles []Role `json:"roles"`
}

func faces() []string {
	return []string{
		"jellyfish", "whale", "penguin", "shark", "octopus", "turtle", "seahorse", "otter",
		"clownfish", "manta-ray", "pebble",
	}
}

// Role is one teammate's setup. Face picks the creature the window draws for the role; empty
// means the default for its name. Model names the model the backend runs the role on; empty is
// the backend's default. Provider names an added install (see Provider) whose binary the role
// runs; empty is the claude or codex found on PATH. Tools are the backend's allowed tools. Write says whether the
// role may change files: without it the backend's tools and sandbox are held to read-only.
type Role struct {
	TeammateID     int64             `json:"teammateId,omitempty"`
	Name           string            `json:"name"`
	Lead           bool              `json:"lead"`
	Write          bool              `json:"write"`
	Backend        string            `json:"backend"`
	PermissionMode string            `json:"permissionMode"`
	Persona        string            `json:"persona"`
	Skills         []string          `json:"skills"`
	Face           string            `json:"face,omitempty"`
	Model          string            `json:"model,omitempty"`
	Provider       string            `json:"provider,omitempty"`
	Tools          []string          `json:"tools"`
	MCPServers     []agent.MCPServer `json:"mcpServers"`
}

var _roleName = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// UnrestrictedBash reports whether a claude tool entry allows every shell command.
func UnrestrictedBash(tool string) bool {
	return slices.Contains([]string{"Bash", "Bash(*)", "Bash(:*)"}, strings.TrimSpace(tool))
}

// ApplyTeam creates team t, or replaces the roles of the team with that name.
func (s *Store) ApplyTeam(ctx context.Context, t Team) (Team, error) {
	return s.applyTeam(ctx, t, false)
}

// CreateTeam creates team t, failing with ErrConflict when a team with that name already
// exists. The check and the insert are one statement, so two concurrent calls for the same
// name cannot both succeed.
func (s *Store) CreateTeam(ctx context.Context, t Team) (Team, error) {
	return s.applyTeam(ctx, t, true)
}

func (s *Store) applyTeam(ctx context.Context, t Team, createOnly bool) (Team, error) {
	if err := t.validate(); err != nil {
		return Team{}, err
	}
	t.Roles = slices.Clone(t.Roles)
	for i, r := range t.Roles {
		names, err := cleanSkills(r)
		if err != nil {
			return Team{}, err
		}
		t.Roles[i].Skills = names
		if _, err := s.RoleBin(ctx, r); err != nil {
			return Team{}, err
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Team{}, fmt.Errorf("begin apply team: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if createOnly {
		if _, err := tx.ExecContext(ctx, "INSERT INTO team (name) VALUES (?)", t.Name); err != nil {
			if isUnique(err) {
				return Team{}, fmt.Errorf("%w: a team named %q already exists", ErrConflict, t.Name)
			}
			return Team{}, fmt.Errorf("insert team: %w", err)
		}
	} else if _, err := tx.ExecContext(ctx,
		"INSERT INTO team (name) VALUES (?) ON CONFLICT (name) DO NOTHING", t.Name); err != nil {
		return Team{}, fmt.Errorf("insert team: %w", err)
	}
	var id int64
	if err := tx.QueryRowContext(ctx, "SELECT id FROM team WHERE name = ?", t.Name).
		Scan(&id); err != nil {
		return Team{}, fmt.Errorf("read team: %w", err)
	}
	behind, err := roleTeammates(ctx, tx, id)
	if err != nil {
		return Team{}, err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM team_member WHERE team_id = ?", id); err != nil {
		return Team{}, fmt.Errorf("delete members: %w", err)
	}
	for _, r := range t.Roles {
		skills, tools, servers, err := encodeLists(r.Skills, r.Tools, r.MCPServers)
		if err != nil {
			return Team{}, err
		}
		tm := cmp.Or(behind[r.Name], t.Name+"-"+r.Name)
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO teammate (name, persona, skills, backend, model, provider, face, tools, "+
				"mcp_servers, write, permission_mode) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) "+
				"ON CONFLICT (name) DO UPDATE SET persona = excluded.persona, skills = excluded.skills, "+
				"backend = excluded.backend, model = excluded.model, provider = excluded.provider, "+
				"face = excluded.face",
			tm, r.Persona, skills, r.Backend, r.Model, r.Provider, r.Face, tools,
			servers, r.Write, r.PermissionMode); err != nil {
			return Team{}, fmt.Errorf("insert teammate %s: %w", tm, err)
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO team_member (team_id, teammate_id, name, lead, tools, mcp_servers, write, "+
				"permission_mode) SELECT ?, id, ?, ?, ?, ?, ?, ? FROM teammate WHERE name = ?",
			id, r.Name, r.Lead, tools, servers, r.Write, r.PermissionMode, tm); err != nil {
			return Team{}, fmt.Errorf("insert member %s: %w", r.Name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return Team{}, fmt.Errorf("commit apply team: %w", err)
	}
	return s.Team(ctx, t.Name)
}

// roleTeammates maps each role name of the team to the name of the Teammate behind it.
func roleTeammates(ctx context.Context, tx *sql.Tx, teamID int64) (map[string]string, error) {
	rows, err := tx.QueryContext(ctx,
		"SELECT member.name, teammate.name FROM team_member member "+
			"JOIN teammate ON teammate.id = member.teammate_id WHERE member.team_id = ?", teamID)
	if err != nil {
		return nil, fmt.Errorf("read members: %w", err)
	}
	defer func() { _ = rows.Close() }()
	behind := make(map[string]string)
	for rows.Next() {
		var role, teammate string
		if err := rows.Scan(&role, &teammate); err != nil {
			return nil, fmt.Errorf("read members: %w", err)
		}
		behind[role] = teammate
	}
	return behind, rows.Err()
}

// Team returns the team with the name, lead first, or ErrNotFound.
func (s *Store) Team(ctx context.Context, name string) (Team, error) {
	var teamID int64
	switch err := s.db.QueryRowContext(ctx, "SELECT id FROM team WHERE name = ?", name).
		Scan(&teamID); {
	case errors.Is(err, sql.ErrNoRows):
		return Team{}, fmt.Errorf("team %s: %w", name, ErrNotFound)
	case err != nil:
		return Team{}, fmt.Errorf("read team %s: %w", name, err)
	}
	roles, err := s.roles(ctx, "team_member", "team_id", teamID)
	if err != nil {
		return Team{}, fmt.Errorf("read team %s: %w", name, err)
	}
	if len(roles) == 0 {
		return Team{}, fmt.Errorf("team %s: %w", name, ErrNotFound)
	}
	return Team{Name: name, Roles: roles}, nil
}

// roles lists the members in table (team_member or space_member) whose key column is id, lead
// first, with the Teammate's persona, skills, backend and face. The access rules are the
// member's own.
func (s *Store) roles(ctx context.Context, table, key string, id int64) ([]Role, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT teammate.id, member.name, member.lead, member.write, teammate.backend, "+
			"member.permission_mode, teammate.persona, teammate.skills, teammate.face, teammate.model, "+
			"teammate.provider, member.tools, member.mcp_servers "+
			"FROM "+table+" member JOIN teammate ON teammate.id = member.teammate_id "+
			"WHERE member."+key+" = ? ORDER BY member.lead DESC, member.id", id)
	if err != nil {
		return nil, err
	}
	return scanAll(rows, scanRole)
}

// Teams lists all teams by name.
func (s *Store) Teams(ctx context.Context) ([]Team, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT name FROM team ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("list teams: %w", err)
	}
	names, err := scanAll(rows, func(r interface{ Scan(...any) error }) (string, error) {
		var n string
		return n, r.Scan(&n)
	})
	if err != nil {
		return nil, err
	}
	teams := make([]Team, 0, len(names))
	for _, n := range names {
		t, err := s.Team(ctx, n)
		if err != nil {
			return nil, err
		}
		teams = append(teams, t)
	}
	return teams, nil
}

func (t Team) validate() error {
	if t.Name == "" {
		return fmt.Errorf("%w: team name is required", ErrInvalid)
	}
	if !_roleName.MatchString(t.Name) {
		return fmt.Errorf("%w: team name %q must be letters, digits, _ or -", ErrInvalid, t.Name)
	}
	if t.Name == SoloTeam && (len(t.Roles) != 1 || t.Roles[0].Name != "lead" || !t.Roles[0].Lead) {
		return fmt.Errorf("%w: team %s must have exactly one lead role named lead",
			ErrInvalid, SoloTeam)
	}
	leads, seen := 0, make(map[string]bool, len(t.Roles))
	for _, r := range t.Roles {
		if err := validateRole(r); err != nil {
			return err
		}
		if seen[r.Name] {
			return fmt.Errorf("%w: role %s appears twice", ErrInvalid, r.Name)
		}
		seen[r.Name] = true
		if r.Lead {
			leads++
		}
	}
	if leads != 1 {
		return fmt.Errorf("%w: team %s needs exactly one lead role, has %d",
			ErrInvalid, t.Name, leads)
	}
	return nil
}

func validateRole(r Role) error {
	if r.Name == "" {
		return fmt.Errorf("%w: role name is required", ErrInvalid)
	}
	if r.Name == "user" || r.Name == ReefAuthor || !_roleName.MatchString(r.Name) {
		return fmt.Errorf("%w: role name %q must be letters, digits, _ or -, and not user or reef",
			ErrInvalid, r.Name)
	}
	if err := ValidateAgent(r.Backend, r.PermissionMode); err != nil {
		return fmt.Errorf("role %s: %w", r.Name, err)
	}
	if r.Face != "" && !slices.Contains(faces(), r.Face) {
		return fmt.Errorf("%w: role %s: face %q must be one of %s",
			ErrInvalid, r.Name, r.Face, strings.Join(faces(), ", "))
	}
	if r.Backend == "claude" && !r.Write && slices.ContainsFunc(r.Tools, UnrestrictedBash) {
		return fmt.Errorf("%w: role %s has no write, so it cannot have unrestricted Bash; "+
			"list command patterns such as Bash(go test:*)", ErrInvalid, r.Name)
	}
	return nil
}

func cleanSkills(r Role) ([]string, error) {
	names := make([]string, 0, len(r.Skills))
	for _, name := range r.Skills {
		name = strings.TrimSpace(name)
		switch {
		case name == "":
			return nil, fmt.Errorf("%w: role %s has an empty skill name", ErrInvalid, r.Name)
		case !skill.ValidName(name):
			return nil, fmt.Errorf("%w: role %s: skill %q must be letters, digits, :, _ or -",
				ErrInvalid, r.Name, name)
		case slices.Contains(names, name):
			return nil, fmt.Errorf("%w: role %s lists skill %q twice", ErrInvalid, r.Name, name)
		}
		names = append(names, name)
	}
	return names, nil
}

// ValidateAgent checks that backend is known and permissionMode, when set, is one it accepts.
func ValidateAgent(backend, permissionMode string) error {
	var modes []string
	switch backend {
	case "claude":
		modes = []string{"default", "acceptEdits"}
	case "codex":
		modes = []string{"read-only", "workspace-write"}
	default:
		return fmt.Errorf("%w: backend must be claude or codex", ErrInvalid)
	}
	if permissionMode != "" && !slices.Contains(modes, permissionMode) {
		return fmt.Errorf("%w: permissionMode for %s must be empty, %s or %s",
			ErrInvalid, backend, modes[0], modes[1])
	}
	return nil
}

func scanRole(r interface{ Scan(...any) error }) (Role, error) {
	var ro Role
	var skills, tools, servers string
	if err := r.Scan(&ro.TeammateID, &ro.Name, &ro.Lead, &ro.Write, &ro.Backend, &ro.PermissionMode, &ro.Persona,
		&skills, &ro.Face, &ro.Model, &ro.Provider, &tools, &servers); err != nil {
		return Role{}, err
	}
	if err := json.Unmarshal([]byte(skills), &ro.Skills); err != nil {
		return Role{}, fmt.Errorf("role %s: bad skills: %w", ro.Name, err)
	}
	if err := json.Unmarshal([]byte(tools), &ro.Tools); err != nil {
		return Role{}, fmt.Errorf("role %s: bad tools: %w", ro.Name, err)
	}
	if err := json.Unmarshal([]byte(servers), &ro.MCPServers); err != nil {
		return Role{}, fmt.Errorf("role %s: bad mcp servers: %w", ro.Name, err)
	}
	return ro, nil
}

// nonNil makes a nil slice encode as [] rather than null.
func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
