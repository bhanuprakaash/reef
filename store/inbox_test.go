package store

import (
	"context"
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
	if err != nil || items == nil || len(items) != 0 {
		t.Errorf("Inbox = %#v, %v, want an empty list", items, err)
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
	if err := e.s.PauseChannel(e.ctx, 1, "hop cap: 4 agent hops without a user message"); err != nil {
		t.Fatal(err)
	}
	e.want("paused 1 hop cap: 4 agent hops without a user message")

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

	if err := e.s.PauseChannel(e.ctx, 1, "capped: more than 25 tool calls"); err != nil {
		t.Fatal(err)
	}
	e.want("paused 1 capped: more than 25 tool calls")
}

func TestInbox_NewestFirst(t *testing.T) {
	e := newInboxEnv(t)
	e.turn(2, "lead", "a", "failed")
	e.say(1, "lead", "hello")
	if err := e.s.PauseChannel(e.ctx, 2, "capped: more than 25 tool calls"); err != nil {
		t.Fatal(err)
	}
	e.want("waiting 1 hello", "paused 2 capped: more than 25 tool calls", "failed 2 lead failed: boom")
}
