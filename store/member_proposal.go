package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// MemberProposal is a Bot's suggestion to add another Bot to its Space. The user decides; only a
// pending one can be approved or rejected.
type MemberProposal struct {
	ID         int64      `json:"id"`
	SpaceID    int64      `json:"spaceId"`
	BotID      int64      `json:"botId"`
	BotName    string     `json:"botName"`
	ProposedBy string     `json:"proposedBy"`
	Reason     string     `json:"reason"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"createdAt"`
	DecidedAt  *time.Time `json:"decidedAt"`
}

const _memberProposalCols = "p.id, p.space_id, p.bot_id, bot.name, p.proposed_by, p.reason, p.status, " +
	"p.created_at, p.decided_at"

const _memberProposalFrom = " FROM member_proposal p JOIN bot ON bot.id = p.bot_id"

// CreateMemberProposal records that proposedBy suggests adding the Bot botID to Space id. A Bot
// already proposed and not yet decided returns the pending proposal.
func (s *Store) CreateMemberProposal(
	ctx context.Context, id int64, proposedBy string, botID int64, reason string,
) (MemberProposal, error) {
	c, err := s.Space(ctx, id)
	if err != nil {
		return MemberProposal{}, err
	}
	if c.Kind == KindConversation || c.Kind == KindConsultation {
		return MemberProposal{}, fmt.Errorf("%w: Space %d is a %s with one Bot", ErrInvalid, id, c.Kind)
	}
	if strings.TrimSpace(reason) == "" {
		return MemberProposal{}, fmt.Errorf("%w: a member proposal needs a reason", ErrInvalid)
	}
	var member bool
	if err := s.db.QueryRowContext(ctx,
		"SELECT EXISTS (SELECT 1 FROM space_member WHERE space_id = ? AND bot_id = ?)", id, botID).
		Scan(&member); err != nil {
		return MemberProposal{}, fmt.Errorf("check member: %w", err)
	}
	if member {
		return MemberProposal{}, fmt.Errorf("%w: bot %d is already in Space %d", ErrConflict, botID, id)
	}
	res, err := s.db.ExecContext(ctx,
		"INSERT OR IGNORE INTO member_proposal (space_id, bot_id, proposed_by, reason, created_at) "+
			"SELECT ?, id, ?, ?, ? FROM bot WHERE id = ?",
		id, proposedBy, reason, time.Now().UTC().Format(time.RFC3339Nano), botID)
	if err != nil {
		return MemberProposal{}, fmt.Errorf("insert member proposal: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		var exists bool
		if err := s.db.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM bot WHERE id = ?)", botID).
			Scan(&exists); err != nil {
			return MemberProposal{}, fmt.Errorf("check bot: %w", err)
		}
		if !exists {
			return MemberProposal{}, fmt.Errorf("%w: unknown bot %d", ErrInvalid, botID)
		}
		return scanMemberProposal(s.db.QueryRowContext(ctx, "SELECT "+_memberProposalCols+_memberProposalFrom+
			" WHERE p.space_id = ? AND p.bot_id = ? AND p.status = 'pending'", id, botID))
	}
	pid, err := res.LastInsertId()
	if err != nil {
		return MemberProposal{}, fmt.Errorf("insert member proposal: %w", err)
	}
	return s.MemberProposal(ctx, pid)
}

// MemberProposal returns the proposal with the id in any status, or ErrNotFound.
func (s *Store) MemberProposal(ctx context.Context, id int64) (MemberProposal, error) {
	p, err := scanMemberProposal(s.db.QueryRowContext(ctx,
		"SELECT "+_memberProposalCols+_memberProposalFrom+" WHERE p.id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return MemberProposal{}, fmt.Errorf("member proposal %d: %w", id, ErrNotFound)
	}
	return p, err
}

// PendingMemberProposals lists the undecided member proposals of Space id, oldest first.
func (s *Store) PendingMemberProposals(ctx context.Context, id int64) ([]MemberProposal, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+_memberProposalCols+_memberProposalFrom+
		" WHERE p.space_id = ? AND p.status = 'pending' ORDER BY p.id", id)
	if err != nil {
		return nil, fmt.Errorf("list member proposals: %w", err)
	}
	return scanAll(rows, scanMemberProposal)
}

// DecideMemberProposal marks the pending proposal id approved or rejected. Of two calls only one
// succeeds.
func (s *Store) DecideMemberProposal(ctx context.Context, id int64, approve bool) (MemberProposal, error) {
	status := "rejected"
	if approve {
		status = "approved"
	}
	res, err := s.db.ExecContext(ctx,
		"UPDATE member_proposal SET status = ?, decided_at = ? WHERE id = ? AND status = 'pending'",
		status, time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return MemberProposal{}, fmt.Errorf("decide member proposal %d: %w", id, err)
	}
	p, err := s.MemberProposal(ctx, id)
	if err != nil {
		return MemberProposal{}, err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return MemberProposal{}, fmt.Errorf("%w: member proposal %d is already %s", ErrConflict, id, p.Status)
	}
	return p, nil
}

func scanMemberProposal(r interface{ Scan(...any) error }) (MemberProposal, error) {
	var p MemberProposal
	var created string
	var decided sql.NullString
	if err := r.Scan(&p.ID, &p.SpaceID, &p.BotID, &p.BotName, &p.ProposedBy, &p.Reason, &p.Status,
		&created, &decided); err != nil {
		return MemberProposal{}, err
	}
	var err error
	if p.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return MemberProposal{}, fmt.Errorf("member proposal %d: bad created_at: %w", p.ID, err)
	}
	if decided.Valid {
		d, err := time.Parse(time.RFC3339Nano, decided.String)
		if err != nil {
			return MemberProposal{}, fmt.Errorf("member proposal %d: bad decided_at: %w", p.ID, err)
		}
		p.DecidedAt = &d
	}
	return p, nil
}
