package channel

import (
	"testing"

	"github.com/bhanuprakaash/reef/worktree"
)

func TestSession_ReadOnlyTurnBlocksTryLockNotWrites(t *testing.T) {
	st, f := openStore(t)
	r := newTreeRig(t, st)
	a, b := r.session(t, "a", f), r.session(t, "b", f)
	tree := worktree.Path(r.dir, f.ProjectID, f.Slug)

	r.run(a, "tester")
	r.wantStarted(t, "a/tester")
	if _, ok := r.trees.TryLock(tree); ok {
		t.Error("TryLock while a tester turn runs on the tree = true, want false")
	}
	r.run(b, "developer")
	r.wantStarted(t, "b/developer")
	r.finish(t, "b/developer")
	r.finish(t, "a/tester")
	unlock, ok := r.trees.TryLock(tree)
	if !ok {
		t.Fatal("TryLock after the tester turn ended = false, want true")
	}
	unlock()
}
