package store

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/bhanuprakaash/reef/agent"
)

// SoloTeam is the lead-only team a channel gets when none is named.
const SoloTeam = "solo"

// Team is a named set of roles, exactly one of them the lead, and the gates between them.
type Team struct {
	Name  string `json:"name"`
	Roles []Role `json:"roles"`
	Gates []Gate `json:"gates,omitempty"`
}

// Gate switches approval on or off for the wakes one role sends another.
type Gate struct {
	From string `json:"from"`
	To   string `json:"to"`
	On   bool   `json:"on"`
}

func faces() []string {
	return []string{
		"jellyfish", "whale", "penguin", "shark", "octopus", "turtle", "seahorse", "otter",
		"clownfish", "manta-ray", "pebble",
	}
}

// Role is one teammate's setup. Face picks the creature the window draws for the role; empty
// means the default for its name. Model names the model the backend runs the role on; empty is
// the backend's default. Tools are the backend's allowed tools. Write says whether the
// role may change files: without it the backend's tools and sandbox are held to read-only.
type Role struct {
	Name           string            `json:"name"`
	Lead           bool              `json:"lead"`
	Write          bool              `json:"write"`
	Backend        string            `json:"backend"`
	PermissionMode string            `json:"permissionMode"`
	Persona        string            `json:"persona"`
	Skills         []string          `json:"skills"`
	Face           string            `json:"face,omitempty"`
	Model          string            `json:"model,omitempty"`
	Tools          []string          `json:"tools"`
	MCPServers     []agent.MCPServer `json:"mcpServers"`
}

var _roleName = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

var _skillName = regexp.MustCompile(`^[A-Za-z0-9:_-]+$`)

// UnrestrictedBash reports whether a claude tool entry allows every shell command.
func UnrestrictedBash(tool string) bool {
	return slices.Contains([]string{"Bash", "Bash(*)", "Bash(:*)"}, strings.TrimSpace(tool))
}

// _defaultGates apply to an edge the team does not list.
var _defaultGates = []Gate{
	{From: "lead", To: "developer", On: true},
	{From: "architect", To: "developer", On: true},
	{From: "developer", To: "tester", On: true},
	{From: "tester", To: "developer", On: true},
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
	if _, err := tx.ExecContext(ctx, "DELETE FROM role WHERE team_id = ?", id); err != nil {
		return Team{}, fmt.Errorf("delete roles: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM gate WHERE team_id = ?", id); err != nil {
		return Team{}, fmt.Errorf("delete gates: %w", err)
	}
	for _, g := range t.Gates {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO gate (team_id, from_role, to_role, enabled) VALUES (?, ?, ?, ?)",
			id, g.From, g.To, g.On); err != nil {
			return Team{}, fmt.Errorf("insert gate %s to %s: %w", g.From, g.To, err)
		}
	}
	for _, r := range t.Roles {
		tools, err := json.Marshal(nonNil(r.Tools))
		if err != nil {
			return Team{}, fmt.Errorf("encode tools: %w", err)
		}
		skills, err := json.Marshal(nonNil(r.Skills))
		if err != nil {
			return Team{}, fmt.Errorf("encode skills: %w", err)
		}
		servers, err := json.Marshal(nonNil(r.MCPServers))
		if err != nil {
			return Team{}, fmt.Errorf("encode mcp servers: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO role (team_id, name, persona, skills, face, tools, mcp_servers, backend, "+
				"permission_mode, lead, write, model) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			id, r.Name, r.Persona, string(skills), r.Face, string(tools), string(servers), r.Backend,
			r.PermissionMode, r.Lead, r.Write, r.Model); err != nil {
			return Team{}, fmt.Errorf("insert role %s: %w", r.Name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return Team{}, fmt.Errorf("commit apply team: %w", err)
	}
	return s.Team(ctx, t.Name)
}

// Team returns the team with the name, lead first, or ErrNotFound.
func (s *Store) Team(ctx context.Context, name string) (Team, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT role.name, lead, write, backend, permission_mode, persona, skills, face, model, tools, mcp_servers "+
			"FROM role JOIN team ON team.id = team_id WHERE team.name = ? ORDER BY lead DESC, role.id",
		name)
	if err != nil {
		return Team{}, fmt.Errorf("read team %s: %w", name, err)
	}
	roles, err := scanAll(rows, scanRole)
	if err != nil {
		return Team{}, err
	}
	if len(roles) == 0 {
		return Team{}, fmt.Errorf("team %s: %w", name, ErrNotFound)
	}
	rows, err = s.db.QueryContext(ctx,
		"SELECT from_role, to_role, enabled FROM gate JOIN team ON team.id = team_id "+
			"WHERE team.name = ? ORDER BY gate.id", name)
	if err != nil {
		return Team{}, fmt.Errorf("read gates of team %s: %w", name, err)
	}
	gates, err := scanAll(rows, func(r interface{ Scan(...any) error }) (Gate, error) {
		var g Gate
		return g, r.Scan(&g.From, &g.To, &g.On)
	})
	if err != nil {
		return Team{}, err
	}
	return Team{Name: name, Roles: roles, Gates: gates}, nil
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

// Gated reports whether a wake from role from to role to waits for approval: the team's
// gate for that edge when it lists one, else the default.
func (t Team) Gated(from, to string) bool {
	for _, g := range slices.Concat(t.Gates, _defaultGates) {
		if g.From == from && g.To == to {
			return g.On
		}
	}
	return false
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
		if r.Name == "" {
			return fmt.Errorf("%w: role name is required", ErrInvalid)
		}
		if r.Name == "user" || r.Name == ReefAuthor || !_roleName.MatchString(r.Name) {
			return fmt.Errorf("%w: role name %q must be letters, digits, _ or -, and not user or reef",
				ErrInvalid, r.Name)
		}
		if seen[r.Name] {
			return fmt.Errorf("%w: role %s appears twice", ErrInvalid, r.Name)
		}
		seen[r.Name] = true
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
		if r.Lead {
			leads++
		}
	}
	for _, g := range t.Gates {
		if !seen[g.From] || !seen[g.To] {
			return fmt.Errorf("%w: gate %s to %s names a role team %s does not have",
				ErrInvalid, g.From, g.To, t.Name)
		}
	}
	if leads != 1 {
		return fmt.Errorf("%w: team %s needs exactly one lead role, has %d",
			ErrInvalid, t.Name, leads)
	}
	return nil
}

// cleanSkills returns r's skill names trimmed, failing with ErrInvalid on an empty, repeated or
// oddly spelled one.
func cleanSkills(r Role) ([]string, error) {
	names := make([]string, 0, len(r.Skills))
	for _, name := range r.Skills {
		name = strings.TrimSpace(name)
		switch {
		case name == "":
			return nil, fmt.Errorf("%w: role %s has an empty skill name", ErrInvalid, r.Name)
		case !_skillName.MatchString(name):
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
	if err := r.Scan(&ro.Name, &ro.Lead, &ro.Write, &ro.Backend, &ro.PermissionMode, &ro.Persona,
		&skills, &ro.Face, &ro.Model, &tools, &servers); err != nil {
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
