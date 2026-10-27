package store

import (
	"context"
	"fmt"
	"slices"
	"time"
)

// _inboxItems selects paused and failed items. A closed Space has none. A failed turn is listed when retry
// may run it again, so a retry, or a send that the role answered, clears it, and of several
// failed turns only the newest shows; a turn the user stopped is not listed. A paused Space
// is dated by its newest turn, which is what paused it. A turn that failed and paused its own
// Space, such as the tool call cap, shows only as paused.
const _inboxItems = `
SELECT 'paused', c.id, c.work_item_id, c.paused,
	COALESCE((SELECT ended_at FROM turn WHERE space_id = c.id ORDER BY id DESC LIMIT 1), c.created_at),
	NULL, NULL
FROM space c WHERE c.paused != '' AND c.closed_at IS NULL
UNION ALL
SELECT 'failed', t.space_id, c.work_item_id, t.role || ' failed: ' || t.error, t.ended_at, t.id, NULL
FROM turn t JOIN space c ON c.id = t.space_id
WHERE ` + _retryable + ` AND t.error != '` + _interruptedError + `' AND c.paused = '' AND c.closed_at IS NULL`

// _inboxWaiting selects the Spaces that run no turn and whose newest message is from an
// agent and held no wake: a message whose wake is paused is not waiting. Nor is the reply a
// turn the user stopped kept, nor the message the user dismissed.
const _inboxWaiting = `
SELECT 'waiting', c.id, c.work_item_id, substr(m.body, 1, 200), m.created_at, NULL, m.id
FROM space c JOIN message m ON m.id = (SELECT MAX(id) FROM message WHERE space_id = c.id)
WHERE c.closed_at IS NULL AND m.author NOT IN ('user', '` + ReefAuthor + `')
	AND m.id IS NOT c.dismissed_message_id
	AND NOT EXISTS (SELECT 1 FROM approval WHERE message_id = m.id)
	AND NOT EXISTS (SELECT 1 FROM turn WHERE space_id = c.id AND status = ?)
	AND NOT EXISTS (SELECT 1 FROM turn WHERE id = (SELECT MAX(id) FROM turn WHERE space_id = c.id)
		AND status = '` + _turnFailed + `' AND error = '` + _interruptedError + `')`

// InboxItem is one thing waiting on the user. Kind is paused (a cap or a failure paused the Space; Text is the reason), failed (a
// turn retry may run again that the user did not stop; TurnID) or waiting (the Space ended on a
// message of an agent that woke nobody; Text starts that message, MessageID is its id).
type InboxItem struct {
	Kind       string    `json:"kind"`
	SpaceID    int64     `json:"spaceId"`
	WorkItemID *int64    `json:"workItemId"`
	Text       string    `json:"text"`
	At         time.Time `json:"at"`
	TurnID     *int64    `json:"turnId,omitempty"`
	MessageID  *int64    `json:"messageId,omitempty"`
}

// Inbox lists the items waiting on the user in every Space, newest first. It is derived from
// the rows of cap holds, Spaces, turns and messages. A waiting item shows only for a
// Space with no other item.
func (s *Store) Inbox(ctx context.Context) ([]InboxItem, error) {
	rows, err := s.db.QueryContext(ctx, _inboxItems)
	if err != nil {
		return nil, fmt.Errorf("list inbox: %w", err)
	}
	items, err := scanAll(rows, scanInboxItem)
	if err != nil {
		return nil, err
	}
	rows, err = s.db.QueryContext(ctx, _inboxWaiting, _turnRunning)
	if err != nil {
		return nil, fmt.Errorf("list inbox: %w", err)
	}
	waiting, err := scanAll(rows, scanInboxItem)
	if err != nil {
		return nil, err
	}
	for _, w := range waiting {
		if !slices.ContainsFunc(items, func(i InboxItem) bool { return i.SpaceID == w.SpaceID }) {
			items = append(items, w)
		}
	}
	slices.SortStableFunc(items, func(a, b InboxItem) int { return b.At.Compare(a.At) })
	return items, nil
}

// DismissWaiting hides the waiting item of the Space while messageID, which must be a
// message of the Space, is its newest message. A later message brings the item back.
func (s *Store) DismissWaiting(ctx context.Context, spaceID, messageID int64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE space SET dismissed_message_id = ? WHERE id = ?
		AND EXISTS (SELECT 1 FROM message WHERE id = ? AND space_id = ?)`,
		messageID, spaceID, messageID, spaceID)
	if err != nil {
		return fmt.Errorf("dismiss waiting: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("dismiss waiting: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("message %d of Space %d: %w", messageID, spaceID, ErrNotFound)
	}
	return nil
}

func scanInboxItem(r interface{ Scan(...any) error }) (InboxItem, error) {
	var i InboxItem
	var at string
	if err := r.Scan(&i.Kind, &i.SpaceID, &i.WorkItemID, &i.Text, &at, &i.TurnID, &i.MessageID); err != nil {
		return InboxItem{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return InboxItem{}, fmt.Errorf("inbox item of Space %d: bad time: %w", i.SpaceID, err)
	}
	i.At = t
	return i, nil
}
