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

// Teammate is one persistent agent: who it is (persona, skills, face) and what runs it (backend,
// model, provider). Tools, MCPServers, Write and PermissionMode are its default access rules,
// copied into a team_member row when it joins a Team; editing them later leaves memberships
// alone.
type Teammate struct {
	ID             int64             `json:"id"`
	Name           string            `json:"name"`
	Persona        string            `json:"persona"`
	Skills         []string          `json:"skills"`
	Backend        string            `json:"backend"`
	Model          string            `json:"model,omitempty"`
	Provider       string            `json:"provider,omitempty"`
	Face           string            `json:"face,omitempty"`
	Tools          []string          `json:"tools"`
	MCPServers     []agent.MCPServer `json:"mcpServers"`
	Write          bool              `json:"write"`
	PermissionMode string            `json:"permissionMode"`
}

// Membership is a Teammate's place in one Team: whether it leads, and the access rules it runs
// with there.
type Membership struct {
	Lead           bool              `json:"lead"`
	Tools          []string          `json:"tools"`
	MCPServers     []agent.MCPServer `json:"mcpServers"`
	Write          bool              `json:"write"`
	PermissionMode string            `json:"permissionMode"`
}

const _teammateCols = "id, name, persona, skills, backend, model, provider, face, tools, " +
	"mcp_servers, write, permission_mode"

func (t Teammate) role() Role {
	return Role{Name: t.Name, Write: t.Write, Backend: t.Backend, PermissionMode: t.PermissionMode,
		Persona: t.Persona, Skills: t.Skills, Face: t.Face, Model: t.Model, Provider: t.Provider,
		Tools: t.Tools, MCPServers: t.MCPServers}
}

// CreateTeammate adds t, or fails with ErrConflict when the name is taken.
func (s *Store) CreateTeammate(ctx context.Context, t Teammate) (Teammate, error) {
	t, err := s.checkTeammate(ctx, t)
	if err != nil {
		return Teammate{}, err
	}
	skills, tools, servers, err := encodeLists(t.Skills, t.Tools, t.MCPServers)
	if err != nil {
		return Teammate{}, err
	}
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO teammate (name, persona, skills, backend, model, provider, face, tools, "+
			"mcp_servers, write, permission_mode) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		t.Name, t.Persona, skills, t.Backend, t.Model, t.Provider, t.Face, tools, servers, t.Write,
		t.PermissionMode)
	if err != nil {
		if isUnique(err) {
			return Teammate{}, fmt.Errorf("%w: a teammate named %q already exists", ErrConflict, t.Name)
		}
		return Teammate{}, fmt.Errorf("insert teammate: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Teammate{}, fmt.Errorf("insert teammate: %w", err)
	}
	return s.Teammate(ctx, id)
}

// Teammate returns the Teammate with the id, or ErrNotFound.
func (s *Store) Teammate(ctx context.Context, id int64) (Teammate, error) {
	t, err := scanTeammate(s.db.QueryRowContext(ctx,
		"SELECT "+_teammateCols+" FROM teammate WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return Teammate{}, fmt.Errorf("teammate %d: %w", id, ErrNotFound)
	}
	return t, err
}

// Teammates lists every Teammate by name.
func (s *Store) Teammates(ctx context.Context) ([]Teammate, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+_teammateCols+" FROM teammate ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("list teammates: %w", err)
	}
	return scanAll(rows, scanTeammate)
}

// UpdateTeammate replaces every field of the Teammate with t.ID. Teams that hold it keep the
// access rules they copied.
func (s *Store) UpdateTeammate(ctx context.Context, t Teammate) (Teammate, error) {
	t, err := s.checkTeammate(ctx, t)
	if err != nil {
		return Teammate{}, err
	}
	skills, tools, servers, err := encodeLists(t.Skills, t.Tools, t.MCPServers)
	if err != nil {
		return Teammate{}, err
	}
	res, err := s.db.ExecContext(ctx,
		"UPDATE teammate SET name = ?, persona = ?, skills = ?, backend = ?, model = ?, provider = ?, "+
			"face = ?, tools = ?, mcp_servers = ?, write = ?, permission_mode = ? WHERE id = ?",
		t.Name, t.Persona, skills, t.Backend, t.Model, t.Provider, t.Face, tools, servers, t.Write,
		t.PermissionMode, t.ID)
	if err != nil {
		if isUnique(err) {
			return Teammate{}, fmt.Errorf("%w: a teammate named %q already exists", ErrConflict, t.Name)
		}
		return Teammate{}, fmt.Errorf("update teammate: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return Teammate{}, fmt.Errorf("teammate %d: %w", t.ID, ErrNotFound)
	}
	return s.Teammate(ctx, t.ID)
}

// DeleteTeammate removes the Teammate, or fails with ErrInvalid naming the Teams it sits in.
func (s *Store) DeleteTeammate(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete teammate: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var name string
	if err := tx.QueryRowContext(ctx, "SELECT name FROM teammate WHERE id = ?", id).
		Scan(&name); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("teammate %d: %w", id, ErrNotFound)
		}
		return fmt.Errorf("read teammate: %w", err)
	}
	rows, err := tx.QueryContext(ctx,
		"SELECT team.name FROM team_member JOIN team ON team.id = team_id "+
			"WHERE teammate_id = ? ORDER BY team.name", id)
	if err != nil {
		return fmt.Errorf("read teams of teammate: %w", err)
	}
	teams, err := scanAll(rows, func(r interface{ Scan(...any) error }) (string, error) {
		var n string
		return n, r.Scan(&n)
	})
	if err != nil {
		return err
	}
	if len(teams) > 0 {
		return fmt.Errorf("%w: teammate %s is in team %s; remove it from the team first",
			ErrInvalid, name, strings.Join(teams, ", "))
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM teammate WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete teammate: %w", err)
	}
	return tx.Commit()
}

// AddMember puts the Teammate in the team with a copy of its default access rules.
func (s *Store) AddMember(
	ctx context.Context, team string, teammateID int64, lead bool,
) (Role, error) {
	err := s.changeMembers(ctx, team, func(tx *sql.Tx, teamID int64) error {
		if err := checkLead(ctx, tx, teamID, teammateID, lead); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx,
			"INSERT INTO team_member (team_id, teammate_id, name, lead, tools, mcp_servers, write, "+
				"permission_mode) SELECT ?, id, name, ?, tools, mcp_servers, write, permission_mode "+
				"FROM teammate WHERE id = ?", teamID, lead, teammateID)
		if err != nil {
			if isUnique(err) {
				return fmt.Errorf("%w: team %s already has teammate %d or a member with its name",
					ErrConflict, team, teammateID)
			}
			return fmt.Errorf("insert member: %w", err)
		}
		if n, err := res.RowsAffected(); err == nil && n == 0 {
			return fmt.Errorf("%w: unknown teammate %d", ErrInvalid, teammateID)
		}
		return nil
	})
	if err != nil {
		return Role{}, err
	}
	return s.member(ctx, team, teammateID)
}

// EditMember replaces the lead flag and access rules of the Teammate's place in the team.
func (s *Store) EditMember(
	ctx context.Context, team string, teammateID int64, m Membership,
) (Role, error) {
	err := s.changeMembers(ctx, team, func(tx *sql.Tx, teamID int64) error {
		var name, backend string
		err := tx.QueryRowContext(ctx, "SELECT name, backend FROM teammate WHERE id = ?", teammateID).
			Scan(&name, &backend)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return fmt.Errorf("teammate %d: %w", teammateID, ErrNotFound)
		case err != nil:
			return fmt.Errorf("read teammate: %w", err)
		}
		if err := validateRole(Role{Name: name, Backend: backend, Write: m.Write,
			PermissionMode: m.PermissionMode, Tools: m.Tools}); err != nil {
			return err
		}
		if err := checkLead(ctx, tx, teamID, teammateID, m.Lead); err != nil {
			return err
		}
		_, tools, servers, err := encodeLists(nil, m.Tools, m.MCPServers)
		if err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx,
			"UPDATE team_member SET lead = ?, tools = ?, mcp_servers = ?, write = ?, "+
				"permission_mode = ? WHERE team_id = ? AND teammate_id = ?",
			m.Lead, tools, servers, m.Write, m.PermissionMode, teamID, teammateID)
		if err != nil {
			return fmt.Errorf("update member: %w", err)
		}
		if n, err := res.RowsAffected(); err == nil && n == 0 {
			return fmt.Errorf("teammate %d in team %s: %w", teammateID, team, ErrNotFound)
		}
		return nil
	})
	if err != nil {
		return Role{}, err
	}
	return s.member(ctx, team, teammateID)
}

// RemoveMember takes the Teammate out of the team.
func (s *Store) RemoveMember(ctx context.Context, team string, teammateID int64) error {
	return s.changeMembers(ctx, team, func(tx *sql.Tx, teamID int64) error {
		res, err := tx.ExecContext(ctx,
			"DELETE FROM team_member WHERE team_id = ? AND teammate_id = ?", teamID, teammateID)
		if err != nil {
			return fmt.Errorf("delete member: %w", err)
		}
		if n, err := res.RowsAffected(); err == nil && n == 0 {
			return fmt.Errorf("teammate %d in team %s: %w", teammateID, team, ErrNotFound)
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
	return tx.Commit()
}

// checkLead refuses lead when another Teammate already leads the team.
func checkLead(ctx context.Context, tx *sql.Tx, teamID, teammateID int64, lead bool) error {
	if !lead {
		return nil
	}
	var name string
	err := tx.QueryRowContext(ctx,
		"SELECT name FROM team_member WHERE team_id = ? AND lead = 1 AND teammate_id != ?",
		teamID, teammateID).Scan(&name)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil
	case err != nil:
		return fmt.Errorf("read lead: %w", err)
	}
	return fmt.Errorf("%w: the team already has a lead, %s", ErrInvalid, name)
}

func (s *Store) member(ctx context.Context, team string, teammateID int64) (Role, error) {
	t, err := s.Team(ctx, team)
	if err != nil {
		return Role{}, err
	}
	for _, r := range t.Roles {
		if r.TeammateID == teammateID {
			return r, nil
		}
	}
	return Role{}, fmt.Errorf("teammate %d in team %s: %w", teammateID, team, ErrNotFound)
}

func (s *Store) checkTeammate(ctx context.Context, t Teammate) (Teammate, error) {
	if err := validateRole(t.role()); err != nil {
		return Teammate{}, err
	}
	skills, err := cleanSkills(t.role())
	if err != nil {
		return Teammate{}, err
	}
	t.Skills = skills
	if _, err := s.RoleBin(ctx, t.role()); err != nil {
		return Teammate{}, err
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

func scanTeammate(r interface{ Scan(...any) error }) (Teammate, error) {
	var t Teammate
	var skills, tools, servers string
	if err := r.Scan(&t.ID, &t.Name, &t.Persona, &skills, &t.Backend, &t.Model, &t.Provider,
		&t.Face, &tools, &servers, &t.Write, &t.PermissionMode); err != nil {
		return Teammate{}, err
	}
	if err := json.Unmarshal([]byte(skills), &t.Skills); err != nil {
		return Teammate{}, fmt.Errorf("teammate %s: bad skills: %w", t.Name, err)
	}
	if err := json.Unmarshal([]byte(tools), &t.Tools); err != nil {
		return Teammate{}, fmt.Errorf("teammate %s: bad tools: %w", t.Name, err)
	}
	if err := json.Unmarshal([]byte(servers), &t.MCPServers); err != nil {
		return Teammate{}, fmt.Errorf("teammate %s: bad mcp servers: %w", t.Name, err)
	}
	return t, nil
}
