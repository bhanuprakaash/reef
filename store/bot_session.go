package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	BotSessionActive   = "active"
	BotSessionReplaced = "replaced"
	BotSessionRevoked  = "revoked"
)

// AdoptedSkill is one picked skill as a BotSession adopted it. Dir holds the copy of its files and
// Version names the content copied: the library version of a Reef skill, a content hash of a skill
// found on disk. A skill that is not Compatible with the backend has no copy and Reason says why.
type AdoptedSkill struct {
	Ref         string `json:"ref"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version,omitempty"`
	Dir         string `json:"dir,omitempty"`
	Compatible  bool   `json:"compatible"`
	Reason      string `json:"reason,omitempty"`
}

// BotSession is the settings one Bot adopted in one Space, as they were when it adopted them. It
// is execution history: editing the Bot changes no BotSession, and a Space holds one active
// BotSession per member Bot.
type BotSession struct {
	ID             int64          `json:"id"`
	SpaceID        int64          `json:"spaceId"`
	BotID          int64          `json:"botId"`
	Backend        string         `json:"backend"`
	Provider       string         `json:"provider,omitempty"`
	Model          string         `json:"model,omitempty"`
	Description    string         `json:"description"`
	Skills         []AdoptedSkill `json:"skills"`
	Tools          []string       `json:"tools"`
	Connections    []Connection   `json:"connections"`
	Write          bool           `json:"write"`
	PermissionMode string         `json:"permissionMode"`
	Status         string         `json:"status"`
	CreatedAt      time.Time      `json:"createdAt"`
	EndedAt        *time.Time     `json:"endedAt"`
}

const _botSessionCols = "id, space_id, bot_id, backend, provider, model, description, skills, tools, " +
	"connections, write, permission_mode, status, created_at, ended_at"

// Apply returns r with the settings the BotSession adopted in place of the Bot's own. Name, Lead,
// Face and Colour stay.
func (b BotSession) Apply(r Role) Role {
	r.Backend, r.Provider, r.Model, r.Description = b.Backend, b.Provider, b.Model, b.Description
	r.Skills = make([]string, len(b.Skills))
	for i, k := range b.Skills {
		r.Skills[i] = k.Ref
	}
	r.Tools, r.Connections = b.Tools, b.Connections
	r.Write, r.PermissionMode = b.Write, b.PermissionMode
	return r
}

// ActiveBotSession returns the active BotSession of the Bot in the Space, or ErrNotFound.
func (s *Store) ActiveBotSession(ctx context.Context, spaceID, botID int64) (BotSession, error) {
	b, err := scanBotSession(s.db.QueryRowContext(ctx,
		"SELECT "+_botSessionCols+" FROM bot_session WHERE space_id = ? AND bot_id = ? AND status = ?",
		spaceID, botID, BotSessionActive))
	if errors.Is(err, sql.ErrNoRows) {
		return BotSession{}, fmt.Errorf("bot %d session in Space %d: %w", botID, spaceID, ErrNotFound)
	}
	return b, err
}

// AdoptBotSession records the Bot's current settings as its active BotSession in the Space. A
// BotSession that was active becomes replaced.
func (s *Store) AdoptBotSession(ctx context.Context, spaceID, botID int64) (BotSession, error) {
	return s.AdoptBotSessionWith(ctx, spaceID, botID, nil)
}

// AdoptBotSessionWith is AdoptBotSession, and when the Bot picked skills it calls adopt with the
// new BotSession before committing and saves the skills it returns in place of the bare picks. If
// adopt fails, nothing is saved: the BotSession that was active stays active.
func (s *Store) AdoptBotSessionWith(
	ctx context.Context, spaceID, botID int64, adopt func(BotSession) ([]AdoptedSkill, error),
) (BotSession, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return BotSession{}, fmt.Errorf("begin adopt bot session: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx,
		"UPDATE bot_session SET status = ?, ended_at = ?, refresh_pending = 0 WHERE space_id = ? AND bot_id = ? AND status = ?",
		BotSessionReplaced, now, spaceID, botID, BotSessionActive); err != nil {
		return BotSession{}, fmt.Errorf("replace bot session: %w", err)
	}
	var picked, refs string
	err = tx.QueryRowContext(ctx, "SELECT connections, skills FROM bot WHERE id = ?", botID).Scan(&picked, &refs)
	if errors.Is(err, sql.ErrNoRows) {
		return BotSession{}, fmt.Errorf("%w: unknown bot %d", ErrInvalid, botID)
	}
	if err != nil {
		return BotSession{}, fmt.Errorf("read bot connections: %w", err)
	}
	var ids []int64
	if err := json.Unmarshal([]byte(picked), &ids); err != nil {
		return BotSession{}, fmt.Errorf("bot %d: bad connections: %w", botID, err)
	}
	var names []string
	if err := json.Unmarshal([]byte(refs), &names); err != nil {
		return BotSession{}, fmt.Errorf("bot %d: bad skills: %w", botID, err)
	}
	picks := make([]AdoptedSkill, len(names))
	for i, ref := range names {
		picks[i].Ref = ref
	}
	skills, err := json.Marshal(picks)
	if err != nil {
		return BotSession{}, fmt.Errorf("encode skills: %w", err)
	}
	all, err := listConnections(ctx, tx)
	if err != nil {
		return BotSession{}, err
	}
	resolved, err := json.Marshal(pickConnections(all, ids))
	if err != nil {
		return BotSession{}, fmt.Errorf("encode connections: %w", err)
	}
	res, err := tx.ExecContext(ctx,
		"INSERT INTO bot_session (space_id, bot_id, backend, provider, model, description, skills, tools, "+
			"connections, write, permission_mode, status, created_at) "+
			"SELECT ?, id, backend, provider, model, description, ?, tools, ?, write, "+
			"permission_mode, ?, ? FROM bot WHERE id = ?",
		spaceID, string(skills), string(resolved), BotSessionActive, now, botID)
	if err != nil {
		return BotSession{}, fmt.Errorf("insert bot session: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return BotSession{}, fmt.Errorf("insert bot session: %w", err)
	}
	if adopt != nil && len(picks) > 0 {
		bs, err := scanBotSession(tx.QueryRowContext(ctx,
			"SELECT "+_botSessionCols+" FROM bot_session WHERE id = ?", id))
		if err != nil {
			return BotSession{}, err
		}
		adopted, err := adopt(bs)
		if err != nil {
			return BotSession{}, err
		}
		body, err := json.Marshal(adopted)
		if err != nil {
			return BotSession{}, fmt.Errorf("encode skills: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "UPDATE bot_session SET skills = ? WHERE id = ?", string(body), id); err != nil {
			return BotSession{}, fmt.Errorf("set bot session skills: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return BotSession{}, fmt.Errorf("commit adopt bot session: %w", err)
	}
	return scanBotSession(s.db.QueryRowContext(ctx,
		"SELECT "+_botSessionCols+" FROM bot_session WHERE id = ?", id))
}

// SetRefreshPending marks the Bot's active BotSession in the Space as waiting for a refresh and
// reports whether there was one. A Bot with no active BotSession has nothing to refresh: its next
// turn adopts the current settings.
func (s *Store) SetRefreshPending(ctx context.Context, spaceID, botID int64) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		"UPDATE bot_session SET refresh_pending = 1 WHERE space_id = ? AND bot_id = ? AND status = ?",
		spaceID, botID, BotSessionActive)
	if err != nil {
		return false, fmt.Errorf("set refresh pending: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("set refresh pending: %w", err)
	}
	return n > 0, nil
}

// ClearRefreshPending ends the wait of the Bot's active BotSession in the Space for a refresh.
func (s *Store) ClearRefreshPending(ctx context.Context, spaceID, botID int64) error {
	if _, err := s.db.ExecContext(ctx,
		"UPDATE bot_session SET refresh_pending = 0 WHERE space_id = ? AND bot_id = ? AND status = ?",
		spaceID, botID, BotSessionActive); err != nil {
		return fmt.Errorf("clear refresh pending: %w", err)
	}
	return nil
}

// RefreshPendingBots returns the Bots of the Space whose active BotSession waits for a refresh.
// Adopting a new BotSession ends the wait.
func (s *Store) RefreshPendingBots(ctx context.Context, spaceID int64) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT bot_id FROM bot_session WHERE space_id = ? AND status = ? AND refresh_pending = 1 ORDER BY bot_id",
		spaceID, BotSessionActive)
	if err != nil {
		return nil, fmt.Errorf("read pending refreshes: %w", err)
	}
	return scanAll(rows, func(r interface{ Scan(...any) error }) (id int64, err error) {
		return id, r.Scan(&id)
	})
}

// EndBotSession marks the active BotSession with the id replaced or revoked, or returns ErrNotFound
// when it is not active.
func (s *Store) EndBotSession(ctx context.Context, id int64, status string) error {
	if status != BotSessionReplaced && status != BotSessionRevoked {
		return fmt.Errorf("%w: a bot session ends as replaced or revoked, not %q", ErrInvalid, status)
	}
	res, err := s.db.ExecContext(ctx,
		"UPDATE bot_session SET status = ?, ended_at = ?, refresh_pending = 0 WHERE id = ? AND status = ?",
		status, time.Now().UTC().Format(time.RFC3339Nano), id, BotSessionActive)
	if err != nil {
		return fmt.Errorf("end bot session: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("active bot session %d: %w", id, ErrNotFound)
	}
	return nil
}

func scanBotSession(r interface{ Scan(...any) error }) (BotSession, error) {
	var b BotSession
	var skills, tools, connections, created string
	var ended sql.NullString
	if err := r.Scan(&b.ID, &b.SpaceID, &b.BotID, &b.Backend, &b.Provider, &b.Model, &b.Description,
		&skills, &tools, &connections, &b.Write, &b.PermissionMode, &b.Status, &created, &ended); err != nil {
		return BotSession{}, err
	}
	if err := json.Unmarshal([]byte(skills), &b.Skills); err != nil {
		return BotSession{}, fmt.Errorf("bot session %d: bad skills: %w", b.ID, err)
	}
	if err := json.Unmarshal([]byte(tools), &b.Tools); err != nil {
		return BotSession{}, fmt.Errorf("bot session %d: bad tools: %w", b.ID, err)
	}
	if err := json.Unmarshal([]byte(connections), &b.Connections); err != nil {
		return BotSession{}, fmt.Errorf("bot session %d: bad connections: %w", b.ID, err)
	}
	var err error
	if b.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return BotSession{}, fmt.Errorf("bot session %d: bad created_at: %w", b.ID, err)
	}
	if ended.Valid {
		e, err := time.Parse(time.RFC3339Nano, ended.String)
		if err != nil {
			return BotSession{}, fmt.Errorf("bot session %d: bad ended_at: %w", b.ID, err)
		}
		b.EndedAt = &e
	}
	return b, nil
}
