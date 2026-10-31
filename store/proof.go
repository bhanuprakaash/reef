package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// ProofCommand is a command the owner ran as proof, with the output it showed.
type ProofCommand struct {
	Command   string `json:"command"`
	Output    string `json:"output"`
	Project   string `json:"project,omitempty" jsonschema:"name of the Project (repository) the command checked; leave out when it checks the whole Work item"`
	ProjectID int64  `json:"projectId,omitempty" jsonschema:"set by Reef from project; leave out"`
}

// ProofRepository is one repository of a submitted assignment as it stood at submit: the
// workspace, its branch, head commit, base, and whether it had uncommitted changes. Checks are
// the proof commands run for its Project.
type ProofRepository struct {
	WorkspaceID int64          `json:"workspaceId"`
	ProjectID   int64          `json:"projectId"`
	ProjectName string         `json:"projectName"`
	Branch      string         `json:"branch"`
	Head        string         `json:"head"`
	BaseRef     string         `json:"baseRef"`
	Dir         string         `json:"dir"`
	Dirty       bool           `json:"dirty"`
	Checks      []ProofCommand `json:"checks"`
}

// LineTick sets whether acceptance line Position of a Work item is ticked, and its note.
type LineTick struct {
	Position int    `json:"position"`
	Checked  bool   `json:"checked"`
	Note     string `json:"note,omitempty"`
}

// Proof is what the owner submits a top-level Work item with: the item's acceptance lines as
// ticked, the commands it ran and the screenshot files it took. SpaceID is the Space it
// submitted from.
type Proof struct {
	SpaceID         int64             `json:"spaceId"`
	AcceptanceLines []AcceptanceLine  `json:"acceptanceLines"`
	Commands        []ProofCommand    `json:"commands"`
	Repositories    []ProofRepository `json:"repositories"`
	Screenshots     []string          `json:"screenshots"`
	SubmittedAt     time.Time         `json:"submittedAt"`
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
	if _, err := tx.ExecContext(ctx, "DELETE FROM proof_repository WHERE work_item_id = ?", id); err != nil {
		return WorkItem{}, fmt.Errorf("clear proof repositories: %w", err)
	}
	for _, r := range p.Repositories {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO proof_repository (work_item_id, workspace_id, project_id, branch, head, base_ref, dir, dirty) "+
				"VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
			id, r.WorkspaceID, r.ProjectID, r.Branch, r.Head, r.BaseRef, r.Dir, r.Dirty); err != nil {
			return WorkItem{}, fmt.Errorf("write proof repository: %w", err)
		}
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
	if p.Repositories, err = s.proofRepositories(ctx, id); err != nil {
		return Proof{}, err
	}
	rest := []ProofCommand{}
	for _, c := range p.Commands {
		i := slices.IndexFunc(p.Repositories, func(r ProofRepository) bool { return r.ProjectID == c.ProjectID })
		if c.ProjectID == 0 || i < 0 {
			rest = append(rest, c)
			continue
		}
		p.Repositories[i].Checks = append(p.Repositories[i].Checks, c)
	}
	p.Commands = rest
	return p, nil
}

func (s *Store) proofRepositories(ctx context.Context, id int64) ([]ProofRepository, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT r.workspace_id, r.project_id, p.name, r.branch, r.head, r.base_ref, r.dir, r.dirty "+
			"FROM proof_repository r JOIN project p ON p.id = r.project_id "+
			"WHERE r.work_item_id = ? ORDER BY r.workspace_id", id)
	if err != nil {
		return nil, fmt.Errorf("read proof repositories: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []ProofRepository{}
	for rows.Next() {
		r := ProofRepository{Checks: []ProofCommand{}}
		if err := rows.Scan(&r.WorkspaceID, &r.ProjectID, &r.ProjectName, &r.Branch, &r.Head, &r.BaseRef,
			&r.Dir, &r.Dirty); err != nil {
			return nil, fmt.Errorf("scan proof repository: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SendBack puts the item in review at the front of the queue, keeps note for the owner's
// next wake (see ClaimSendBack) and rejects the wrap-up proposal its Space waits on, which the
// next submit replaces. A blank note is ErrInvalid.
func (s *Store) SendBack(ctx context.Context, id int64, note string) (WorkItem, error) {
	if strings.TrimSpace(note) == "" {
		return WorkItem{}, fmt.Errorf("%w: a note for the owner is required to send work item %d back", ErrInvalid, id)
	}
	p, err := s.Proof(ctx, id)
	if err != nil {
		return WorkItem{}, err
	}
	if _, err := s.db.ExecContext(ctx,
		"UPDATE proof SET send_back_note = ? WHERE work_item_id = ?", note, id); err != nil {
		return WorkItem{}, fmt.Errorf("keep send-back note: %w", err)
	}
	w, err := s.MoveWorkItem(ctx, id, _queued, ByUser)
	if err != nil {
		return WorkItem{}, err
	}
	pending, err := s.PendingProposal(ctx, p.SpaceID)
	switch {
	case errors.Is(err, ErrNotFound), err == nil && pending.Kind != ProposalWrap:
		return w, nil
	case err != nil:
		return WorkItem{}, err
	}
	_, err = s.RejectProposal(ctx, pending.ID)
	return w, err
}

// ClaimSendBack returns the Space the item was submitted from and the note it was sent back
// with, and forgets the note: only the first caller gets ok.
func (s *Store) ClaimSendBack(ctx context.Context, id int64) (spaceID int64, note string, ok bool, err error) {
	err = s.db.QueryRowContext(ctx,
		"SELECT space_id, send_back_note FROM proof WHERE work_item_id = ? AND send_back_note != ''", id).
		Scan(&spaceID, &note)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", false, nil
	}
	if err != nil {
		return 0, "", false, fmt.Errorf("read send-back note: %w", err)
	}
	res, err := s.db.ExecContext(ctx,
		"UPDATE proof SET send_back_note = '' WHERE work_item_id = ? AND send_back_note = ?", id, note)
	if err != nil {
		return 0, "", false, fmt.Errorf("claim send-back note: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, "", false, fmt.Errorf("claim send-back note: %w", err)
	}
	return spaceID, note, n > 0, nil
}

// UnwrappedSpaces returns the open Spaces of items in review whose wrap-up no one has started,
// as when the server quit right after the submit.
func (s *Store) UnwrappedSpaces(ctx context.Context) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT proof.space_id FROM proof JOIN work_item w ON w.id = proof.work_item_id "+
			"JOIN space ON space.id = proof.space_id "+
			"WHERE proof.wrap_started = 0 AND w.status = ? AND space.closed_at IS NULL", _review)
	if err != nil {
		return nil, fmt.Errorf("list Spaces without a wrap-up: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan Space without a wrap-up: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ClaimWrap reports whether Space id submitted its Work item, which is still in review, and no
// one has started its wrap-up yet, and marks the wrap-up started: only the first caller gets true.
func (s *Store) ClaimWrap(ctx context.Context, spaceID int64) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		"UPDATE proof SET wrap_started = 1 WHERE space_id = ? AND wrap_started = 0 "+
			"AND work_item_id IN (SELECT id FROM work_item WHERE status = ?)", spaceID, _review)
	if err != nil {
		return false, fmt.Errorf("claim wrap-up: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("claim wrap-up: %w", err)
	}
	return n > 0, nil
}
