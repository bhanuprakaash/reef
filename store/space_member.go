package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// SpaceTeam is the roster of Space id: its own members, lead first, as the roles a turn runs
// with. Name is the Team's, empty for a Space
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

// AddSpaceMember puts the Bot in Space id with a copy of its default access rules. A
// direct conversation keeps its one Bot.
func (s *Store) AddSpaceMember(ctx context.Context, id, botID int64) (Role, error) {
	c, err := s.Space(ctx, id)
	if err != nil {
		return Role{}, err
	}
	if c.Kind == KindConversation {
		return Role{}, fmt.Errorf("%w: Space %d is a direct conversation with one Bot", ErrInvalid, id)
	}
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO space_member (space_id, bot_id, name, lead, tools, mcp_servers, write, "+
			"permission_mode) SELECT ?, id, name, 0, tools, mcp_servers, write, permission_mode "+
			"FROM bot WHERE id = ?", id, botID)
	if err != nil {
		if isUnique(err) {
			return Role{}, fmt.Errorf("%w: Space %d already has bot %d or a member with its name",
				ErrConflict, id, botID)
		}
		return Role{}, fmt.Errorf("insert Space member: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return Role{}, fmt.Errorf("%w: unknown bot %d", ErrInvalid, botID)
	}
	roles, err := s.roles(ctx, "space_member", "space_id", id)
	if err != nil {
		return Role{}, fmt.Errorf("read members of Space %d: %w", id, err)
	}
	for _, r := range roles {
		if r.BotID == botID {
			return r, nil
		}
	}
	return Role{}, fmt.Errorf("bot %d in Space %d: %w", botID, id, ErrNotFound)
}

// MoveConversation turns the direct conversation id into a free Space with no Team: the
// conversation's Bot leads it and the Bot botID joins, each with its default
// access rules. The conversation is marked moved to the new Space, which it returns, in the
// same transaction.
func (s *Store) MoveConversation(ctx context.Context, id, botID int64) (Space, error) {
	conv, err := s.Space(ctx, id)
	if err != nil {
		return Space{}, err
	}
	switch {
	case conv.Kind != KindConversation:
		return Space{}, fmt.Errorf("%w: Space %d is not a direct conversation", ErrInvalid, id)
	case conv.MovedTo != nil:
		return Space{}, fmt.Errorf("%w: conversation %d already moved to Space %d",
			ErrConflict, id, *conv.MovedTo)
	}
	team, err := s.SpaceTeam(ctx, id)
	if err != nil {
		return Space{}, err
	}
	if len(team.Roles) == 0 {
		return Space{}, fmt.Errorf("%w: conversation has no lead", ErrInvalid)
	}
	leadID := team.Roles[0].BotID
	moved := Space{ProjectID: conv.ProjectID, Name: conv.Name, Role: team.Roles[0].Name, Backend: conv.Backend,
		PermissionMode: conv.PermissionMode, Kind: KindFree}
	return s.insertSpace(ctx, moved, nil, func(tx *sql.Tx, newID int64) error {
		res, err := tx.ExecContext(ctx,
			"INSERT INTO space_member (space_id, bot_id, name, lead, tools, mcp_servers, write, "+
				"permission_mode) SELECT ?, id, name, id = ?, tools, mcp_servers, write, "+
				"permission_mode FROM bot WHERE id IN (?, ?)", newID, leadID, leadID, botID)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err == nil && n < 2 {
			return fmt.Errorf("%w: bot %d is unknown or leads the conversation", ErrInvalid, botID)
		}
		res, err = tx.ExecContext(ctx,
			"UPDATE space SET moved_to = ? WHERE id = ? AND moved_to IS NULL", newID, id)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err == nil && n == 0 {
			return fmt.Errorf("%w: conversation %d already moved", ErrConflict, id)
		}
		return nil
	})
}

// SpaceMemberBotID returns the id of the Bot that holds role in Space id, or
// ErrNotFound.
func (s *Store) SpaceMemberBotID(ctx context.Context, id int64, role string) (int64, error) {
	var botID int64
	err := s.db.QueryRowContext(ctx,
		"SELECT member.bot_id FROM space_member member JOIN bot ON bot.id = member.bot_id "+
			"WHERE member.space_id = ? AND bot.name = ?", id, role).
		Scan(&botID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("role %s in Space %d: %w", role, id, ErrNotFound)
	}
	if err != nil {
		return 0, fmt.Errorf("read bot of role %s: %w", role, err)
	}
	return botID, nil
}
