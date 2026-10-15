package store

import (
	"context"
	"fmt"
	"slices"
	"time"
)

// _inboxItems selects the items of the first three kinds. A failed turn is listed when retry
// may run it again, so a retry, or a send that the role answered, clears it, and of several
// failed turns only the newest shows; a turn the user stopped is not listed. A paused channel
// is dated by its newest turn, which is what paused it. A turn that failed and paused its own
// channel, such as the tool call cap, shows only as paused.
const _inboxItems = `
SELECT 'approval', a.channel_id, c.feature_id, a.from_role || ' wants to wake ' || a.to_role,
	a.created_at, a.id, NULL, NULL
FROM approval a JOIN channel c ON c.id = a.channel_id
WHERE a.status = ? AND a.kind = ?
UNION ALL
SELECT 'paused', c.id, c.feature_id, c.paused,
	COALESCE((SELECT ended_at FROM turn WHERE channel_id = c.id ORDER BY id DESC LIMIT 1), c.created_at),
	NULL, NULL, NULL
FROM channel c WHERE c.paused != ''
UNION ALL
SELECT 'failed', t.channel_id, c.feature_id, t.role || ' failed: ' || t.error, t.ended_at, NULL, t.id, NULL
FROM turn t JOIN channel c ON c.id = t.channel_id
WHERE ` + _retryable + ` AND t.error != '` + _interruptedError + `' AND c.paused = ''`

// _inboxWaiting selects the channels that run no turn and whose newest message is from an
// agent and held no wake at a gate: a message that woke somebody, or whose wake the user
// decided on, is not waiting. Nor is the reply a turn the user stopped kept, nor the message
// the user dismissed.
const _inboxWaiting = `
SELECT 'waiting', c.id, c.feature_id, substr(m.body, 1, 200), m.created_at, NULL, NULL, m.id
FROM channel c JOIN message m ON m.id = (SELECT MAX(id) FROM message WHERE channel_id = c.id)
WHERE m.author NOT IN ('user', '` + ReefAuthor + `')
	AND m.id IS NOT c.dismissed_message_id
	AND NOT EXISTS (SELECT 1 FROM approval WHERE message_id = m.id)
	AND NOT EXISTS (SELECT 1 FROM turn WHERE channel_id = c.id AND status = ?)
	AND NOT EXISTS (SELECT 1 FROM turn WHERE id = (SELECT MAX(id) FROM turn WHERE channel_id = c.id)
		AND status = '` + _turnFailed + `' AND error = '` + _interruptedError + `')`

// InboxItem is one thing waiting on the user. Kind is approval (a gate holds a wake;
// ApprovalID), paused (a cap or a failure paused the channel; Text is the reason), failed (a
// turn retry may run again that the user did not stop; TurnID) or waiting (the channel ended on a
// message of an agent that woke nobody; Text starts that message, MessageID is its id).
type InboxItem struct {
	Kind       string    `json:"kind"`
	ChannelID  int64     `json:"channelId"`
	FeatureID  *int64    `json:"featureId"`
	Text       string    `json:"text"`
	At         time.Time `json:"at"`
	ApprovalID *int64    `json:"approvalId,omitempty"`
	TurnID     *int64    `json:"turnId,omitempty"`
	MessageID  *int64    `json:"messageId,omitempty"`
}

// Inbox lists the items waiting on the user in every channel, newest first. It is derived from
// the rows of approvals, channels, turns and messages. A waiting item shows only for a
// channel with no other item.
func (s *Store) Inbox(ctx context.Context) ([]InboxItem, error) {
	rows, err := s.db.QueryContext(ctx, _inboxItems, ApprovalPending, ApprovalGate)
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
		if !slices.ContainsFunc(items, func(i InboxItem) bool { return i.ChannelID == w.ChannelID }) {
			items = append(items, w)
		}
	}
	slices.SortStableFunc(items, func(a, b InboxItem) int { return b.At.Compare(a.At) })
	return items, nil
}

// DismissWaiting hides the waiting item of the channel while messageID, which must be a
// message of the channel, is its newest message. A later message brings the item back.
func (s *Store) DismissWaiting(ctx context.Context, channelID, messageID int64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE channel SET dismissed_message_id = ? WHERE id = ?
		AND EXISTS (SELECT 1 FROM message WHERE id = ? AND channel_id = ?)`,
		messageID, channelID, messageID, channelID)
	if err != nil {
		return fmt.Errorf("dismiss waiting: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("dismiss waiting: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("message %d of channel %d: %w", messageID, channelID, ErrNotFound)
	}
	return nil
}

func scanInboxItem(r interface{ Scan(...any) error }) (InboxItem, error) {
	var i InboxItem
	var at string
	if err := r.Scan(&i.Kind, &i.ChannelID, &i.FeatureID, &i.Text, &at, &i.ApprovalID, &i.TurnID, &i.MessageID); err != nil {
		return InboxItem{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return InboxItem{}, fmt.Errorf("inbox item of channel %d: bad time: %w", i.ChannelID, err)
	}
	i.At = t
	return i, nil
}
