package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"
)

// A permission prompt is a tool call a Bot's agent may not make until the user allows it.
const (
	PromptWaiting = "waiting"
	PromptAllowed = "allowed"
	PromptDenied  = "denied"
	// PromptExpired is a prompt that was denied with no answer from the user: it stays in the Space
	// and the user may still allow it.
	PromptExpired = "expired"

	// PromptRestartReason is why a prompt that waited when its server stopped expired.
	PromptRestartReason = "reef restarted"

	AnswerOnce   = "once"
	AnswerAlways = "always"
	AnswerDeny   = "deny"
)

const _promptCols = "id, space_id, bot_id, COALESCE(turn_id, 0), request_id, tool, input, rule, path, " +
	"status, answer, reason, granted, created_at, answered_at"

// PermissionPrompt is one prompt. Input is the tool call's input as JSON, Rule the allow rule
// the backend suggests for it and Path the folder it reaches outside the Bot's own. Granted is
// set on a prompt allowed after it expired: the Bot's next prompt for the same rule in the Space
// is allowed once without asking.
type PermissionPrompt struct {
	ID         int64           `json:"id"`
	SpaceID    int64           `json:"spaceId"`
	BotID      int64           `json:"botId"`
	TurnID     int64           `json:"turnId"`
	RequestID  string          `json:"-"`
	Tool       string          `json:"tool"`
	Input      json.RawMessage `json:"input"`
	Rule       string          `json:"rule"`
	Path       string          `json:"path"`
	Status     string          `json:"status"`
	Answer     string          `json:"answer"`
	Reason     string          `json:"reason"`
	Granted    bool            `json:"granted"`
	CreatedAt  time.Time       `json:"createdAt"`
	AnsweredAt *time.Time      `json:"answeredAt"`
}

// CreatePermissionPrompt stores p, in the status it has: waiting, or expired for a prompt that is
// denied as it arrives.
func (s *Store) CreatePermissionPrompt(ctx context.Context, p PermissionPrompt) (PermissionPrompt, error) {
	if p.Status == "" {
		p.Status = PromptWaiting
	}
	if len(p.Input) == 0 {
		p.Input = json.RawMessage("{}")
	}
	p.CreatedAt = time.Now().UTC()
	var turnID, answeredAt any
	if p.TurnID != 0 {
		turnID = p.TurnID
	}
	if p.Status != PromptWaiting {
		answeredAt = p.CreatedAt.Format(time.RFC3339Nano)
	}
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO permission_prompt (space_id, bot_id, turn_id, request_id, tool, input, rule, path, "+
			"status, answer, reason, created_at, answered_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		p.SpaceID, p.BotID, turnID, p.RequestID, p.Tool, string(p.Input), p.Rule, p.Path, p.Status, p.Answer,
		p.Reason, p.CreatedAt.Format(time.RFC3339Nano), answeredAt)
	if err != nil {
		return PermissionPrompt{}, s.writeErr(ctx, "insert permission prompt", err, ErrInvalid,
			parent{table: "space", id: &p.SpaceID}, parent{table: "bot", id: &p.BotID})
	}
	if p.ID, err = res.LastInsertId(); err != nil {
		return PermissionPrompt{}, fmt.Errorf("insert permission prompt: %w", err)
	}
	if p.Status != PromptWaiting {
		at := p.CreatedAt
		p.AnsweredAt = &at
	}
	return p, nil
}

// PermissionPrompt returns the prompt with the id, or ErrNotFound.
func (s *Store) PermissionPrompt(ctx context.Context, id int64) (PermissionPrompt, error) {
	p, err := scanPrompt(s.db.QueryRowContext(ctx,
		"SELECT "+_promptCols+" FROM permission_prompt WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return PermissionPrompt{}, fmt.Errorf("permission prompt %d: %w", id, ErrNotFound)
	}
	return p, err
}

// SpacePermissionPrompts lists the prompts of the Space that wait or expired, oldest first.
func (s *Store) SpacePermissionPrompts(ctx context.Context, spaceID int64) ([]PermissionPrompt, error) {
	if err := s.requireSpace(ctx, spaceID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+_promptCols+" FROM permission_prompt WHERE space_id = ? AND status IN (?, ?) ORDER BY id",
		spaceID, PromptWaiting, PromptExpired)
	if err != nil {
		return nil, fmt.Errorf("list permission prompts: %w", err)
	}
	return scanAll(rows, scanPrompt)
}

// ResolvePermissionPrompt moves prompt id from status from to status to, keeping answer and reason.
// A prompt not in status from is ErrConflict. grant marks a prompt allowed after it expired.
func (s *Store) ResolvePermissionPrompt(
	ctx context.Context, id int64, from, to, answer, reason string, grant bool,
) (PermissionPrompt, error) {
	res, err := s.db.ExecContext(ctx,
		"UPDATE permission_prompt SET status = ?, answer = ?, reason = ?, granted = ?, answered_at = ? "+
			"WHERE id = ? AND status = ?",
		to, answer, reason, grant, time.Now().UTC().Format(time.RFC3339Nano), id, from)
	if err != nil {
		return PermissionPrompt{}, fmt.Errorf("resolve permission prompt: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		p, err := s.PermissionPrompt(ctx, id)
		if err != nil {
			return PermissionPrompt{}, err
		}
		return PermissionPrompt{}, fmt.Errorf("permission prompt %d is %s, not %s: %w",
			id, p.Status, from, ErrConflict)
	}
	return s.PermissionPrompt(ctx, id)
}

// ExpireWaitingPrompts moves the waiting prompts of every Space except those keep says a live
// process owns to expired with reason: the agent that asked is gone.
func (s *Store) ExpireWaitingPrompts(ctx context.Context, keep func(spaceID int64) bool, reason string) error {
	rows, err := s.db.QueryContext(ctx,
		"SELECT DISTINCT space_id FROM permission_prompt WHERE status = ?", PromptWaiting)
	if err != nil {
		return fmt.Errorf("list waiting permission prompts: %w", err)
	}
	ids, err := scanAll(rows, func(r interface{ Scan(...any) error }) (id int64, err error) {
		return id, r.Scan(&id)
	})
	if err != nil {
		return fmt.Errorf("list waiting permission prompts: %w", err)
	}
	for _, id := range ids {
		if keep != nil && keep(id) {
			continue
		}
		_, err := s.db.ExecContext(ctx,
			"UPDATE permission_prompt SET status = ?, answer = ?, reason = ?, answered_at = ? "+
				"WHERE space_id = ? AND status = ?",
			PromptExpired, AnswerDeny, reason, time.Now().UTC().Format(time.RFC3339Nano), id, PromptWaiting)
		if err != nil {
			return fmt.Errorf("expire waiting permission prompts: %w", err)
		}
	}
	return nil
}

// TakeGrantedPrompt reports whether the Bot has a prompt in the Space for rule that the user
// allowed after it expired, and uses that grant up.
func (s *Store) TakeGrantedPrompt(ctx context.Context, spaceID, botID int64, rule string) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		"UPDATE permission_prompt SET granted = 0 WHERE id = (SELECT id FROM permission_prompt "+
			"WHERE space_id = ? AND bot_id = ? AND rule = ? AND granted = 1 ORDER BY id LIMIT 1)",
		spaceID, botID, rule)
	if err != nil {
		return false, fmt.Errorf("take granted permission prompt: %w", err)
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// AddBotTool adds rule to the tools of the Bot and of its active BotSession in the Space, when
// they lack it. A Bot with no BotSession in the Space yet adopts the Bot's tools at its first turn.
func (s *Store) AddBotTool(ctx context.Context, spaceID, botID int64, rule string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin add tool: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var tools string
	err = tx.QueryRowContext(ctx, "SELECT tools FROM bot WHERE id = ?", botID).Scan(&tools)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("bot %d: %w", botID, ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("read bot tools: %w", err)
	}
	if tools, err = withTool(tools, rule); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE bot SET tools = ? WHERE id = ?", tools, botID); err != nil {
		return fmt.Errorf("update bot tools: %w", err)
	}
	var sessionID int64
	err = tx.QueryRowContext(ctx,
		"SELECT id, tools FROM bot_session WHERE space_id = ? AND bot_id = ? AND status = ?",
		spaceID, botID, BotSessionActive).Scan(&sessionID, &tools)
	if err == nil {
		if tools, err = withTool(tools, rule); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "UPDATE bot_session SET tools = ? WHERE id = ?", tools, sessionID)
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("update bot session tools: %w", err)
	}
	return tx.Commit()
}

// withTool returns the JSON list tools with rule in it.
func withTool(tools, rule string) (string, error) {
	var list []string
	if err := json.Unmarshal([]byte(tools), &list); err != nil {
		return "", fmt.Errorf("bad tools: %w", err)
	}
	if !slices.Contains(list, rule) {
		list = append(list, rule)
	}
	out, err := json.Marshal(list)
	if err != nil {
		return "", fmt.Errorf("encode tools: %w", err)
	}
	return string(out), nil
}

func scanPrompt(r interface{ Scan(...any) error }) (PermissionPrompt, error) {
	var p PermissionPrompt
	var input, created string
	var answered sql.NullString
	if err := r.Scan(&p.ID, &p.SpaceID, &p.BotID, &p.TurnID, &p.RequestID, &p.Tool, &input, &p.Rule, &p.Path,
		&p.Status, &p.Answer, &p.Reason, &p.Granted, &created, &answered); err != nil {
		return PermissionPrompt{}, err
	}
	p.Input = json.RawMessage(input)
	var err error
	if p.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return PermissionPrompt{}, fmt.Errorf("permission prompt %d: bad created_at: %w", p.ID, err)
	}
	if answered.Valid {
		at, err := time.Parse(time.RFC3339Nano, answered.String)
		if err != nil {
			return PermissionPrompt{}, fmt.Errorf("permission prompt %d: bad answered_at: %w", p.ID, err)
		}
		p.AnsweredAt = &at
	}
	return p, nil
}
