package server

import (
	"context"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

func TestMCPProjectAssociationAndRelationship(t *testing.T) {
	ctx := context.Background()
	st, _ := mcpFixture(t)
	if _, err := st.CreateProject(ctx, store.Project{Name: "infra", Path: "/tmp/infra", Purpose: "deploys reef"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateProject(ctx, store.Project{Name: "blog", Path: "/tmp/blog"}); err != nil {
		t.Fatal(err)
	}
	msg, err := st.CreateMessage(ctx, store.Message{SpaceID: 4, Author: "user", Body: "infra deploys reef"})
	if err != nil {
		t.Fatal(err)
	}
	tok, err := testToken(t, st, ctx, 4, "architect")
	if err != nil {
		t.Fatal(err)
	}
	call := callTool(t, st, tok)

	got, isErr := call("list_projects", nil)
	want := `{"projects":[{"associated":true,"baseBranch":"","id":1,"name":"reef","path":"/tmp/reef","purpose":""},` +
		`{"associated":false,"baseBranch":"","id":2,"name":"infra","path":"/tmp/infra","purpose":"deploys reef"},` +
		`{"associated":false,"baseBranch":"","id":3,"name":"blog","path":"/tmp/blog","purpose":""}]}`
	if got != want || isErr {
		t.Errorf("list_projects = %q (error %v), want %q", got, isErr, want)
	}

	if _, isErr := call("associate_project", map[string]any{"projectId": 2}); isErr {
		t.Fatal("associate_project failed")
	}
	if _, isErr := call("associate_project", map[string]any{"projectId": 2}); isErr {
		t.Fatal("second associate_project failed")
	}
	msgs, err := st.Messages(ctx, 4, msg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || msgs[0].Author != "reef" || msgs[0].BotID == nil ||
		msgs[0].Body != "architect associated Project infra with this conversation." {
		t.Errorf("history after two associates = %+v, want one attributed note", msgs)
	}

	if got, isErr := call("relate_projects", map[string]any{"from": 3, "to": 1, "fact": "x"}); !isErr ||
		!strings.Contains(got, "call associate_project first") {
		t.Errorf("relate_projects from an unassociated Project = %q (error %v), want associate_project hint", got, isErr)
	}
	if _, isErr := call("relate_projects", map[string]any{"from": 2, "to": 1, "fact": "infra deploys reef"}); isErr {
		t.Fatal("relate_projects failed")
	}
	got, _ = call("read_project", map[string]any{"id": 1})
	want = `{"associated":true,"baseBranch":"","id":1,"name":"reef","path":"/tmp/reef","purpose":"",` +
		`"relationships":[{"author":"architect","body":"infra deploys reef","botId":2,"createdAt":"T","id":1,` +
		`"kind":"fact","links":[{"rel":"source","to":"space:4"},{"rel":"about","to":"project:2"},` +
		`{"rel":"about","to":"project:1"},{"rel":"about","to":"message:5"}],"replacedBy":null,"retiredAt":null,` +
		`"role":"architect","scope":"project:2","status":"accepted","why":""}]}`
	if got != want {
		t.Errorf("read_project = %s, want %s", got, want)
	}

	if _, isErr := call("dissociate_project", map[string]any{"projectId": 2}); isErr {
		t.Fatal("dissociate_project failed")
	}
	ps, err := st.SpaceProjects(ctx, 4)
	if err != nil || len(ps) != 1 || ps[0].ID != 1 {
		t.Errorf("projects after dissociate = %+v (%v), want only project 1", ps, err)
	}
}
