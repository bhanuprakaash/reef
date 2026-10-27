package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// SpaceTeam is the roster of Space id: its own members, lead first, as the roles a turn runs
// with. The access rules are the ones copied when the Space was made or the member added, so
// editing a Team or a Teammate later leaves them alone. Name is the Team's, empty for a Space
// with no Team.
func (s *Store) SpaceTeam(ctx context.Context, id int64) (Team, error) {
	c, err := s.Space(ctx, id)
	if err != nil {
		return Team{}, err
	}
	roles, err := s.roles(ctx, "space_member", "space_id", id)
	if err != nil {
		return Team{}, fmt.Errorf("read members of Space %d: %w", id, err)
	}
	return Team{Name: c.Team, Roles: roles}, nil
}

// AddSpaceMember puts the Teammate in Space id with a copy of its default access rules. A
// direct conversation keeps its one Teammate.
func (s *Store) AddSpaceMember(ctx context.Context, id, teammateID int64) (Role, error) {
	c, err := s.Space(ctx, id)
	if err != nil {
		return Role{}, err
	}
	if c.Kind == KindConversation {
		return Role{}, fmt.Errorf("%w: Space %d is a direct conversation with one Teammate", ErrInvalid, id)
	}
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO space_member (space_id, teammate_id, name, lead, tools, mcp_servers, write, "+
			"permission_mode) SELECT ?, id, name, 0, tools, mcp_servers, write, permission_mode "+
			"FROM teammate WHERE id = ?", id, teammateID)
	if err != nil {
		if isUnique(err) {
			return Role{}, fmt.Errorf("%w: Space %d already has teammate %d or a member with its name",
				ErrConflict, id, teammateID)
		}
		return Role{}, fmt.Errorf("insert Space member: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return Role{}, fmt.Errorf("%w: unknown teammate %d", ErrInvalid, teammateID)
	}
	roles, err := s.roles(ctx, "space_member", "space_id", id)
	if err != nil {
		return Role{}, fmt.Errorf("read members of Space %d: %w", id, err)
	}
	for _, r := range roles {
		if r.TeammateID == teammateID {
			return r, nil
		}
	}
	return Role{}, fmt.Errorf("teammate %d in Space %d: %w", teammateID, id, ErrNotFound)
}

// SpaceMemberTeammateID returns the id of the Teammate that holds role in Space id, or
// ErrNotFound.
func (s *Store) SpaceMemberTeammateID(ctx context.Context, id int64, role string) (int64, error) {
	var teammateID int64
	err := s.db.QueryRowContext(ctx,
		"SELECT teammate_id FROM space_member WHERE space_id = ? AND name = ?", id, role).
		Scan(&teammateID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("role %s in Space %d: %w", role, id, ErrNotFound)
	}
	if err != nil {
		return 0, fmt.Errorf("read teammate of role %s: %w", role, err)
	}
	return teammateID, nil
}
