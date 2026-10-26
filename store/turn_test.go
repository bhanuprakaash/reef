package store

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bhanuprakaash/reef/agent"
)

type turnRow struct {
	ID     int64
	Status string
	Error  string
}

type messageRow struct {
	Author string
	Body   string
}

func turnRows(t *testing.T, s *Store) []turnRow {
	t.Helper()
	ts, err := s.Turns(context.Background(), TurnFilter{})
	if err != nil {
		t.Fatal(err)
	}
	var rows []turnRow
	for _, tr := range ts {
		rows = append(rows, turnRow{ID: tr.ID, Status: tr.Status, Error: tr.Error})
	}
	return rows
}

func messageRows(t *testing.T, s *Store, channelID int64) []messageRow {
	t.Helper()
	ms, err := s.Messages(context.Background(), channelID, 0)
	if err != nil {
		t.Fatal(err)
	}
	var rows []messageRow
	for _, m := range ms {
		rows = append(rows, messageRow{m.Author, m.Body})
	}
	return rows
}

func TestFailRunningTurns(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	if _, err := s.CreateProject(ctx, Project{Name: "reef", Path: "/tmp/reef"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a", "b", "c"} {
		if _, err := s.CreateChannel(ctx, Channel{ProjectID: 1, Name: name}); err != nil {
			t.Fatal(err)
		}
	}
	for _, tr := range []Turn{
		{ChannelID: 1, Role: "architect", Backend: "claude"},
		{ChannelID: 2, Role: "developer", Backend: "codex"},
		{ChannelID: 1, Role: "developer", Backend: "claude"},
		{ChannelID: 3, Role: "tester", Backend: "claude"},
	} {
		if _, err := s.CreateTurn(ctx, tr); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.EndTurn(ctx, Turn{ID: 4, Status: "done"}); err != nil {
		t.Fatal(err)
	}

	failed, err := s.FailRunningTurns(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(failed) != 3 {
		t.Errorf("failed %d turns, want 3", len(failed))
	}

	wantTurns := []turnRow{
		{ID: 1, Status: "failed", Error: "server restarted"},
		{ID: 2, Status: "failed", Error: "server restarted"},
		{ID: 3, Status: "failed", Error: "server restarted"},
		{ID: 4, Status: "done"},
	}
	if got := turnRows(t, s); !reflect.DeepEqual(got, wantTurns) {
		t.Errorf("turns = %v, want %v", got, wantTurns)
	}
	tr, err := s.Turn(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if tr.EndedAt == nil {
		t.Error("turn 1 has no endedAt")
	}

	wantMessages := map[int64][]messageRow{
		1: {{"reef", "Turn 1 by architect, turn 3 by developer failed: server restarted. The worktree may be dirty; retry or roll back."}},
		2: {{"reef", "Turn 2 by developer failed: server restarted. The worktree may be dirty; retry or roll back."}},
		3: nil,
	}
	for ch, want := range wantMessages {
		if got := messageRows(t, s, ch); !reflect.DeepEqual(got, want) {
			t.Errorf("channel %d messages = %v, want %v", ch, got, want)
		}
	}

	failed, err = s.FailRunningTurns(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(failed) != 0 {
		t.Errorf("second run failed %d turns, want 0", len(failed))
	}
	for ch, want := range wantMessages {
		if got := messageRows(t, s, ch); !reflect.DeepEqual(got, want) {
			t.Errorf("channel %d messages after second run = %v, want %v", ch, got, want)
		}
	}
}

func TestFailRunningTurns_MissingDirectMessageDoesNotBlock(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.CreateProject(ctx, Project{Name: "reef", Path: "/tmp/reef"}); err != nil {
		t.Fatal(err)
	}
	ch, err := s.CreateChannel(ctx, Channel{ProjectID: 1, Name: "c", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	// Insert a running turn that references a missing direct message. Foreign keys are
	// on for this store's own connections, so open a second connection with them off to
	// create the dangling reference that a crash could leave behind.
	db2, err := sql.Open("sqlite", "file:"+dir+"/reef.db?_pragma=foreign_keys(OFF)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db2.Close() }()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db2.ExecContext(ctx,
		"INSERT INTO turn (channel_id, role, backend, status, started_at, wake_text, direct_message_id) "+
			"VALUES (?, ?, ?, ?, ?, ?, ?)",
		ch.ID, "architect", "claude", "running", now, "do", 999999); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FailRunningTurns(ctx, nil); err != nil {
		t.Fatalf("missing direct message blocked recovery: %v", err)
	}
	tr, err := s.Turn(ctx, 1)
	if err != nil || tr.Status != "failed" || tr.Error != "server restarted" {
		t.Fatalf("turn = %+v, %v, want failed server restarted", tr, err)
	}
	direct, err := s.DirectMessages(ctx, ch.ID, 0)
	if err != nil || len(direct) != 0 {
		t.Fatalf("direct = %+v, %v, want none", direct, err)
	}
}

func TestFailRunningTurns_ValidDirectRecovery(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.CreateProject(ctx, Project{Name: "reef", Path: "/tmp/reef"}); err != nil {
		t.Fatal(err)
	}
	ch, err := s.CreateChannel(ctx, Channel{ProjectID: 1, Name: "c", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	req, err := s.CreateDirectMessage(ctx, DirectMessage{ChannelID: ch.ID, From: "lead", To: "architect", Body: "do"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateTurn(ctx, Turn{ChannelID: ch.ID, Role: "architect", Backend: "claude",
		WakeText: "do", DirectMessageID: req.IDRef()}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FailRunningTurns(ctx, nil); err != nil {
		t.Fatal(err)
	}
	direct, err := s.DirectMessages(ctx, ch.ID, 0)
	if err != nil || len(direct) != 2 {
		t.Fatalf("direct = %+v, %v", direct, err)
	}
	if direct[0].Status != DirectSent || direct[0].From != "lead" || direct[0].To != "architect" {
		t.Fatalf("request = %+v, want sent lead->architect", direct[0])
	}
	if direct[1].Status != DirectFailed || direct[1].From != "architect" || direct[1].To != "lead" ||
		direct[1].ReplyToID == nil || *direct[1].ReplyToID != req.ID {
		t.Fatalf("response = %+v, want failed architect->lead replying to %d", direct[1], req.ID)
	}
}

func TestClearStaleActiveTurns(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.CreateProject(ctx, Project{Name: "reef", Path: "/tmp/reef"}); err != nil {
		t.Fatal(err)
	}
	stale, err := s.CreateChannel(ctx, Channel{ProjectID: 1, Name: "stale", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	kept, err := s.CreateChannel(ctx, Channel{ProjectID: 1, Name: "kept", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, "UPDATE channel SET active_turn = ? WHERE id IN (?, ?)",
		ActiveTurnMessage, stale.ID, kept.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.ClearStaleActiveTurns(ctx, func(channelID int64) bool {
		return channelID == kept.ID
	}); err != nil {
		t.Fatal(err)
	}
	if c, err := s.Channel(ctx, stale.ID); err != nil || c.ActiveTurn != "" {
		t.Fatalf("stale channel active_turn = %q, want cleared", c.ActiveTurn)
	}
	if c, err := s.Channel(ctx, kept.ID); err != nil || c.ActiveTurn != ActiveTurnMessage {
		t.Fatalf("kept channel active_turn = %q, want preserved", c.ActiveTurn)
	}
}

// TestClearActiveTurnIf clears the hint a chain owns and leaves the hint a newer chain
// wrote after it alone, so a delayed teardown cannot blank a newer chain's starting state.
func TestClearActiveTurnIf(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.CreateProject(ctx, Project{Name: "reef", Path: "/tmp/reef"}); err != nil {
		t.Fatal(err)
	}
	ch, err := s.CreateChannel(ctx, Channel{ProjectID: 1, Name: "c", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	// Clearing with no known hint changes nothing rather than guessing.
	if err := s.ClearActiveTurnIf(ctx, ch.ID, ActiveTurnNone); err != nil {
		t.Fatal(err)
	}
	if err := s.ClearActiveTurnIf(ctx, ch.ID, ActiveTurnMessage); err != nil {
		t.Fatal(err)
	}
	if err := s.SetActiveTurn(ctx, ch.ID, ActiveTurnMessage); err != nil {
		t.Fatal(err)
	}
	if err := s.ClearActiveTurnIf(ctx, ch.ID, ActiveTurnMessage); err != nil {
		t.Fatal(err)
	}
	if c, err := s.Channel(ctx, ch.ID); err != nil || c.ActiveTurn != "" {
		t.Fatalf("active_turn = %q, %v, want cleared", c.ActiveTurn, err)
	}
	if err := s.SetActiveTurn(ctx, ch.ID, ActiveTurnStarting); err != nil {
		t.Fatal(err)
	}
	if err := s.ClearActiveTurnIf(ctx, ch.ID, ActiveTurnMessage); err != nil {
		t.Fatal(err)
	}
	if c, err := s.Channel(ctx, ch.ID); err != nil || c.ActiveTurn != ActiveTurnStarting {
		t.Fatalf("active_turn = %q, %v, want another chain's starting preserved during clear", c.ActiveTurn, err)
	}
}

func TestSetTurnSession(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	if _, err := s.CreateProject(ctx, Project{Name: "reef", Path: "/tmp/reef"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateChannel(ctx, Channel{ProjectID: 1, Name: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateTurn(ctx, Turn{ChannelID: 1, Role: "architect", Backend: "claude"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetTurnSession(ctx, 1, "abc"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FailRunningTurns(ctx, nil); err != nil {
		t.Fatal(err)
	}
	tr, err := s.Turn(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if tr.Status != "failed" || tr.SessionID != "abc" {
		t.Errorf("turn = %q %q, want failed abc", tr.Status, tr.SessionID)
	}
	if _, err := s.SetTurnSession(ctx, 1, "def"); !errors.Is(err, ErrInvalid) {
		t.Errorf("session on ended turn: err = %v, want ErrInvalid", err)
	}
}

func TestSetTurnRetried(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.CreateProject(ctx, Project{Name: "reef", Path: "/tmp/reef"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateChannel(ctx, Channel{ProjectID: 1, Name: "a"}); err != nil {
		t.Fatal(err)
	}
	msg, err := s.CreateMessage(ctx, Message{ChannelID: 1, Author: "user", Body: "go"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateTurn(ctx, Turn{
		ChannelID: 1, Role: "developer", Backend: "claude", WakeText: "build it", WakeParent: &msg.ID,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateTurn(ctx, Turn{ChannelID: 1, Role: "tester", Backend: "claude"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.EndTurn(ctx, Turn{ID: 1, Status: "failed", Error: "boom"}); err != nil {
		t.Fatal(err)
	}

	if err := s.SetTurnRetried(ctx, 2, true); !errors.Is(err, ErrConflict) {
		t.Errorf("retry a running turn = %v, want ErrConflict", err)
	}
	if err := s.SetTurnRetried(ctx, 9, true); !errors.Is(err, ErrNotFound) {
		t.Errorf("retry an unknown turn = %v, want ErrNotFound", err)
	}
	if err := s.SetTurnRetried(ctx, 1, true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTurnRetried(ctx, 1, true); !errors.Is(err, ErrConflict) {
		t.Errorf("retry twice = %v, want ErrConflict", err)
	}
	got, err := s.Turn(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Retried || got.WakeText != "build it" || got.WakeParent == nil || *got.WakeParent != 1 {
		t.Errorf("turn = %+v, want retried with wake %q in thread 1", got, "build it")
	}
	if err := s.SetTurnRetried(ctx, 1, false); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Turn(ctx, 1); got.Retried {
		t.Errorf("turn = %+v, want not retried", got)
	}
}

func TestEndTurn_InspectFields(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.CreateProject(ctx, Project{Name: "reef", Path: "/tmp/reef"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateChannel(ctx, Channel{ProjectID: 1, Name: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateTurn(ctx, Turn{ChannelID: 1, Role: "architect", Backend: "claude"}); err != nil {
		t.Fatal(err)
	}
	cost := 0.0123
	if _, err := s.EndTurn(ctx, Turn{
		ID: 1, Status: "done", Model: "claude-opus-5-5", CostUSD: &cost, Prompt: "do it",
	}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Turn(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Model != "claude-opus-5-5" || got.CostUSD == nil || *got.CostUSD != cost || got.Prompt != "do it" {
		t.Errorf("turn = %+v, want model claude-opus-5-5, cost %v, prompt %q", got, cost, "do it")
	}
}

// A session's earlier totals are the latest one a turn reported, plus the usage of the turns
// after it that reported none. Turns stored before totals were kept hold a running total as
// their cost, and their usage adds up.
func TestSessionBefore(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.CreateProject(ctx, Project{Name: "reef", Path: "/tmp/reef"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateChannel(ctx, Channel{ProjectID: 1, Name: "a"}); err != nil {
		t.Fatal(err)
	}
	end := func(session string, t0 Turn) int64 {
		t.Helper()
		tn, err := s.CreateTurn(ctx, Turn{ChannelID: 1, Role: "architect", Backend: "claude", SessionID: session})
		if err != nil {
			t.Fatal(err)
		}
		t0.ID, t0.Status = tn.ID, "done"
		if _, err := s.EndTurn(ctx, t0); err != nil {
			t.Fatal(err)
		}
		return tn.ID
	}
	f := func(v float64) *float64 { return &v }
	before := func(session string, id int64) (float64, agent.Usage) {
		t.Helper()
		cost, err := s.SessionCostBefore(ctx, session, id)
		if err != nil {
			t.Fatal(err)
		}
		usage, err := s.SessionUsageBefore(ctx, session, id)
		if err != nil {
			t.Fatal(err)
		}
		return cost, usage
	}

	end("s1", Turn{CostUSD: f(1.00), Usage: agent.Usage{InputTokens: 10, OutputTokens: 1}})
	end("other", Turn{CostUSD: f(9.00), Usage: agent.Usage{InputTokens: 99}})
	last := end("s1", Turn{CostUSD: f(1.10), Usage: agent.Usage{InputTokens: 20, OutputTokens: 2}})
	cost, usage := before("s1", last+1)
	if want := (agent.Usage{InputTokens: 30, OutputTokens: 3}); cost != 1.10 || usage != want {
		t.Errorf("old turns only: cost %v, usage %+v, want 1.10, %+v", cost, usage, want)
	}
	if cost, usage := before("s1", 1); cost != 0 || usage != (agent.Usage{}) {
		t.Errorf("before its first turn: cost %v, usage %+v, want none", cost, usage)
	}

	total := agent.Usage{InputTokens: 100, OutputTokens: 10, CacheReadTokens: 7, CacheWriteTokens: 3}
	end("s1", Turn{CostUSD: f(0.15), SessionCostUSD: f(1.25), SessionUsage: &total, Usage: agent.Usage{InputTokens: 70}})
	stopped := end("s1", Turn{Usage: agent.Usage{InputTokens: 5, OutputTokens: 1}})
	cost, usage = before("s1", stopped+1)
	want := agent.Usage{InputTokens: 105, OutputTokens: 11, CacheReadTokens: 7, CacheWriteTokens: 3}
	if cost != 1.25 || usage != want {
		t.Errorf("after a turn with totals: cost %v, usage %+v, want 1.25, %+v", cost, usage, want)
	}
	tn, err := s.Turn(ctx, stopped-1)
	if err != nil {
		t.Fatal(err)
	}
	if tn.SessionCostUSD == nil || *tn.SessionCostUSD != 1.25 || tn.SessionUsage == nil || *tn.SessionUsage != total {
		t.Errorf("stored totals = %v, %+v, want 1.25, %+v", tn.SessionCostUSD, tn.SessionUsage, total)
	}
}

func TestToolCall(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.CreateProject(ctx, Project{Name: "reef", Path: "/tmp/reef"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateChannel(ctx, Channel{ProjectID: 1, Name: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateTurn(ctx, Turn{ChannelID: 1, Role: "architect", Backend: "claude"}); err != nil {
		t.Fatal(err)
	}
	tc, err := s.CreateToolCall(ctx, 1, 0, "Bash", `{"command":"ls"}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetToolCallResult(ctx, tc.ID, "a.txt\n"); err != nil {
		t.Fatal(err)
	}
	got, err := s.ToolCalls(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	want := []ToolCall{{ID: 1, TurnID: 1, Name: "Bash", Input: `{"command":"ls"}`, Result: "a.txt\n"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tool calls = %+v, want %+v", got, want)
	}
}

func TestCreateToolCall_CutsLongInput(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.CreateProject(ctx, Project{Name: "reef", Path: "/tmp/reef"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateChannel(ctx, Channel{ProjectID: 1, Name: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateTurn(ctx, Turn{ChannelID: 1, Role: "architect", Backend: "claude"}); err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("a", 5000)
	tc, err := s.CreateToolCall(ctx, 1, 0, "Bash", long)
	if err != nil {
		t.Fatal(err)
	}
	if len(tc.Input) != 4096 {
		t.Errorf("input len = %d, want 4096", len(tc.Input))
	}
	if err := s.SetToolCallResult(ctx, tc.ID, long); err != nil {
		t.Fatal(err)
	}
	got, err := s.ToolCalls(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got[0].Result) != 4096 {
		t.Errorf("result len = %d, want 4096", len(got[0].Result))
	}
}
