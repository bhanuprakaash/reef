package channel

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/store"
)

// recordingAgent notes how it was started and answers each prompt with "ok". Like the
// real agent calling propose_wrap, it stores a proposal when it gets the wrap prompt
// and propose is set.
type recordingAgent struct {
	st        *store.Store
	channelID int64
	propose   bool
	startErr  error

	cfg     agent.Config
	started bool
	resumed string
	prompts []string
	events  chan agent.Event
}

func newRecordingAgent() *recordingAgent {
	return &recordingAgent{events: make(chan agent.Event, 8)}
}

func (a *recordingAgent) Start(_ context.Context, cfg agent.Config) error {
	a.cfg, a.started = cfg, true
	return a.startErr
}

func (a *recordingAgent) Resume(_ context.Context, cfg agent.Config, sessionID string) error {
	a.cfg, a.resumed = cfg, sessionID
	return nil
}

func (a *recordingAgent) Events() <-chan agent.Event { return a.events }
func (a *recordingAgent) Interrupt() error           { return nil }

func (a *recordingAgent) Send(ctx context.Context, prompt string) error {
	a.prompts = append(a.prompts, prompt)
	if strings.HasSuffix(prompt, _wrapPrompt) && a.propose {
		_, err := a.st.CreateProposal(ctx, store.Proposal{
			ChannelID: a.channelID, Role: "architect", Brief: "new brief", Notes: "my notes",
		})
		if err != nil {
			return err
		}
	}
	a.events <- agent.Event{Kind: agent.KindSession, SessionID: "s1"}
	a.events <- agent.Event{Kind: agent.KindText, Text: "ok"}
	a.events <- agent.Event{Kind: agent.KindTurnEnd}
	return nil
}

func (a *recordingAgent) factory(string) (agent.Agent, error) { return a, nil }

func sessionChannel(t *testing.T, st *store.Store, f store.Feature, quick bool) store.Channel {
	t.Helper()
	c := store.Channel{
		ProjectID: f.ProjectID, Name: "chat", Role: "architect", Backend: "claude",
		PermissionMode: "acceptEdits",
	}
	if !quick {
		c.FeatureID = &f.ID
	}
	ch, err := st.CreateChannel(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	return ch
}

func discard(agent.Event) {}

func TestOpen_StartsNewSession(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionChannel(t, st, f, false)
	ag := newRecordingAgent()
	dir := t.TempDir()

	s, err := open(ctx, st, dir, "/bin/reef", ch.ID, ag.factory)
	if err != nil {
		t.Fatal(err)
	}
	if !ag.started || ag.resumed != "" {
		t.Errorf("started = %v, resumed = %q, want a new session", ag.started, ag.resumed)
	}
	tok := ag.cfg.MCPServers["reef"].Env["REEF_TOKEN"]
	want := agent.Config{
		Cwd: "/tmp/shop", PermissionMode: "acceptEdits",
		MCPServers: map[string]agent.MCPServer{"reef": {
			Command: "/bin/reef", Args: []string{"mcp", "--data", dir},
			Env: map[string]string{"REEF_TOKEN": tok},
		}},
	}
	if !reflect.DeepEqual(ag.cfg, want) {
		t.Errorf("config = %+v, want %+v", ag.cfg, want)
	}
	if got, err := st.Token(ctx, tok); err != nil || got.ChannelID != ch.ID || got.Role != "architect" {
		t.Errorf("Token = %+v, %v, want a token of channel %d for architect", got, err, ch.ID)
	}
	if !InUse(dir, ch.ID) {
		t.Error("InUse while open = false, want true")
	}

	if err := s.SendAs(ctx, "hi", "hi", discard); err != nil {
		t.Fatal(err)
	}
	if len(ag.prompts) != 1 ||
		!strings.HasPrefix(ag.prompts[0], `You are the architect on feature "Login" (login) in project "shop".`) ||
		!strings.HasSuffix(ag.prompts[0], "\n\nhi") {
		t.Errorf("prompts = %q, want the context before hi", ag.prompts)
	}

	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Token(ctx, tok); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Token after Close error = %v, want ErrNotFound", err)
	}
	if InUse(dir, ch.ID) {
		t.Error("InUse after Close = true, want false")
	}
}

func TestOpen_QuickSession(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionChannel(t, st, f, true)
	ag := newRecordingAgent()

	s, err := open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, ag.factory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if err := s.SendAs(ctx, "hi", "hi", discard); err != nil {
		t.Fatal(err)
	}

	want := "You are the architect in project \"shop\" (/tmp/shop).\n" +
		"This is a quick session: it has no feature yet, so there is no brief, notes or decisions.\n" +
		"\n\nhi"
	if len(ag.prompts) != 1 || ag.prompts[0] != want {
		t.Errorf("prompts = %q, want [%q]", ag.prompts, want)
	}
}

func TestOpen_ResumesNewestSession(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionChannel(t, st, f, false)
	for _, session := range []string{"sess-old", "sess-new", ""} {
		turn, err := st.CreateTurn(ctx, store.Turn{
			ChannelID: ch.ID, Role: "architect", Backend: "claude", SessionID: session,
		})
		if err != nil {
			t.Fatal(err)
		}
		if session == "" {
			continue
		}
		turn.Status = "done"
		if _, err := st.EndTurn(ctx, turn); err != nil {
			t.Fatal(err)
		}
	}
	ag := newRecordingAgent()

	s, err := open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, ag.factory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if ag.started || ag.resumed != "sess-new" {
		t.Errorf("started = %v, resumed = %q, want sess-new resumed", ag.started, ag.resumed)
	}
	if ag.cfg.PermissionMode != "acceptEdits" {
		t.Errorf("permission mode = %q, want acceptEdits", ag.cfg.PermissionMode)
	}
	stale, err := st.Turn(ctx, 3)
	if err != nil {
		t.Fatal(err)
	}
	if stale.Status != "failed" || stale.Error != "chat exited" {
		t.Errorf("stale turn = %q %q, want failed %q", stale.Status, stale.Error, "chat exited")
	}

	if err := s.SendAs(ctx, "hi", "hi", discard); err != nil {
		t.Fatal(err)
	}
	if len(ag.prompts) != 1 || ag.prompts[0] != "hi" {
		t.Errorf("prompts = %q, want [hi] without the context", ag.prompts)
	}
}

func TestOpen_Errors(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionChannel(t, st, f, false)
	bad, err := st.CreateChannel(ctx, store.Channel{
		ProjectID: f.ProjectID, Name: "typo", Role: "architect", Backend: "nope",
	})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		give int64
		want string
	}{
		{"unknown channel", 99, "channel 99: not found"},
		{"unknown backend", bad.ID, `unknown backend "nope"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Open(ctx, st, t.TempDir(), "/bin/reef", tt.give)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Open error = %v, want %q", err, tt.want)
			}
		})
	}

	t.Run("open elsewhere", func(t *testing.T) {
		dir := t.TempDir()
		s, err := open(ctx, st, dir, "/bin/reef", ch.ID, newRecordingAgent().factory)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = s.Close() }()
		_, err = open(ctx, st, dir, "/bin/reef", ch.ID, newRecordingAgent().factory)
		if err == nil || err.Error() != "channel 1 is open elsewhere: conflict" {
			t.Errorf("second open error = %v, want the open-elsewhere error", err)
		}
	})

	t.Run("agent does not start", func(t *testing.T) {
		dir := t.TempDir()
		ag := newRecordingAgent()
		ag.startErr = errors.New("no claude")
		_, err := open(ctx, st, dir, "/bin/reef", ch.ID, ag.factory)
		if err == nil || err.Error() != "no claude" {
			t.Errorf("open error = %v, want no claude", err)
		}
		if InUse(dir, ch.ID) {
			t.Error("InUse after failed open = true, want false")
		}
		tok := ag.cfg.MCPServers["reef"].Env["REEF_TOKEN"]
		if _, err := st.Token(ctx, tok); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("Token after failed open error = %v, want ErrNotFound", err)
		}
	})
}

func TestSession_Wrap(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionChannel(t, st, f, false)
	ag := newRecordingAgent()
	ag.st, ag.channelID, ag.propose = st, ch.ID, true
	s, err := open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, ag.factory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	p, err := s.Wrap(ctx, discard)
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != 1 || p.Brief != "new brief" {
		t.Errorf("proposal = %+v, want proposal 1 with the new brief", p)
	}
	msgs, err := st.Messages(ctx, ch.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 || msgs[0].Author != "user" || msgs[0].Body != "/wrap" {
		t.Errorf("messages = %+v, want the user message /wrap first", msgs)
	}
	if !strings.HasSuffix(ag.prompts[0], _wrapPrompt) {
		t.Errorf("prompt = %q, want it to end with the wrap prompt", ag.prompts[0])
	}

	ag.propose = false
	if _, err := s.Wrap(ctx, discard); !errors.Is(err, ErrNoProposal) {
		t.Errorf("Wrap with an older proposal pending error = %v, want ErrNoProposal", err)
	}
}
