package server

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bhanuprakaash/reef/space"
	"github.com/bhanuprakaash/reef/store"
	"github.com/bhanuprakaash/reef/worktree"
)

// cleanMergedWorkspaces cleans every workspace whose requests are all merged, retrying the ones
// an earlier read could not finish.
func (s *server) cleanMergedWorkspaces(ctx context.Context) {
	ws, err := s.st.MergedWorkspaces(ctx)
	if err != nil {
		log.Printf("list merged workspaces: %v", err)
		return
	}
	for _, w := range ws {
		if ctx.Err() != nil {
			return
		}
		if err := s.cleanWorkspace(ctx, w); err != nil {
			log.Printf("clean workspace %d: %v", w.ID, err)
		}
	}
}

// cleanWorkspace removes the worktree of w and keeps its branch. Whatever the branch does not
// keep, uncommitted changes (untracked files included) or a HEAD no branch contains, is first
// saved as a commit at a ref in the project's repository, which stays after the folder is gone.
// Nothing is removed until that ref exists; a failure is stored on the row as `cleanupError`,
// leaves the workspace as it is and is returned, and cleaning again tries once more. It holds the
// write lock of the folder an agent of w works from, so no writer changes the tree between the
// save and the removal, and is a conflict while a turn runs there. Other workspaces are not
// touched. Cleaning a removed workspace does nothing.
func (s *server) cleanWorkspace(ctx context.Context, w store.Workspace) error {
	if w.State == store.WorkspaceRemoved {
		return nil
	}
	unlock, ok := s.trees.TryLock(s.writerKey(w))
	if !ok {
		err := fmt.Errorf("%w: a turn is running in workspace %d; cleanup runs when it ends", store.ErrConflict, w.ID)
		return errors.Join(err, s.st.RecordWorkspaceCleanup(ctx, w.ID, "", cleanupText(err), false))
	}
	defer unlock()
	w, err := s.st.Workspace(ctx, w.ID)
	if err != nil || w.State == store.WorkspaceRemoved {
		return err
	}
	snapshot := ""
	fail := func(err error) error {
		return errors.Join(err, s.st.RecordWorkspaceCleanup(ctx, w.ID, snapshot, cleanupText(err), false))
	}
	p, err := s.st.Project(ctx, w.ProjectID)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(w.Dir, ".git")); err == nil {
		unsaved, err := worktree.Unsaved(ctx, w.Dir)
		if err != nil {
			return fail(err)
		}
		if unsaved {
			ref, err := s.snapshotRef(w)
			if err != nil {
				return fail(err)
			}
			if err := worktree.Checkpoint(ctx, w.Dir, ref); err != nil {
				return fail(fmt.Errorf("save changes before removing %s: %w", w.Dir, err))
			}
			snapshot = ref
			if err := s.st.RecordWorkspaceCleanup(ctx, w.ID, snapshot, "", false); err != nil {
				return err
			}
		}
		if err := worktree.RemoveForce(ctx, p.Path, w.Dir); err != nil {
			return fail(err)
		}
	}
	return s.st.RecordWorkspaceCleanup(ctx, w.ID, snapshot, "", true)
}

func cleanupText(err error) string {
	return strings.Join(strings.Fields(err.Error()), " ")
}

// writerKey is the key of the write lock of the folder an agent of w works from: its Work
// item's root, which holds every repository of the item, or its Space's own folder for a quick
// fix.
func (s *server) writerKey(w store.Workspace) string {
	if w.WorkItemID != nil {
		return space.WorkRoot(s.dataDir, strconv.FormatInt(*w.WorkItemID, 10))
	}
	return space.ChatDir(s.dataDir, *w.SpaceID)
}

// snapshotRef is the ref that keeps what w held, refs/reef/snapshots/<data folder>/<workspace
// id>, qualified by the data folder like a checkpoint ref because copies of Reef on other data
// folders can share the repository.
func (s *server) snapshotRef(w store.Workspace) (string, error) {
	dir, err := filepath.Abs(s.dataDir)
	if err != nil {
		return "", fmt.Errorf("snapshot ref: %w", err)
	}
	sum := sha256.Sum256([]byte(dir))
	return fmt.Sprintf("refs/reef/snapshots/%x/%d", sum[:6], w.ID), nil
}
