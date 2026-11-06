package store

import (
	"context"
	"fmt"
	"unicode/utf8"
)

// _toolCallMax is how much of a tool call's input and result the store keeps, each.
const _toolCallMax = 4 << 10

// ToolCall is one tool call of a turn. Input and Result are cut to 4 KB.
type ToolCall struct {
	ID     int64  `json:"id"`
	TurnID int64  `json:"turnId"`
	Name   string `json:"name"`
	Input  string `json:"input"`
	Result string `json:"result"`
	// ParentID is the Agent call whose subagent made this one, or 0 for a top-level call.
	ParentID int64 `json:"parentId,omitempty"`
}

// CreateToolCall records a tool call of turnID, before its result is known. parentID is the
// tool call it nests under, or 0.
func (s *Store) CreateToolCall(
	ctx context.Context, turnID, parentID int64, name, input string,
) (ToolCall, error) {
	input = cut(input)
	var parentRef any
	if parentID != 0 {
		parentRef = parentID
	}
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO tool_call (turn_id, name, input, parent_id) VALUES (?, ?, ?, ?)",
		turnID, name, input, parentRef)
	if err != nil {
		return ToolCall{}, s.writeErr(ctx, "insert tool call", err, ErrInvalid,
			parent{table: "turn", id: &turnID})
	}
	id, err := res.LastInsertId()
	if err != nil {
		return ToolCall{}, fmt.Errorf("insert tool call: %w", err)
	}
	return ToolCall{ID: id, TurnID: turnID, Name: name, Input: input, ParentID: parentID}, nil
}

// SetToolCallResult records the result of tool call id.
func (s *Store) SetToolCallResult(ctx context.Context, id int64, result string) error {
	_, err := s.db.ExecContext(ctx,
		"UPDATE tool_call SET result = ? WHERE id = ?", cut(result), id)
	if err != nil {
		return fmt.Errorf("set tool call result: %w", err)
	}
	return nil
}

// ToolCalls lists the turn's tool calls, oldest first.
func (s *Store) ToolCalls(ctx context.Context, turnID int64) ([]ToolCall, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, turn_id, name, input, result, COALESCE(parent_id, 0) "+
			"FROM tool_call WHERE turn_id = ? ORDER BY id",
		turnID)
	if err != nil {
		return nil, fmt.Errorf("list tool calls: %w", err)
	}
	return scanAll(rows, scanToolCall)
}

func scanToolCall(r interface{ Scan(...any) error }) (ToolCall, error) {
	var c ToolCall
	if err := r.Scan(&c.ID, &c.TurnID, &c.Name, &c.Input, &c.Result, &c.ParentID); err != nil {
		return ToolCall{}, err
	}
	return c, nil
}

// cut truncates s to at most _toolCallMax bytes, backing off to the start of a rune so a
// multi-byte character is never split.
func cut(s string) string {
	if len(s) <= _toolCallMax {
		return s
	}
	n := _toolCallMax
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
