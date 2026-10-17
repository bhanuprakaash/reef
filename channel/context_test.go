package channel

import (
	"context"
	"os/exec"
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
	repo := t.TempDir()
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"-c", "user.name=t", "-c", "user.email=t@t", "commit", "--allow-empty", "-m", "first"},
	} {
		out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	p, err := st.CreateProject(ctx, store.Project{Name: "shop", Path: repo})
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
	for _, body := range []string{"use OAuth", "store sessions in SQLite\nsecret second line", strings.Repeat("é", 101)} {
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
	if _, err := st.CreateChannel(ctx, store.Channel{
		ProjectID: f.ProjectID, FeatureID: &f.ID, Name: "empty",
	}); err != nil {
		t.Fatal(err)
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
Stages in order: ideation, grill, plan, tickets, implement, testing, live-testing, post-release. Call set_stage when the work moves to the next one.

## Brief
Use Google OAuth.

## Your notes
Keep the session in a cookie.

## Index

### Decisions
decision 1: use OAuth (user, ` + day + `)
decision 2: store sessions in SQLite (user, ` + day + `)
decision 3: ` + strings.Repeat("é", 100) + `… (user, ` + day + `)

### Channels
channel 1: spike, ` + day + `, 1 messages
channel 2: build, ` + day + `, 2 messages

### Open tasks
task 1 [todo]: write tests

Details are on demand through the reef MCP tools read_decision, read_thread and search_sessions. Record decisions with record_decision. When the user and the team agree on the goal, scope or plan, call propose_brief.
`
	if got != want {
		t.Errorf("Context =\n%s\nwant\n%s", got, want)
	}
	if strings.Contains(got, "secret message body") || strings.Contains(got, "secret second line") {
		t.Error("Context contains a message body or a second line of a decision")
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
Stages in order: ideation, grill, plan, tickets, implement, testing, live-testing, post-release. Call set_stage when the work moves to the next one.

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

Details are on demand through the reef MCP tools read_decision, read_thread and search_sessions. Record decisions with record_decision. When the user and the team agree on the goal, scope or plan, call propose_brief.
`
	if got != want {
		t.Errorf("Context =\n%s\nwant\n%s", got, want)
	}
}

func TestContext_FixesFeature(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	if _, err := st.UpdateFeature(ctx, store.Feature{ID: f.ID, Name: f.Name, Goal: f.Goal, Kind: "fixes"}); err != nil {
		t.Fatal(err)
	}

	got, err := Context(ctx, st, f.ID, "architect")
	if err != nil {
		t.Fatal(err)
	}
	want := "Goal: Google login\nThis feature is a list of fixes tracked as tasks, not one piece of work with stages.\n\n## Brief"
	if !strings.Contains(got, want) || strings.Contains(got, "Stage:") {
		t.Errorf("Context =\n%s\nwant it to contain %q and no Stage line", got, want)
	}
}

func TestContext_ProjectMemory(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	if _, err := st.PutProjectMemory(ctx,
		store.ProjectMemory{ProjectID: f.ProjectID, Body: "Use Go, not Python."}); err != nil {
		t.Fatal(err)
	}

	got, err := Context(ctx, st, f.ID, "architect")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "\n## Project memory\nUse Go, not Python.\n") {
		t.Errorf("Context =\n%s\nwant it to contain the project memory section", got)
	}
}

func TestProjectContext_Memory(t *testing.T) {
	p := store.Project{Name: "shop", Path: "/work/shop"}

	if got := projectContext(p, "lead", "", ""); strings.Contains(got, "Project memory") {
		t.Errorf("projectContext with no memory = %q, want no Project memory section", got)
	}

	got := projectContext(p, "lead", "Use Go, not Python.", "")
	want := `You are the lead in project "shop" (/work/shop).
This is a quick session: it has no feature yet, so there is no brief, notes or decisions.

## Project memory
Use Go, not Python.
`
	if got != want {
		t.Errorf("projectContext with memory =\n%s\nwant\n%s", got, want)
	}
}

func TestContext_Profile(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)

	first := func() string {
		t.Helper()
		got, err := Context(ctx, st, f.ID, "architect")
		if err != nil {
			t.Fatal(err)
		}
		return strings.SplitN(got, "\n", 3)[1]
	}
	if got := first(); got != "Goal: Google login" {
		t.Errorf("second line with no profile = %q, want the goal", got)
	}

	tests := []struct {
		name string
		give store.Profile
		want string
	}{
		{"name only", store.Profile{Name: "Ada"}, "You work with Ada."},
		{"occupation", store.Profile{Name: "Ada", Occupation: "Developer"},
			"You work with Ada, a developer."},
		{"other occupation", store.Profile{Name: "Ada", Occupation: "Other"}, "You work with Ada."},
		{"occupation and style", store.Profile{Name: "Ada", Occupation: "Developer",
			Style: []string{"Short answers", "Ask before big changes"}},
			"You work with Ada, a developer. They like: short answers, ask before big changes."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := st.PutProfile(ctx, tt.give); err != nil {
				t.Fatal(err)
			}
			if got := first(); got != tt.want {
				t.Errorf("second line = %q, want %q", got, tt.want)
			}
		})
	}

	p := store.Project{Name: "shop", Path: "/work/shop"}
	want := `You are the lead in project "shop" (/work/shop).
This is a quick session: it has no feature yet, so there is no brief, notes or decisions.
You work with Ada, a developer. They like: short answers, ask before big changes.
`
	who, err := profileLine(ctx, st)
	if err != nil {
		t.Fatal(err)
	}
	if got := projectContext(p, "lead", "", who); got != want {
		t.Errorf("projectContext =\n%s\nwant\n%s", got, want)
	}
}

func TestContext_DecisionFirstLine(t *testing.T) {
	tests := []struct {
		name string
		give string
		want string
	}{
		{
			name: "windows line ending",
			give: "first line\r\nsecond",
			want: "first line",
		},
		{
			name: "carriage return",
			give: "first line\rsecond",
			want: "first line",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			st, f := openStore(t)
			d, err := st.CreateDecision(ctx, store.Decision{
				FeatureID: f.ID, Body: tt.give, Author: "user",
			})
			if err != nil {
				t.Fatal(err)
			}

			got, err := Context(ctx, st, f.ID, "developer")
			if err != nil {
				t.Fatal(err)
			}
			want := "decision 1: " + tt.want + " (user, " + d.CreatedAt.Format("2006-01-02") + ")"
			if !strings.Contains(got, want) {
				t.Errorf("Context index does not contain %q:\n%s", want, got)
			}
		})
	}
}

func TestTeamIntro(t *testing.T) {
	lead, architect := store.Role{Name: "lead", Lead: true}, store.Role{Name: "architect"}
	team := store.Team{Name: "core", Roles: []store.Role{lead, architect}}
	const common = "\n## Team\nYour team: @lead, @architect. Use post_to_channel and @mentions " +
		"for work the whole team needs. Use send_to_role for private coordination with exactly one " +
		"teammate; their ordinary reply returns directly once, and continuing requires another " +
		"send_to_role. A public message that wakes you starts with [channel N, message N] or " +
		"[channel N, message N, thread N]: your " +
		"plain reply goes where the header says, a top-level one starting your own thread; pass " +
		"thread as the parent of post_to_channel when the header has one.\n"
	tests := []struct {
		name string
		team store.Team
		give store.Role
		want string
	}{
		{"solo team", store.Team{Name: "solo", Roles: []store.Role{lead}}, lead, ""},
		{"specialist", team, architect, common},
		{"lead", team, lead,
			common + "Do not summarize or repeat a specialist's post back to the user: they see it.\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := teamIntro(tt.team, tt.give); got != tt.want {
				t.Errorf("teamIntro = %q, want %q", got, tt.want)
			}
		})
	}
}
