package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/bhanuprakaash/reef/space"
	"github.com/bhanuprakaash/reef/store"
	"github.com/bhanuprakaash/reef/worktree"
)

func (s *server) turnOpRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /turns/{id}/retry", s.retryTurn)
	mux.HandleFunc("/turns/{id}/retry", methodNotAllowed)
	mux.HandleFunc("POST /turns/{id}/rollback", s.rollbackTurn)
	mux.HandleFunc("/turns/{id}/rollback", methodNotAllowed)
}

// retryTurn runs the wake of a failed turn again, as a new turn of its role in a new chain,
// and marks the failed turn retried. A turn the store reports not retryable is 409, and so is a
// Space that has a turn running or is paused.
func (s *server) retryTurn(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	ctx := context.WithoutCancel(r.Context())
	t, err := s.st.Turn(ctx, id)
	if err != nil {
		writeError(w, err)
		return
	}
	if ok, err := s.st.Retryable(ctx, id); err != nil || !ok {
		if err == nil {
			err = fmt.Errorf("turn %d cannot be retried: %w", id, store.ErrConflict)
		}
		writeError(w, err)
		return
	}
	c, ch, err := s.claim(ctx, t.SpaceID, _message)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.st.SetTurnRetried(ctx, id, true); err != nil {
		writeError(w, c.release(err))
		return
	}
	// A retry is the user's: from stays the user for a direct wake too, so it counts as no hop.
	rerun := wake{from: _userAuthor, role: t.Role, botID: t.BotID, text: t.WakeText, parent: t.WakeParent}
	if t.WakeMessageID != nil {
		rerun.messageID = *t.WakeMessageID
	}
	if !t.Sent {
		rerun.scopes = t.Scopes
	}
	if t.DirectMessageID != nil {
		direct, err := s.st.DirectMessage(ctx, *t.DirectMessageID)
		if err != nil {
			_ = s.st.SetTurnRetried(ctx, id, false)
			writeError(w, c.release(err))
			return
		}
		rerun.direct = &direct
	}
	if err := c.resume(ctx, ch, []wake{rerun}); err != nil {
		if rerr := s.st.SetTurnRetried(ctx, id, false); rerr != nil {
			err = errors.Join(err, rerr)
		}
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// rollbackTurn puts the Space's worktree back as it was before the turn ran, and posts a
// note from reef in the Space. A turn with no checkpoint is 400; a Space that has a turn
// running is 409. The agent's session is not rewound.
func (s *server) rollbackTurn(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	ctx := context.WithoutCancel(r.Context())
	t, err := s.st.Turn(ctx, id)
	if err != nil {
		writeError(w, err)
		return
	}
	if len(t.Checkpoints) == 0 {
		writeError(w, fmt.Errorf("%w: turn %d has no checkpoint", store.ErrInvalid, id))
		return
	}
	c, ch, err := s.claim(ctx, t.SpaceID, _noTurn)
	if err != nil {
		writeError(w, err)
		return
	}
	err = s.restoreTree(ctx, c, ch, t)
	if err == nil {
		err = s.noteRollback(ctx, ch, id)
	}
	if err = c.release(err); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// noteRollback posts the rollback note of turn id in every Space of ch's work item, since
// work item Spaces share a worktree; a quick session's Space has no work item to share with.
func (s *server) noteRollback(ctx context.Context, ch store.Space, id int64) error {
	chs := []store.Space{ch}
	if ch.WorkItemID != nil {
		var err error
		if chs, err = s.st.Spaces(ctx, store.SpaceFilter{WorkItemID: ch.WorkItemID}); err != nil {
			return err
		}
	}
	body := fmt.Sprintf("Rolled back to before turn %d.", id)
	for _, c := range chs {
		if _, err := s.st.CreateMessage(ctx, store.Message{
			SpaceID: c.ID, Author: store.ReefAuthor, Body: body,
		}); err != nil {
			return err
		}
	}
	return nil
}

// restoreTree puts every workspace t checkpointed back to its checkpoint. It is a conflict,
// not a wait, when t was checkpointed in a workspace the Space's work item does not have now,
// when a `reef chat` has the Space open, or when a turn of any Space runs on the work item's
// folder.
func (s *server) restoreTree(ctx context.Context, c *chain, ch store.Space, t store.Turn) error {
	restores, err := s.checkpointTrees(ctx, ch, t)
	if err != nil {
		return err
	}
	c.l.mu.Lock()
	open := c.l.sess != nil
	c.l.mu.Unlock()
	if !open && space.InUse(s.dataDir, t.SpaceID) {
		return fmt.Errorf("space %d is open elsewhere: %w", t.SpaceID, store.ErrConflict)
	}
	dir, _, err := space.WorkDir(ctx, s.st, s.dataDir, ch.ID)
	if err != nil {
		return err
	}
	unlock, ok := s.trees.TryLock(dir)
	if !ok {
		return fmt.Errorf("%s has a turn running: %w", dir, store.ErrConflict)
	}
	defer unlock()
	for _, r := range restores {
		if err := worktree.Restore(ctx, r.dir, r.ref); err != nil {
			return err
		}
	}
	return nil
}

type restore struct{ dir, ref string }

// checkpointTrees is the workspace folder and ref of each checkpoint of t. A checkpoint taken
// in a workspace that the work item of the Space ch does not have now, or that git cannot read
// there, is a conflict: a free Space's turn from before an attach has none to give.
func (s *server) checkpointTrees(ctx context.Context, ch store.Space, t store.Turn) ([]restore, error) {
	if ch.WorkItemID == nil {
		return nil, fmt.Errorf("%w: Space %d has no work item, so no checkpoint to restore",
			store.ErrConflict, ch.ID)
	}
	f, err := s.st.WorkItem(ctx, *ch.WorkItemID)
	if err != nil {
		return nil, err
	}
	ws, err := s.itemWorkspaces(ctx, f)
	if err != nil {
		return nil, err
	}
	var out []restore
	for _, c := range t.Checkpoints {
		i := slices.IndexFunc(ws, func(w store.Workspace) bool { return w.ID == c.WorkspaceID })
		if i < 0 {
			return nil, fmt.Errorf("turn %d was checkpointed in workspace %d, which work item %d does not have: %w",
				t.ID, c.WorkspaceID, f.ID, store.ErrConflict)
		}
		from, _, err := worktree.Origin(ctx, ws[i].Dir, c.Ref)
		if err != nil {
			return nil, err
		}
		if from != ws[i].Dir {
			return nil, fmt.Errorf("turn %d was checkpointed in %s, not %s: %w",
				t.ID, from, ws[i].Dir, store.ErrConflict)
		}
		out = append(out, restore{ws[i].Dir, c.Ref})
	}
	return out, nil
}
