package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// The statuses of a Proposal.
const (
	ProposalPending  = "pending"
	ProposalApproved = "approved"
	ProposalRejected = "rejected"
)

const _proposalCols = "id, channel_id, role, brief, notes, decisions, status, created_at, decided_at"

// ProposedDecision is a decision a proposal would record.
type ProposedDecision struct {
	Body string `json:"body"`
	Why  string `json:"why"`
}

// Proposal is what a role suggests at wrap-up: the new brief, its notes and the
// decisions it noticed. Approving it applies all three.
type Proposal struct {
	ID        int64              `json:"id"`
	ChannelID int64              `json:"channelId"`
	Role      string             `json:"role"`
	Brief     string             `json:"brief"`
	Notes     string             `json:"notes"`
	Decisions []ProposedDecision `json:"decisions"`
	Status    string             `json:"status"`
	CreatedAt time.Time          `json:"createdAt"`
	DecidedAt *time.Time         `json:"decidedAt"`
}

// CreateProposal adds p as pending. The channel must belong to a feature.
func (s *Store) CreateProposal(ctx context.Context, p Proposal) (Proposal, error) {
	if p.Role == "" {
		return Proposal{}, fmt.Errorf("%w: proposal role is required", ErrInvalid)
	}
	if p.Brief == "" {
		return Proposal{}, fmt.Errorf("%w: proposal brief is required", ErrInvalid)
	}
	for _, d := range p.Decisions {
		if d.Body == "" {
			return Proposal{}, fmt.Errorf("%w: decision body is required", ErrInvalid)
		}
	}
	c, err := s.Channel(ctx, p.ChannelID)
	if err != nil {
		return Proposal{}, err
	}
	if c.FeatureID == nil {
		return Proposal{}, fmt.Errorf("%w: channel %d has no feature", ErrInvalid, c.ID)
	}
	if p.Decisions == nil {
		p.Decisions = []ProposedDecision{}
	}
	decisions, err := json.Marshal(p.Decisions)
	if err != nil {
		return Proposal{}, fmt.Errorf("encode decisions: %w", err)
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO proposal (channel_id, role, brief, notes, decisions, created_at) "+
			"VALUES (?, ?, ?, ?, ?, ?)",
		p.ChannelID, p.Role, p.Brief, p.Notes, string(decisions), now.Format(time.RFC3339Nano))
	if err != nil {
		return Proposal{}, s.writeErr(ctx, "insert proposal", err, ErrNotFound,
			parent{table: "channel", id: &p.ChannelID})
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Proposal{}, fmt.Errorf("insert proposal: %w", err)
	}
	p.ID, p.Status, p.CreatedAt = id, ProposalPending, now
	return p, nil
}

// Proposal returns the proposal with the id, or ErrNotFound.
func (s *Store) Proposal(ctx context.Context, id int64) (Proposal, error) {
	return getProposal(ctx, s.db, id)
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

// ApproveProposal applies the pending proposal in one transaction: a new brief
// version and the role's notes, both authored by the proposal's role, and each
// proposed decision. Nothing is applied if any step fails.
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
	err = tx.QueryRowContext(ctx,
		"SELECT feature_id FROM channel WHERE id = ?", p.ChannelID).Scan(&featureID)
	if err != nil {
		return Proposal{}, fmt.Errorf("read feature of proposal %d: %w", id, err)
	}
	now := time.Now().UTC()
	at := now.Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, _insertBrief, featureID, p.Brief, p.Role, at); err != nil {
		return Proposal{}, fmt.Errorf("insert brief: %w", err)
	}
	if _, err := tx.ExecContext(ctx, _upsertRoleNotes, featureID, p.Role, p.Notes, at); err != nil {
		return Proposal{}, fmt.Errorf("upsert role notes: %w", err)
	}
	for _, d := range p.Decisions {
		if _, err := tx.ExecContext(ctx, _insertDecision,
			featureID, nil, d.Body, d.Why, p.Role, at); err != nil {
			return Proposal{}, fmt.Errorf("insert decision: %w", err)
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

type rowQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func getProposal(ctx context.Context, q rowQuerier, id int64) (Proposal, error) {
	row := q.QueryRowContext(ctx, "SELECT "+_proposalCols+" FROM proposal WHERE id = ?", id)
	p, err := scanProposal(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Proposal{}, fmt.Errorf("proposal %d: %w", id, ErrNotFound)
	}
	return p, err
}

func pendingProposal(ctx context.Context, q rowQuerier, id int64) (Proposal, error) {
	p, err := getProposal(ctx, q, id)
	if err != nil {
		return Proposal{}, err
	}
	if p.Status != ProposalPending {
		return Proposal{}, fmt.Errorf("%w: proposal %d is already %s", ErrInvalid, id, p.Status)
	}
	return p, nil
}

// decideProposal sets p's status and commits tx.
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
	var decisions, created string
	var decided sql.NullString
	if err := r.Scan(&p.ID, &p.ChannelID, &p.Role, &p.Brief, &p.Notes, &decisions,
		&p.Status, &created, &decided); err != nil {
		return Proposal{}, err
	}
	if err := json.Unmarshal([]byte(decisions), &p.Decisions); err != nil {
		return Proposal{}, fmt.Errorf("proposal %d: bad decisions: %w", p.ID, err)
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
