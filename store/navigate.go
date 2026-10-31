package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Pin kinds. A chat is a Space of kind conversation; a space is any other Space.
const (
	PinBot   = "bot"
	PinChat  = "chat"
	PinSpace = "space"
)

// Pin is a user's pin of a Bot, a personal chat or a Space. Pins are global: no Project
// filters them. Title is the target's current name.
type Pin struct {
	Kind      string    `json:"kind"`
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"createdAt"`
}

// SetPin pins the target, which must exist and be of that kind. Pinning it again keeps the
// first pin time.
func (s *Store) SetPin(ctx context.Context, kind string, id int64) error {
	var q string
	switch kind {
	case PinBot:
		q = "SELECT 1 FROM bot WHERE id = ?"
	case PinChat:
		q = "SELECT 1 FROM space WHERE id = ? AND kind = 'conversation'"
	case PinSpace:
		q = "SELECT 1 FROM space WHERE id = ? AND kind != 'conversation'"
	default:
		return fmt.Errorf("%w: pin kind %q is not bot, chat or space", ErrInvalid, kind)
	}
	var one int
	if err := s.db.QueryRowContext(ctx, q, id).Scan(&one); errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%s %d: %w", kind, id, ErrNotFound)
	} else if err != nil {
		return fmt.Errorf("pin %s %d: %w", kind, id, err)
	}
	if _, err := s.db.ExecContext(ctx,
		"INSERT INTO pin (kind, target_id, created_at) VALUES (?, ?, ?) ON CONFLICT DO NOTHING",
		kind, id, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("pin %s %d: %w", kind, id, err)
	}
	return nil
}

// Unpin removes the pin; one that is not there is not an error.
func (s *Store) Unpin(ctx context.Context, kind string, id int64) error {
	if _, err := s.db.ExecContext(ctx,
		"DELETE FROM pin WHERE kind = ? AND target_id = ?", kind, id); err != nil {
		return fmt.Errorf("unpin %s %d: %w", kind, id, err)
	}
	return nil
}

// Pins lists every pin, newest first.
func (s *Store) Pins(ctx context.Context) ([]Pin, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT p.kind, p.target_id, COALESCE(b.name, c.name), p.created_at FROM pin p
		LEFT JOIN bot b ON p.kind = 'bot' AND b.id = p.target_id
		LEFT JOIN space c ON p.kind != 'bot' AND c.id = p.target_id
		WHERE COALESCE(b.name, c.name) IS NOT NULL
		ORDER BY p.created_at DESC, p.rowid DESC`)
	if err != nil {
		return nil, fmt.Errorf("list pins: %w", err)
	}
	return scanAll(rows, func(r interface{ Scan(...any) error }) (Pin, error) {
		var p Pin
		var created string
		if err := r.Scan(&p.Kind, &p.ID, &p.Title, &created); err != nil {
			return Pin{}, err
		}
		var err error
		p.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
		return p, err
	})
}

// ConversationPage narrows and pages a Bot's conversations. ProjectID keeps those associated
// with the Project. Before is the id of the last conversation of the previous page. Limit
// 0 lists all. Resumable drops the conversations that moved to a Space.
type ConversationPage struct {
	ProjectID *int64
	Before    *int64
	Limit     int
	Resumable bool
}

// _activity orders conversations: the time of the newest message, or else of the creation.
// It is rounded to milliseconds so that the text compares as time does.
const _activity = "strftime('%Y-%m-%d %H:%M:%f', COALESCE(m.created_at, space.created_at))"

// _cursorActivity is _activity of the conversation after which the page starts.
const _cursorActivity = "(SELECT strftime('%Y-%m-%d %H:%M:%f', COALESCE(" +
	"(SELECT created_at FROM message WHERE space_id = cur.id ORDER BY id DESC LIMIT 1), " +
	"cur.created_at)), cur.id FROM space cur WHERE cur.id = ?)"

// BotConversationsPage is BotConversations narrowed and paged by p. A page ends where the
// next begins, by (activity, id), so pages never repeat or skip a conversation unless one
// of them gets a message while the pages are read.
func (s *Store) BotConversationsPage(
	ctx context.Context, botID int64, p ConversationPage,
) ([]SpacePreview, error) {
	where := " AND space.kind = ? AND EXISTS " +
		"(SELECT 1 FROM space_member WHERE space_id = space.id AND bot_id = ?)"
	args := []any{KindConversation, botID}
	if p.ProjectID != nil {
		where += " AND EXISTS (SELECT 1 FROM space_project WHERE space_id = space.id AND project_id = ?)"
		args = append(args, *p.ProjectID)
	}
	if p.Resumable {
		where += " AND space.moved_to IS NULL"
	}
	if p.Before != nil {
		where += " AND (" + _activity + ", space.id) < " + _cursorActivity
		args = append(args, *p.Before)
	}
	order := _activity + " DESC, space.id DESC"
	if p.Limit > 0 {
		order += fmt.Sprintf(" LIMIT %d", p.Limit)
	}
	return s.spacePreviews(ctx, where, args, order)
}

// MessagesPage lists the newest limit messages of the Space with id < before (all when
// before is 0), oldest first. The oldest id is the next page's before.
func (s *Store) MessagesPage(ctx context.Context, spaceID, before int64, limit int) ([]Message, error) {
	if err := s.requireSpace(ctx, spaceID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, space_id, parent_id, author, bot_id, body, created_at FROM ("+
			"SELECT * FROM message WHERE space_id = ? AND (? = 0 OR id < ?) ORDER BY id DESC LIMIT ?"+
			") ORDER BY id",
		spaceID, before, before, limit)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	return scanAll(rows, scanMessage)
}

// FoundItem is a Bot, personal chat, Space or Work item whose name contains the query. A
// Work item also has the Project it belongs to.
type FoundItem struct {
	Kind      string `json:"kind"`
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	ProjectID int64  `json:"projectId,omitempty"`
}

// Find lists up to limit matches of each kind, ignoring ASCII case, in the order Bots,
// chats, Spaces and Work items, each by name.
func (s *Store) Find(ctx context.Context, query string, limit int) ([]FoundItem, error) {
	like := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(query) + "%"
	rows, err := s.db.QueryContext(ctx,
		`SELECT kind, id, title, project_id FROM (
		  SELECT * FROM (SELECT 'bot' AS kind, id, name AS title, 0 AS project_id, 1 AS ord FROM bot
		    WHERE name LIKE ?1 ESCAPE '\' ORDER BY name, id LIMIT ?2)
		  UNION ALL SELECT * FROM (SELECT 'chat', id, name, 0, 2 FROM space
		    WHERE kind = 'conversation' AND name LIKE ?1 ESCAPE '\' ORDER BY name, id LIMIT ?2)
		  UNION ALL SELECT * FROM (SELECT 'space', id, name, 0, 3 FROM space
		    WHERE kind != 'conversation' AND name LIKE ?1 ESCAPE '\' ORDER BY name, id LIMIT ?2)
		  UNION ALL SELECT * FROM (SELECT 'workItem', id, title, project_id, 4 FROM work_item
		    WHERE title LIKE ?1 ESCAPE '\' ORDER BY title, id LIMIT ?2)
		) ORDER BY ord, title, id`, like, limit)
	if err != nil {
		return nil, fmt.Errorf("find: %w", err)
	}
	return scanAll(rows, func(r interface{ Scan(...any) error }) (FoundItem, error) {
		var f FoundItem
		err := r.Scan(&f.Kind, &f.ID, &f.Title, &f.ProjectID)
		return f, err
	})
}
