package space

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/memory"
	"github.com/bhanuprakaash/reef/store"
)

func testBotID(t *testing.T, st *store.Store, name string) *int64 {
	t.Helper()
	bots, err := st.Bots(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, bot := range bots {
		if bot.Name == name {
			return &bot.ID
		}
	}
	bot, err := st.CreateBot(context.Background(), store.Bot{Name: name, Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	return &bot.ID
}

func newTestRunner(t *testing.T, st *store.Store, ag agent.Agent, spaceID int64, role, backend, preamble string) *Runner {
	t.Helper()
	r := NewRunner(st, ag, spaceID, role, backend, preamble)
	r.botID = testBotID(t, st, role)
	return r
}

func testApplyTeam(t *testing.T, st *store.Store, ctx context.Context, team store.Team) (store.Team, error) {
	t.Helper()
	_, teamErr := st.Team(ctx, team.Name)
	for i, role := range team.Roles {
		if role.BotID != 0 {
			continue
		}
		id := testBotID(t, st, role.Name)
		bot := store.Bot{ID: *id, Name: role.Name, Description: role.Description, Skills: role.Skills, Backend: role.Backend, Model: role.Model, Provider: role.Provider, Face: role.Face, Colour: role.Colour, Tools: role.Tools, Connections: connectionIDs(role.Connections), Write: role.Write, PermissionMode: role.PermissionMode}
		if teamErr != nil {
			if _, err := st.UpdateBot(ctx, bot); err != nil {
				return store.Team{}, err
			}
		}
		team.Roles[i].BotID = *id
	}
	spec := store.TeamSpec{Name: team.Name, Responsibility: team.Responsibility}
	for _, role := range team.Roles {
		spec.Roles = append(spec.Roles, store.Member{BotID: role.BotID, Lead: role.Lead})
	}
	return st.ApplyTeam(ctx, spec)
}

func openStore(t *testing.T) (*store.Store, store.WorkItem) {
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
	f, err := st.CreateWorkItem(ctx, store.WorkItem{
		ProjectID: p.ID, Title: "Login", Description: "Google login",
	})
	if err != nil {
		t.Fatal(err)
	}
	return st, f
}

func testMemory(t *testing.T, st *store.Store) *memory.Store {
	t.Helper()
	ms, err := memory.Open(st.DB(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return ms
}

func remember(
	t *testing.T, ms *memory.Store, scope memory.Ref, kind, author, body string,
) memory.Item {
	t.Helper()
	r, err := ms.Remember(context.Background(),
		memory.Writer{Author: author, Role: author, Scope: scope}, memory.Note{Kind: kind, Body: body})
	if err != nil {
		t.Fatal(err)
	}
	if r.Item.Status != memory.StatusAccepted {
		t.Fatalf("remember %q: status %s, want accepted", body, r.Item.Status)
	}
	return r.Item
}

func workScope(f store.WorkItem) memory.Ref {
	return memory.Ref{Kind: memory.ScopeWorkItem, ID: strconv.FormatInt(f.ID, 10)}
}

func projectScope(f store.WorkItem) memory.Ref {
	return memory.Ref{Kind: memory.ScopeProject, ID: strconv.FormatInt(f.ProjectID, 10)}
}

func botScope(role string) memory.Ref {
	return memory.Ref{Kind: memory.ScopeBot, ID: role}
}

func TestContext(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)

	if _, err := st.PutBrief(ctx, store.Brief{WorkItemID: f.ID, Body: "old brief", Author: "user"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutBrief(ctx, store.Brief{WorkItemID: f.ID, Body: "Use Google OAuth.", Author: "user"}); err != nil {
		t.Fatal(err)
	}
	ms := testMemory(t, st)
	var decided []memory.Item
	for _, body := range []string{"use OAuth", "store sessions in SQLite\nsecret second line", strings.Repeat("é", 101)} {
		decided = append(decided, remember(t, ms, workScope(f), memory.KindDecision, "user", body))
	}
	remember(t, ms, botScope("architect"), memory.KindLesson, "architect", "Keep the session in a cookie.")
	remember(t, ms, botScope("developer"), memory.KindLesson, "developer", "developer-only note")
	var spaces []store.Space
	for i, name := range []string{"spike", "build"} {
		c, err := st.CreateSpace(ctx, store.Space{ProjectID: f.ProjectID, WorkItemID: &f.ID, Name: name})
		if err != nil {
			t.Fatal(err)
		}
		spaces = append(spaces, c)
		for j := 0; j <= i; j++ {
			if _, err := st.CreateMessage(ctx, store.Message{
				SpaceID: c.ID, Author: "user", Body: "secret message body " + name,
			}); err != nil {
				t.Fatal(err)
			}
		}
		closeSpace(t, st, c.ID)
	}
	if _, err := st.CreateSpace(ctx, store.Space{
		ProjectID: f.ProjectID, WorkItemID: &f.ID, Name: "empty",
	}); err != nil {
		t.Fatal(err)
	}

	got, err := Context(ctx, st, ms, f.ID, "architect")
	if err != nil {
		t.Fatal(err)
	}
	day := decided[0].CreatedAt.Format("2006-01-02")
	if day != spaces[0].CreatedAt.Format("2006-01-02") {
		t.Skip("test ran across midnight UTC")
	}
	want := `## You
You are architect on work item "Login" (1) in project "shop".
Description: Google login

## How to work
Details are on demand through the reef MCP tools recall, read_memory, read_thread and search_sessions. Record decisions with remember. When the user and the team agree on the goal, scope or plan, call propose_brief.

## What we know
### Brief
Use Google OAuth.

### Decisions
decision 3: ` + strings.Repeat("é", 100) + `… (user, ` + day + `)
decision 2: store sessions in SQLite (user, ` + day + `)
decision 1: use OAuth (user, ` + day + `)

### Your items
lesson 4: Keep the session in a cookie. (architect, ` + day + `)
1 of 1; recall(..., scope=bot)

### On demand
2 earlier Spaces (search_sessions).
`
	if got != want {
		t.Errorf("Context =\n%s\nwant\n%s", got, want)
	}
	if strings.Contains(got, "secret message body") || strings.Contains(got, "secret second line") {
		t.Error("Context contains a message body or a second line of a decision")
	}
}

func TestContext_EmptyWorkItem(t *testing.T) {
	st, f := openStore(t)

	got, err := Context(context.Background(), st, testMemory(t, st), f.ID, "architect")
	if err != nil {
		t.Fatal(err)
	}
	want := `## You
You are architect on work item "Login" (1) in project "shop".
Description: Google login

## How to work
Details are on demand through the reef MCP tools recall, read_memory, read_thread and search_sessions. Record decisions with remember. When the user and the team agree on the goal, scope or plan, call propose_brief.
`
	if got != want {
		t.Errorf("Context =\n%s\nwant\n%s", got, want)
	}
}

func TestContext_AcceptanceLines(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	g, err := st.CreateWorkItem(ctx, store.WorkItem{
		ProjectID: f.ProjectID, Title: "Logout", Description: "end the session",
		AcceptanceLines: []store.AcceptanceLine{
			{Text: "cookie is cleared", Checked: true}, {Text: "login page shows"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := Context(ctx, st, testMemory(t, st), g.ID, "architect")
	if err != nil {
		t.Fatal(err)
	}
	want := "Description: end the session\nAcceptance lines:\n1. [x] cookie is cleared\n2. [ ] login page shows\n"
	if !strings.Contains(got, want) {
		t.Errorf("Context =\n%s\nwant it to contain %q", got, want)
	}
}

func TestContext_OmitsDirectMessages(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch, err := st.CreateSpace(ctx, store.Space{
		ProjectID: f.ProjectID, WorkItemID: &f.ID, Name: "earlier", Role: "lead"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateDirectMessage(ctx, store.DirectMessage{
		SpaceID: ch.ID, From: "developer", To: "tester", FromBotID: testBotID(t, st, "developer"), ToBotID: testBotID(t, st, "tester"), Body: "private fixture detail"}); err != nil {
		t.Fatal(err)
	}
	got, err := Context(ctx, st, testMemory(t, st), f.ID, "tester")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "private fixture detail") || strings.Contains(got, "[direct") ||
		strings.Contains(got, "earlier") {
		t.Errorf("Context = %q, want no direct message and no Space that only has one", got)
	}
}

func TestFirstPrompt_QuickSession(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	p, err := st.Project(ctx, f.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	lead := store.Role{Name: "lead", Lead: true}
	team := store.Team{Name: "core", Roles: []store.Role{lead, {Name: "architect"}}}

	ms := testMemory(t, st)
	got, err := firstPrompt(ctx, st, ms, p.ID, nil, 0, store.Team{}, lead)
	if err != nil {
		t.Fatal(err)
	}
	want := `## You
You are lead.
This is a free Space: it has no work item, so there is no brief or work item items.

## How to work
Details are on demand through the reef MCP tools recall and read_memory. remember saves to this Space by default; use scope project or bot for what holds across the project or for you.
`
	if got != want {
		t.Errorf("quick session with no memory =\n%s\nwant\n%s", got, want)
	}

	day := remember(t, ms, projectScope(f), memory.KindFact, "user", "Use Go, not Python.").
		CreatedAt.Format(_dateLayout)
	remember(t, ms, botScope("1"), memory.KindLesson, "lead", "Ask before renaming.")
	if _, err := st.PutProfile(ctx, store.Profile{Name: "Ada"}); err != nil {
		t.Fatal(err)
	}
	p.BaseBranch = "dev"
	if _, err := st.UpdateProject(ctx, p); err != nil {
		t.Fatal(err)
	}
	got, err = firstPrompt(ctx, st, ms, p.ID, nil, 0, team, lead)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"## You\nYou are lead.\nThis is a free Space: " +
			"it has no work item, so there is no brief or work item items.\nYou work with Ada.\n" +
			"\n## How to work\nYour team: @lead, @architect. ",
		"\n\n## What we know\n### Project\nfact 1: Use Go, not Python. (user, " + day + ")\n" +
			"1 of 1; recall(..., scope=project)\n\n" +
			"### Your items\nlesson 2: Ask before renaming. (lead, " + day + ")\n" +
			"1 of 1; recall(..., scope=bot)\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("quick session =\n%s\nwant it to contain %q", got, want)
		}
	}
	for _, want := range []string{"recall", "read_memory", "remember saves to this Space by default"} {
		if !strings.Contains(got, want) {
			t.Errorf("quick session =\n%s\nwant it to name %q", got, want)
		}
	}
	if strings.Contains(got, "read_thread") {
		t.Errorf("quick session =\n%s\nwant no work item tools", got)
	}
}

func TestContext_Profile(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)

	first := func() string {
		t.Helper()
		got, err := Context(ctx, st, testMemory(t, st), f.ID, "architect")
		if err != nil {
			t.Fatal(err)
		}
		return strings.SplitN(got, "\n", 4)[2]
	}
	if got := first(); got != "Description: Google login" {
		t.Errorf("second line with no profile = %q, want the description", got)
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
			ms := testMemory(t, st)
			d := remember(t, ms, workScope(f), memory.KindDecision, "user", tt.give)

			got, err := Context(ctx, st, ms, f.ID, "developer")
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

func TestDeliveryIntro(t *testing.T) {
	lead, architect := store.Role{Name: "lead", Lead: true}, store.Role{Name: "architect"}
	team := store.Team{Name: "core", Roles: []store.Role{lead, architect}}
	const common = "Your team: @lead, @architect. Use post_to_space for what the whole team needs; " +
		"only a line that starts with @name wakes that bot, an @name inside a sentence wakes " +
		"nobody. Use send_to_role for detailed coordination with exactly one bot. Routed " +
		"messages are delivered after your turn ends and the reply arrives in a later turn: " +
		"send the request, then end your turn instead of waiting for the reply. A direct reply " +
		"returns once; continuing needs another send_to_role. Publish one concise outcome with " +
		"post_to_space when a direct exchange ends with something the team or the user needs. " +
		"A message that wakes you starts with [Space N, message N] or [Space N, message N, " +
		"thread N], and may end its header with \"Asked together:\" naming everyone asked: " +
		"answer your part only. Your plain reply goes where the header says, a top-level one " +
		"starting your own thread; pass thread as the parent of post_to_space when the header " +
		"has one. Text you write after your last post_to_space is posted only when you did other " +
		"work after that post.\n"
	tests := []struct {
		name string
		team store.Team
		give store.Role
		want string
	}{
		{"solo team", store.Team{Name: "solo", Roles: []store.Role{lead}}, lead, ""},
		{"specialist", team, architect, common + "When you finish work a bot handed you, post the " +
			"result for the user in the Space. Start a line with that bot's @name only when it must " +
			"decide something, you are blocked, or it asked for a report back.\n"},
		{"lead", team, lead,
			common + "Do not summarize or repeat a specialist's post back to the user: they see it. " +
				"Hand off work whose result the user needs with a line that starts with @name, not " +
				"send_to_role, and ask for a report back only when you will act on the result: the " +
				"bot posts it to the user itself.\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deliveryIntro(tt.team, tt.give); got != tt.want {
				t.Errorf("deliveryIntro = %q, want %q", got, tt.want)
			}
		})
	}
}

func testBotSessionID(t *testing.T, st *store.Store, spaceID int64, botID *int64) *int64 {
	t.Helper()
	bs, err := st.ActiveBotSession(context.Background(), spaceID, *botID)
	if errors.Is(err, store.ErrNotFound) {
		bs, err = st.AdoptBotSession(context.Background(), spaceID, *botID)
	}
	if err != nil {
		t.Fatal(err)
	}
	return &bs.ID
}

func connectionIDs(cs []store.Connection) []int64 {
	ids := make([]int64, len(cs))
	for i, c := range cs {
		ids[i] = c.ID
	}
	return ids
}
