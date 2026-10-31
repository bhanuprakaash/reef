package store

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
)

// SpaceProposal is a Bot's suggestion for a new Space: its participants and coordinator. It
// creates nothing; the user accepts or changes it in the new Space form, then dismisses it.
type SpaceProposal struct {
	ID               int64   `json:"id"`
	ProposedBy       string  `json:"proposedBy"`
	Name             string  `json:"name"`
	BotIDs           []int64 `json:"botIds"`
	CoordinatorBotID int64   `json:"coordinatorBotId"`
}

// CreateSpaceProposal stores p with its Bots de-duplicated. The coordinator, or the first Bot
// when it is 0, must be one of them, and every Bot must exist.
func (s *Store) CreateSpaceProposal(ctx context.Context, p SpaceProposal) (SpaceProposal, error) {
	if p.ProposedBy == "" || strings.TrimSpace(p.Name) == "" {
		return SpaceProposal{}, fmt.Errorf("%w: a Space proposal needs a proposer and a name", ErrInvalid)
	}
	var ids []int64
	for _, id := range p.BotIDs {
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	p.BotIDs = ids
	if len(p.BotIDs) == 0 {
		return SpaceProposal{}, fmt.Errorf("%w: a Space needs at least one Bot", ErrInvalid)
	}
	if p.CoordinatorBotID == 0 {
		p.CoordinatorBotID = p.BotIDs[0]
	}
	if !slices.Contains(p.BotIDs, p.CoordinatorBotID) {
		return SpaceProposal{}, fmt.Errorf("%w: coordinator %d is not among the Bots", ErrInvalid, p.CoordinatorBotID)
	}
	for _, id := range p.BotIDs {
		if _, err := s.Bot(ctx, id); err != nil {
			return SpaceProposal{}, fmt.Errorf("%w: unknown bot %d", ErrInvalid, id)
		}
	}
	encoded, err := json.Marshal(p.BotIDs)
	if err != nil {
		return SpaceProposal{}, fmt.Errorf("encode bot ids: %w", err)
	}
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO space_proposal (proposed_by, name, bot_ids, coordinator_bot_id, created_at) VALUES (?, ?, ?, ?, ?)",
		p.ProposedBy, p.Name, string(encoded), p.CoordinatorBotID, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return SpaceProposal{}, fmt.Errorf("insert Space proposal: %w", err)
	}
	if p.ID, err = res.LastInsertId(); err != nil {
		return SpaceProposal{}, fmt.Errorf("Space proposal id: %w", err)
	}
	return p, nil
}

func (s *Store) spaceProposalItems(ctx context.Context) ([]InboxItem, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, proposed_by, name, bot_ids, coordinator_bot_id, created_at FROM space_proposal")
	if err != nil {
		return nil, fmt.Errorf("list Space proposals: %w", err)
	}
	return scanAll(rows, func(r interface{ Scan(...any) error }) (InboxItem, error) {
		var p SpaceProposal
		var ids, at string
		if err := r.Scan(&p.ID, &p.ProposedBy, &p.Name, &ids, &p.CoordinatorBotID, &at); err != nil {
			return InboxItem{}, err
		}
		if err := json.Unmarshal([]byte(ids), &p.BotIDs); err != nil {
			return InboxItem{}, fmt.Errorf("Space proposal %d: bad bot ids: %w", p.ID, err)
		}
		t, err := time.Parse(time.RFC3339Nano, at)
		if err != nil {
			return InboxItem{}, fmt.Errorf("Space proposal %d: bad time: %w", p.ID, err)
		}
		return InboxItem{
			Kind: KindSpaceProposed, Group: GroupFYI, At: t, ID: &p.ID, Proposal: &p,
			Text: fmt.Sprintf("%s proposed a Space: %s", p.ProposedBy, p.Name),
		}, nil
	})
}
