package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// TokenBudget is the default token budget of a top-level Work item or free Space, and what
// continuing over budget leaves free.
const TokenBudget = 1_000_000

// HopCap is how many talk-only turns woken by agent messages a Space runs in a row: a user
// message or a turn that calls a tool other than Reef's own starts the count again.
const HopCap = 6

// Active-turn kinds stored on space.active_turn, a live-state hint for processes
// that cannot see the server's in-memory chain (such as reef mcp).
const (
	ActiveTurnNone     = ""
	ActiveTurnStarting = "starting"
	ActiveTurnMessage  = "message"
	ActiveTurnWrap     = "wrap"
)

// validActiveTurn reports whether kind is one of the stored active-turn kinds.
func validActiveTurn(kind string) bool {
	switch kind {
	case ActiveTurnNone, ActiveTurnStarting, ActiveTurnMessage, ActiveTurnWrap:
		return true
	}
	return false
}

// Space kinds: a Work item's Space, a free Space with no Work item, and a direct conversation
// with one Bot.
const (
	KindWorkItem     = "work_item"
	KindFree         = "free"
	KindConversation = "conversation"
)

// A Space's project is its work item's project, or its first associated project, or 0.
const _spaceCols = "space.id, COALESCE(space.project_id, " +
	"(SELECT MIN(project_id) FROM space_project WHERE space_id = space.id), 0), " +
	"space.work_item_id, space.name, " +
	"COALESCE((SELECT bot.name FROM space_member JOIN bot ON bot.id = space_member.bot_id " +
	"WHERE space_member.space_id = space.id AND space_member.lead = 1), space.role), space.paused, " +
	"COALESCE((SELECT root.token_budget FROM work_item item JOIN work_item root " +
	"ON root.id = COALESCE(item.parent_id, item.id) WHERE item.id = space.work_item_id), " +
	"space.token_budget), space.created_at, space.closed_at, space.active_turn, " +
	"COALESCE((SELECT name FROM team WHERE id = space.team_id), ''), space.kind, space.moved_to, " +
	"(SELECT bot_id FROM space_member WHERE space_id = space.id AND lead = 1)"

// Space with a nil WorkItemID is a free Space or a direct conversation. ProjectID is the
// Space's first associated project, 0 when it has none; only the Space of a work item must
// have one. Team is empty for a
// Space made from one Bot. Who is in the Space is its space_member list of Bot ids, set when the
// Space was made. BotID is the lead Bot and Role is that Bot's current name.
type Space struct {
	ID          int64      `json:"id"`
	Kind        string     `json:"kind"`
	ProjectID   int64      `json:"projectId"`
	WorkItemID  *int64     `json:"workItemId"`
	Name        string     `json:"name"`
	Role        string     `json:"role"`
	BotID       *int64     `json:"botId"`
	Paused      string     `json:"paused"`
	TokenBudget int64      `json:"tokenBudget"`
	Team        string     `json:"team"`
	CreatedAt   time.Time  `json:"createdAt"`
	ClosedAt    *time.Time `json:"closedAt"`
	// MovedTo is the Space a direct conversation became when a Bot was added: the
	// conversation is read-only from then on.
	MovedTo *int64 `json:"movedTo"`
	// ActiveTurn is the kind of turn currently running on the Space ("message" or
	// "wrap", or "" when idle). It is a live-state hint for processes that
	// cannot see the server's in-memory chain, such as reef mcp; it is not client data.
	ActiveTurn string `json:"-"`
}

// Writable is nil when the Space takes new messages and turns. A closed Space and a moved
// conversation refuse them with an error wrapping ErrConflict; the moved one names the
// Space it became.
func (c Space) Writable() error {
	if c.ClosedAt != nil {
		return fmt.Errorf("space %d is closed: %w", c.ID, ErrConflict)
	}
	if c.MovedTo != nil {
		return fmt.Errorf("conversation %d moved to Space %d: %w", c.ID, *c.MovedTo, ErrConflict)
	}
	return nil
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

// SpaceFilter narrows Spaces when set: to those associated with a project, to those whose
// only project is OnlyProjectID, and to one work item.
type SpaceFilter struct {
	ProjectID     *int64
	OnlyProjectID *int64
	WorkItemID    *int64
}

func (f SpaceFilter) where() (string, []any) {
	var q string
	args := []any{}
	if f.ProjectID != nil {
		q, args = q+" AND EXISTS (SELECT 1 FROM space_project WHERE space_id = space.id "+
			"AND project_id = ?)", append(args, *f.ProjectID)
	}
	if f.OnlyProjectID != nil {
		q, args = q+" AND space.id IN (SELECT space_id FROM space_project GROUP BY space_id "+
			"HAVING COUNT(*) = 1 AND MIN(project_id) = ?)", append(args, *f.OnlyProjectID)
	}
	if f.WorkItemID != nil {
		q, args = q+" AND space.work_item_id = ?", append(args, *f.WorkItemID)
	}
	return q, args
}

// CreateSpace adds c with a copy of the members of team c.Team (solo when empty), which must
// exist. A set WorkItemID must be a work item of c.ProjectID, and that item must have no
// other active Space.
func (s *Store) CreateSpace(ctx context.Context, c Space) (Space, error) {
	if err := c.validate(); err != nil {
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
	c.Kind = KindFree
	if c.WorkItemID != nil {
		c.Kind = KindWorkItem
	}
	return s.insertSpace(ctx, c, &teamID, func(tx *sql.Tx, id int64) error {
		_, err := tx.ExecContext(ctx,
			"INSERT INTO space_member (space_id, bot_id, lead) SELECT ?, bot_id, lead "+
				"FROM team_member WHERE team_id = ? ORDER BY lead DESC, id", id, teamID)
		if err != nil {
			return err
		}
		return ensureCoordinator(ctx, tx, id)
	})
}

// ensureCoordinator makes the first member of Space id its coordinator when none leads.
func ensureCoordinator(ctx context.Context, x execer, id int64) error {
	_, err := x.ExecContext(ctx,
		"UPDATE space_member SET lead = 1 WHERE id = (SELECT MIN(id) FROM space_member WHERE space_id = ?) "+
			"AND NOT EXISTS (SELECT 1 FROM space_member WHERE space_id = ? AND lead = 1)", id, id)
	return err
}

// CreateBotSpace adds c with no Team and one member, the Bot, which leads it. c.Kind is
// KindConversation for a direct conversation, which cannot have a work item; any other value makes a free Space, or a work item's Space when WorkItemID is set.
func (s *Store) CreateBotSpace(ctx context.Context, c Space, botID int64) (Space, error) {
	if err := c.validate(); err != nil {
		return Space{}, err
	}
	switch {
	case c.Kind == KindConversation && c.WorkItemID != nil:
		return Space{}, fmt.Errorf("%w: a direct conversation has no work item", ErrInvalid)
	case c.Kind != KindConversation && c.WorkItemID != nil:
		c.Kind = KindWorkItem
	case c.Kind != KindConversation:
		c.Kind = KindFree
	}
	return s.insertSpace(ctx, c, nil, func(tx *sql.Tx, id int64) error {
		res, err := tx.ExecContext(ctx,
			"INSERT INTO space_member (space_id, bot_id, lead) SELECT ?, id, 1 FROM bot WHERE id = ?",
			id, botID)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err == nil && n == 0 {
			return fmt.Errorf("%w: unknown bot %d", ErrInvalid, botID)
		}
		return nil
	})
}

// insertSpace stores c, then calls members to copy its members, in one transaction.
func (s *Store) insertSpace(
	ctx context.Context, c Space, teamID *int64, members func(tx *sql.Tx, id int64) error,
) (Space, error) {
	if err := s.checkWorkItemInProject(ctx, c.WorkItemID, c.ProjectID); err != nil {
		return Space{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Space{}, fmt.Errorf("begin create Space: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC()
	var projectID *int64
	if c.WorkItemID != nil {
		projectID = &c.ProjectID
	}
	res, err := tx.ExecContext(ctx,
		"INSERT INTO space (project_id, work_item_id, name, role, token_budget, created_at, "+
			"team_id, kind) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		projectID, c.WorkItemID, c.Name, c.Role, TokenBudget, now.Format(time.RFC3339Nano), teamID, c.Kind)
	if err != nil {
		_ = tx.Rollback()
		return Space{}, s.spaceWriteErr(ctx, "insert space", err, c.WorkItemID,
			parent{table: "project", id: &c.ProjectID}, parent{table: "work_item", id: c.WorkItemID})
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Space{}, fmt.Errorf("insert Space: %w", err)
	}
	if c.ProjectID != 0 {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO space_project (space_id, project_id) VALUES (?, ?)", id, c.ProjectID); err != nil {
			_ = tx.Rollback()
			return Space{}, s.writeErr(ctx, "insert Space project", err, ErrInvalid,
				parent{table: "project", id: &c.ProjectID})
		}
	}
	if err := members(tx, id); err != nil {
		return Space{}, fmt.Errorf("insert Space members: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Space{}, fmt.Errorf("commit create Space: %w", err)
	}
	return s.Space(ctx, id)
}

// spaceWriteErr turns a failed write of a Space row into ErrConflict when the work item
// already has an active Space, and otherwise as writeErr does.
func (s *Store) spaceWriteErr(
	ctx context.Context, op string, err error, workItemID *int64, parents ...parent,
) error {
	if isUnique(err) && workItemID != nil {
		return fmt.Errorf("%w: work item %d already has an active Space", ErrConflict, *workItemID)
	}
	return s.writeErr(ctx, op, err, ErrInvalid, parents...)
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

// WorkItemActiveSpace returns the item's open Space, or ErrNotFound when it has none.
func (s *Store) WorkItemActiveSpace(ctx context.Context, workItemID int64) (Space, error) {
	row := s.db.QueryRowContext(ctx,
		"SELECT "+_spaceCols+" FROM space WHERE work_item_id = ? AND closed_at IS NULL", workItemID)
	c, err := scanSpace(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Space{}, fmt.Errorf("work item %d has no active Space: %w", workItemID, ErrNotFound)
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
	return s.spacePreviews(ctx, where, args, "space.id")
}

// BotConversations is the direct conversations with the Bot, the one with the newest
// message (or, with none, the newest created) first.
func (s *Store) BotConversations(ctx context.Context, botID int64) ([]SpacePreview, error) {
	return s.spacePreviews(ctx,
		" AND space.kind = ? AND EXISTS "+
			"(SELECT 1 FROM space_member WHERE space_id = space.id AND bot_id = ?)",
		[]any{KindConversation, botID},
		"COALESCE(m.created_at, space.created_at) DESC, space.id DESC")
}

func (s *Store) spacePreviews(
	ctx context.Context, where string, args []any, order string,
) ([]SpacePreview, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+_spaceCols+", m.author, substr(m.body, 1, "+_previewChars+"), m.created_at FROM space "+
			"LEFT JOIN message m ON m.id = "+
			"(SELECT MAX(id) FROM message WHERE space_id = space.id) "+
			"WHERE 1 = 1"+where+" ORDER BY "+order, args...)
	if err != nil {
		return nil, fmt.Errorf("list Space previews: %w", err)
	}
	return scanAll(rows, scanSpacePreview)
}

// UpdateSpace sets the name of c.ID. Its projects change through AddSpaceProject and
// RemoveSpaceProject, and its work item changes only from nil, through AttachSpace.
func (s *Store) UpdateSpace(ctx context.Context, c Space) (Space, error) {
	if err := c.validate(); err != nil {
		return Space{}, err
	}
	res, err := s.db.ExecContext(ctx,
		"UPDATE space SET name_auto = CASE WHEN name = ? THEN name_auto ELSE 0 END, "+
			"name = ? WHERE id = ?",
		c.Name, c.Name, c.ID)
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

// AttachSpace gives free Space id the work item, which must have no other active Space. The
// Space moves to the work item's project, whatever it was associated with. Its kind becomes
// KindWorkItem.
func (s *Store) AttachSpace(ctx context.Context, id, workItemID int64) (Space, error) {
	c, err := s.Space(ctx, id)
	if err != nil {
		return Space{}, err
	}
	switch {
	case c.WorkItemID != nil:
		return Space{}, fmt.Errorf("%w: Space %d already has work item %d", ErrInvalid, id, *c.WorkItemID)
	case c.Kind != KindFree:
		return Space{}, fmt.Errorf("%w: only a free Space attaches to a work item, Space %d is a %s",
			ErrInvalid, id, c.Kind)
	}
	f, err := s.WorkItem(ctx, workItemID)
	if errors.Is(err, ErrNotFound) {
		return Space{}, fmt.Errorf("%w: unknown work item %d", ErrInvalid, workItemID)
	}
	if err != nil {
		return Space{}, fmt.Errorf("check work item: %w", err)
	}
	if err := s.checkWorkItemInProject(ctx, &workItemID, f.ProjectID); err != nil {
		return Space{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Space{}, fmt.Errorf("begin attach Space: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx,
		"UPDATE space SET work_item_id = ?, kind = ?, project_id = ? WHERE id = ? AND work_item_id IS NULL",
		workItemID, KindWorkItem, f.ProjectID, id)
	if err != nil {
		return Space{}, s.spaceWriteErr(ctx, "attach Space", err, &workItemID)
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT OR IGNORE INTO space_project (space_id, project_id) VALUES (?, ?)", id, f.ProjectID); err != nil {
		return Space{}, fmt.Errorf("associate Space with project: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Space{}, fmt.Errorf("commit attach Space: %w", err)
	}
	return s.Space(ctx, id)
}

// SpaceProjects lists the projects Space id is associated with, by id.
func (s *Store) SpaceProjects(ctx context.Context, id int64) ([]Project, error) {
	if err := s.requireSpace(ctx, id); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT project.id, project.name, project.path, project.base_branch, project.purpose, project.created_at "+
			"FROM space_project JOIN project ON project.id = space_project.project_id "+
			"WHERE space_project.space_id = ? ORDER BY project.id", id)
	if err != nil {
		return nil, fmt.Errorf("list Space projects: %w", err)
	}
	return scanAll(rows, scanProject)
}

// AddSpaceProject associates Space id with the project; doing it twice changes nothing. A
// change leaves a note in the Space's history, by the Bot with id by, or by the user when
// by is nil.
func (s *Store) AddSpaceProject(ctx context.Context, id, projectID int64, by *int64) error {
	if err := s.requireProjectEdit(ctx, id); err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx,
		"INSERT OR IGNORE INTO space_project (space_id, project_id) VALUES (?, ?)", id, projectID)
	if err != nil {
		return s.writeErr(ctx, "add Space project", err, ErrNotFound,
			parent{table: "project", id: &projectID})
	}
	return s.noteSpaceProject(ctx, res, id, projectID, by, "%s associated Project %s with this conversation.")
}

// RemoveSpaceProject drops the association of Space id with the project, which the Space
// need not have, and notes it as AddSpaceProject does. The Space, its history and its other
// projects stay.
func (s *Store) RemoveSpaceProject(ctx context.Context, id, projectID int64, by *int64) error {
	if err := s.requireProjectEdit(ctx, id); err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx,
		"DELETE FROM space_project WHERE space_id = ? AND project_id = ?", id, projectID)
	if err != nil {
		return fmt.Errorf("remove Space project: %w", err)
	}
	return s.noteSpaceProject(ctx, res, id, projectID, by, "%s removed Project %s from this conversation.")
}

// noteSpaceProject writes the note format, filled with who and the project's name, as a Reef
// message when res changed a row.
func (s *Store) noteSpaceProject(
	ctx context.Context, res sql.Result, id, projectID int64, by *int64, format string,
) error {
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return err
	}
	var project string
	if err := s.db.QueryRowContext(ctx, "SELECT name FROM project WHERE id = ?", projectID).
		Scan(&project); err != nil {
		return fmt.Errorf("read project %d: %w", projectID, err)
	}
	who := "You"
	if by != nil {
		if err := s.db.QueryRowContext(ctx, "SELECT name FROM bot WHERE id = ?", *by).
			Scan(&who); err != nil {
			return fmt.Errorf("read bot %d: %w", *by, err)
		}
	}
	_, err := s.CreateMessage(ctx, Message{
		SpaceID: id, Author: ReefAuthor, BotID: by, Body: fmt.Sprintf(format, who, project),
	})
	return err
}

// requireProjectEdit is ErrInvalid when Space id belongs to a work item, whose Space stays in
// its work item's project.
func (s *Store) requireProjectEdit(ctx context.Context, id int64) error {
	c, err := s.Space(ctx, id)
	if err != nil {
		return err
	}
	if c.WorkItemID != nil {
		return fmt.Errorf("%w: Space %d belongs to work item %d and stays in its project",
			ErrInvalid, id, *c.WorkItemID)
	}
	return nil
}

// PauseSpace pauses the Space; the reason must not be empty. Its Work item, if in progress,
// goes to needs_you: the item keeps its worktree, so the Team stays busy.
func (s *Store) PauseSpace(ctx context.Context, id int64, reason string) error {
	if reason == "" {
		return fmt.Errorf("%w: pause reason is required", ErrInvalid)
	}
	if err := s.setSpace(ctx, id, "paused = ?", reason); err != nil {
		return err
	}
	return s.moveSpaceItem(ctx, id, _inProgress, _needsYou)
}

// moveSpaceItem moves the Work item of the Space from one status to another, and does nothing
// when the Space has no item or the item has another status.
func (s *Store) moveSpaceItem(ctx context.Context, id int64, from, to string) error {
	var itemID *int64
	if err := s.db.QueryRowContext(ctx, "SELECT work_item_id FROM space WHERE id = ?", id).Scan(&itemID); err != nil {
		return fmt.Errorf("read Space item: %w", err)
	}
	if itemID == nil {
		return nil
	}
	w, err := s.WorkItem(ctx, *itemID)
	if err != nil || w.Status != from {
		return err
	}
	_, err = s.MoveWorkItem(ctx, w.ID, to, BySpace)
	return err
}

// ContinueSpace clears the pause, drops the pending holds it left and raises the budget to
// at least budget: its top-level Work item's, or the Space's own when it has no item. Its Work
// item, if in needs_you, goes back to in_progress.
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
		"UPDATE work_item SET token_budget = MAX(token_budget, ?1) WHERE id = "+
			"(SELECT COALESCE(item.parent_id, item.id) FROM work_item item "+
			"WHERE item.id = (SELECT work_item_id FROM space WHERE id = ?2))",
		budget, id); err != nil {
		return fmt.Errorf("update work item budget: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE approval SET status = ? WHERE space_id = ? AND status = ? AND kind = ?",
		HoldDropped, id, HoldPending, _holdKind); err != nil {
		return fmt.Errorf("drop holds: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit continue: %w", err)
	}
	return s.moveSpaceItem(ctx, id, _needsYou, _inProgress)
}

// CloseSpace sets closed_at on the Space and gives the direct requests waiting in it a stopped
// response, in one transaction.
func (s *Store) CloseSpace(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin close Space: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx,
		"UPDATE space SET closed_at = COALESCE(closed_at, ?) WHERE id = ?",
		time.Now().UTC().Format(time.RFC3339Nano), id); err != nil {
		return fmt.Errorf("close Space: %w", err)
	}
	if err := s.stopWaitingDirects(ctx, tx, "space_id = ?", id); err != nil {
		return err
	}
	return tx.Commit()
}

// ReopenSpace clears the Space's closed_at. It is ErrConflict when the Space's work item has
// another active Space.
func (s *Store) ReopenSpace(ctx context.Context, id int64) error {
	err := s.setSpace(ctx, id, "closed_at = NULL")
	if isUnique(err) {
		return fmt.Errorf("%w: the work item of Space %d already has an active Space", ErrConflict, id)
	}
	return err
}

// SpaceTokens sums input, output and cache write tokens against the Space's budget: every
// Space of its top-level Work item, sub-items included, shares one; a free Space counts only
// its own. Cache reads are not counted.
func (s *Store) SpaceTokens(ctx context.Context, id int64) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx,
		"SELECT COALESCE(SUM(turn.input_tokens + turn.output_tokens + turn.cache_write_tokens), 0) "+
			"FROM turn JOIN space ON space.id = turn.space_id WHERE space.id = ?1 OR space.work_item_id IN "+
			"(SELECT w.id FROM work_item w WHERE COALESCE(w.parent_id, w.id) = "+
			"(SELECT COALESCE(item.parent_id, item.id) FROM work_item item "+
			"WHERE item.id = (SELECT work_item_id FROM space WHERE id = ?1)))",
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

// DeleteSpace removes the Space and, by cascade, its messages and turns. A Space its Work item
// was submitted from, while the item is in review, is ErrConflict: the proof goes with it.
func (s *Store) DeleteSpace(ctx context.Context, id int64) error {
	var reviewing bool
	if err := s.db.QueryRowContext(ctx,
		"SELECT EXISTS (SELECT 1 FROM proof JOIN work_item w ON w.id = proof.work_item_id "+
			"WHERE proof.space_id = ? AND w.status = ?)", id, _review).Scan(&reviewing); err != nil {
		return fmt.Errorf("read proof of Space: %w", err)
	}
	if reviewing {
		return fmt.Errorf("%w: space %d is the proof of a work item in review", ErrConflict, id)
	}
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
	if err := r.Scan(&p.ID, &p.ProjectID, &p.WorkItemID, &p.Name, &p.Role,
		&p.Paused, &p.TokenBudget, &created, &closed, &p.ActiveTurn, &p.Team,
		&p.Kind, &p.MovedTo, &p.BotID,
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
	if err := r.Scan(&c.ID, &c.ProjectID, &c.WorkItemID, &c.Name, &c.Role,
		&c.Paused, &c.TokenBudget, &created, &closed, &c.ActiveTurn, &c.Team,
		&c.Kind, &c.MovedTo, &c.BotID); err != nil {
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

// SetActiveTurn records the kind of turn currently running on Space id: "message" or
// "wrap", or "" when idle. It is a live-state hint for processes that cannot
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
