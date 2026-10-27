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

// Proposal kinds. A wrap ends the channel; a brief only updates the feature's brief.
const (
	ProposalWrap  = "wrap"
	ProposalBrief = "brief"
)

const _proposalCols = "id, channel_id, role, kind, brief, items, retire, status, " +
	"created_at, decided_at"

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
	ChannelID int64          `json:"channelId"`
	Role      string         `json:"role"`
	Kind      string         `json:"kind"`
	Brief     string         `json:"brief"`
	Items     []ProposedItem `json:"items"`
	Retire    []int64        `json:"retire"`
	Status    string         `json:"status"`
	CreatedAt time.Time      `json:"createdAt"`
	DecidedAt *time.Time     `json:"decidedAt"`
}

// CreateProposal adds p as pending, of kind wrap unless p.Kind says otherwise. The channel
// must belong to a feature. The channel has one pending proposal at most: a wrap rejects the
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
	c, err := scanChannel(tx.QueryRowContext(ctx,
		"SELECT "+_channelCols+" FROM channel WHERE id = ?", p.ChannelID))
	if errors.Is(err, sql.ErrNoRows) {
		return Proposal{}, fmt.Errorf("channel %d: %w", p.ChannelID, ErrNotFound)
	}
	if err != nil {
		return Proposal{}, err
	}
	if c.FeatureID == nil {
		return Proposal{}, fmt.Errorf("%w: channel %d has no feature", ErrInvalid, c.ID)
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
	reject := "UPDATE proposal SET status = ?, decided_at = ? WHERE channel_id = ? AND status = ?"
	args := []any{ProposalRejected, at, p.ChannelID, ProposalPending}
	if p.Kind == ProposalBrief {
		var wraps int
		if err := tx.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM proposal WHERE channel_id = ? AND kind = ? AND status = ?",
			p.ChannelID, ProposalWrap, ProposalPending).Scan(&wraps); err != nil {
			return Proposal{}, fmt.Errorf("check pending wrap: %w", err)
		}
		if wraps > 0 {
			return Proposal{}, fmt.Errorf("%w: channel %d has a wrap-up waiting for approval",
				ErrInvalid, p.ChannelID)
		}
		reject, args = reject+" AND kind = ?", append(args, ProposalBrief)
	}
	if _, err := tx.ExecContext(ctx, reject, args...); err != nil {
		return Proposal{}, fmt.Errorf("reject older proposals: %w", err)
	}
	res, err := tx.ExecContext(ctx,
		"INSERT INTO proposal (channel_id, role, kind, brief, items, retire, created_at) "+
			"VALUES (?, ?, ?, ?, ?, ?, ?)",
		p.ChannelID, p.Role, p.Kind, p.Brief, string(items), string(retire), at)
	if err != nil {
		return Proposal{}, s.writeErr(ctx, "insert proposal", err, ErrNotFound,
			parent{table: "channel", id: &p.ChannelID})
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

// PendingProposal returns the channel's newest pending proposal, or ErrNotFound.
func (s *Store) PendingProposal(ctx context.Context, channelID int64) (Proposal, error) {
	row := s.db.QueryRowContext(ctx,
		"SELECT "+_proposalCols+" FROM proposal WHERE channel_id = ? AND status = ? "+
			"ORDER BY id DESC LIMIT 1", channelID, ProposalPending)
	p, err := scanProposal(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Proposal{}, fmt.Errorf("pending proposal of channel %d: %w", channelID, ErrNotFound)
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

// PendingProposals returns every pending proposal, oldest first. With featureID
// set, only those of that feature's channels. With projectID set, only that project's.
func (s *Store) PendingProposals(
	ctx context.Context, featureID, projectID *int64,
) ([]Proposal, error) {
	query, args := "SELECT "+_proposalCols+" FROM proposal WHERE status = ?", []any{ProposalPending}
	if featureID != nil {
		query += " AND channel_id IN (SELECT id FROM channel WHERE feature_id = ?)"
		args = append(args, *featureID)
	}
	if projectID != nil {
		query += " AND channel_id IN (SELECT id FROM channel WHERE project_id = ?)"
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

// ApproveProposal saves the pending proposal's brief, closes the channel of a wrap and marks
// the proposal approved, in one transaction. A brief proposal is refused once the channel is
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
	var featureID int64
	var closed sql.NullString
	err = tx.QueryRowContext(ctx,
		"SELECT feature_id, closed_at FROM channel WHERE id = ?", p.ChannelID).
		Scan(&featureID, &closed)
	if err != nil {
		return Proposal{}, fmt.Errorf("read feature of proposal %d: %w", id, err)
	}
	if p.Kind == ProposalBrief && closed.Valid {
		return Proposal{}, fmt.Errorf("%w: channel %d is closed", ErrInvalid, p.ChannelID)
	}
	now := time.Now().UTC()
	at := now.Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, _insertBrief, featureID, p.Brief, p.Role, at); err != nil {
		return Proposal{}, fmt.Errorf("insert brief: %w", err)
	}
	if p.Kind == ProposalWrap {
		if _, err := tx.ExecContext(ctx,
			"UPDATE channel SET closed_at = ? WHERE id = ?", at, p.ChannelID); err != nil {
			return Proposal{}, fmt.Errorf("close channel: %w", err)
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
	if err := r.Scan(&p.ID, &p.ChannelID, &p.Role, &p.Kind, &p.Brief, &items,
		&retire, &p.Status, &created, &decided); err != nil {
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
