package channel

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/store"
)

// gateAgent holds its turn in Send until the test opens the gate of its name, which is the
// channel's name and the role: "a/developer".
type gateAgent struct {
	*recordingAgent
	channel string
	gates   map[string]chan struct{}
	started chan<- string
	name    string
}

func (a *gateAgent) Start(ctx context.Context, cfg agent.Config) error {
	a.name = a.channel + "/tester"
	if slices.Contains(cfg.Tools, "Task") {
		a.name = a.channel + "/developer"
	}
	return a.recordingAgent.Start(ctx, cfg)
}

func (a *gateAgent) Send(ctx context.Context, prompt string) error {
	a.started <- a.name
	select {
	case <-a.gates[a.name]:
	case <-ctx.Done():
		return ctx.Err()
	}
	return a.recordingAgent.Send(ctx, prompt)
}

type treeRig struct {
	st      *store.Store
	dir     string
	trees   *Trees
	gates   map[string]chan struct{}
	started chan string
	errs    chan error
}

func newTreeRig(t *testing.T, st *store.Store) *treeRig {
	t.Helper()
	if _, err := st.ApplyTeam(context.Background(), store.Team{Name: "crew", Roles: []store.Role{
		{Name: "lead", Lead: true, Backend: "claude"},
		{Name: "developer", Write: true, Backend: "claude"},
		{Name: "tester", Backend: "claude"},
	}}); err != nil {
		t.Fatal(err)
	}
	return &treeRig{
		st: st, dir: t.TempDir(), trees: NewTrees(), errs: make(chan error, 8),
		started: make(chan string, 8),
		gates: map[string]chan struct{}{
			"a/developer": make(chan struct{}), "a/tester": make(chan struct{}),
			"b/developer": make(chan struct{}), "b/tester": make(chan struct{}),
			"c/developer": make(chan struct{}),
		},
	}
}

// session opens a channel called name on f.
func (r *treeRig) session(t *testing.T, name string, f store.Feature) *Session {
	t.Helper()
	ch, err := r.st.CreateChannel(context.Background(), store.Channel{
		ProjectID: f.ProjectID, FeatureID: &f.ID, Name: name, Role: "lead", Backend: "claude",
		Team: "crew",
	})
	if err != nil {
		t.Fatal(err)
	}
	mk := func(string) (agent.Agent, error) {
		return &gateAgent{
			recordingAgent: newRecordingAgent(), channel: name, gates: r.gates, started: r.started,
		}, nil
	}
	s, err := open(context.Background(), r.st, r.dir, "/bin/reef", ch.ID, mk)
	if err != nil {
		t.Fatal(err)
	}
	s.trees = r.trees
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func (r *treeRig) run(s *Session, role string) {
	go func() { r.errs <- s.RunAs(context.Background(), role, "go", nil, discard) }()
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
	close(r.gates[name])
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
	other, err := st.CreateFeature(context.Background(), store.Feature{
		ProjectID: f.ProjectID, Slug: "search", Name: "Search", Goal: "Search",
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
	go func() { done <- b.RunAs(ctx, "developer", "go", nil, discard) }()
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
