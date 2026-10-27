package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ProofCommand is a command the lead ran as proof, with the output it showed.
type ProofCommand struct {
	Command string `json:"command"`
	Output  string `json:"output"`
}

// LineTick sets whether acceptance line Position of a Work item is ticked, and its note.
type LineTick struct {
	Position int    `json:"position"`
	Checked  bool   `json:"checked"`
	Note     string `json:"note,omitempty"`
}

// Proof is what the lead submits a top-level Work item with: the item's acceptance lines as
// ticked, the commands it ran and the screenshot files it took. SpaceID is the Space it
// submitted from.
type Proof struct {
	SpaceID         int64            `json:"spaceId"`
	AcceptanceLines []AcceptanceLine `json:"acceptanceLines"`
	Commands        []ProofCommand   `json:"commands"`
	Screenshots     []string         `json:"screenshots"`
	SubmittedAt     time.Time        `json:"submittedAt"`
}

// SubmitForReview ticks the lines of top-level item id as ticks say, stores p as its proof and
// moves it to review as BySubmit. Lines that ticks leave out keep their tick and note. A
// refused move changes nothing; an unknown line position is ErrInvalid.
func (s *Store) SubmitForReview(ctx context.Context, id int64, ticks []LineTick, p Proof) (WorkItem, error) {
	w, err := s.WorkItem(ctx, id)
	if err != nil {
		return WorkItem{}, err
	}
	if w.ParentID != nil {
		return WorkItem{}, fmt.Errorf("%w: work item %d is a sub-item, submit its parent %d",
			ErrInvalid, id, *w.ParentID)
	}
	for _, t := range ticks {
		if t.Position < 1 || t.Position > len(w.AcceptanceLines) {
			return WorkItem{}, fmt.Errorf("%w: work item %d has no acceptance line %d", ErrInvalid, id, t.Position)
		}
	}
	if _, err := s.MoveWorkItem(ctx, id, _review, BySubmit); err != nil {
		return WorkItem{}, err
	}
	commands, err := json.Marshal(append([]ProofCommand{}, p.Commands...))
	if err != nil {
		return WorkItem{}, fmt.Errorf("encode commands: %w", err)
	}
	screenshots, err := json.Marshal(append([]string{}, p.Screenshots...))
	if err != nil {
		return WorkItem{}, fmt.Errorf("encode screenshots: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return WorkItem{}, fmt.Errorf("begin submit: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, t := range ticks {
		if _, err := tx.ExecContext(ctx,
			"UPDATE acceptance_line SET checked = ?, note = ? WHERE work_item_id = ? AND position = ?",
			t.Checked, t.Note, id, t.Position); err != nil {
			return WorkItem{}, fmt.Errorf("tick acceptance line: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT OR REPLACE INTO proof (work_item_id, space_id, commands, screenshots, submitted_at, "+
			"wrap_started) VALUES (?, ?, ?, ?, ?, 0)",
		id, p.SpaceID, commands, screenshots, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return WorkItem{}, fmt.Errorf("write proof: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return WorkItem{}, fmt.Errorf("commit submit: %w", err)
	}
	return s.WorkItem(ctx, id)
}

// Proof returns what the item was last submitted with, with its acceptance lines as they are
// now, or ErrNotFound when it was never submitted.
func (s *Store) Proof(ctx context.Context, id int64) (Proof, error) {
	var p Proof
	var commands, screenshots, submitted string
	err := s.db.QueryRowContext(ctx,
		"SELECT space_id, commands, screenshots, submitted_at FROM proof WHERE work_item_id = ?", id).
		Scan(&p.SpaceID, &commands, &screenshots, &submitted)
	if errors.Is(err, sql.ErrNoRows) {
		return Proof{}, fmt.Errorf("proof of work item %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return Proof{}, fmt.Errorf("read proof: %w", err)
	}
	if err := json.Unmarshal([]byte(commands), &p.Commands); err != nil {
		return Proof{}, fmt.Errorf("proof of work item %d: bad commands: %w", id, err)
	}
	if err := json.Unmarshal([]byte(screenshots), &p.Screenshots); err != nil {
		return Proof{}, fmt.Errorf("proof of work item %d: bad screenshots: %w", id, err)
	}
	if p.SubmittedAt, err = time.Parse(time.RFC3339Nano, submitted); err != nil {
		return Proof{}, fmt.Errorf("proof of work item %d: bad submitted_at: %w", id, err)
	}
	w, err := s.WorkItem(ctx, id)
	if err != nil {
		return Proof{}, err
	}
	p.AcceptanceLines = w.AcceptanceLines
	return p, nil
}

// ClaimWrap reports whether Space id submitted its Work item for review and no one has started
// its wrap-up yet, and marks the wrap-up started: only the first caller gets true.
func (s *Store) ClaimWrap(ctx context.Context, spaceID int64) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		"UPDATE proof SET wrap_started = 1 WHERE space_id = ? AND wrap_started = 0", spaceID)
	if err != nil {
		return false, fmt.Errorf("claim wrap-up: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("claim wrap-up: %w", err)
	}
	return n > 0, nil
}
