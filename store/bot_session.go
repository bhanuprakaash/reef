package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/bhanuprakaash/reef/agent"
)

const (
	BotSessionActive   = "active"
	BotSessionReplaced = "replaced"
	BotSessionRevoked  = "revoked"
)

// BotSession is the settings one Bot adopted in one Space, as they were when it adopted them. It
// is execution history: editing the Bot changes no BotSession, and a Space holds one active
// BotSession per member Bot.
type BotSession struct {
	ID             int64             `json:"id"`
	SpaceID        int64             `json:"spaceId"`
	BotID          int64             `json:"botId"`
	Backend        string            `json:"backend"`
	Provider       string            `json:"provider,omitempty"`
	Model          string            `json:"model,omitempty"`
	Description    string            `json:"description"`
	Skills         []string          `json:"skills"`
	Tools          []string          `json:"tools"`
	MCPServers     []agent.MCPServer `json:"mcpServers"`
	Write          bool              `json:"write"`
	PermissionMode string            `json:"permissionMode"`
	Status         string            `json:"status"`
	CreatedAt      time.Time         `json:"createdAt"`
	EndedAt        *time.Time        `json:"endedAt"`
}

const _botSessionCols = "id, space_id, bot_id, backend, provider, model, description, skills, tools, " +
	"mcp_servers, write, permission_mode, status, created_at, ended_at"

// Apply returns r with the settings the BotSession adopted in place of the Bot's own. Name, Lead,
// Face and Colour stay.
func (b BotSession) Apply(r Role) Role {
	r.Backend, r.Provider, r.Model, r.Description = b.Backend, b.Provider, b.Model, b.Description
	r.Skills, r.Tools, r.MCPServers = b.Skills, b.Tools, b.MCPServers
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
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return BotSession{}, fmt.Errorf("begin adopt bot session: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx,
		"UPDATE bot_session SET status = ?, ended_at = ? WHERE space_id = ? AND bot_id = ? AND status = ?",
		BotSessionReplaced, now, spaceID, botID, BotSessionActive); err != nil {
		return BotSession{}, fmt.Errorf("replace bot session: %w", err)
	}
	res, err := tx.ExecContext(ctx,
		"INSERT INTO bot_session (space_id, bot_id, backend, provider, model, description, skills, tools, "+
			"mcp_servers, write, permission_mode, status, created_at) "+
			"SELECT ?, id, backend, provider, model, description, skills, tools, mcp_servers, write, "+
			"permission_mode, ?, ? FROM bot WHERE id = ?",
		spaceID, BotSessionActive, now, botID)
	if err != nil {
		return BotSession{}, fmt.Errorf("insert bot session: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return BotSession{}, fmt.Errorf("insert bot session: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return BotSession{}, fmt.Errorf("%w: unknown bot %d", ErrInvalid, botID)
	}
	if err := tx.Commit(); err != nil {
		return BotSession{}, fmt.Errorf("commit adopt bot session: %w", err)
	}
	return scanBotSession(s.db.QueryRowContext(ctx,
		"SELECT "+_botSessionCols+" FROM bot_session WHERE id = ?", id))
}

// EndBotSession marks the active BotSession with the id replaced or revoked, or returns ErrNotFound
// when it is not active.
func (s *Store) EndBotSession(ctx context.Context, id int64, status string) error {
	if status != BotSessionReplaced && status != BotSessionRevoked {
		return fmt.Errorf("%w: a bot session ends as replaced or revoked, not %q", ErrInvalid, status)
	}
	res, err := s.db.ExecContext(ctx,
		"UPDATE bot_session SET status = ?, ended_at = ? WHERE id = ? AND status = ?",
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
	var skills, tools, servers, created string
	var ended sql.NullString
	if err := r.Scan(&b.ID, &b.SpaceID, &b.BotID, &b.Backend, &b.Provider, &b.Model, &b.Description,
		&skills, &tools, &servers, &b.Write, &b.PermissionMode, &b.Status, &created, &ended); err != nil {
		return BotSession{}, err
	}
	if err := json.Unmarshal([]byte(skills), &b.Skills); err != nil {
		return BotSession{}, fmt.Errorf("bot session %d: bad skills: %w", b.ID, err)
	}
	if err := json.Unmarshal([]byte(tools), &b.Tools); err != nil {
		return BotSession{}, fmt.Errorf("bot session %d: bad tools: %w", b.ID, err)
	}
	if err := json.Unmarshal([]byte(servers), &b.MCPServers); err != nil {
		return BotSession{}, fmt.Errorf("bot session %d: bad mcp servers: %w", b.ID, err)
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
