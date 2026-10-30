package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Proposal statuses. Only a pending proposal can be approved or rejected.
const (
	ProposalPending  = "pending"
	ProposalApproved = "approved"
	ProposalRejected = "rejected"
)

// Proposal kinds. A wrap ends the Space; a brief only updates the item's brief.
const (
	ProposalWrap  = "wrap"
	ProposalBrief = "brief"
)

const _proposalCols = "id, space_id, role, kind, brief, items, retire, status, " +
	"created_at, decided_at, bot_id"

// ProposedItem is a memory item that approving its proposal saves. Scope is the memory scope
// kind: work_item, project, teammate or team. Replaces are the memory items it replaces.
type ProposedItem struct {
	Kind     string  `json:"kind"`
	Body     string  `json:"body"`
	Why      string  `json:"why,omitempty"`
	Scope    string  `json:"scope,omitempty"`
	Replaces []int64 `json:"replaces,omitempty"`
}

// Proposal is what a role suggests: at wrap-up the new brief, the memory items it noticed and
// the ids of those to retire; a brief proposal has only the brief.
type Proposal struct {
	ID        int64          `json:"id"`
	SpaceID   int64          `json:"spaceId"`
	BotID     *int64         `json:"botId,omitempty"`
	Role      string         `json:"role"`
	Kind      string         `json:"kind"`
	Brief     string         `json:"brief"`
	Items     []ProposedItem `json:"items"`
	Retire    []int64        `json:"retire"`
	Status    string         `json:"status"`
	CreatedAt time.Time      `json:"createdAt"`
	DecidedAt *time.Time     `json:"decidedAt"`
}

// CreateProposal adds p as pending, of kind wrap unless p.Kind says otherwise. The Space
// must belong to a work item. The Space has one pending proposal at most: a wrap rejects the
// older pending ones of any kind, and a brief rejects the older pending briefs but is
// refused while a wrap is pending.
func (s *Store) CreateProposal(ctx context.Context, p Proposal) (Proposal, error) {
	if p.Kind == "" {
		p.Kind = ProposalWrap
	}
	if p.Kind != ProposalWrap && p.Kind != ProposalBrief {
		return Proposal{}, fmt.Errorf("%w: unknown proposal kind %q", ErrInvalid, p.Kind)
	}
	if p.Role == "" {
		return Proposal{}, fmt.Errorf("%w: proposal role is required", ErrInvalid)
	}
	if p.Brief == "" {
		return Proposal{}, fmt.Errorf("%w: proposal brief is required", ErrInvalid)
	}
	for _, it := range p.Items {
		if it.Body == "" {
			return Proposal{}, fmt.Errorf("%w: item body is required", ErrInvalid)
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Proposal{}, fmt.Errorf("begin proposal: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	c, err := scanSpace(tx.QueryRowContext(ctx,
		"SELECT "+_spaceCols+" FROM space WHERE id = ?", p.SpaceID))
	if errors.Is(err, sql.ErrNoRows) {
		return Proposal{}, fmt.Errorf("space %d: %w", p.SpaceID, ErrNotFound)
	}
	if err != nil {
		return Proposal{}, err
	}
	if c.WorkItemID == nil {
		return Proposal{}, fmt.Errorf("%w: Space %d has no work item", ErrInvalid, c.ID)
	}
	dedupeRetired(&p)
	items, err := json.Marshal(p.Items)
	if err != nil {
		return Proposal{}, fmt.Errorf("encode items: %w", err)
	}
	retire, err := json.Marshal(p.Retire)
	if err != nil {
		return Proposal{}, fmt.Errorf("encode retire: %w", err)
	}
	now := time.Now().UTC()
	at := now.Format(time.RFC3339Nano)
	reject := "UPDATE proposal SET status = ?, decided_at = ? WHERE space_id = ? AND status = ?"
	args := []any{ProposalRejected, at, p.SpaceID, ProposalPending}
	if p.Kind == ProposalBrief {
		var wraps int
		if err := tx.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM proposal WHERE space_id = ? AND kind = ? AND status = ?",
			p.SpaceID, ProposalWrap, ProposalPending).Scan(&wraps); err != nil {
			return Proposal{}, fmt.Errorf("check pending wrap: %w", err)
		}
		if wraps > 0 {
			return Proposal{}, fmt.Errorf("%w: Space %d has a wrap-up waiting for approval",
				ErrInvalid, p.SpaceID)
		}
		reject, args = reject+" AND kind = ?", append(args, ProposalBrief)
	}
	if _, err := tx.ExecContext(ctx, reject, args...); err != nil {
		return Proposal{}, fmt.Errorf("reject older proposals: %w", err)
	}
	res, err := tx.ExecContext(ctx,
		"INSERT INTO proposal (space_id, role, kind, brief, items, retire, created_at, bot_id) "+
			"VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		p.SpaceID, p.Role, p.Kind, p.Brief, string(items), string(retire), at, p.BotID)
	if err != nil {
		return Proposal{}, s.writeErr(ctx, "insert proposal", err, ErrNotFound,
			parent{table: "space", id: &p.SpaceID})
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Proposal{}, fmt.Errorf("insert proposal: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Proposal{}, fmt.Errorf("commit proposal: %w", err)
	}
	p.ID, p.Status, p.CreatedAt = id, ProposalPending, now
	return p, nil
}

func dedupeRetired(p *Proposal) {
	seen := map[int64]bool{}
	claim := func(ids []int64) []int64 {
		kept := make([]int64, 0, len(ids))
		for _, id := range ids {
			if !seen[id] {
				seen[id] = true
				kept = append(kept, id)
			}
		}
		return kept
	}
	items := make([]ProposedItem, len(p.Items))
	for i, it := range p.Items {
		it.Replaces = claim(it.Replaces)
		items[i] = it
	}
	p.Items, p.Retire = items, claim(p.Retire)
}

// PendingProposal returns the Space's newest pending proposal, or ErrNotFound.
func (s *Store) PendingProposal(ctx context.Context, spaceID int64) (Proposal, error) {
	row := s.db.QueryRowContext(ctx,
		"SELECT "+_proposalCols+" FROM proposal WHERE space_id = ? AND status = ? "+
			"ORDER BY id DESC LIMIT 1", spaceID, ProposalPending)
	p, err := scanProposal(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Proposal{}, fmt.Errorf("pending proposal of Space %d: %w", spaceID, ErrNotFound)
	}
	return p, err
}

// Proposal returns the proposal with the id in any status, or ErrNotFound.
func (s *Store) Proposal(ctx context.Context, id int64) (Proposal, error) {
	row := s.db.QueryRowContext(ctx, "SELECT "+_proposalCols+" FROM proposal WHERE id = ?", id)
	p, err := scanProposal(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Proposal{}, fmt.Errorf("proposal %d: %w", id, ErrNotFound)
	}
	return p, err
}

// PendingProposals returns every pending proposal, oldest first. With workItemID
// set, only those of that item's Spaces. With projectID set, only that project's.
func (s *Store) PendingProposals(
	ctx context.Context, workItemID, projectID *int64,
) ([]Proposal, error) {
	query, args := "SELECT "+_proposalCols+" FROM proposal WHERE status = ?", []any{ProposalPending}
	if workItemID != nil {
		query += " AND space_id IN (SELECT id FROM space WHERE work_item_id = ?)"
		args = append(args, *workItemID)
	}
	if projectID != nil {
		query += " AND space_id IN (SELECT id FROM space WHERE project_id = ?)"
		args = append(args, *projectID)
	}
	rows, err := s.db.QueryContext(ctx, query+" ORDER BY id", args...)
	if err != nil {
		return nil, fmt.Errorf("list pending proposals: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ps []Proposal
	for rows.Next() {
		p, err := scanProposal(rows)
		if err != nil {
			return nil, err
		}
		ps = append(ps, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list pending proposals: %w", err)
	}
	return ps, nil
}

// ApproveProposal saves the pending proposal's brief, closes the Space of a wrap, gives the
// direct requests waiting in it a stopped response and marks the proposal approved, in one
// transaction. A brief proposal is refused once the Space is
// closed.
func (s *Store) ApproveProposal(ctx context.Context, id int64) (Proposal, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Proposal{}, fmt.Errorf("begin approve: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	p, err := pendingProposal(ctx, tx, id)
	if err != nil {
		return Proposal{}, err
	}
	var workItemID int64
	var closed sql.NullString
	err = tx.QueryRowContext(ctx,
		"SELECT work_item_id, closed_at FROM space WHERE id = ?", p.SpaceID).
		Scan(&workItemID, &closed)
	if err != nil {
		return Proposal{}, fmt.Errorf("read work item of proposal %d: %w", id, err)
	}
	if p.Kind == ProposalBrief && closed.Valid {
		return Proposal{}, fmt.Errorf("%w: Space %d is closed", ErrInvalid, p.SpaceID)
	}
	now := time.Now().UTC()
	at := now.Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, _insertBrief, workItemID, p.Brief, p.Role, at); err != nil {
		return Proposal{}, fmt.Errorf("insert brief: %w", err)
	}
	if p.Kind == ProposalWrap {
		if _, err := tx.ExecContext(ctx,
			"UPDATE space SET closed_at = ? WHERE id = ?", at, p.SpaceID); err != nil {
			return Proposal{}, fmt.Errorf("close Space: %w", err)
		}
		if err := s.stopWaitingDirects(ctx, tx, "space_id = ?", p.SpaceID); err != nil {
			return Proposal{}, err
		}
	}
	return decideProposal(ctx, tx, p, ProposalApproved, now)
}

// RejectProposal marks the pending proposal rejected and changes nothing else.
func (s *Store) RejectProposal(ctx context.Context, id int64) (Proposal, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Proposal{}, fmt.Errorf("begin reject: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	p, err := pendingProposal(ctx, tx, id)
	if err != nil {
		return Proposal{}, err
	}
	return decideProposal(ctx, tx, p, ProposalRejected, time.Now().UTC())
}

func pendingProposal(ctx context.Context, tx *sql.Tx, id int64) (Proposal, error) {
	row := tx.QueryRowContext(ctx, "SELECT "+_proposalCols+" FROM proposal WHERE id = ?", id)
	p, err := scanProposal(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Proposal{}, fmt.Errorf("proposal %d: %w", id, ErrNotFound)
		}
		return Proposal{}, err
	}
	if p.Status != ProposalPending {
		return Proposal{}, fmt.Errorf("%w: proposal %d is already %s", ErrInvalid, id, p.Status)
	}
	return p, nil
}

func decideProposal(
	ctx context.Context, tx *sql.Tx, p Proposal, status string, at time.Time,
) (Proposal, error) {
	if _, err := tx.ExecContext(ctx,
		"UPDATE proposal SET status = ?, decided_at = ? WHERE id = ?",
		status, at.Format(time.RFC3339Nano), p.ID); err != nil {
		return Proposal{}, fmt.Errorf("update proposal: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Proposal{}, fmt.Errorf("commit proposal %d: %w", p.ID, err)
	}
	p.Status, p.DecidedAt = status, &at
	return p, nil
}

func scanProposal(r interface{ Scan(...any) error }) (Proposal, error) {
	var p Proposal
	var items, retire, created string
	var decided sql.NullString
	if err := r.Scan(&p.ID, &p.SpaceID, &p.Role, &p.Kind, &p.Brief, &items,
		&retire, &p.Status, &created, &decided, &p.BotID); err != nil {
		return Proposal{}, err
	}
	if err := json.Unmarshal([]byte(items), &p.Items); err != nil {
		return Proposal{}, fmt.Errorf("proposal %d: bad items: %w", p.ID, err)
	}
	if err := json.Unmarshal([]byte(retire), &p.Retire); err != nil {
		return Proposal{}, fmt.Errorf("proposal %d: bad retire: %w", p.ID, err)
	}
	t, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return Proposal{}, fmt.Errorf("proposal %d: bad created_at: %w", p.ID, err)
	}
	p.CreatedAt = t
	if decided.Valid {
		d, err := time.Parse(time.RFC3339Nano, decided.String)
		if err != nil {
			return Proposal{}, fmt.Errorf("proposal %d: bad decided_at: %w", p.ID, err)
		}
		p.DecidedAt = &d
	}
	return p, nil
}
