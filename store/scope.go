package store

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// Scope is a thread a wake was routed in, Thread 0 for the top level. Asked is the lines
// that name the roles one message woke together. On a turn, Read is the newest message of the
// scope the turn sent; 0 when it sent none.
type Scope struct {
	Thread int64    `json:"thread"`
	Asked  []string `json:"asked,omitempty"`
	Read   int64    `json:"read,omitempty"`
}

// Scopes is stored as JSON; none is NULL, which is also what a turn from before scopes has.
type Scopes []Scope

// Value implements driver.Valuer.
func (s Scopes) Value() (driver.Value, error) {
	if len(s) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("encode scopes: %w", err)
	}
	return string(b), nil
}

// Scan implements sql.Scanner.
func (s *Scopes) Scan(src any) error {
	text, ok := src.(string)
	if !ok {
		*s = nil
		return nil
	}
	if err := json.Unmarshal([]byte(text), s); err != nil {
		return fmt.Errorf("decode scopes: %w", err)
	}
	return nil
}

// SentScopes lists the scopes of the turns of role in the session of channelID that sent
// their prompt, oldest first. It reads no prompt, so it stays cheap as the channel grows.
func (s *Store) SentScopes(
	ctx context.Context, channelID int64, role, sessionID string,
) (Scopes, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT scopes FROM turn WHERE channel_id = ? AND role = ? AND session_id = ? "+
			"AND sent = 1 AND scopes IS NOT NULL ORDER BY id", channelID, role, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list sent scopes: %w", err)
	}
	perTurn, err := scanAll(rows, func(r interface{ Scan(...any) error }) (Scopes, error) {
		var sc Scopes
		err := r.Scan(&sc)
		return sc, err
	})
	if err != nil {
		return nil, err
	}
	var out Scopes
	for _, sc := range perTurn {
		out = append(out, sc...)
	}
	return out, nil
}
