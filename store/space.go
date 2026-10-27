package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// TokenBudget is the default Space budget and what continuing over budget leaves free.
const TokenBudget = 1_000_000

// HopCap is how many turns woken by agent messages a Space runs between user messages.
const HopCap = 6

// Active-turn kinds stored on space.active_turn, a live-state hint for processes
// that cannot see the server's in-memory chain (such as reef mcp).
const (
	ActiveTurnNone     = ""
	ActiveTurnStarting = "starting"
	ActiveTurnMessage  = "message"
	ActiveTurnWrap     = "wrap"
	ActiveTurnPromote  = "promote"
)

// validActiveTurn reports whether kind is one of the stored active-turn kinds.
func validActiveTurn(kind string) bool {
	switch kind {
	case ActiveTurnNone, ActiveTurnStarting, ActiveTurnMessage, ActiveTurnWrap, ActiveTurnPromote:
		return true
	}
	return false
}

const _spaceCols = "space.id, space.project_id, space.feature_id, space.name, " +
	"space.role, space.backend, space.permission_mode, space.paused, " +
	"space.token_budget, space.created_at, space.closed_at, space.active_turn, " +
	"COALESCE((SELECT name FROM team WHERE id = space.team_id), '" + SoloTeam + "')"

// Space with a nil FeatureID is a quick session.
type Space struct {
	ID             int64      `json:"id"`
	ProjectID      int64      `json:"projectId"`
	FeatureID      *int64     `json:"featureId"`
	Name           string     `json:"name"`
	Role           string     `json:"role"`
	Backend        string     `json:"backend"`
	PermissionMode string     `json:"permissionMode"`
	Paused         string     `json:"paused"`
	TokenBudget    int64      `json:"tokenBudget"`
	Team           string     `json:"team"`
	CreatedAt      time.Time  `json:"createdAt"`
	ClosedAt       *time.Time `json:"closedAt"`
	// ActiveTurn is the kind of turn currently running on the Space ("message",
	// "wrap", "promote", or "" when idle). It is a live-state hint for processes that
	// cannot see the server's in-memory chain, such as reef mcp; it is not client data.
	ActiveTurn string `json:"-"`
}

// SpacePreview is a Space with its newest message, nil when it has none.
type SpacePreview struct {
	Space
	LastMessage *LastMessage `json:"lastMessage"`
}

// _previewChars is how many characters of the newest message a Space preview keeps.
const _previewChars = "200"

// LastMessage is the author, body (cut to its first 200 characters) and time of a Space's
// newest message.
type LastMessage struct {
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
}

// SpaceFilter narrows Spaces to one project and/or one feature when set.
type SpaceFilter struct {
	ProjectID *int64
	FeatureID *int64
}

func (f SpaceFilter) where() (string, []any) {
	var q string
	args := []any{}
	if f.ProjectID != nil {
		q, args = q+" AND space.project_id = ?", append(args, *f.ProjectID)
	}
	if f.FeatureID != nil {
		q, args = q+" AND space.feature_id = ?", append(args, *f.FeatureID)
	}
	return q, args
}

// CreateSpace adds c in team c.Team (solo when empty), which must exist. A set
// FeatureID must be a feature of c.ProjectID.
func (s *Store) CreateSpace(ctx context.Context, c Space) (Space, error) {
	if err := c.validate(); err != nil {
		return Space{}, err
	}
	if err := s.checkFeatureInProject(ctx, c.FeatureID, c.ProjectID); err != nil {
		return Space{}, err
	}
	if c.Team == "" {
		c.Team = SoloTeam
	}
	var teamID int64
	err := s.db.QueryRowContext(ctx, "SELECT id FROM team WHERE name = ?", c.Team).Scan(&teamID)
	if errors.Is(err, sql.ErrNoRows) {
		return Space{}, fmt.Errorf("%w: unknown team %s", ErrInvalid, c.Team)
	}
	if err != nil {
		return Space{}, fmt.Errorf("read team: %w", err)
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO space (project_id, feature_id, name, role, backend, permission_mode, "+
			"token_budget, created_at, team_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		c.ProjectID, c.FeatureID, c.Name, c.Role, c.Backend, c.PermissionMode, TokenBudget,
		now.Format(time.RFC3339Nano), teamID)
	if err != nil {
		return Space{}, s.writeErr(ctx, "insert space", err, ErrInvalid,
			parent{table: "project", id: &c.ProjectID}, parent{table: "feature", id: c.FeatureID})
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Space{}, fmt.Errorf("insert Space: %w", err)
	}
	c.ID, c.Paused, c.TokenBudget, c.CreatedAt = id, "", TokenBudget, now
	return c, nil
}

// Space returns the Space with the id, or ErrNotFound.
func (s *Store) Space(ctx context.Context, id int64) (Space, error) {
	row := s.db.QueryRowContext(ctx, "SELECT "+_spaceCols+" FROM space WHERE id = ?", id)
	c, err := scanSpace(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Space{}, fmt.Errorf("space %d: %w", id, ErrNotFound)
	}
	return c, err
}

// FeatureLatestSpace returns the feature's Space with the newest message, or, when it
// has none, its most recently created Space. ErrNotFound when the feature has no Space.
func (s *Store) FeatureLatestSpace(ctx context.Context, featureID int64) (Space, error) {
	row := s.db.QueryRowContext(ctx,
		"SELECT "+_spaceCols+` FROM space LEFT JOIN message m ON m.space_id = space.id
		 WHERE space.feature_id = ?
		 GROUP BY space.id
		 ORDER BY MAX(m.id) IS NULL, MAX(m.id) DESC, space.created_at DESC
		 LIMIT 1`,
		featureID)
	c, err := scanSpace(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Space{}, fmt.Errorf("feature %d has no Space: %w", featureID, ErrNotFound)
	}
	return c, err
}

// Spaces lists Spaces by id, narrowed by the set fields of f.
func (s *Store) Spaces(ctx context.Context, f SpaceFilter) ([]Space, error) {
	where, args := f.where()
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+_spaceCols+" FROM space WHERE 1 = 1"+where+" ORDER BY space.id", args...)
	if err != nil {
		return nil, fmt.Errorf("list Spaces: %w", err)
	}
	return scanAll(rows, scanSpace)
}

// SpacePreviews is Spaces with each Space's newest message, in one query.
func (s *Store) SpacePreviews(ctx context.Context, f SpaceFilter) ([]SpacePreview, error) {
	where, args := f.where()
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+_spaceCols+", m.author, substr(m.body, 1, "+_previewChars+"), m.created_at FROM space "+
			"LEFT JOIN message m ON m.id = "+
			"(SELECT MAX(id) FROM message WHERE space_id = space.id) "+
			"WHERE 1 = 1"+where+" ORDER BY space.id", args...)
	if err != nil {
		return nil, fmt.Errorf("list Space previews: %w", err)
	}
	return scanAll(rows, scanSpacePreview)
}

// UpdateSpace sets the name and permission mode of c.ID. A Space never changes
// project, and its feature changes only from nil, through PromoteSpace.
func (s *Store) UpdateSpace(ctx context.Context, c Space) (Space, error) {
	if err := c.validate(); err != nil {
		return Space{}, err
	}
	res, err := s.db.ExecContext(ctx,
		"UPDATE space SET name_auto = CASE WHEN name = ? THEN name_auto ELSE 0 END, "+
			"name = ?, permission_mode = ? WHERE id = ?",
		c.Name, c.Name, c.PermissionMode, c.ID)
	if err != nil {
		return Space{}, fmt.Errorf("update Space: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return Space{}, fmt.Errorf("update Space: %w", err)
	}
	if n == 0 {
		return Space{}, fmt.Errorf("space %d: %w", c.ID, ErrNotFound)
	}
	return s.Space(ctx, c.ID)
}

// Name steps of a Space whose name the provider may propose, stored on space.name_auto.
// A Space is user-named (0) unless it was created with a default name; a rename to any
// other name puts it back to 0 in the same statement that sets the name.
const (
	_nameUser      = 0
	_namePending   = 1
	_nameProposing = 2
)

// MarkNamePending lets the provider propose a name for Space id, once.
func (s *Store) MarkNamePending(ctx context.Context, id int64) error {
	return s.setSpace(ctx, id, "name_auto = ?", _namePending)
}

// ClaimName reports whether the caller took the one naming attempt of Space id: true only
// while the name is pending, so the first caller wins and a user-named Space is never claimed.
func (s *Store) ClaimName(ctx context.Context, id int64) (bool, error) {
	return s.nameStep(ctx, "UPDATE space SET name_auto = ? WHERE id = ? AND name_auto = ?",
		_nameProposing, id, _namePending)
}

// FinishName ends the attempt ClaimName began: it sets the name to name, or keeps the current
// one when name is empty. It reports false, changing nothing, when a rename ended the attempt
// first.
func (s *Store) FinishName(ctx context.Context, id int64, name string) (bool, error) {
	return s.nameStep(ctx,
		"UPDATE space SET name = COALESCE(NULLIF(?, ''), name), name_auto = ? "+
			"WHERE id = ? AND name_auto = ?",
		name, _nameUser, id, _nameProposing)
}

func (s *Store) nameStep(ctx context.Context, query string, args ...any) (bool, error) {
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return false, fmt.Errorf("update Space name: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("update Space name: %w", err)
	}
	return n > 0, nil
}

// PromoteSpace attaches quick session id to feature f: the existing one when f.ID
// is set, else a new one. A Space that already has a feature is ErrInvalid.
func (s *Store) PromoteSpace(ctx context.Context, id int64, f Feature) (Space, error) {
	tx, err := s.promote(ctx, id, f)
	if err != nil {
		return Space{}, err
	}
	if err := tx.Commit(); err != nil {
		return Space{}, fmt.Errorf("commit promote: %w", err)
	}
	return s.Space(ctx, id)
}

// CheckPromote returns the error PromoteSpace would return, and changes nothing.
func (s *Store) CheckPromote(ctx context.Context, id int64, f Feature) error {
	tx, err := s.promote(ctx, id, f)
	if err != nil {
		return err
	}
	return tx.Rollback()
}

// promote does the work of PromoteSpace in a transaction it leaves open for the
// caller to commit or roll back. On error it has rolled the transaction back.
func (s *Store) promote(ctx context.Context, id int64, f Feature) (_ *sql.Tx, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin promote: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	var projectID int64
	var featureID *int64
	err = tx.QueryRowContext(ctx, "SELECT project_id, feature_id FROM space WHERE id = ?", id).
		Scan(&projectID, &featureID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("space %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("read Space %d: %w", id, err)
	}
	if featureID != nil {
		return nil, fmt.Errorf("%w: Space %d already has feature %d", ErrInvalid, id, *featureID)
	}
	if f.ID != 0 {
		var inProject bool
		err = tx.QueryRowContext(ctx,
			"SELECT EXISTS (SELECT 1 FROM feature WHERE id = ? AND project_id = ?)",
			f.ID, projectID).Scan(&inProject)
		if err != nil {
			return nil, fmt.Errorf("check feature: %w", err)
		}
		if !inProject {
			return nil, fmt.Errorf("%w: feature %d is not in project %d", ErrInvalid, f.ID, projectID)
		}
	} else {
		if err := f.validateSlug(); err != nil {
			return nil, err
		}
		if err := f.validate(); err != nil {
			return nil, err
		}
		f.ProjectID = projectID
		if f, err = insertFeature(ctx, tx, f); err != nil {
			return nil, err
		}
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE space SET feature_id = ? WHERE id = ?", f.ID, id); err != nil {
		return nil, fmt.Errorf("update Space: %w", err)
	}
	return tx, nil
}

// PauseSpace pauses the Space; the reason must not be empty.
func (s *Store) PauseSpace(ctx context.Context, id int64, reason string) error {
	if reason == "" {
		return fmt.Errorf("%w: pause reason is required", ErrInvalid)
	}
	return s.setSpace(ctx, id, "paused = ?", reason)
}

// ContinueSpace clears the pause, drops the pending holds it left and raises the budget to
// at least budget.
func (s *Store) ContinueSpace(ctx context.Context, id, budget int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin continue: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx,
		"UPDATE space SET paused = '', token_budget = MAX(token_budget, ?) WHERE id = ? AND paused <> ''",
		budget, id)
	if err != nil {
		return fmt.Errorf("update Space: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update Space: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("space %d is not paused: %w", id, ErrConflict)
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE approval SET status = ? WHERE space_id = ? AND status = ? AND kind = ?",
		ApprovalRejected, id, ApprovalPending, ApprovalHold); err != nil {
		return fmt.Errorf("drop holds: %w", err)
	}
	return tx.Commit()
}

// ReopenSpace clears the Space's closed_at.
func (s *Store) ReopenSpace(ctx context.Context, id int64) error {
	return s.setSpace(ctx, id, "closed_at = NULL")
}

// SpaceTokens sums input, output and cache write tokens; cache reads are not counted.
func (s *Store) SpaceTokens(ctx context.Context, id int64) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx,
		"SELECT COALESCE(SUM(input_tokens + output_tokens + cache_write_tokens), 0) FROM turn WHERE space_id = ?",
		id).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("sum Space tokens: %w", err)
	}
	return n, nil
}

func (s *Store) setSpace(ctx context.Context, id int64, set string, args ...any) error {
	res, err := s.db.ExecContext(ctx, "UPDATE space SET "+set+" WHERE id = ?", append(args, id)...)
	if err != nil {
		return fmt.Errorf("update Space: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update Space: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("space %d: %w", id, ErrNotFound)
	}
	return nil
}

// DeleteSpace removes the Space and, by cascade, its messages and turns.
func (s *Store) DeleteSpace(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM space WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete Space: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete Space: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("space %d: %w", id, ErrNotFound)
	}
	return nil
}

func (c Space) validate() error {
	if c.Name == "" {
		return fmt.Errorf("%w: Space name is required", ErrInvalid)
	}
	return nil
}

func scanSpacePreview(r interface{ Scan(...any) error }) (SpacePreview, error) {
	var p SpacePreview
	var created string
	var author, body, lastCreated, closed sql.NullString
	// active_turn is a server-side hint (not client data), so the preview reads it without
	// exposing it: the Space JSON omits it.
	if err := r.Scan(&p.ID, &p.ProjectID, &p.FeatureID, &p.Name, &p.Role, &p.Backend,
		&p.PermissionMode, &p.Paused, &p.TokenBudget, &created, &closed, &p.ActiveTurn, &p.Team,
		&author, &body, &lastCreated); err != nil {
		return SpacePreview{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return SpacePreview{}, fmt.Errorf("space %d: bad created_at: %w", p.ID, err)
	}
	p.CreatedAt = t
	if p.ClosedAt, err = parseClosed(closed); err != nil {
		return SpacePreview{}, fmt.Errorf("space %d: %w", p.ID, err)
	}
	if !lastCreated.Valid {
		return p, nil
	}
	t, err = time.Parse(time.RFC3339Nano, lastCreated.String)
	if err != nil {
		return SpacePreview{}, fmt.Errorf("space %d: bad last message created_at: %w", p.ID, err)
	}
	p.LastMessage = &LastMessage{Author: author.String, Body: body.String, CreatedAt: t}
	return p, nil
}

func scanSpace(r interface{ Scan(...any) error }) (Space, error) {
	var c Space
	var created string
	var closed sql.NullString
	if err := r.Scan(&c.ID, &c.ProjectID, &c.FeatureID, &c.Name, &c.Role, &c.Backend,
		&c.PermissionMode, &c.Paused, &c.TokenBudget, &created, &closed, &c.ActiveTurn, &c.Team); err != nil {
		return Space{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return Space{}, fmt.Errorf("space %d: bad created_at: %w", c.ID, err)
	}
	c.CreatedAt = t
	if c.ClosedAt, err = parseClosed(closed); err != nil {
		return Space{}, fmt.Errorf("space %d: %w", c.ID, err)
	}
	return c, nil
}

// SetActiveTurn records the kind of turn currently running on Space id: "message",
// "wrap", "promote" or "" when idle. It is a live-state hint for processes that cannot
// see the server's in-memory chain, such as reef mcp.
func (s *Store) SetActiveTurn(ctx context.Context, id int64, kind string) error {
	if !validActiveTurn(kind) {
		return fmt.Errorf("%w: unknown active turn kind %q", ErrInvalid, kind)
	}
	if _, err := s.db.ExecContext(ctx, "UPDATE space SET active_turn = ? WHERE id = ?", kind, id); err != nil {
		return fmt.Errorf("set active turn: %w", err)
	}
	return nil
}

// ClearActiveTurnIf clears Space id's active-turn hint when it still shows kind, and
// leaves any other value alone. A chain knows the hint it last wrote and clears only that
// value, so its teardown that runs after a newer chain claimed the Space cannot blank the
// newer chain's hint. Clearing when the hint does not show kind is not an error.
func (s *Store) ClearActiveTurnIf(ctx context.Context, id int64, kind string) error {
	if !validActiveTurn(kind) {
		return fmt.Errorf("%w: unknown active turn kind %q", ErrInvalid, kind)
	}
	if kind == ActiveTurnNone {
		return nil
	}
	if _, err := s.db.ExecContext(ctx,
		"UPDATE space SET active_turn = '' WHERE id = ? AND active_turn = ?", id, kind); err != nil {
		return fmt.Errorf("clear active turn: %w", err)
	}
	return nil
}

// ClearStaleActiveTurns resets active_turn for Spaces that are not live. keep returns
// true for Spaces a live process still owns (such as reef chat), which are left alone.
func (s *Store) ClearStaleActiveTurns(ctx context.Context, keep func(spaceID int64) bool) error {
	rows, err := s.db.QueryContext(ctx, "SELECT id FROM space WHERE active_turn <> ''")
	if err != nil {
		return fmt.Errorf("list active turns: %w", err)
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan Space: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("list active turns: %w", err)
	}
	_ = rows.Close()
	for _, id := range ids {
		if keep != nil && keep(id) {
			continue
		}
		if _, err := s.db.ExecContext(ctx, "UPDATE space SET active_turn = '' WHERE id = ?", id); err != nil {
			return fmt.Errorf("clear active turn: %w", err)
		}
	}
	return nil
}
