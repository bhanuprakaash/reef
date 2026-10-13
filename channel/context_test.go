package channel

import (
	"context"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

func openStore(t *testing.T) (*store.Store, store.Feature) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	p, err := st.CreateProject(ctx, store.Project{Name: "shop", Path: "/tmp/shop"})
	if err != nil {
		t.Fatal(err)
	}
	f, err := st.CreateFeature(ctx, store.Feature{
		ProjectID: p.ID, Slug: "login", Name: "Login", Goal: "Google login",
	})
	if err != nil {
		t.Fatal(err)
	}
	return st, f
}

func TestContext(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)

	if _, err := st.PutBrief(ctx, store.Brief{FeatureID: f.ID, Body: "old brief", Author: "user"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutBrief(ctx, store.Brief{FeatureID: f.ID, Body: "Use Google OAuth.", Author: "user"}); err != nil {
		t.Fatal(err)
	}
	for _, n := range []store.RoleNotes{
		{FeatureID: f.ID, Role: "architect", Body: "Keep the session in a cookie."},
		{FeatureID: f.ID, Role: "developer", Body: "developer-only note"},
	} {
		if _, err := st.PutRoleNotes(ctx, n); err != nil {
			t.Fatal(err)
		}
	}
	var decided []store.Decision
	for _, body := range []string{"use OAuth", "store sessions in SQLite"} {
		d, err := st.CreateDecision(ctx, store.Decision{FeatureID: f.ID, Body: body, Author: "user"})
		if err != nil {
			t.Fatal(err)
		}
		decided = append(decided, d)
	}
	var channels []store.Channel
	for i, name := range []string{"spike", "build"} {
		c, err := st.CreateChannel(ctx, store.Channel{ProjectID: f.ProjectID, FeatureID: &f.ID, Name: name})
		if err != nil {
			t.Fatal(err)
		}
		channels = append(channels, c)
		for j := 0; j <= i; j++ {
			if _, err := st.CreateMessage(ctx, store.Message{
				ChannelID: c.ID, Author: "user", Body: "secret message body " + name,
			}); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, task := range []store.Task{
		{ProjectID: f.ProjectID, FeatureID: &f.ID, Title: "write tests"},
		{ProjectID: f.ProjectID, FeatureID: &f.ID, Title: "set up OAuth app", Status: "done"},
	} {
		if _, err := st.CreateTask(ctx, task); err != nil {
			t.Fatal(err)
		}
	}

	got, err := Context(ctx, st, f.ID, "architect")
	if err != nil {
		t.Fatal(err)
	}
	day := decided[0].CreatedAt.Format("2006-01-02")
	if day != channels[0].CreatedAt.Format("2006-01-02") {
		t.Skip("test ran across midnight UTC")
	}
	want := `You are the architect on feature "Login" (login) in project "shop".
Goal: Google login
Stage: ideation

## Brief
Use Google OAuth.

## Your notes
Keep the session in a cookie.

## Index

### Decisions
decision 1: use OAuth (user, ` + day + `)
decision 2: store sessions in SQLite (user, ` + day + `)

### Channels
channel 1: spike, ` + day + `, 1 messages
channel 2: build, ` + day + `, 2 messages

### Open tasks
task 1 [todo]: write tests

Details are on demand through the reef MCP tools read_decision, read_thread and search_sessions. Record decisions with record_decision.
`
	if got != want {
		t.Errorf("Context =\n%s\nwant\n%s", got, want)
	}
	if strings.Contains(got, "secret message body") {
		t.Error("Context contains a message body")
	}
}

func TestContext_EmptyFeature(t *testing.T) {
	st, f := openStore(t)

	got, err := Context(context.Background(), st, f.ID, "architect")
	if err != nil {
		t.Fatal(err)
	}
	want := `You are the architect on feature "Login" (login) in project "shop".
Goal: Google login
Stage: ideation

## Brief
No brief yet.

## Your notes
No notes yet.

## Index

### Decisions
none

### Channels
none

### Open tasks
none

Details are on demand through the reef MCP tools read_decision, read_thread and search_sessions. Record decisions with record_decision.
`
	if got != want {
		t.Errorf("Context =\n%s\nwant\n%s", got, want)
	}
}
