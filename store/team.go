package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/skill"
)

// SoloTeam is the lead-only team a Space gets when none is named.
const SoloTeam = "solo"

// Team is a named roster of Bots, at most one of them the suggested coordinator.
type Team struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	Responsibility string `json:"responsibility"`
	Roles          []Role `json:"roles"`
}

// Member places an existing Bot in a Team, and Lead marks the coordinator.
type Member struct {
	BotID int64 `json:"botId"`
	Lead  bool  `json:"lead"`
}

// TeamSpec is the Team to create or replace: a name, a responsibility and the Bots in it. A
// Team holds no configuration of its own; each Bot keeps its one profile.
type TeamSpec struct {
	Name           string   `json:"name"`
	Responsibility string   `json:"responsibility"`
	Roles          []Member `json:"roles"`
}

func faces() []string {
	return []string{
		"jellyfish", "whale", "penguin", "shark", "octopus", "turtle", "seahorse", "otter",
		"clownfish", "manta-ray", "pebble",
	}
}

func colours() []string {
	return []string{"orange", "red", "yellow", "green", "teal", "blue", "violet", "pink"}
}

// Role is a Bot as it sits in a Team or Space: its profile, read from the Bot, and Lead from its
// membership. Face picks the creature the window draws for the role and Colour
// its body colour; empty Colour is the creature's own. Model names the model the backend runs the role on; empty is
// the backend's default. Provider names an added install (see Provider) whose binary the role
// runs; empty is the claude or codex found on PATH. Tools are the backend's allowed tools. Write says whether the
// role may change files: without it the backend's tools and sandbox are held to read-only.
type Role struct {
	BotID          int64             `json:"botId,omitempty"`
	Name           string            `json:"name"`
	Lead           bool              `json:"lead"`
	Write          bool              `json:"write"`
	Backend        string            `json:"backend"`
	PermissionMode string            `json:"permissionMode"`
	Description    string            `json:"description"`
	Skills         []string          `json:"skills"`
	Face           string            `json:"face,omitempty"`
	Colour         string            `json:"colour,omitempty"`
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

// ApplyTeam creates team t, or replaces the members of the team with that name. Every member
// names an existing Bot.
func (s *Store) ApplyTeam(ctx context.Context, t TeamSpec) (Team, error) {
	return s.applyTeam(ctx, t, false)
}

// CreateTeam creates team t, failing with ErrConflict when a team with that name already
// exists. The check and the insert are one statement, so two concurrent calls for the same
// name cannot both succeed.
func (s *Store) CreateTeam(ctx context.Context, t TeamSpec) (Team, error) {
	return s.applyTeam(ctx, t, true)
}

func (s *Store) applyTeam(ctx context.Context, t TeamSpec, createOnly bool) (Team, error) {
	if err := t.validate(); err != nil {
		return Team{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Team{}, fmt.Errorf("begin apply team: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if createOnly {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO team (name, responsibility) VALUES (?, ?)",
			t.Name, t.Responsibility); err != nil {
			if isUnique(err) {
				return Team{}, fmt.Errorf("%w: a team named %q already exists", ErrConflict, t.Name)
			}
			return Team{}, fmt.Errorf("insert team: %w", err)
		}
	} else if _, err := tx.ExecContext(ctx,
		"INSERT INTO team (name, responsibility) VALUES (?, ?) ON CONFLICT (name) DO UPDATE "+
			"SET responsibility = excluded.responsibility", t.Name, t.Responsibility); err != nil {
		return Team{}, fmt.Errorf("insert team: %w", err)
	}
	var id int64
	if err := tx.QueryRowContext(ctx, "SELECT id FROM team WHERE name = ?", t.Name).
		Scan(&id); err != nil {
		return Team{}, fmt.Errorf("read team: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM team_member WHERE team_id = ?", id); err != nil {
		return Team{}, fmt.Errorf("delete members: %w", err)
	}
	for _, m := range t.Roles {
		res, err := tx.ExecContext(ctx,
			"INSERT INTO team_member (team_id, bot_id, lead) SELECT ?, id, ? FROM bot WHERE id = ?",
			id, m.Lead, m.BotID)
		if err != nil {
			return Team{}, fmt.Errorf("insert member %d: %w", m.BotID, err)
		}
		if n, err := res.RowsAffected(); err == nil && n == 0 {
			return Team{}, fmt.Errorf("%w: unknown bot %d", ErrInvalid, m.BotID)
		}
	}
	if t.Name == SoloTeam {
		var leads int
		if err := tx.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM team_member JOIN bot ON bot.id = bot_id "+
				"WHERE team_id = ? AND lead = 1 AND bot.name = 'lead'", id).Scan(&leads); err != nil {
			return Team{}, fmt.Errorf("read lead: %w", err)
		}
		if leads != 1 || len(t.Roles) != 1 {
			return Team{}, fmt.Errorf("%w: team %s must have exactly one lead, the bot named lead",
				ErrInvalid, SoloTeam)
		}
	}
	if err := tx.Commit(); err != nil {
		return Team{}, fmt.Errorf("commit apply team: %w", err)
	}
	return s.Team(ctx, t.Name)
}

// Team returns the team with the name, lead first, or ErrNotFound.
func (s *Store) Team(ctx context.Context, name string) (Team, error) {
	var teamID int64
	var responsibility string
	switch err := s.db.QueryRowContext(ctx,
		"SELECT id, responsibility FROM team WHERE name = ?", name).
		Scan(&teamID, &responsibility); {
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
	return Team{ID: teamID, Name: name, Responsibility: responsibility, Roles: roles}, nil
}

// TeamByID returns the team with the id, lead first, or ErrNotFound.
func (s *Store) TeamByID(ctx context.Context, id int64) (Team, error) {
	var name string
	switch err := s.db.QueryRowContext(ctx, "SELECT name FROM team WHERE id = ?", id).
		Scan(&name); {
	case errors.Is(err, sql.ErrNoRows):
		return Team{}, fmt.Errorf("team %d: %w", id, ErrNotFound)
	case err != nil:
		return Team{}, fmt.Errorf("read team %d: %w", id, err)
	}
	return s.Team(ctx, name)
}

// isStopWord is true for the words left out of a SuggestTeam match: they say nothing about who
// owns the work.
func isStopWord(w string) bool {
	switch w {
	case "a", "an", "and", "are", "as", "at", "be", "by", "for", "from", "in", "is", "it", "its",
		"of", "on", "or", "that", "the", "this", "to", "was", "with":
		return true
	}
	return false
}

// SuggestTeam returns the Team whose responsibility text best matches any word of text, or
// the default team, solo, when none does. It is ErrNotFound only when solo is gone too.
func (s *Store) SuggestTeam(ctx context.Context, text string) (Team, error) {
	var words []string
	for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if !isStopWord(w) {
			words = append(words, `"`+w+`"`)
		}
	}
	name := SoloTeam
	if len(words) > 0 {
		err := s.db.QueryRowContext(ctx,
			"SELECT team.name FROM team_fts JOIN team ON team.id = team_fts.rowid "+
				"WHERE team_fts MATCH ? ORDER BY bm25(team_fts), team.id LIMIT 1",
			strings.Join(words, " OR ")).Scan(&name)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return Team{}, fmt.Errorf("match teams: %w", err)
		}
	}
	return s.Team(ctx, name)
}

// roles lists the members in table (team_member or space_member) whose key column is id, lead
// first, with the Bot's configuration.
func (s *Store) roles(ctx context.Context, table, key string, id int64) ([]Role, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT bot.id, bot.name, member.lead, bot.write, bot.backend, "+
			"bot.permission_mode, bot.description, bot.skills, bot.face, bot.colour, bot.model, "+
			"bot.provider, bot.tools, bot.mcp_servers "+
			"FROM "+table+" member JOIN bot ON bot.id = member.bot_id "+
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
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		teams = append(teams, t)
	}
	return teams, nil
}

// DeleteTeam removes the named Team. Its Bots, its Spaces and their members stay.
func (s *Store) DeleteTeam(ctx context.Context, name string) error {
	if name == SoloTeam {
		return fmt.Errorf("%w: team %s is the default team and cannot be deleted", ErrInvalid, SoloTeam)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete team: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var id int64
	err = tx.QueryRowContext(ctx, "SELECT id FROM team WHERE name = ?", name).Scan(&id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("team %s: %w", name, ErrNotFound)
	case err != nil:
		return fmt.Errorf("read team %s: %w", name, err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE space SET team_id = NULL WHERE team_id = ?", id); err != nil {
		return fmt.Errorf("detach Spaces from team %s: %w", name, err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM team WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete team %s: %w", name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete team %s: %w", name, err)
	}
	return nil
}

func (t TeamSpec) validate() error {
	if t.Name == "" {
		return fmt.Errorf("%w: team name is required", ErrInvalid)
	}
	if !_roleName.MatchString(t.Name) {
		return fmt.Errorf("%w: team name %q must be letters, digits, _ or -", ErrInvalid, t.Name)
	}
	leads, seen := 0, make(map[int64]bool, len(t.Roles))
	for _, m := range t.Roles {
		if m.BotID == 0 {
			return fmt.Errorf("%w: every member needs a botId", ErrInvalid)
		}
		if seen[m.BotID] {
			return fmt.Errorf("%w: bot %d appears twice", ErrInvalid, m.BotID)
		}
		seen[m.BotID] = true
		if m.Lead {
			leads++
		}
	}
	if leads > 1 {
		return fmt.Errorf("%w: team %s has %d coordinator suggestions, at most one", ErrInvalid, t.Name, leads)
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
	if r.Colour != "" && !slices.Contains(colours(), r.Colour) {
		return fmt.Errorf("%w: role %s: colour %q must be one of %s",
			ErrInvalid, r.Name, r.Colour, strings.Join(colours(), ", "))
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
	if err := r.Scan(&ro.BotID, &ro.Name, &ro.Lead, &ro.Write, &ro.Backend, &ro.PermissionMode, &ro.Description,
		&skills, &ro.Face, &ro.Colour, &ro.Model, &ro.Provider, &tools, &servers); err != nil {
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
