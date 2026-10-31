package space

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/memory"
	"github.com/bhanuprakaash/reef/store"
)

func openRefreshSession(
	t *testing.T, st *store.Store, ch store.Space, dir string, mk func(string) (agent.Agent, error),
) *Session {
	t.Helper()
	s, err := Open(context.Background(), st, testMemory(t, st), dir, "/bin/reef", ch.ID, NewTrees(t.TempDir()), mk)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func messageBodies(t *testing.T, st *store.Store, spaceID int64) []string {
	t.Helper()
	msgs, err := st.Messages(context.Background(), spaceID, 0)
	if err != nil {
		t.Fatal(err)
	}
	var bodies []string
	for _, m := range msgs {
		bodies = append(bodies, m.Author+": "+m.Body)
	}
	return bodies
}

func TestSession_RefreshStartsNewContextWithHandoverAndFailureRetries(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionSpace(t, st, f, false)
	architect := testBotID(t, st, "architect")
	editBot(t, st, "architect", func(b *store.Bot) { b.Model = "sonnet" })
	ms := testMemory(t, st)
	remember(t, ms, memory.Ref{Kind: memory.ScopeWorkItem, ID: "1"}, memory.KindDecision, "user", "Use Google OAuth.")
	done, err := st.CreateWorkItem(ctx, store.WorkItem{ProjectID: f.ProjectID, ParentID: &f.ID, Title: "Add callback route"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.MoveWorkItem(ctx, done.ID, "done", store.ByLead); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateWorkItem(ctx, store.WorkItem{ProjectID: f.ProjectID, ParentID: &f.ID, Title: "Write the login tests"}); err != nil {
		t.Fatal(err)
	}

	var ags []*recordingAgent
	var next func() *recordingAgent
	mk := func(string) (agent.Agent, error) {
		ag := next()
		ags = append(ags, ag)
		return ag, nil
	}
	next = newRecordingAgent
	dir := t.TempDir()
	s := openRefreshSession(t, st, ch, dir, mk)
	for i := range 8 {
		if err := sendAs(ctx, st, ch.ID, s.Run, "message "+string(rune('a'+i)), discard); err != nil {
			t.Fatal(err)
		}
	}

	editBot(t, st, "architect", func(b *store.Bot) { b.Model = "opus" })
	if err := s.Refresh(ctx, *architect); err != nil {
		t.Fatal(err)
	}
	if len(ags) != 1 || !ags[0].closed {
		t.Fatalf("agents = %d, old closed = %v, want the one old agent closed", len(ags), ags[0].closed)
	}
	failing := newRecordingAgent()
	failing.startErr = errors.New("model opus is unavailable")
	next = func() *recordingAgent { return failing }
	if err := s.Run(ctx, "after refresh", nil, discard); err == nil {
		t.Fatal("run with a replacement that fails to start = nil, want an error")
	}
	if notes := messageBodies(t, st, ch.ID); strings.Contains(strings.Join(notes, "\n"), "refreshed") {
		t.Errorf("messages = %q, want no refreshed notice before the replacement started", notes)
	}

	good := newRecordingAgent()
	good.session = "s2"
	next = func() *recordingAgent { return good }
	if err := s.Run(ctx, "retry", nil, discard); err != nil {
		t.Fatal(err)
	}
	if !good.started || good.resumed != "" || good.cfg.Model != "opus" {
		t.Errorf("replacement started = %v, resumed %q, model %q; want a new context on opus", good.started, good.resumed, good.cfg.Model)
	}
	if len(good.prompts) != 1 {
		t.Fatalf("replacement prompts = %q, want one", good.prompts)
	}
	prompt := good.prompts[0]
	for _, want := range []string{
		"## Handover", "Goal: Login", "decision 1: Use Google OAuth.",
		"sub-item 2: Add callback route", "sub-item 3: Write the login tests (backlog)",
		"Workspace: " + dir + "/worktrees/1/1, branch work-item/1",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("handover missing %q in:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "message a") || !strings.Contains(prompt, "message h") {
		t.Errorf("handover = \n%s\nwant the last messages and not the first", prompt)
	}
	if notes := messageBodies(t, st, ch.ID); !slices.Contains(notes, "reef: architect refreshed: claude, opus") {
		t.Errorf("messages = %q, want the refreshed notice", notes)
	}

	if err := s.Run(ctx, "again", nil, discard); err != nil {
		t.Fatal(err)
	}
	if len(good.prompts) != 2 || strings.Contains(good.prompts[1], "## Handover") {
		t.Errorf("second prompt = %q, want no handover", good.prompts[1:])
	}
	for _, ag := range ags[1:] {
		if ag.resumed != "" {
			t.Errorf("replacement resumed %q, want the replaced session never resumed", ag.resumed)
		}
	}
	old, err := st.Turns(ctx, store.TurnFilter{SpaceID: &ch.ID, Status: "done"})
	if err != nil || len(old) != 10 {
		t.Errorf("done turns = %d, %v, want the 8 earlier ones kept and 2 new", len(old), err)
	}
	active, err := st.ActiveBotSession(ctx, ch.ID, *architect)
	if err != nil || active.ID != 2 || active.Model != "opus" {
		t.Errorf("active bot session = %+v, %v, want id 2 on opus", active, err)
	}
}

func TestSession_RenewalKeepsAdoptedSettingsAndHandsOver(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionSpace(t, st, f, false)
	architect := testBotID(t, st, "architect")
	editBot(t, st, "architect", func(b *store.Bot) { b.Model = "sonnet" })
	bs, err := st.AdoptBotSession(ctx, ch.ID, *architect)
	if err != nil {
		t.Fatal(err)
	}
	turn, err := st.CreateTurn(ctx, store.Turn{
		SpaceID: ch.ID, Role: "architect", BotID: architect, BotSessionID: &bs.ID, Backend: "claude",
		SessionID: "s1", WakeText: "go",
	})
	if err != nil {
		t.Fatal(err)
	}
	turn.Status, turn.ContextTokens = "done", 152_006
	if _, err := st.EndTurn(ctx, turn); err != nil {
		t.Fatal(err)
	}
	editBot(t, st, "architect", func(b *store.Bot) { b.Model = "opus" })

	ag := newRecordingAgent()
	s := openRefreshSession(t, st, ch, t.TempDir(), ag.factory)
	if err := sendAs(ctx, st, ch.ID, s.Run, "next", discard); err != nil {
		t.Fatal(err)
	}
	if !ag.started || ag.resumed != "" || ag.cfg.Model != "sonnet" {
		t.Errorf("started = %v, resumed %q, model %q; want a new context on the adopted sonnet", ag.started, ag.resumed, ag.cfg.Model)
	}
	if len(ag.prompts) != 1 || !strings.Contains(ag.prompts[0], "## Handover") {
		t.Errorf("prompts = %q, want a handover", ag.prompts)
	}
	if notes := messageBodies(t, st, ch.ID); strings.Contains(strings.Join(notes, "\n"), "refreshed") {
		t.Errorf("messages = %q, want no refreshed notice for a renewal", notes)
	}
}

func TestSession_ReplacedBotSessionIsNotReusedFromAnotherProcess(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionSpace(t, st, f, false)
	architect := testBotID(t, st, "architect")
	first, second := newRecordingAgent(), newRecordingAgent()
	ags := []*recordingAgent{first, second}
	mk := func(string) (agent.Agent, error) {
		ag := ags[0]
		ags = ags[1:]
		return ag, nil
	}
	s := openRefreshSession(t, st, ch, t.TempDir(), mk)
	if err := sendAs(ctx, st, ch.ID, s.Run, "one", discard); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := st.RevokeBot(ctx, *architect); err != nil {
		t.Fatal(err)
	}
	if err := sendAs(ctx, st, ch.ID, s.Run, "two", discard); err != nil {
		t.Fatal(err)
	}
	if !first.closed || !second.started || len(first.prompts) != 1 {
		t.Errorf("first closed = %v, second started = %v, first prompts = %d; want the revoked context closed and replaced",
			first.closed, second.started, len(first.prompts))
	}
}

func TestSession_RenewsWhenTheLastModelCallReadAFullContext(t *testing.T) {
	for _, tc := range []struct {
		name  string
		usage []agent.Usage
		renew bool
	}{
		{"claude below", []agent.Usage{
			{InputTokens: 5, CacheReadTokens: 22_898, CacheWriteTokens: 1_100},
			{InputTokens: 6, CacheReadTokens: 54_002, CacheWriteTokens: 2_000},
		}, false},
		{"claude above", []agent.Usage{
			{InputTokens: 5, CacheReadTokens: 22_898},
			{InputTokens: 6, CacheReadTokens: 140_000, CacheWriteTokens: 12_000},
		}, true},
		{"codex above", []agent.Usage{
			{InputTokens: 20_000, CacheReadTokens: 140_000, OutputTokens: 900},
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			st, f := openStore(t)
			ch := sessionSpace(t, st, f, false)
			first, second := newRecordingAgent(), newRecordingAgent()
			first.usage = tc.usage
			ags := []*recordingAgent{first, second}
			mk := func(string) (agent.Agent, error) {
				ag := ags[0]
				if len(ags) > 1 {
					ags = ags[1:]
				}
				return ag, nil
			}
			s := openRefreshSession(t, st, ch, t.TempDir(), mk)
			for _, text := range []string{"one", "two"} {
				if err := sendAs(ctx, st, ch.ID, s.Run, text, discard); err != nil {
					t.Fatal(err)
				}
			}
			wantFirst := 2
			if tc.renew {
				wantFirst = 1
			}
			if second.started != tc.renew || len(first.prompts) != wantFirst {
				t.Errorf("second agent started = %v, first agent prompts = %d, want %v and %d",
					second.started, len(first.prompts), tc.renew, wantFirst)
			}
		})
	}
}
