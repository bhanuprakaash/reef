package store

import (
	"context"
	"fmt"
	"slices"
	"time"
)

// The groups of the inbox. Needs you leaves when its cause is resolved; only FYI can be
// dismissed.
const (
	GroupNeedsYou = "needs_you"
	GroupFYI      = "fyi"
)

// Kinds of FYI item that inbox_fyi stores, one row each.
const (
	KindSubItem  = "sub_item"
	KindProposed = "proposed"
	KindStarted  = "started"

	KindSpaceProposed = "space_proposed"
)

// _inboxItems selects paused, held and failed items. A closed Space has none. A paused Space
// with a wake held by the pause is held, one without is paused. A failed turn is listed when retry
// may run it again, so a retry, or a send that the role answered, clears it, and of several
// failed turns only the newest shows; a turn the user stopped is not listed. A paused Space
// is dated by its newest turn, which is what paused it. A turn that failed and paused its own
// Space, such as the tool call cap, shows only as paused.
const _inboxItems = `
SELECT CASE WHEN h.n > 0 THEN 'held' ELSE 'paused' END, c.id, c.work_item_id,
	CASE WHEN h.n > 0 THEN c.paused || ' (' || h.n || CASE h.n WHEN 1 THEN ' wake' ELSE ' wakes' END || ' held)'
		ELSE c.paused END,
	COALESCE((SELECT ended_at FROM turn WHERE space_id = c.id ORDER BY id DESC LIMIT 1), c.created_at),
	NULL, NULL, NULL
FROM space c
LEFT JOIN (SELECT space_id, COUNT(*) n FROM approval WHERE kind = '` + _holdKind + `' AND status = '` + HoldPending + `'
	GROUP BY space_id) h ON h.space_id = c.id
WHERE c.paused != '' AND c.closed_at IS NULL
UNION ALL
SELECT 'failed', t.space_id, c.work_item_id, t.role || ' failed: ' || t.error, t.ended_at, t.id, NULL, NULL
FROM turn t JOIN space c ON c.id = t.space_id
WHERE ` + _retryable + ` AND t.error != '` + _interruptedError + `' AND c.paused = '' AND c.closed_at IS NULL`

// _inboxWork selects the items of Work items, which have no Space of their own: a top-level
// item in review, dated by its proof, and each FYI row.
const _inboxWork = `
SELECT 'review', 0, w.id, w.title, COALESCE(p.submitted_at, w.created_at), NULL, NULL, NULL
FROM work_item w LEFT JOIN proof p ON p.work_item_id = w.id
WHERE w.status = '` + _review + `' AND w.parent_id IS NULL
UNION ALL
SELECT kind, 0, work_item_id, text, created_at, NULL, NULL, id FROM inbox_fyi`

// _inboxWaiting selects the Spaces that run no turn and whose newest message is from an
// agent and held no wake: a message whose wake is paused is not waiting. Nor is the reply a
// turn the user stopped kept, nor the message the user dismissed. A message that ends in a
// question mark, ignoring trailing spaces and the marks *, _, ), " and `, is a question; any
// other is a reply.
const _inboxWaiting = `
SELECT CASE WHEN rtrim(m.body, ' ' || char(9) || char(10) || char(13) || char(96) || '*_)"') LIKE '%?'
		THEN 'question' ELSE 'reply' END,
	c.id, c.work_item_id, substr(m.body, 1, 2000), m.created_at, NULL, m.id, NULL
FROM space c JOIN message m ON m.id = (SELECT MAX(id) FROM message WHERE space_id = c.id)
WHERE c.closed_at IS NULL AND m.author NOT IN ('user', '` + ReefAuthor + `')
	AND m.id IS NOT c.dismissed_message_id
	AND NOT EXISTS (SELECT 1 FROM approval WHERE message_id = m.id)
	AND NOT EXISTS (SELECT 1 FROM turn WHERE space_id = c.id AND status = ?)
	AND NOT EXISTS (SELECT 1 FROM turn WHERE id = (SELECT MAX(id) FROM turn WHERE space_id = c.id)
		AND status = '` + _turnFailed + `' AND error = '` + _interruptedError + `')`

// InboxItem is one thing waiting on the user, in Group. In Needs you, Kind is paused (a cap or
// a failure paused the Space; Text is the reason), held (the same, with wakes the pause held;
// Text ends with how many) or failed (a turn retry may run again that the user did not stop;
// TurnID), review (a top-level Work item in review) or question (the Space ended on a message
// of an agent that woke nobody and ends in a question mark; Text starts that message, MessageID
// is its id). In FYI, Kind is reply (the same, with no question mark) or sub_item, proposed or
// started (a row of inbox_fyi; ID). An item of a Work item has no Space: SpaceID is 0.
type InboxItem struct {
	Kind       string         `json:"kind"`
	Group      string         `json:"group"`
	SpaceID    int64          `json:"spaceId"`
	WorkItemID *int64         `json:"workItemId"`
	Text       string         `json:"text"`
	At         time.Time      `json:"at"`
	TurnID     *int64         `json:"turnId,omitempty"`
	MessageID  *int64         `json:"messageId,omitempty"`
	ID         *int64         `json:"id,omitempty"`
	Proposal   *SpaceProposal `json:"proposal,omitempty"`
}

func groupOf(kind string) string {
	switch kind {
	case "paused", "held", "failed", "review", "question":
		return GroupNeedsYou
	}
	return GroupFYI
}

// Inbox lists the items waiting on the user, newest first. Apart from the FYI rows it is
// derived from the rows of Spaces, holds, turns, messages and Work items. A question or reply
// shows only for a Space with no item in Needs you.
func (s *Store) Inbox(ctx context.Context) ([]InboxItem, error) {
	items, err := s.inboxQuery(ctx, _inboxItems)
	if err != nil {
		return nil, err
	}
	waiting, err := s.inboxQuery(ctx, _inboxWaiting, _turnRunning)
	if err != nil {
		return nil, err
	}
	for _, w := range waiting {
		if !slices.ContainsFunc(items, func(i InboxItem) bool { return i.SpaceID == w.SpaceID }) {
			items = append(items, w)
		}
	}
	work, err := s.inboxQuery(ctx, _inboxWork)
	if err != nil {
		return nil, err
	}
	items = append(items, work...)
	proposals, err := s.spaceProposalItems(ctx)
	if err != nil {
		return nil, err
	}
	items = append(items, proposals...)
	slices.SortStableFunc(items, func(a, b InboxItem) int { return b.At.Compare(a.At) })
	return items, nil
}

func (s *Store) inboxQuery(ctx context.Context, query string, args ...any) ([]InboxItem, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list inbox: %w", err)
	}
	return scanAll(rows, scanInboxItem)
}

// AddFYI adds an FYI item of the given kind about a Work item. It stays until the user
// dismisses it.
func (s *Store) AddFYI(ctx context.Context, kind string, workItemID int64, text string) error {
	return addFYI(ctx, s.db, kind, workItemID, text)
}

func addFYI(ctx context.Context, x execer, kind string, workItemID int64, text string) error {
	if _, err := x.ExecContext(ctx,
		"INSERT INTO inbox_fyi (kind, work_item_id, text, created_at) VALUES (?, ?, ?, ?)",
		kind, workItemID, text, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("add %s FYI of work item %d: %w", kind, workItemID, err)
	}
	return nil
}

// DismissInbox removes the FYI item i is for good. A reply is hidden while its message
// is the Space's newest; a later message brings it back. An item in Needs you is
// ErrConflict: it leaves when its cause is resolved.
func (s *Store) DismissInbox(ctx context.Context, i InboxItem) error {
	switch groupOf(i.Kind) {
	case GroupNeedsYou:
		return fmt.Errorf("%w: a %s item cannot be dismissed, it leaves when its cause is resolved",
			ErrConflict, i.Kind)
	}
	switch {
	case i.Kind == KindSpaceProposed && i.ID != nil:
		return s.dismissSpaceProposal(ctx, *i.ID)
	case i.Kind == "reply" && i.MessageID != nil:
		return s.dismissWaiting(ctx, i.SpaceID, *i.MessageID)
	case i.ID != nil && slices.Contains([]string{KindSubItem, KindProposed, KindStarted}, i.Kind):
		res, err := s.db.ExecContext(ctx, "DELETE FROM inbox_fyi WHERE id = ? AND kind = ?", *i.ID, i.Kind)
		if err != nil {
			return fmt.Errorf("dismiss %s FYI: %w", i.Kind, err)
		}
		if n, err := res.RowsAffected(); err != nil || n == 0 {
			return fmt.Errorf("%s FYI %d: %w", i.Kind, *i.ID, ErrNotFound)
		}
		return nil
	}
	return fmt.Errorf("%w: a %q item is dismissed by its id, or for a reply its Space and message",
		ErrInvalid, i.Kind)
}

func (s *Store) dismissSpaceProposal(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM space_proposal WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("dismiss Space proposal: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return fmt.Errorf("Space proposal %d: %w", id, ErrNotFound)
	}
	return nil
}

func (s *Store) dismissWaiting(ctx context.Context, spaceID, messageID int64) error {
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
	if err := r.Scan(&i.Kind, &i.SpaceID, &i.WorkItemID, &i.Text, &at, &i.TurnID, &i.MessageID, &i.ID); err != nil {
		return InboxItem{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return InboxItem{}, fmt.Errorf("inbox item of Space %d: bad time: %w", i.SpaceID, err)
	}
	i.At, i.Group = t, groupOf(i.Kind)
	return i, nil
}
