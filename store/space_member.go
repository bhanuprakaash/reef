package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
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

// AddSpaceMember puts the Bot in Space id. A
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
		"INSERT INTO space_member (space_id, bot_id, lead) SELECT ?, id, 0 FROM bot WHERE id = ?",
		id, botID)
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

// AddSpaceCrew puts every Bot of the Team that is not yet in Space id into it, with no lead.
// The Space keeps its coordinator. It returns the Space's members.
func (s *Store) AddSpaceCrew(ctx context.Context, id int64, crew string) ([]Role, error) {
	c, err := s.Space(ctx, id)
	if err != nil {
		return nil, err
	}
	if c.Kind == KindConversation {
		return nil, fmt.Errorf("%w: Space %d is a direct conversation with one Bot", ErrInvalid, id)
	}
	if _, err := s.Team(ctx, crew); err != nil {
		return nil, err
	}
	if _, err := s.db.ExecContext(ctx,
		"INSERT OR IGNORE INTO space_member (space_id, bot_id, lead) "+
			"SELECT ?, bot_id, 0 FROM team_member WHERE team_id = (SELECT id FROM team WHERE name = ?) "+
			"ORDER BY id", id, crew); err != nil {
		return nil, fmt.Errorf("insert Space crew: %w", err)
	}
	return s.roles(ctx, "space_member", "space_id", id)
}

// CreateSpaceOf adds c with exactly the Bots as members; the coordinator, or the first Bot when
// it is 0, leads. c.Team, when set, names the Team the Space was started from.
func (s *Store) CreateSpaceOf(ctx context.Context, c Space, bots []int64, coordinator int64) (Space, error) {
	if err := c.validate(); err != nil {
		return Space{}, err
	}
	if len(bots) == 0 {
		return Space{}, fmt.Errorf("%w: a Space needs at least one Bot", ErrInvalid)
	}
	if coordinator == 0 {
		coordinator = bots[0]
	}
	if !slices.Contains(bots, coordinator) {
		return Space{}, fmt.Errorf("%w: coordinator %d is not among the Bots", ErrInvalid, coordinator)
	}
	var teamID *int64
	if c.Team != "" {
		var id int64
		if err := s.db.QueryRowContext(ctx, "SELECT id FROM team WHERE name = ?", c.Team).Scan(&id); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return Space{}, fmt.Errorf("%w: unknown team %s", ErrInvalid, c.Team)
			}
			return Space{}, fmt.Errorf("read team: %w", err)
		}
		teamID = &id
	}
	c.Kind = KindFree
	if c.WorkItemID != nil {
		c.Kind = KindWorkItem
	}
	return s.insertSpace(ctx, c, teamID, func(tx *sql.Tx, id int64) error {
		seen := map[int64]bool{}
		for _, b := range bots {
			if seen[b] {
				continue
			}
			seen[b] = true
			res, err := tx.ExecContext(ctx,
				"INSERT OR IGNORE INTO space_member (space_id, bot_id, lead) SELECT ?, id, id = ? FROM bot WHERE id = ?",
				id, coordinator, b)
			if err != nil {
				return err
			}
			if n, err := res.RowsAffected(); err == nil && n == 0 {
				return fmt.Errorf("%w: unknown bot %d", ErrInvalid, b)
			}
		}
		return nil
	})
}

// MoveConversation turns the direct conversation id into a free Space with no Team: the
// conversation's Bot leads it and the Bot botID joins. The conversation is marked moved to the new Space, which it returns, in the
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
	moved := Space{ProjectID: conv.ProjectID, Name: conv.Name, Role: team.Roles[0].Name, Kind: KindFree}
	return s.insertSpace(ctx, moved, nil, func(tx *sql.Tx, newID int64) error {
		res, err := tx.ExecContext(ctx,
			"INSERT INTO space_member (space_id, bot_id, lead) SELECT ?, id, id = ? "+
				"FROM bot WHERE id IN (?, ?)", newID, leadID, leadID, botID)
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
