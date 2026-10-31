package store

import (
	"context"
	"fmt"
)

// InterruptedReason is the pause reason of an assignment's Space that a restart cut off. What
// the cut action did cannot be told, so it is not run again until the user has looked.
const InterruptedReason = "interrupted while running; check its activity and changes before continuing"

// FirstTurnPending reports whether Space id has no turn and no message from anyone but reef: the
// queue opened it and the first turn has not started.
func (s *Store) FirstTurnPending(ctx context.Context, id int64) (bool, error) {
	var pending bool
	err := s.db.QueryRowContext(ctx,
		"SELECT NOT EXISTS (SELECT 1 FROM turn WHERE space_id = ?) "+
			"AND NOT EXISTS (SELECT 1 FROM message WHERE space_id = ? AND author != ?)",
		id, id, ReefAuthor).Scan(&pending)
	if err != nil {
		return false, fmt.Errorf("read first turn of Space %d: %w", id, err)
	}
	return pending, nil
}

// PauseInterrupted pauses, for the user to look at, each unpaused Space of an item in progress
// that the last server run cut off: one whose turn is in failed, and one whose first message was
// stored but whose first turn never started. A Space the user paused or a cap paused keeps its
// reason. It returns the ids of the Spaces it paused.
func (s *Store) PauseInterrupted(ctx context.Context, failed []Turn) ([]int64, error) {
	ids := map[int64]bool{}
	for _, t := range failed {
		ids[t.SpaceID] = true
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT c.id FROM space c WHERE c.kind = ? "+
			"AND NOT EXISTS (SELECT 1 FROM turn WHERE space_id = c.id) "+
			"AND EXISTS (SELECT 1 FROM message WHERE space_id = c.id AND author != ?)",
		KindWorkItem, ReefAuthor)
	if err != nil {
		return nil, fmt.Errorf("list Spaces cut before their first turn: %w", err)
	}
	unstarted, err := scanAll(rows, func(r interface{ Scan(...any) error }) (int64, error) {
		var id int64
		return id, r.Scan(&id)
	})
	if err != nil {
		return nil, fmt.Errorf("list Spaces cut before their first turn: %w", err)
	}
	for _, id := range unstarted {
		ids[id] = true
	}
	var paused []int64
	for id := range ids {
		c, err := s.Space(ctx, id)
		if err != nil {
			return nil, err
		}
		if c.Kind != KindWorkItem || c.WorkItemID == nil || c.ClosedAt != nil || c.Paused != "" {
			continue
		}
		w, err := s.WorkItem(ctx, *c.WorkItemID)
		if err != nil {
			return nil, err
		}
		if w.Status != _inProgress {
			continue
		}
		if err := s.PauseSpace(ctx, id, InterruptedReason); err != nil {
			return nil, err
		}
		paused = append(paused, id)
	}
	return paused, nil
}

// ItemsTargeting lists the top-level items, of any home Project, that have a target in project
// id and are not closed.
func (s *Store) ItemsTargeting(ctx context.Context, id int64) ([]WorkItem, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT w.id FROM work_item w JOIN work_item_target t ON t.work_item_id = w.id "+
			"WHERE t.project_id = ? AND w.parent_id IS NULL AND w.closed_at IS NULL ORDER BY w.id", id)
	if err != nil {
		return nil, fmt.Errorf("list items targeting project %d: %w", id, err)
	}
	ids, err := scanAll(rows, func(r interface{ Scan(...any) error }) (int64, error) {
		var id int64
		return id, r.Scan(&id)
	})
	if err != nil {
		return nil, err
	}
	var items []WorkItem
	for _, wid := range ids {
		w, err := s.WorkItem(ctx, wid)
		if err != nil {
			return nil, err
		}
		items = append(items, w)
	}
	return items, nil
}
