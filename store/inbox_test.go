package store

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
)

type inboxEnv struct {
	t   *testing.T
	s   *Store
	ctx context.Context
}

func newInboxEnv(t *testing.T) *inboxEnv {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	e := &inboxEnv{t: t, s: s, ctx: context.Background()}
	if _, err := s.CreateProject(e.ctx, Project{Name: "shop", Path: "/tmp/shop"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateChannel(e.ctx, Channel{ProjectID: 1, Name: "one"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateChannel(e.ctx, Channel{ProjectID: 1, Name: "two"}); err != nil {
		t.Fatal(err)
	}
	return e
}

func (e *inboxEnv) say(channel int64, author, body string) Message {
	e.t.Helper()
	m, err := e.s.CreateMessage(e.ctx, Message{ChannelID: channel, Author: author, Body: body})
	if err != nil {
		e.t.Fatal(err)
	}
	return m
}

func (e *inboxEnv) turn(channel int64, role, wake, status string) Turn {
	e.t.Helper()
	t, err := e.s.CreateTurn(e.ctx, Turn{ChannelID: channel, Role: role, Backend: "claude", WakeText: wake})
	if err != nil {
		e.t.Fatal(err)
	}
	if status == "running" {
		return t
	}
	t.Status, t.Error = status, ""
	if status == "failed" {
		t.Error = "boom"
	}
	if t, err = e.s.EndTurn(e.ctx, t); err != nil {
		e.t.Fatal(err)
	}
	return t
}

// inbox returns the items as "kind channel text", newest first.
func (e *inboxEnv) inbox() []string {
	e.t.Helper()
	items, err := e.s.Inbox(e.ctx)
	if err != nil {
		e.t.Fatal(err)
	}
	out := []string{}
	for _, i := range items {
		out = append(out, fmt.Sprintf("%s %d %s", i.Kind, i.ChannelID, i.Text))
	}
	return out
}

func (e *inboxEnv) want(want ...string) {
	e.t.Helper()
	if want == nil {
		want = []string{}
	}
	if got := e.inbox(); !reflect.DeepEqual(got, want) {
		e.t.Errorf("inbox = %q, want %q", got, want)
	}
}

func TestInbox_Empty(t *testing.T) {
	e := newInboxEnv(t)
	items, err := e.s.Inbox(e.ctx)
	if err != nil || len(items) != 0 {
		t.Errorf("Inbox = %#v, %v, want none", items, err)
	}
}

func TestInbox_Approval(t *testing.T) {
	e := newInboxEnv(t)
	m := e.say(1, "lead", "@architect plan")
	a := Approval{ChannelID: 1, From: "lead", To: "architect", MessageID: m.ID, Text: "plan"}
	made, err := e.s.CreateApproval(e.ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	a.Kind = ApprovalHold
	if _, err := e.s.CreateApproval(e.ctx, a); err != nil {
		t.Fatal(err)
	}
	e.want("approval 1 lead wants to wake architect")
	items, _ := e.s.Inbox(e.ctx)
	if items[0].ApprovalID == nil || *items[0].ApprovalID != made.ID || items[0].TurnID != nil {
		t.Errorf("item = %+v, want approval id %d and no turn id", items[0], made.ID)
	}

	if err := e.s.DecideApproval(e.ctx, made.ID, ApprovalRejected); err != nil {
		t.Fatal(err)
	}
	e.want()
}

func TestInbox_Paused(t *testing.T) {
	e := newInboxEnv(t)
	e.say(1, "user", "go")
	if err := e.s.PauseChannel(e.ctx, 1, "hop cap: 6 agent hops without a user message"); err != nil {
		t.Fatal(err)
	}
	e.want("paused 1 hop cap: 6 agent hops without a user message")

	if err := e.s.ContinueChannel(e.ctx, 1, TokenBudget); err != nil {
		t.Fatal(err)
	}
	e.want()
}

func TestInbox_Failed(t *testing.T) {
	e := newInboxEnv(t)
	e.turn(1, "lead", "", "failed")
	e.want()

	first := e.turn(1, "lead", "build it", "failed")
	e.want("failed 1 lead failed: boom")
	items, _ := e.s.Inbox(e.ctx)
	if items[0].TurnID == nil || *items[0].TurnID != first.ID || items[0].ApprovalID != nil {
		t.Errorf("item = %+v, want turn id %d and no approval id", items[0], first.ID)
	}

	e.turn(1, "lead", "build it again", "failed")
	items, _ = e.s.Inbox(e.ctx)
	if len(items) != 1 || *items[0].TurnID != first.ID+1 {
		t.Errorf("items = %+v, want only the newest failed turn", items)
	}

	e.turn(1, "architect", "plan", "failed")
	e.want("failed 1 architect failed: boom", "failed 1 lead failed: boom")
	if err := e.s.SetTurnRetried(e.ctx, first.ID+2, true); err != nil {
		t.Fatal(err)
	}
	e.want("failed 1 lead failed: boom")

	e.turn(1, "lead", "build it a third time", "done")
	e.want()
}

func TestInbox_FailedTurnThatPausedItsChannelIsNotAlsoListedAsFailed(t *testing.T) {
	e := newInboxEnv(t)
	e.turn(1, "lead", "build it", "failed")
	if err := e.s.PauseChannel(e.ctx, 1, "token budget of 1000000 used: 1000312 tokens"); err != nil {
		t.Fatal(err)
	}
	e.want("paused 1 token budget of 1000000 used: 1000312 tokens")

	if err := e.s.ContinueChannel(e.ctx, 1, TokenBudget); err != nil {
		t.Fatal(err)
	}
	e.want("failed 1 lead failed: boom")
}

func TestInbox_StoppedTurnIsNotAnItem(t *testing.T) {
	e := newInboxEnv(t)
	stopped := e.turn(1, "lead", "build it", "running")
	if _, err := e.s.EndTurn(e.ctx, Turn{ID: stopped.ID, Status: "failed", Error: "interrupted"}); err != nil {
		t.Fatal(err)
	}
	e.say(1, "lead", "half a reply")
	e.want()
	restarted := e.turn(2, "lead", "build it", "running")
	if _, err := e.s.FailRunningTurns(e.ctx, nil); err != nil {
		t.Fatal(err)
	}
	e.want("failed 2 lead failed: server restarted")
	if ok, err := e.s.Retryable(e.ctx, stopped.ID); err != nil || !ok {
		t.Errorf("Retryable(stopped) = %t, %v, want true", ok, err)
	}
	if ok, err := e.s.Retryable(e.ctx, restarted.ID); err != nil || !ok {
		t.Errorf("Retryable(restarted) = %t, %v, want true", ok, err)
	}
}

func TestInbox_Waiting(t *testing.T) {
	e := newInboxEnv(t)
	e.say(1, "user", "go")
	e.say(1, "lead", "done, what next?")
	e.say(2, "architect", "plan posted")
	e.say(2, "reef", "Turn 3 by architect failed: server restarted.")
	e.want("waiting 1 done, what next?")

	e.say(1, "user", "ship it")
	e.want()

	e.say(1, "lead", "shipped")
	run := e.turn(1, "lead", "ship it", "running")
	e.want()
	run.Status = "done"
	if _, err := e.s.EndTurn(e.ctx, run); err != nil {
		t.Fatal(err)
	}
	e.want("waiting 1 shipped")

	if err := e.s.PauseChannel(e.ctx, 1, "token budget of 1000000 used: 1000312 tokens"); err != nil {
		t.Fatal(err)
	}
	e.want("paused 1 token budget of 1000000 used: 1000312 tokens")
}

func TestInbox_ClosedChannelIsNotAnItem(t *testing.T) {
	e := newInboxEnv(t)
	e.say(1, "lead", "Wrap proposed.")
	e.want("waiting 1 Wrap proposed.")

	if _, err := e.s.db.ExecContext(e.ctx, "UPDATE channel SET closed_at = ? WHERE id = 1", "2026-01-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	e.want()

	if err := e.s.ReopenChannel(e.ctx, 1); err != nil {
		t.Fatal(err)
	}
	e.want("waiting 1 Wrap proposed.")
}

func TestInbox_NewestFirst(t *testing.T) {
	e := newInboxEnv(t)
	e.turn(2, "lead", "a", "failed")
	e.say(1, "lead", "hello")
	if err := e.s.PauseChannel(e.ctx, 2, "token budget of 1000000 used: 1000312 tokens"); err != nil {
		t.Fatal(err)
	}
	e.want("waiting 1 hello", "paused 2 token budget of 1000000 used: 1000312 tokens")
}

func TestInbox_DismissWaiting(t *testing.T) {
	e := newInboxEnv(t)
	e.say(1, "user", "go")
	reply := e.say(1, "lead", "done, your call")
	other := e.say(2, "lead", "also done")
	e.want("waiting 2 also done", "waiting 1 done, your call")
	items, _ := e.s.Inbox(e.ctx)
	if items[1].MessageID == nil || *items[1].MessageID != reply.ID {
		t.Fatalf("item = %+v, want message id %d", items[1], reply.ID)
	}

	if err := e.s.DismissWaiting(e.ctx, 1, reply.ID); err != nil {
		t.Fatal(err)
	}
	e.want("waiting 2 also done")

	e.say(1, "lead", "one more thing")
	e.want("waiting 1 one more thing", "waiting 2 also done")

	if err := e.s.DismissWaiting(e.ctx, 1, other.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("DismissWaiting(message of another channel) = %v, want ErrNotFound", err)
	}
}

func TestInbox_DismissLeavesApprovalsAlone(t *testing.T) {
	e := newInboxEnv(t)
	m := e.say(1, "lead", "@architect plan")
	a := Approval{ChannelID: 1, From: "lead", To: "architect", MessageID: m.ID, Text: "plan"}
	if _, err := e.s.CreateApproval(e.ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := e.s.DismissWaiting(e.ctx, 1, m.ID); err != nil {
		t.Fatal(err)
	}
	e.want("approval 1 lead wants to wake architect")
}
