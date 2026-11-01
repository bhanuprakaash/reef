package server

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/bhanuprakaash/reef/store"
	"github.com/bhanuprakaash/reef/worktree"
)

// NoteInterrupted says, in each Space a restart paused, what its cut turn left in every
// repository: the files that differ from the checkpoint the turn took when it started. A Space
// whose cut turn took no checkpoint, or that had no turn, gets no note.
func NoteInterrupted(ctx context.Context, st *store.Store, paused []int64, failed []store.Turn) error {
	cut := map[int64]store.Turn{}
	for _, t := range failed {
		if len(t.Checkpoints) > 0 {
			cut[t.SpaceID] = t
		}
	}
	for _, id := range paused {
		t, ok := cut[id]
		if !ok {
			continue
		}
		sp, err := st.Space(ctx, id)
		if err != nil || sp.WorkItemID == nil {
			return err
		}
		item, err := st.WorkItem(ctx, *sp.WorkItemID)
		if err != nil {
			return err
		}
		top, err := strconv.ParseInt(item.TreeKey(), 10, 64)
		if err != nil {
			return err
		}
		rows, err := st.WorkItemWorkspaces(ctx, top)
		if err != nil {
			return err
		}
		dirs := map[int64]string{}
		for _, w := range rows {
			dirs[w.ID] = w.Dir
		}
		var parts []string
		for _, c := range t.Checkpoints {
			parts = append(parts, changedSince(ctx, st, c, dirs[c.WorkspaceID]))
		}
		body := "Interrupted by a restart. Since the turn started, " + strings.Join(parts, "; ") + "."
		if _, err := st.CreateMessage(ctx, store.Message{SpaceID: id, Author: store.ReefAuthor, Body: body}); err != nil {
			return err
		}
	}
	return nil
}

func changedSince(ctx context.Context, st *store.Store, c store.TurnCheckpoint, dir string) string {
	name := fmt.Sprintf("project %d", c.ProjectID)
	if p, err := st.Project(ctx, c.ProjectID); err == nil {
		name = p.Name
	}
	files, err := worktree.Diff(ctx, dir, c.Ref)
	switch {
	case err != nil:
		return fmt.Sprintf("%s: could not be read (%v)", name, err)
	case len(files) == 0:
		return name + ": no changes"
	case len(files) == 1:
		return name + ": 1 file changed"
	}
	return fmt.Sprintf("%s: %d files changed", name, len(files))
}
