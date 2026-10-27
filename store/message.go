package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ReefAuthor is the author of the notes Reef itself writes into a Space.
const ReefAuthor = "reef"

// Message is append-only: there is no update or delete. A reply has the thread root as
// ParentID; a top-level message has none.
type Message struct {
	ID        int64     `json:"id"`
	SpaceID   int64     `json:"spaceId"`
	ParentID  *int64    `json:"parentId"`
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
}

// Kinds of search hit.
const (
	HitPublic = "public"
	HitDirect = "direct"
)

// MessageHit is one message that matched a search. A direct hit also names the two roles
// and the chain, and its MessageID is the direct message id.
type MessageHit struct {
	SpaceID   int64  `json:"spaceId"`
	MessageID int64  `json:"messageId"`
	Kind      string `json:"kind"`
	Author    string `json:"author"`
	From      string `json:"from,omitempty"`
	To        string `json:"to,omitempty"`
	ChainID   int64  `json:"chainId,omitempty"`
	Snippet   string `json:"snippet"`
	at        time.Time
}

// CreateMessage appends m to its Space.
func (s *Store) CreateMessage(ctx context.Context, m Message) (Message, error) {
	if m.Author == "" {
		return Message{}, fmt.Errorf("%w: message author is required", ErrInvalid)
	}
	if m.Body == "" {
		return Message{}, fmt.Errorf("%w: message body is required", ErrInvalid)
	}
	if m.ParentID != nil {
		var root bool
		err := s.db.QueryRowContext(ctx,
			"SELECT parent_id IS NULL FROM message WHERE id = ? AND space_id = ?",
			*m.ParentID, m.SpaceID).Scan(&root)
		switch {
		case errors.Is(err, sql.ErrNoRows), err == nil && !root:
			return Message{}, fmt.Errorf(
				"%w: parent %d is not a top-level message of Space %d",
				ErrInvalid, *m.ParentID, m.SpaceID)
		case err != nil:
			return Message{}, fmt.Errorf("check parent: %w", err)
		}
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO message (space_id, parent_id, author, body, created_at) "+
			"VALUES (?, ?, ?, ?, ?)",
		m.SpaceID, m.ParentID, m.Author, m.Body, now.Format(time.RFC3339Nano))
	if err != nil {
		return Message{}, s.writeErr(ctx, "insert message", err, ErrInvalid,
			parent{table: "space", id: &m.SpaceID})
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Message{}, fmt.Errorf("insert message: %w", err)
	}
	m.ID, m.CreatedAt = id, now
	return m, nil
}

// Messages lists the Space's messages with id > after, oldest first.
func (s *Store) Messages(ctx context.Context, spaceID, after int64) ([]Message, error) {
	if err := s.requireSpace(ctx, spaceID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, space_id, parent_id, author, body, created_at FROM message "+
			"WHERE space_id = ? AND id > ? ORDER BY id",
		spaceID, after)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	return scanAll(rows, scanMessage)
}

// MessagesBy lists the messages of author in the Space that were stored after since,
// oldest first.
func (s *Store) MessagesBy(
	ctx context.Context, spaceID int64, author string, since time.Time,
) ([]Message, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, space_id, parent_id, author, body, created_at FROM message "+
			"WHERE space_id = ? AND author = ? ORDER BY id",
		spaceID, author)
	if err != nil {
		return nil, fmt.Errorf("list messages by %s: %w", author, err)
	}
	all, err := scanAll(rows, scanMessage)
	if err != nil {
		return nil, err
	}
	var msgs []Message
	for _, m := range all {
		if m.CreatedAt.After(since) {
			msgs = append(msgs, m)
		}
	}
	return msgs, nil
}

// LastMessageID returns the id of the Space's newest message, 0 when it has none.
func (s *Store) LastMessageID(ctx context.Context, spaceID int64) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx,
		"SELECT COALESCE(MAX(id), 0) FROM message WHERE space_id = ?", spaceID).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("last message: %w", err)
	}
	return id, nil
}

// Thread lists the top-level message root of the Space and its replies, oldest first.
func (s *Store) Thread(ctx context.Context, spaceID, root int64) ([]Message, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, space_id, parent_id, author, body, created_at FROM message "+
			"WHERE space_id = ? AND (id = ? OR parent_id = ?) ORDER BY id",
		spaceID, root, root)
	if err != nil {
		return nil, fmt.Errorf("list thread: %w", err)
	}
	msgs, err := scanAll(rows, scanMessage)
	if err != nil {
		return nil, err
	}
	if len(msgs) == 0 || msgs[0].ParentID != nil {
		return nil, fmt.Errorf("thread %d: %w", root, ErrNotFound)
	}
	return msgs, nil
}

// ThreadOwner returns the author of the root of the thread that message id is in.
func (s *Store) ThreadOwner(ctx context.Context, id int64) (string, error) {
	var author string
	err := s.db.QueryRowContext(ctx,
		"SELECT r.author FROM message m JOIN message r ON r.id = COALESCE(m.parent_id, m.id) "+
			"WHERE m.id = ?", id).Scan(&author)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("message %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return "", fmt.Errorf("thread owner: %w", err)
	}
	return author, nil
}

// CountMessages returns how many messages the Space has.
func (s *Store) CountMessages(ctx context.Context, spaceID int64) (int, error) {
	if err := s.requireSpace(ctx, spaceID); err != nil {
		return 0, err
	}
	var n int
	err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM message WHERE space_id = ?", spaceID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count messages: %w", err)
	}
	return n, nil
}

// SearchMessages finds messages in the feature's Spaces whose body contains
// query, ignoring ASCII case, newest first. A hit's snippet is about 120
// characters of the body around the match.
func (s *Store) SearchMessages(
	ctx context.Context, featureID int64, query string, limit int,
) ([]MessageHit, error) {
	like := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(query)
	rows, err := s.db.QueryContext(ctx,
		`SELECT m.space_id, m.id, m.author,
		  substr(m.body, max(instr(lower(m.body), lower(?)) - 40, 1), 120), m.created_at
		FROM message m JOIN space c ON c.id = m.space_id
		WHERE c.feature_id = ? AND m.body LIKE '%' || ? || '%' ESCAPE '\'
		ORDER BY m.id DESC LIMIT ?`,
		query, featureID, like, limit)
	if err != nil {
		return nil, fmt.Errorf("search messages: %w", err)
	}
	return scanAll(rows, func(r interface{ Scan(...any) error }) (MessageHit, error) {
		h := MessageHit{Kind: HitPublic}
		var created string
		if err := r.Scan(&h.SpaceID, &h.MessageID, &h.Author, &h.Snippet, &created); err != nil {
			return MessageHit{}, err
		}
		var err error
		h.at, err = time.Parse(time.RFC3339Nano, created)
		return h, err
	})
}

// SearchDirectMessages is SearchMessages over direct messages. A role other than "" sees
// only the messages it sent or received.
func (s *Store) SearchDirectMessages(
	ctx context.Context, featureID int64, query string, limit int, role string,
) ([]MessageHit, error) {
	like := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(query)
	rows, err := s.db.QueryContext(ctx,
		`SELECT d.space_id, d.id, d.from_role, d.to_role, d.chain_id,
		  substr(d.body, max(instr(lower(d.body), lower(?)) - 40, 1), 120), d.created_at
		FROM direct_message d JOIN space c ON c.id = d.space_id
		WHERE c.feature_id = ? AND d.body LIKE '%' || ? || '%' ESCAPE '\'
		  AND (? = '' OR d.from_role = ? OR d.to_role = ?)
		ORDER BY d.id DESC LIMIT ?`,
		query, featureID, like, role, role, role, limit)
	if err != nil {
		return nil, fmt.Errorf("search direct messages: %w", err)
	}
	return scanAll(rows, func(r interface{ Scan(...any) error }) (MessageHit, error) {
		h := MessageHit{Kind: HitDirect}
		var created string
		if err := r.Scan(&h.SpaceID, &h.MessageID, &h.From, &h.To, &h.ChainID,
			&h.Snippet, &created); err != nil {
			return MessageHit{}, err
		}
		h.Author = h.From
		var err error
		h.at, err = time.Parse(time.RFC3339Nano, created)
		return h, err
	})
}

// SearchHistory merges SearchMessages and SearchDirectMessages, newest first, up to limit.
func (s *Store) SearchHistory(
	ctx context.Context, featureID int64, query string, limit int, role string,
) ([]MessageHit, error) {
	hits, err := s.SearchMessages(ctx, featureID, query, limit)
	if err != nil {
		return nil, err
	}
	direct, err := s.SearchDirectMessages(ctx, featureID, query, limit, role)
	if err != nil {
		return nil, err
	}
	hits = append(hits, direct...)
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].at.After(hits[j].at) })
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

func scanMessage(r interface{ Scan(...any) error }) (Message, error) {
	var m Message
	var created string
	if err := r.Scan(&m.ID, &m.SpaceID, &m.ParentID, &m.Author, &m.Body, &created); err != nil {
		return Message{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return Message{}, fmt.Errorf("message %d: bad created_at: %w", m.ID, err)
	}
	m.CreatedAt = t
	return m, nil
}
