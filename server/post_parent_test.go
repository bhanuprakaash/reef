package server

import (
	"context"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

// TestPostToSpace_DirectResponseGoesWhereItsSourceWas: a Bot woken by a direct response has no
// thread in its header, so its post goes where the public message that started the chain was
// answered. A top-level message from the user, sent after a thread was closed, gets a top-level
// post even when the Bot names the old thread.
func TestPostToSpace_DirectResponseGoesWhereItsSourceWas(t *testing.T) {
	ctx := context.Background()
	st, _ := mcpFixture(t)
	if _, err := testServerCreateTeam(t, st, ctx, store.Team{Name: "core", Roles: []store.Role{
		{Name: "lead", Lead: true, Backend: "claude"},
		{Name: "tester", Backend: "claude"},
	}}); err != nil {
		t.Fatal(err)
	}
	one := int64(1)
	closeSpace(t, st, 1)
	ch, err := st.CreateSpace(ctx, store.Space{
		ProjectID: 1, WorkItemID: &one, Name: "team", Team: "core", Role: "lead",
	})
	if err != nil {
		t.Fatal(err)
	}
	lead := testServerBot(t, st, "lead")
	msg := func(parent *int64, author, body string) store.Message {
		t.Helper()
		m, err := st.CreateMessage(ctx, store.Message{SpaceID: ch.ID, ParentID: parent, Author: author, Body: body})
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	root := msg(nil, "lead", "plan")
	inThread := msg(&root.ID, "user", "in the thread")
	general := msg(nil, "user", "a new question")

	postAfterResponse := func(source store.Message, supplied map[string]any) store.Message {
		t.Helper()
		turn, err := st.CreateTurn(ctx, store.Turn{
			SpaceID: ch.ID, Role: "lead", BotID: lead, Backend: "claude", WakeText: "w", WakeMessageID: &source.ID,
		})
		if err != nil {
			t.Fatal(err)
		}
		tok, err := testToken(t, st, ctx, ch.ID, "lead")
		if err != nil {
			t.Fatal(err)
		}
		if got, isErr := callTool(t, st, tok)("send_to_role", map[string]any{"to": "tester", "body": "hi"}); isErr {
			t.Fatalf("send_to_role = %q, want success", got)
		}
		turn.Status = "done"
		if _, err := st.EndTurn(ctx, turn); err != nil {
			t.Fatal(err)
		}
		direct, err := st.DirectMessages(ctx, ch.ID, 0)
		if err != nil {
			t.Fatal(err)
		}
		request := direct[len(direct)-1]
		answer, err := st.CreateDirectResponse(ctx, request.ID, request.ToBotID, "tester", "ok")
		if err != nil {
			t.Fatal(err)
		}
		turn, err = st.CreateTurn(ctx, store.Turn{
			SpaceID: ch.ID, Role: "lead", BotID: lead, Backend: "claude",
			WakeText: "[direct response]", DirectMessageID: answer.IDRef(),
		})
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			turn.Status = "done"
			if _, err := st.EndTurn(ctx, turn); err != nil {
				t.Fatal(err)
			}
		}()
		supplied["body"] = "answer"
		got, isErr := callTool(t, st, tok)("post_to_space", supplied)
		if isErr {
			t.Fatalf("post_to_space = %q, want success", got)
		}
		msgs, err := st.Messages(ctx, ch.ID, 0)
		if err != nil {
			t.Fatal(err)
		}
		return msgs[len(msgs)-1]
	}

	if got := postAfterResponse(general, map[string]any{"parent": root.ID}); got.ParentID != nil {
		t.Errorf("answer to a top-level message, naming the old thread %d, is in thread %d, want top level",
			root.ID, *got.ParentID)
	}
	if got := postAfterResponse(inThread, map[string]any{}); got.ParentID == nil || *got.ParentID != root.ID {
		t.Errorf("answer to a message in thread %d is at %v, want that thread", root.ID, got.ParentID)
	}
}
