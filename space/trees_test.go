package space

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/store"
)

// blockingAgent holds its turn in Send until the test opens the block of its name, which is the
// Space's name and the role: "a/developer".
type blockingAgent struct {
	*recordingAgent
	space   string
	blocks  map[string]chan struct{}
	started chan<- string
	name    string
}

func (a *blockingAgent) Start(ctx context.Context, cfg agent.Config) error {
	a.name = a.space + "/tester"
	if slices.Contains(cfg.Tools, "Task") {
		a.name = a.space + "/developer"
	}
	return a.recordingAgent.Start(ctx, cfg)
}

func (a *blockingAgent) Send(ctx context.Context, prompt string) error {
	a.started <- a.name
	select {
	case <-a.blocks[a.name]:
	case <-ctx.Done():
		return ctx.Err()
	}
	return a.recordingAgent.Send(ctx, prompt)
}

type treeRig struct {
	st      *store.Store
	dir     string
	trees   *Trees
	blocks  map[string]chan struct{}
	started chan string
	errs    chan error
}

func newTreeRig(t *testing.T, st *store.Store) *treeRig {
	t.Helper()
	if _, err := testApplyTeam(t, st, context.Background(), store.Team{Name: "crew", Roles: []store.Role{
		{Name: "lead", Lead: true, Backend: "claude"},
		{Name: "developer", Write: true, Backend: "claude"},
		{Name: "tester", Backend: "claude"},
	}}); err != nil {
		t.Fatal(err)
	}
	return &treeRig{
		st: st, dir: t.TempDir(), trees: NewTrees(t.TempDir()), errs: make(chan error, 8),
		started: make(chan string, 8),
		blocks: map[string]chan struct{}{
			"a/developer": make(chan struct{}), "a/tester": make(chan struct{}),
			"b/developer": make(chan struct{}), "b/tester": make(chan struct{}),
			"c/developer": make(chan struct{}),
		},
	}
}

func (r *treeRig) session(t *testing.T, name string, f store.WorkItem) *Session {
	t.Helper()
	ch, err := r.st.CreateSpace(context.Background(), store.Space{
		ProjectID: f.ProjectID, WorkItemID: &f.ID, Name: name, Role: "lead",
		Team: "crew",
	})
	if err != nil {
		t.Fatal(err)
	}
	closeSpace(t, r.st, ch.ID)
	mk := func(string) (agent.Agent, error) {
		return &blockingAgent{
			recordingAgent: newRecordingAgent(), space: name, blocks: r.blocks, started: r.started,
		}, nil
	}
	s, err := Open(context.Background(), r.st, testMemory(t, r.st), r.dir, "/bin/reef", ch.ID, r.trees, mk)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func (r *treeRig) run(s *Session, role string) {
	go func() { r.errs <- s.RunAs(context.Background(), role, "go", nil, nil, discard) }()
}

func (r *treeRig) wantStarted(t *testing.T, want string) {
	t.Helper()
	select {
	case got := <-r.started:
		if got != want {
			t.Fatalf("started %s, want %s", got, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("%s did not start", want)
	}
}

func (r *treeRig) wantNoStart(t *testing.T, why string) {
	t.Helper()
	select {
	case got := <-r.started:
		t.Fatalf("%s started while %s", got, why)
	case <-time.After(200 * time.Millisecond):
	}
}

func (r *treeRig) finish(t *testing.T, name string) {
	t.Helper()
	close(r.blocks[name])
	select {
	case err := <-r.errs:
		if err != nil {
			t.Fatalf("turn of %s: %v", name, err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("turn of %s did not end", name)
	}
}

func TestSession_WriteTurnsOnOneTreeRunOneAtATime(t *testing.T) {
	st, f := openStore(t)
	r := newTreeRig(t, st)
	a, b := r.session(t, "a", f), r.session(t, "b", f)

	r.run(a, "developer")
	r.wantStarted(t, "a/developer")
	r.run(b, "tester")
	r.wantStarted(t, "b/tester")
	r.finish(t, "b/tester")

	r.run(b, "developer")
	r.wantNoStart(t, "a/developer held the tree")
	r.finish(t, "a/developer")
	r.wantStarted(t, "b/developer")
	r.finish(t, "b/developer")
}

func TestSession_WriteTurnsOnTwoTreesRunTogether(t *testing.T) {
	st, f := openStore(t)
	other, err := st.CreateWorkItem(context.Background(), store.WorkItem{
		ProjectID: f.ProjectID, Title: "Search", Description: "Search",
	})
	if err != nil {
		t.Fatal(err)
	}
	r := newTreeRig(t, st)
	a, c := r.session(t, "a", f), r.session(t, "c", other)

	r.run(a, "developer")
	r.wantStarted(t, "a/developer")
	r.run(c, "developer")
	r.wantStarted(t, "c/developer")
	r.finish(t, "a/developer")
	r.finish(t, "c/developer")
}

func TestSession_FirstWakesOfOneFeatureAddItsWorktreeOnce(t *testing.T) {
	st, f := openStore(t)
	r := newTreeRig(t, st)
	a, b := r.session(t, "a", f), r.session(t, "b", f)

	r.run(a, "tester")
	r.run(b, "tester")
	var got []string
	for range 2 {
		select {
		case name := <-r.started:
			got = append(got, name)
		case err := <-r.errs:
			t.Fatalf("turn: %v", err)
		case <-time.After(5 * time.Second):
			t.Fatalf("started %q, want both testers", got)
		}
	}
	slices.Sort(got)
	if want := []string{"a/tester", "b/tester"}; !slices.Equal(got, want) {
		t.Fatalf("started %q, want %q", got, want)
	}
	r.finish(t, "a/tester")
	r.finish(t, "b/tester")
}

func TestSession_StopEndsTheWaitForATree(t *testing.T) {
	st, f := openStore(t)
	r := newTreeRig(t, st)
	a, b := r.session(t, "a", f), r.session(t, "b", f)
	r.run(a, "developer")
	r.wantStarted(t, "a/developer")

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- b.RunAs(ctx, "developer", "go", nil, nil, discard) }()
	r.wantNoStart(t, "a/developer held the tree")
	cancel()
	select {
	case err := <-done:
		if err == nil || ctx.Err() == nil {
			t.Errorf("waiting turn error = %v, want the cancelled ctx", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled turn still waits for the tree")
	}
	turns, err := st.Turns(context.Background(), store.TurnFilter{})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, tn := range turns {
		if tn.Status == "failed" {
			got = append(got, tn.Role+": "+tn.Error)
		}
	}
	if want := []string{"developer: interrupted"}; !slices.Equal(got, want) {
		t.Errorf("failed turns = %q, want %q", got, want)
	}
	r.finish(t, "a/developer")
}

func TestTrees_ShareLocksAcrossInstancesOfOneDataFolder(t *testing.T) {
	dataDir := t.TempDir()
	a, b := NewTrees(dataDir), NewTrees(dataDir)
	unlock, err := a.Lock(context.Background(), "/tree")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("a writer in the other instance waits until ctx ends", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		if _, err := b.Lock(ctx, "/tree"); !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Lock error = %v, want %v", err, context.DeadlineExceeded)
		}
	})
	t.Run("another tree is free", func(t *testing.T) {
		unlock, ok := b.TryLock("/other")
		if !ok {
			t.Fatal("TryLock(/other) = false, want true")
		}
		unlock()
	})
	t.Run("the writer goes on when the lock is freed", func(t *testing.T) {
		got := make(chan error, 1)
		go func() {
			unlock, err := b.Lock(context.Background(), "/tree")
			if err == nil {
				unlock()
			}
			got <- err
		}()
		unlock()
		select {
		case err := <-got:
			if err != nil {
				t.Errorf("Lock error = %v, want nil", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Lock still waits after the holder let go")
		}
	})
}

func TestTrees_ReadersDoNotBlockWritersButStopTryLock(t *testing.T) {
	dataDir := t.TempDir()
	a, b := NewTrees(dataDir), NewTrees(dataDir)
	endRead := a.Read("/tree")

	unlock, err := b.Lock(context.Background(), "/tree")
	if err != nil {
		t.Fatalf("Lock with a reader in the other instance: %v", err)
	}
	unlock()
	if _, ok := b.TryLock("/tree"); ok {
		t.Error("TryLock with a reader in the other instance = true, want false")
	}
	endRead()
	unlock, ok := b.TryLock("/tree")
	if !ok {
		t.Fatal("TryLock after the reader ended = false, want true")
	}
	if _, ok := a.TryLock("/tree"); ok {
		t.Error("TryLock while the other instance holds the tree = true, want false")
	}
	unlock()
}

type lockProbeAgent struct {
	*recordingAgent
	trees   *Trees
	canLock chan<- bool
}

func (a *lockProbeAgent) Send(ctx context.Context, prompt string) error {
	pctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	unlock, err := a.trees.Lock(pctx, a.cfg.Cwd)
	if err == nil {
		unlock()
	}
	a.canLock <- err == nil
	return a.recordingAgent.Send(ctx, prompt)
}

func TestSession_ShellThatChangesFilesRunsAsAWriteTurn(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	if _, err := testApplyTeam(t, st, ctx, store.Team{Name: "crew", Roles: []store.Role{
		{Name: "lead", Lead: true, Backend: "claude"},
		{Name: "committer", Backend: "claude", Tools: []string{"Read", "Bash(git commit:*)"}},
		{Name: "differ", Backend: "claude", Tools: []string{"Read", "Bash(git diff:*)"}},
		{Name: "reader", Backend: "claude", Tools: []string{"Read", "Bash(git status:*)", "Bash(ls:*)"}},
	}}); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		role           string
		wantCanLock    bool
		wantCheckpoint bool
	}{
		{"committer", false, true},
		{"differ", false, true},
		{"reader", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			ch, err := st.CreateSpace(ctx, store.Space{
				ProjectID: f.ProjectID, WorkItemID: &f.ID, Name: tt.role, Role: "lead",
				Team: "crew",
			})
			if err != nil {
				t.Fatal(err)
			}
			closeSpace(t, st, ch.ID)
			trees := NewTrees(t.TempDir())
			canLock := make(chan bool, 1)
			mk := func(string) (agent.Agent, error) {
				return &lockProbeAgent{recordingAgent: newRecordingAgent(), trees: trees, canLock: canLock}, nil
			}
			s, err := Open(ctx, st, testMemory(t, st), t.TempDir(), "/bin/reef", ch.ID, trees, mk)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = s.Close() }()
			if err := s.RunAs(ctx, tt.role, "go", nil, nil, discard); err != nil {
				t.Fatal(err)
			}
			if got := <-canLock; got != tt.wantCanLock {
				t.Errorf("another writer could take the tree during the turn = %v, want %v", got, tt.wantCanLock)
			}
			turns, err := st.Turns(ctx, store.TurnFilter{SpaceID: &ch.ID})
			if err != nil || len(turns) != 1 {
				t.Fatalf("turns = %+v, %v, want one", turns, err)
			}
			if got := turns[0].Checkpoint != nil; got != tt.wantCheckpoint {
				t.Errorf("turn has a checkpoint = %v, want %v", got, tt.wantCheckpoint)
			}
		})
	}
}
