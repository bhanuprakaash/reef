package server

import (
	"context"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/memory"
	"github.com/bhanuprakaash/reef/store"
)

func TestMCPMemoryScopesForAnyNamedBot(t *testing.T) {
	ctx := context.Background()
	st, _ := mcpFixture(t)
	ms := openMemory(t, st)
	prakash := *testServerBot(t, st, "Prakash")
	sam := *testServerBot(t, st, "Sam")
	if _, err := st.CreateProject(ctx, store.Project{Name: "web", Path: "/tmp/web"}); err != nil {
		t.Fatal(err)
	}
	callIn := func(c store.Space, botID int64) func(string, map[string]any) (string, bool) {
		tok, err := st.CreateToken(ctx, c.ID, botID)
		if err != nil {
			t.Fatal(err)
		}
		return callToolMemory(t, st, ms, tok, t.TempDir())
	}
	chat := func(botID int64, projects ...int64) (store.Space, func(string, map[string]any) (string, bool)) {
		c, err := st.CreateBotSpace(ctx, store.Space{Kind: store.KindConversation, Name: "chat"}, botID)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range projects {
			if err := st.AddSpaceProject(ctx, c.ID, p, nil); err != nil {
				t.Fatal(err)
			}
		}
		return c, callIn(c, botID)
	}
	c, call := chat(prakash, 1, 2)
	w, err := st.CreateWorkItem(ctx, store.WorkItem{ProjectID: 1, Title: "Invoices"})
	if err != nil {
		t.Fatal(err)
	}
	wi := w.ID
	wc, err := st.CreateBotSpace(ctx, store.Space{ProjectID: 1, WorkItemID: &wi, Name: "billing"}, prakash)
	if err != nil {
		t.Fatal(err)
	}
	workCall := callIn(wc, prakash)

	saved := func(call func(string, map[string]any) (string, bool), give map[string]any, wantScope string) {
		t.Helper()
		got, isErr := call("remember", give)
		if isErr || !strings.Contains(got, `"scope":"`+wantScope+`"`) ||
			!strings.Contains(got, `"author":"Prakash"`) {
			t.Fatalf("remember %v = %q (error %v), want scope %s by Prakash", give, got, isErr, wantScope)
		}
	}
	saved(workCall, map[string]any{"kind": "decision", "body": "bill monthly"}, "work_item:3")
	saved(call, map[string]any{"kind": "finding", "body": "chat finding"}, "conversation:"+strconv.FormatInt(c.ID, 10))
	saved(call, map[string]any{
		"kind": "lesson", "body": "I prefer small diffs on web", "scope": "bot", "about": []string{"project:2"},
	}, "bot:"+strconv.FormatInt(prakash, 10))
	saved(call, map[string]any{
		"kind": "fact", "body": "web builds with vite", "scope": "project", "projectId": 2,
	}, "project:2")

	for _, tt := range []struct {
		name string
		give map[string]any
		want string
	}{
		{"two Projects, none named", map[string]any{"scope": "project"},
			"invalid: this Space has several Projects (1, 2); say which with projectId"},
		{"a Project the conversation does not have", map[string]any{"scope": "project", "projectId": 3},
			`invalid: you have no "project" scope here`},
		{"team", map[string]any{"scope": "team"},
			`invalid: scope "team" is not work_item, space, conversation, project or bot`},
	} {
		tt.give["kind"], tt.give["body"] = "fact", "x"
		if got, isErr := call("remember", tt.give); got != tt.want || !isErr {
			t.Errorf("%s = %q (error %v), want %q", tt.name, got, isErr, tt.want)
		}
	}

	before, err := ms.Read(ctx, 4)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.RemoveSpaceProject(ctx, c.ID, 2, nil); err != nil {
		t.Fatal(err)
	}
	after, err := ms.Read(ctx, 4)
	source := []memory.Link{{Rel: memory.RelSource, ToKind: "space", ToID: strconv.FormatInt(c.ID, 10)}}
	if err != nil || !reflect.DeepEqual(before, after) || after.BotID == nil || *after.BotID != prakash ||
		!reflect.DeepEqual(after.Links, source) {
		t.Errorf("project item after removing the association = %+v, %v, want it unchanged", after, err)
	}
	if got, isErr := call("recall", map[string]any{"query": "chat finding"}); isErr ||
		!strings.Contains(got, `"body":"chat finding"`) {
		t.Errorf("recall of the conversation item after the removal = %q (error %v)", got, isErr)
	}
	if got, _ := call("recall", map[string]any{"query": "vite"}); strings.Contains(got, "vite") {
		t.Errorf("recall of Project 2 after the removal = %q, want no hit", got)
	}

	_, samCall := chat(sam, 2)
	if got, _ := samCall("recall", map[string]any{"query": "vite"}); !strings.Contains(got, `"scope":"project:2"`) {
		t.Errorf("another Bot's recall of Project 2 = %q, want the project item", got)
	}
	if got, _ := samCall("recall", map[string]any{"query": "small diffs"}); strings.Contains(got, "small diffs") {
		t.Errorf("another Bot's recall = %q, want no personal item of Prakash", got)
	}
}
