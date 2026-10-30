package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/bhanuprakaash/reef/agent"
)

// Bot is one persistent agent and its configuration.
type Bot struct {
	ID             int64             `json:"id"`
	Name           string            `json:"name"`
	Description    string            `json:"description"`
	Skills         []string          `json:"skills"`
	Backend        string            `json:"backend"`
	Model          string            `json:"model,omitempty"`
	Provider       string            `json:"provider,omitempty"`
	Face           string            `json:"face,omitempty"`
	Colour         string            `json:"colour,omitempty"`
	Tools          []string          `json:"tools"`
	MCPServers     []agent.MCPServer `json:"mcpServers"`
	Write          bool              `json:"write"`
	PermissionMode string            `json:"permissionMode"`
}

type Membership struct {
	Lead bool `json:"lead"`
}

const _botCols = "id, name, description, skills, backend, model, provider, face, colour, tools, " +
	"mcp_servers, write, permission_mode"

func (t Bot) role() Role {
	return Role{Name: t.Name, Write: t.Write, Backend: t.Backend, PermissionMode: t.PermissionMode,
		Description: t.Description, Skills: t.Skills, Face: t.Face, Colour: t.Colour, Model: t.Model, Provider: t.Provider,
		Tools: t.Tools, MCPServers: t.MCPServers}
}

// CreateBot adds t, or fails with ErrConflict when the name is taken.
func (s *Store) CreateBot(ctx context.Context, t Bot) (Bot, error) {
	t, err := s.checkBot(ctx, t)
	if err != nil {
		return Bot{}, err
	}
	skills, tools, servers, err := encodeLists(t.Skills, t.Tools, t.MCPServers)
	if err != nil {
		return Bot{}, err
	}
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO bot (name, description, skills, backend, model, provider, face, colour, "+
			"tools, mcp_servers, write, permission_mode) "+
			"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		t.Name, t.Description, skills, t.Backend, t.Model, t.Provider, t.Face, t.Colour, tools, servers,
		t.Write, t.PermissionMode)
	if err != nil {
		if isUnique(err) {
			return Bot{}, fmt.Errorf("%w: a bot named %q already exists", ErrConflict, t.Name)
		}
		return Bot{}, fmt.Errorf("insert bot: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Bot{}, fmt.Errorf("insert bot: %w", err)
	}
	return s.Bot(ctx, id)
}

// Bot returns the Bot with the id, or ErrNotFound.
func (s *Store) Bot(ctx context.Context, id int64) (Bot, error) {
	t, err := scanBot(s.db.QueryRowContext(ctx,
		"SELECT "+_botCols+" FROM bot WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return Bot{}, fmt.Errorf("bot %d: %w", id, ErrNotFound)
	}
	return t, err
}

// Bots lists every Bot by name.
func (s *Store) Bots(ctx context.Context) ([]Bot, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+_botCols+" FROM bot ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("list bots: %w", err)
	}
	return scanAll(rows, scanBot)
}

// UpdateBot replaces every field of the Bot with t.ID.
func (s *Store) UpdateBot(ctx context.Context, t Bot) (Bot, error) {
	t, err := s.checkBot(ctx, t)
	if err != nil {
		return Bot{}, err
	}
	skills, tools, servers, err := encodeLists(t.Skills, t.Tools, t.MCPServers)
	if err != nil {
		return Bot{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Bot{}, fmt.Errorf("begin update bot: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx,
		"UPDATE bot SET name = ?, description = ?, skills = ?, backend = ?, model = ?, provider = ?, "+
			"face = ?, colour = ?, tools = ?, mcp_servers = ?, write = ?, permission_mode = ? "+
			"WHERE id = ?",
		t.Name, t.Description, skills, t.Backend, t.Model, t.Provider, t.Face, t.Colour, tools, servers,
		t.Write, t.PermissionMode, t.ID)
	if err != nil {
		if isUnique(err) {
			return Bot{}, fmt.Errorf("%w: a bot named %q already exists", ErrConflict, t.Name)
		}
		return Bot{}, fmt.Errorf("update bot: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return Bot{}, fmt.Errorf("bot %d: %w", t.ID, ErrNotFound)
	}
	for _, table := range []string{"team_member", "space_member"} {
		if _, err := tx.ExecContext(ctx, "UPDATE "+table+" SET name = ? WHERE bot_id = ?", t.Name, t.ID); err != nil {
			return Bot{}, fmt.Errorf("update bot membership: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return Bot{}, fmt.Errorf("commit update bot: %w", err)
	}
	return s.Bot(ctx, t.ID)
}

// DeleteBot removes the Bot, or fails with ErrInvalid naming the Teams it sits in.
func (s *Store) DeleteBot(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete bot: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var name string
	if err := tx.QueryRowContext(ctx, "SELECT name FROM bot WHERE id = ?", id).
		Scan(&name); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("bot %d: %w", id, ErrNotFound)
		}
		return fmt.Errorf("read bot: %w", err)
	}
	rows, err := tx.QueryContext(ctx,
		"SELECT team.name FROM team_member JOIN team ON team.id = team_id "+
			"WHERE bot_id = ? ORDER BY team.name", id)
	if err != nil {
		return fmt.Errorf("read teams of bot: %w", err)
	}
	teams, err := scanAll(rows, func(r interface{ Scan(...any) error }) (string, error) {
		var n string
		return n, r.Scan(&n)
	})
	if err != nil {
		return err
	}
	if len(teams) > 0 {
		return fmt.Errorf("%w: bot %s is in team %s; remove it from the team first",
			ErrInvalid, name, strings.Join(teams, ", "))
	}
	var spaces int
	if err := tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM space_member WHERE bot_id = ?", id).Scan(&spaces); err != nil {
		return fmt.Errorf("read spaces of bot: %w", err)
	}
	if spaces > 0 {
		return fmt.Errorf("%w: bot %s is a member of %d Spaces; delete them first",
			ErrInvalid, name, spaces)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM bot WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete bot: %w", err)
	}
	return tx.Commit()
}

// AddMember puts the Bot in the team with a copy of its default access rules.
func (s *Store) AddMember(
	ctx context.Context, team string, botID int64, lead bool,
) (Role, error) {
	err := s.changeMembers(ctx, team, func(tx *sql.Tx, teamID int64) error {
		if err := checkLead(ctx, tx, teamID, botID, lead); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx,
			"INSERT INTO team_member (team_id, bot_id, name, lead, tools, mcp_servers, write, "+
				"permission_mode) SELECT ?, id, name, ?, tools, mcp_servers, write, permission_mode "+
				"FROM bot WHERE id = ?", teamID, lead, botID)
		if err != nil {
			if isUnique(err) {
				return fmt.Errorf("%w: team %s already has bot %d or a member with its name",
					ErrConflict, team, botID)
			}
			return fmt.Errorf("insert member: %w", err)
		}
		if n, err := res.RowsAffected(); err == nil && n == 0 {
			return fmt.Errorf("%w: unknown bot %d", ErrInvalid, botID)
		}
		return nil
	})
	if err != nil {
		return Role{}, err
	}
	return s.member(ctx, team, botID)
}

// EditMember replaces the lead flag of the Bot's place in the team.
func (s *Store) EditMember(
	ctx context.Context, team string, botID int64, m Membership,
) (Role, error) {
	err := s.changeMembers(ctx, team, func(tx *sql.Tx, teamID int64) error {
		if err := checkLead(ctx, tx, teamID, botID, m.Lead); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx,
			"UPDATE team_member SET lead = ? WHERE team_id = ? AND bot_id = ?",
			m.Lead, teamID, botID)
		if err != nil {
			return fmt.Errorf("update member: %w", err)
		}
		if n, err := res.RowsAffected(); err == nil && n == 0 {
			return fmt.Errorf("bot %d in team %s: %w", botID, team, ErrNotFound)
		}
		return nil
	})
	if err != nil {
		return Role{}, err
	}
	return s.member(ctx, team, botID)
}

// RemoveMember takes the Bot out of the team.
func (s *Store) RemoveMember(ctx context.Context, team string, botID int64) error {
	return s.changeMembers(ctx, team, func(tx *sql.Tx, teamID int64) error {
		res, err := tx.ExecContext(ctx,
			"DELETE FROM team_member WHERE team_id = ? AND bot_id = ?", teamID, botID)
		if err != nil {
			return fmt.Errorf("delete member: %w", err)
		}
		if n, err := res.RowsAffected(); err == nil && n == 0 {
			return fmt.Errorf("bot %d in team %s: %w", botID, team, ErrNotFound)
		}
		return nil
	})
}

// changeMembers runs change in one transaction on the named team. The solo team keeps its one
// lead, so it cannot be changed.
func (s *Store) changeMembers(
	ctx context.Context, team string, change func(tx *sql.Tx, teamID int64) error,
) error {
	if team == SoloTeam {
		return fmt.Errorf("%w: team %s keeps its one lead", ErrInvalid, SoloTeam)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin change members: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var teamID int64
	err = tx.QueryRowContext(ctx, "SELECT id FROM team WHERE name = ?", team).Scan(&teamID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("team %s: %w", team, ErrNotFound)
	case err != nil:
		return fmt.Errorf("read team %s: %w", team, err)
	}
	if err := change(tx, teamID); err != nil {
		return err
	}
	var leads int
	if err := tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM team_member WHERE team_id = ? AND lead = 1", teamID).Scan(&leads); err != nil {
		return fmt.Errorf("read lead: %w", err)
	}
	if leads != 1 {
		return fmt.Errorf("%w: team %s needs exactly one lead, has %d", ErrInvalid, team, leads)
	}
	return tx.Commit()
}

// checkLead refuses lead when another Bot already leads the team.
func checkLead(ctx context.Context, tx *sql.Tx, teamID, botID int64, lead bool) error {
	if !lead {
		return nil
	}
	var name string
	err := tx.QueryRowContext(ctx,
		"SELECT name FROM team_member WHERE team_id = ? AND lead = 1 AND bot_id != ?",
		teamID, botID).Scan(&name)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil
	case err != nil:
		return fmt.Errorf("read lead: %w", err)
	}
	return fmt.Errorf("%w: the team already has a lead, %s", ErrInvalid, name)
}

func (s *Store) member(ctx context.Context, team string, botID int64) (Role, error) {
	t, err := s.Team(ctx, team)
	if err != nil {
		return Role{}, err
	}
	for _, r := range t.Roles {
		if r.BotID == botID {
			return r, nil
		}
	}
	return Role{}, fmt.Errorf("bot %d in team %s: %w", botID, team, ErrNotFound)
}

func (s *Store) checkBot(ctx context.Context, t Bot) (Bot, error) {
	if err := validateRole(t.role()); err != nil {
		return Bot{}, err
	}
	skills, err := cleanSkills(t.role())
	if err != nil {
		return Bot{}, err
	}
	t.Skills = skills
	if _, err := s.RoleBin(ctx, t.role()); err != nil {
		return Bot{}, err
	}
	return t, nil
}

func encodeLists(
	skills, tools []string, servers []agent.MCPServer,
) (string, string, string, error) {
	sk, err := json.Marshal(nonNil(skills))
	if err != nil {
		return "", "", "", fmt.Errorf("encode skills: %w", err)
	}
	tl, err := json.Marshal(nonNil(tools))
	if err != nil {
		return "", "", "", fmt.Errorf("encode tools: %w", err)
	}
	sv, err := json.Marshal(nonNil(servers))
	if err != nil {
		return "", "", "", fmt.Errorf("encode mcp servers: %w", err)
	}
	return string(sk), string(tl), string(sv), nil
}

func scanBot(r interface{ Scan(...any) error }) (Bot, error) {
	var t Bot
	var skills, tools, servers string
	if err := r.Scan(&t.ID, &t.Name, &t.Description, &skills, &t.Backend, &t.Model, &t.Provider,
		&t.Face, &t.Colour, &tools, &servers, &t.Write, &t.PermissionMode); err != nil {
		return Bot{}, err
	}
	if err := json.Unmarshal([]byte(skills), &t.Skills); err != nil {
		return Bot{}, fmt.Errorf("bot %s: bad skills: %w", t.Name, err)
	}
	if err := json.Unmarshal([]byte(tools), &t.Tools); err != nil {
		return Bot{}, fmt.Errorf("bot %s: bad tools: %w", t.Name, err)
	}
	if err := json.Unmarshal([]byte(servers), &t.MCPServers); err != nil {
		return Bot{}, fmt.Errorf("bot %s: bad mcp servers: %w", t.Name, err)
	}
	return t, nil
}
