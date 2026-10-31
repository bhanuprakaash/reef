package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/bhanuprakaash/reef/memory"
	"github.com/bhanuprakaash/reef/store"
)

var _timestamp = regexp.MustCompile(`"(createdAt|updatedAt|retiredAt)":"[^"]*"`)

// mcpFixture has work items 1 (auth) and 2 (billing). The returned token is the
// architect in Space 1; Space 2 is a second Space of auth, Space 3 is
// billing's and Space 4 is a quick session.
// closeSpace ends the Space, as approving its wrap-up does, so its work item may have another.
func closeSpace(t *testing.T, st *store.Store, id int64) {
	t.Helper()
	if _, err := st.DB().Exec("UPDATE space SET closed_at = ? WHERE id = ?",
		time.Now().UTC().Format(time.RFC3339Nano), id); err != nil {
		t.Fatal(err)
	}
}

func testServerBot(t *testing.T, st *store.Store, name string) *int64 {
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

func testServerTeam(t *testing.T, st *store.Store, ctx context.Context, team store.Team) (store.Team, error) {
	t.Helper()
	for i, role := range team.Roles {
		if role.BotID == 0 {
			id := *testServerBot(t, st, role.Name)
			bot := store.Bot{ID: id, Name: role.Name, Backend: role.Backend, Description: role.Description, Skills: role.Skills, Model: role.Model, Provider: role.Provider, Face: role.Face, Colour: role.Colour, Tools: role.Tools, Connections: connectionIDs(role.Connections), Write: role.Write, PermissionMode: role.PermissionMode}
			if _, err := st.UpdateBot(ctx, bot); err != nil {
				return store.Team{}, err
			}
			team.Roles[i].BotID = id
		}
	}
	spec := store.TeamSpec{Name: team.Name, Responsibility: team.Responsibility}
	for _, role := range team.Roles {
		spec.Roles = append(spec.Roles, store.Member{BotID: role.BotID, Lead: role.Lead})
	}
	return st.ApplyTeam(ctx, spec)
}

func testServerCreateTeam(t *testing.T, st *store.Store, ctx context.Context, team store.Team) (store.Team, error) {
	t.Helper()
	for i, role := range team.Roles {
		if role.BotID == 0 {
			id := *testServerBot(t, st, role.Name)
			bot := store.Bot{ID: id, Name: role.Name, Backend: role.Backend, Description: role.Description, Skills: role.Skills, Model: role.Model, Provider: role.Provider, Face: role.Face, Colour: role.Colour, Tools: role.Tools, Connections: connectionIDs(role.Connections), Write: role.Write, PermissionMode: role.PermissionMode}
			if _, err := st.UpdateBot(ctx, bot); err != nil {
				return store.Team{}, err
			}
			team.Roles[i].BotID = id
		}
	}
	spec := store.TeamSpec{Name: team.Name, Responsibility: team.Responsibility}
	for _, role := range team.Roles {
		spec.Roles = append(spec.Roles, store.Member{BotID: role.BotID, Lead: role.Lead})
	}
	return st.CreateTeam(ctx, spec)
}

func testToken(t *testing.T, st *store.Store, ctx context.Context, spaceID int64, name string) (store.Token, error) {
	t.Helper()
	id, err := st.SpaceMemberBotID(ctx, spaceID, name)
	if err != nil {
		return store.Token{}, err
	}
	return st.CreateToken(ctx, spaceID, id)
}

func mcpFixture(t *testing.T) (*store.Store, store.Token) {
	t.Helper()
	ctx := context.Background()
	st, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	one, two := int64(1), int64(2)
	_, err = st.CreateProject(ctx, store.Project{Name: "reef", Path: "/tmp/reef"})
	must(err)
	_, err = st.CreateWorkItem(ctx, store.WorkItem{ProjectID: 1, Title: "Auth"})
	must(err)
	_, err = st.CreateWorkItem(ctx, store.WorkItem{ProjectID: 1, Title: "Billing"})
	must(err)
	for _, c := range []store.Space{
		{ProjectID: 1, WorkItemID: &one, Name: "architect"},
		{ProjectID: 1, WorkItemID: &one, Name: "developer"},
		{ProjectID: 1, WorkItemID: &two, Name: "architect"},
		{ProjectID: 1, Name: "quick"},
	} {
		if c.Name == "developer" {
			closeSpace(t, st, 1)
		}
		name := c.Name
		if name == "quick" {
			name = "architect"
		}
		_, err = st.CreateBotSpace(ctx, c, *testServerBot(t, st, name))
		must(err)
		if c.Name == "developer" {
			closeSpace(t, st, 2)
			must(st.ReopenSpace(ctx, 1))
		}
	}
	for _, m := range []store.Message{
		{SpaceID: 1, Author: "user", Body: "use sessions for auth"},
		{SpaceID: 2, Author: "developer", Body: "100% sure, use_sessions"},
		{SpaceID: 3, Author: "user", Body: "use sessions for billing"},
		{SpaceID: 2, ParentID: &two, Author: "architect", Body: "agreed"},
	} {
		_, err = st.CreateMessage(ctx, m)
		must(err)
	}
	_, err = st.PutBrief(ctx, store.Brief{WorkItemID: 1, Body: "Build login", Author: "user"})
	must(err)
	_, err = st.PutRoleNotes(ctx, store.RoleNotes{WorkItemID: 1, Role: "architect", Body: "keep it small"})
	must(err)
	_, err = st.CreateDecision(ctx, store.Decision{WorkItemID: 2, Body: "use stripe", Author: "user"})
	must(err)
	tok, err := testToken(t, st, ctx, 1, "architect")
	must(err)
	return st, tok
}

func openMemory(t *testing.T, st *store.Store) *memory.Store {
	t.Helper()
	ms, err := memory.Open(st.DB(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return ms
}

func callTool(t *testing.T, st *store.Store, tok store.Token) func(string, map[string]any) (string, bool) {
	t.Helper()
	return callToolResult(t, st, tok, t.TempDir())
}

// callToolResult is callTool with control over dataDir, so a test can put a
// fake browser.sock there before the client connects.
func callToolResult(
	t *testing.T, st *store.Store, tok store.Token, dataDir string,
) func(string, map[string]any) (string, bool) {
	t.Helper()
	return callToolMemory(t, st, openMemory(t, st), tok, dataDir)
}

func callToolMemory(
	t *testing.T, st *store.Store, ms *memory.Store, tok store.Token, dataDir string,
) func(string, map[string]any) (string, bool) {
	t.Helper()
	ctx := context.Background()
	ct, st2 := mcp.NewInMemoryTransports()
	ss, err := NewMCP(st, ms, tok, dataDir).Connect(ctx, st2, nil)
	if err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close(); _ = ss.Wait() })
	return func(name string, args map[string]any) (string, bool) {
		t.Helper()
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		text, ok := res.Content[0].(*mcp.TextContent)
		if !ok {
			t.Fatalf("content is %T, want text", res.Content[0])
		}
		return _timestamp.ReplaceAllString(text.Text, `"$1":"T"`), res.IsError
	}
}

// fakeBrowserSocket listens on dataDir/browser.sock and answers every request
// line with reply, as the Reef app does. It reads the request first: closing
// before the client wrote it fails the client with a broken pipe.
func fakeBrowserSocket(t *testing.T, dataDir, reply string) {
	t.Helper()
	l, err := net.Listen("unix", filepath.Join(dataDir, _browserSocketFile))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			_, _ = bufio.NewReader(conn).ReadString('\n')
			_, _ = conn.Write([]byte(reply))
			_ = conn.Close()
		}
	}()
}

func TestMCPTools(t *testing.T) {
	st, tok := mcpFixture(t)
	call := callTool(t, st, tok)
	tests := []struct {
		name    string
		tool    string
		give    map[string]any
		want    string
		wantErr bool
	}{
		{
			name: "get brief only",
			tool: "get_brief",
			want: `{"brief":"Build login"}`,
		},
		{
			name: "search skips another work item",
			tool: "search_sessions",
			give: map[string]any{"query": "SESSIONS"},
			want: `{"hits":[{"author":"developer","kind":"public","messageId":2,` +
				`"snippet":"100% sure, use_sessions","spaceId":2},{"author":"user",` +
				`"kind":"public","messageId":1,"snippet":"use sessions for auth","spaceId":1}]}`,
		},
		{
			name: "search treats underscore as text",
			tool: "search_sessions",
			give: map[string]any{"query": "use_s"},
			want: `{"hits":[{"author":"developer","kind":"public","messageId":2,` +
				`"snippet":"100% sure, use_sessions","spaceId":2}]}`,
		},
		{
			name: "search limit",
			tool: "search_sessions",
			give: map[string]any{"query": "sessions", "limit": 1},
			want: `{"hits":[{"author":"developer","kind":"public","messageId":2,` +
				`"snippet":"100% sure, use_sessions","spaceId":2}]}`,
		},
		{
			name: "read thread after an id",
			tool: "read_thread",
			give: map[string]any{"spaceId": 2, "after": 2},
			want: `{"messages":[{"author":"architect","body":"agreed",` +
				`"createdAt":"T","id":4,"parentId":2,"spaceId":2}]}`,
		},
		{
			name: "read thread of a root",
			tool: "read_thread",
			give: map[string]any{"spaceId": 2, "root": 2},
			want: `{"messages":[{"author":"developer","body":"100% sure, use_sessions",` +
				`"createdAt":"T","id":2,"parentId":null,"spaceId":2},` +
				`{"author":"architect","body":"agreed","createdAt":"T","id":4,` +
				`"parentId":2,"spaceId":2}]}`,
		},
		{
			name:    "read thread of a reply",
			tool:    "read_thread",
			give:    map[string]any{"spaceId": 2, "root": 4},
			want:    "thread 4: not found",
			wantErr: true,
		},
		{
			name: "post to Space as the token's role",
			tool: "post_to_space",
			give: map[string]any{"body": "@developer take it"},
			want: `{"author":"architect","body":"@developer take it",` +
				`"botId":2,"createdAt":"T","id":5,"parentId":null,"spaceId":1}`,
		},
		{
			name: "post to Space in a thread",
			tool: "post_to_space",
			give: map[string]any{"body": "done", "parent": 1},
			want: `{"author":"architect","body":"done","botId":2,"createdAt":"T","id":6,` +
				`"parentId":1,"spaceId":1}`,
		},
		{
			name:    "post to Space under a reply",
			tool:    "post_to_space",
			give:    map[string]any{"body": "x", "parent": 6},
			want:    "invalid: parent 6 is not a top-level message of Space 1",
			wantErr: true,
		},
		{
			name:    "read another work item's thread",
			tool:    "read_thread",
			give:    map[string]any{"spaceId": 3},
			want:    "space 3: not found",
			wantErr: true,
		},
		{
			name:    "navigate with the app closed",
			tool:    "browser_navigate",
			give:    map[string]any{"url": "http://localhost:8080"},
			want:    "browser unavailable: the Reef app is not open",
			wantErr: true,
		},
		{
			name:    "screenshot with the app closed",
			tool:    "browser_screenshot",
			want:    "browser unavailable: the Reef app is not open",
			wantErr: true,
		},
		{
			name:    "snapshot with the app closed",
			tool:    "browser_snapshot",
			want:    "browser unavailable: the Reef app is not open",
			wantErr: true,
		},
		{
			name:    "click with the app closed",
			tool:    "browser_click",
			give:    map[string]any{"ref": "e1"},
			want:    "browser unavailable: the Reef app is not open",
			wantErr: true,
		},
		{
			name:    "type with the app closed",
			tool:    "browser_type",
			give:    map[string]any{"ref": "e1", "text": "hi"},
			want:    "browser unavailable: the Reef app is not open",
			wantErr: true,
		},
		{
			name:    "console with the app closed",
			tool:    "browser_console",
			want:    "browser unavailable: the Reef app is not open",
			wantErr: true,
		},
		{
			name:    "network with the app closed",
			tool:    "browser_network",
			want:    "browser unavailable: the Reef app is not open",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, isErr := call(tt.tool, tt.give)
			if got != tt.want || isErr != tt.wantErr {
				t.Errorf("%s = %q (error %v), want %q (error %v)",
					tt.tool, got, isErr, tt.want, tt.wantErr)
			}
		})
	}
}

func TestMCPProposeWrap(t *testing.T) {
	st, tok := mcpFixture(t)
	call := callTool(t, st, tok)

	got, isErr := call("propose_wrap", map[string]any{
		"brief": "v2",
		"items": []any{
			map[string]any{"kind": "decision", "body": "use sessions", "why": "simple"},
			map[string]any{"kind": "lesson", "body": "reply short", "why": "", "scope": "bot"},
		},
	})
	if want := `{"id":1,"message":"proposal saved; the user will approve it"}`; got != want || isErr {
		t.Errorf("propose_wrap = %q (error %v), want %q", got, isErr, want)
	}
	p, err := st.PendingProposal(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	want := store.Proposal{
		ID: 1, SpaceID: 1, BotID: tok.BotID, Role: "architect", Kind: store.ProposalWrap, Brief: "v2",
		Items: []store.ProposedItem{
			{Kind: "decision", Body: "use sessions", Why: "simple", Scope: "work_item"},
			{Kind: "lesson", Body: "reply short", Scope: "bot"},
		},
		Retire: []int64{},
		Status: store.ProposalPending,
	}
	p.CreatedAt = time.Time{}
	if !reflect.DeepEqual(p, want) {
		t.Errorf("proposal = %+v, want %+v", p, want)
	}

	got, isErr = call("propose_wrap", map[string]any{"brief": ""})
	if want := "invalid: proposal brief is required"; got != want || !isErr {
		t.Errorf("propose_wrap with empty brief = %q (error %v), want %q", got, isErr, want)
	}
}

func TestMCPRemember(t *testing.T) {
	st, architect := mcpFixture(t)
	developer, err := testToken(t, st, context.Background(), 2, "developer")
	if err != nil {
		t.Fatal(err)
	}
	dev := callTool(t, st, developer)
	arch := callTool(t, st, architect)
	item := func(id int, kind, scope, role, status string) string {
		space := map[string]string{"developer": "2", "architect": "1"}[role]
		botID := map[string]string{"developer": "3", "architect": "2"}[role]
		return `{"item":{"author":"` + role + `","body":"item ` + strconv.Itoa(id) + `","botId":` + botID + `,"createdAt":"T","id":` + strconv.Itoa(id) +
			`,"kind":"` + kind + `","links":[{"rel":"source","to":"space:` + space + `"}],"replacedBy":null,"retiredAt":null,"role":"` + role +
			`","scope":"` + scope + `","status":"` + status + `","why":""}}`
	}
	tests := []struct {
		name    string
		call    func(string, map[string]any) (string, bool)
		give    map[string]any
		want    string
		wantErr bool
	}{
		{
			name: "a decision is accepted on the work item",
			call: dev,
			give: map[string]any{"kind": "decision", "body": "item 1"},
			want: item(1, "decision", "work_item:1", "developer", "accepted"),
		},
		{
			name: "scope bot is the role",
			call: dev,
			give: map[string]any{"kind": "lesson", "body": "item 2", "scope": "bot"},
			want: item(2, "lesson", "bot:3", "developer", "accepted"),
		},
		{
			name: "a developer's convention is a candidate",
			call: dev,
			give: map[string]any{"kind": "convention", "body": "item 3", "scope": "project"},
			want: item(3, "convention", "project:1", "developer", "candidate"),
		},
		{
			name: "an architect's convention is a candidate",
			call: arch,
			give: map[string]any{"kind": "convention", "body": "item 4"},
			want: item(4, "convention", "work_item:1", "architect", "candidate"),
		},
		{
			name:    "scope team is refused",
			call:    dev,
			give:    map[string]any{"kind": "lesson", "body": "item 6", "scope": "team"},
			want:    `invalid: scope "team" is not work_item, space, conversation, project or bot`,
			wantErr: true,
		},
		{
			name: "nothing was stored for the refused scope",
			call: dev,
			give: map[string]any{"kind": "fact", "body": "item 5"},
			want: item(5, "fact", "work_item:1", "developer", "accepted"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, isErr := tt.call("remember", tt.give)
			if got != tt.want || isErr != tt.wantErr {
				t.Errorf("remember = %q (error %v), want %q (error %v)", got, isErr, tt.want, tt.wantErr)
			}
		})
	}
}

func TestMCPRememberDefaultsToTheSpacesOwnShelf(t *testing.T) {
	ctx := context.Background()
	st, tok := mcpFixture(t)
	ms := openMemory(t, st)
	callIn := func(spaceID int64, role string) func(string, map[string]any) (string, bool) {
		tok, err := testToken(t, st, ctx, spaceID, role)
		if err != nil {
			t.Fatal(err)
		}
		return callToolMemory(t, st, ms, tok, t.TempDir())
	}
	chat := func() int64 {
		c, err := st.CreateBotSpace(ctx,
			store.Space{ProjectID: 1, Name: "chat", Kind: store.KindConversation}, 1)
		if err != nil {
			t.Fatal(err)
		}
		return c.ID
	}
	first, second := chat(), chat()

	free := callIn(4, tok.Role)
	got, isErr := free("remember", map[string]any{"kind": "fact", "body": "Lunch is at noon"})
	if !strings.Contains(got, `"scope":"space:4"`) || isErr {
		t.Errorf("remember in a free Space = %q (error %v), want scope space:4", got, isErr)
	}
	got, isErr = free("remember", map[string]any{"kind": "fact", "body": "x", "scope": "work_item"})
	if want := "this Space has no work item"; got != want || !isErr {
		t.Errorf("work_item in a free Space = %q (error %v), want %q", got, isErr, want)
	}
	got, isErr = callIn(1, "architect")("remember", map[string]any{"kind": "fact", "body": "x", "scope": "space"})
	if want := `invalid: you have no "space" scope here`; got != want || !isErr {
		t.Errorf("space on a work item Space = %q (error %v), want %q", got, isErr, want)
	}

	got, isErr = callIn(first, "lead")("remember", map[string]any{"kind": "fact", "body": "Prefers tabs"})
	if !strings.Contains(got, `"scope":"conversation:`+strconv.FormatInt(first, 10)+`"`) || isErr {
		t.Errorf("remember in a conversation = %q (error %v), want its conversation scope", got, isErr)
	}
	for _, give := range []map[string]any{
		{"query": "tabs"}, {"query": "tabs", "scope": "conversation"},
	} {
		got, isErr = callIn(second, "lead")("recall", give)
		if !strings.Contains(got, `"body":"Prefers tabs"`) || isErr {
			t.Errorf("recall %v in the next conversation = %q (error %v), want the item", give, got, isErr)
		}
	}
	if got, _ = free("recall", map[string]any{"query": "tabs"}); strings.Contains(got, "Prefers tabs") {
		t.Errorf("recall in a Space of another role = %q, want no conversation item", got)
	}
}

func TestMCPRememberReportsLookalikesAndEmbedErrors(t *testing.T) {
	st, tok := mcpFixture(t)
	ms, err := memory.Open(st.DB(), failEmbedder{})
	if err != nil {
		t.Fatal(err)
	}
	call := callToolMemory(t, st, ms, tok, t.TempDir())
	call("remember", map[string]any{"kind": "decision", "body": "use sessions for login"})

	got, isErr := call("remember", map[string]any{"kind": "decision", "body": "use sessions for login."})
	want := `{"embedError":"provider down","item":{"author":"architect","body":"use sessions for login.",` +
		`"botId":2,"createdAt":"T","id":2,"kind":"decision","links":[{"rel":"source","to":"space:1"}],` +
		`"replacedBy":null,"retiredAt":null,"role":"architect","scope":"work_item:1","status":"accepted","why":""},` +
		`"similar":[{"body":"use sessions for login","id":1}]}`
	if got != want || isErr {
		t.Errorf("remember = %q (error %v), want %q", got, isErr, want)
	}
}

func TestMCPMemoryScopes(t *testing.T) {
	st, auth := mcpFixture(t)
	billing, err := testToken(t, st, context.Background(), 3, "architect")
	if err != nil {
		t.Fatal(err)
	}
	authCall := callTool(t, st, auth)
	billingCall := callTool(t, st, billing)
	authCall("remember", map[string]any{"kind": "decision", "body": "use sqlite for auth"})
	billingCall("remember", map[string]any{"kind": "decision", "body": "use sqlite for billing"})
	billingCall("remember", map[string]any{"kind": "fact", "body": "billing runs nightly", "scope": "project"})

	tests := []struct {
		name    string
		tool    string
		give    map[string]any
		want    string
		wantErr bool
	}{
		{
			name: "recall finds its own work item and the project, never another work item",
			tool: "recall",
			give: map[string]any{"query": "sqlite"},
			want: `{"hits":[{"body":"use sqlite for auth","id":1,"kind":"decision","scope":"work_item:1",` +
				`"status":"accepted","via":"direct"}]}`,
		},
		{
			name: "recall in the project scope",
			tool: "recall",
			give: map[string]any{"query": "billing", "scope": "project"},
			want: `{"hits":[{"body":"billing runs nightly","id":3,"kind":"fact","scope":"project:1",` +
				`"status":"accepted","via":"direct"}]}`,
		},
		{
			name:    "read another work item's item",
			tool:    "read_memory",
			give:    map[string]any{"id": 2},
			want:    "memory 2: not found",
			wantErr: true,
		},
		{
			name:    "read an unknown id",
			tool:    "read_memory",
			give:    map[string]any{"id": 99},
			want:    "memory 99: not found",
			wantErr: true,
		},
		{
			name:    "replace another work item's item",
			tool:    "remember",
			give:    map[string]any{"kind": "decision", "body": "x", "replaces": []int{2}},
			want:    "memory 2: not found",
			wantErr: true,
		},
		{
			name:    "replace an item of another shelf of this space",
			tool:    "remember",
			give:    map[string]any{"kind": "decision", "body": "x", "replaces": []int{3}},
			want:    "replace 3: invalid: memory 3 is in scope project:1, not work_item:1",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, isErr := authCall(tt.tool, tt.give)
			if got != tt.want || isErr != tt.wantErr {
				t.Errorf("%s = %q (error %v), want %q (error %v)", tt.tool, got, isErr, tt.want, tt.wantErr)
			}
		})
	}
	got, isErr := authCall("read_memory", map[string]any{"id": 3})
	want := `{"author":"architect","body":"billing runs nightly","botId":2,"createdAt":"T","id":3,"kind":"fact",` +
		`"links":[{"rel":"source","to":"space:3"}],"replacedBy":null,"retiredAt":null,"role":"architect","scope":"project:1",` +
		`"status":"accepted","why":""}`
	if got != want || isErr {
		t.Errorf("read_memory of a project item = %q (error %v), want %q", got, isErr, want)
	}
}

func TestMCPRetireOwnMemory(t *testing.T) {
	st, tok := mcpFixture(t)
	other, err := testToken(t, st, context.Background(), 3, "architect")
	if err != nil {
		t.Fatal(err)
	}
	call := callTool(t, st, tok)
	call("remember", map[string]any{"kind": "lesson", "body": "run tests first", "scope": "bot"})
	callTool(t, st, other)("remember", map[string]any{"kind": "lesson", "body": "tests first, always", "scope": "bot"})
	call("remember", map[string]any{"kind": "lesson", "body": "old tip", "scope": "bot"})
	call("remember", map[string]any{"kind": "fact", "body": "shared fact", "scope": "project"})

	call("remember", map[string]any{
		"kind": "lesson", "body": "run tests first", "scope": "bot", "replaces": []int{1, 2},
	})
	got, isErr := call("read_memory", map[string]any{"id": 5})
	want := `{"author":"architect","body":"run tests first","botId":2,"createdAt":"T","id":5,"kind":"lesson",` +
		`"links":[{"rel":"source","to":"space:1"},{"rel":"replaces","to":"memory:1"},` +
		`{"rel":"replaces","to":"memory:2"},{"rel":"source","to":"space:3"}],` +
		`"replacedBy":null,"retiredAt":null,"role":"architect","scope":"bot:2","status":"accepted","why":""}`
	if got != want || isErr {
		t.Errorf("consolidated item = %q (error %v), want %q", got, isErr, want)
	}
	got, isErr = call("read_memory", map[string]any{"id": 1})
	if !strings.Contains(got, `"replacedBy":5`) || !strings.Contains(got, `"status":"retired"`) || isErr {
		t.Errorf("replaced item = %q (error %v), want retired and replaced by 5", got, isErr)
	}
	got, isErr = call("retire_memory", map[string]any{"id": 3})
	want = `{"author":"architect","body":"old tip","botId":2,"createdAt":"T","id":3,"kind":"lesson",` +
		`"links":[{"rel":"source","to":"space:1"}],"replacedBy":null,"retiredAt":"T","role":"architect",` +
		`"scope":"bot:2","status":"retired","why":""}`
	if got != want || isErr {
		t.Errorf("retire_memory of a bot item = %q (error %v), want %q", got, isErr, want)
	}
	got, isErr = call("retire_memory", map[string]any{"id": 4})
	want = "memory 4: not found"
	if got != want || !isErr {
		t.Errorf("retire_memory of a project item = %q (error %v), want %q", got, isErr, want)
	}
}

func TestMCPReviewTokenOnlyTouchesOwnMemory(t *testing.T) {
	ctx := context.Background()
	st, tok := mcpFixture(t)
	botID, err := st.SpaceMemberBotID(ctx, tok.SpaceID, "architect")
	if err != nil {
		t.Fatal(err)
	}
	review, err := st.CreateReviewToken(ctx, tok.SpaceID, botID)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := st.Token(ctx, review.Token); err != nil || !got.Review {
		t.Fatalf("stored token = %+v, %v, want a review token", got, err)
	}
	ct, st2 := mcp.NewInMemoryTransports()
	ss, err := NewMCP(st, openMemory(t, st), review, t.TempDir()).Connect(ctx, st2, nil)
	if err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close(); _ = ss.Wait() })
	call := func(name string, args map[string]any) (string, error) {
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			return "", err
		}
		text, _ := res.Content[0].(*mcp.TextContent)
		if res.IsError {
			return text.Text, errors.New(text.Text)
		}
		return text.Text, nil
	}

	var names []string
	for tool, err := range cs.Tools(ctx, nil) {
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, tool.Name)
	}
	slices.Sort(names)
	if want := []string{"read_memory", "recall", "remember", "retire_memory"}; !slices.Equal(names, want) {
		t.Errorf("tools = %v, want %v", names, want)
	}
	if _, err := call("post_to_space", map[string]any{"body": "hi"}); err == nil {
		t.Error("post_to_space was allowed to a review")
	}
	if _, err := call("remember", map[string]any{"kind": "fact", "body": "shared", "scope": "project"}); err == nil {
		t.Error("remember with scope project was allowed to a review")
	}
	if _, err := call("remember", map[string]any{"kind": "fact", "body": "shared"}); err == nil {
		t.Error("remember with no scope was allowed to a review")
	}
	if _, err := call("remember", map[string]any{"kind": "lesson", "body": "run tests first", "scope": "bot"}); err != nil {
		t.Errorf("remember with scope bot: %v", err)
	}
	got, err := call("retire_memory", map[string]any{"id": 1})
	if err != nil || !strings.Contains(got, `"status":"retired"`) {
		t.Errorf("retire_memory = %q, %v, want the item retired", got, err)
	}
}

func TestMCPProposeBrief(t *testing.T) {
	st, tok := mcpFixture(t)
	call := callTool(t, st, tok)

	got, isErr := call("propose_brief", map[string]any{"brief": "plan: ship login first"})
	if want := `{"id":1,"message":"proposal saved; the user will approve it"}`; got != want || isErr {
		t.Errorf("propose_brief = %q (error %v), want %q", got, isErr, want)
	}
	p, err := st.PendingProposal(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	want := store.Proposal{
		ID: 1, SpaceID: 1, BotID: tok.BotID, Role: "architect", Kind: store.ProposalBrief,
		Brief: "plan: ship login first", Items: []store.ProposedItem{},
		Retire: []int64{}, Status: store.ProposalPending,
	}
	p.CreatedAt = time.Time{}
	if !reflect.DeepEqual(p, want) {
		t.Errorf("proposal = %+v, want %+v", p, want)
	}

	got, isErr = call("propose_brief", map[string]any{"brief": ""})
	if want := "invalid: proposal brief is required"; got != want || !isErr {
		t.Errorf("propose_brief with empty brief = %q (error %v), want %q", got, isErr, want)
	}
}

func TestMCPProposeWrapRejectsBadMemoryIDs(t *testing.T) {
	st, tok := mcpFixture(t)
	billing, err := testToken(t, st, context.Background(), 3, "architect")
	if err != nil {
		t.Fatal(err)
	}
	callTool(t, st, billing)("remember", map[string]any{"kind": "fact", "body": "billing item"})
	call := callTool(t, st, tok)
	call("remember", map[string]any{"kind": "fact", "body": "live item", "scope": "bot"})
	call("remember", map[string]any{"kind": "fact", "body": "stale item", "replaces": []any{2}, "scope": "bot"})
	call("remember", map[string]any{"kind": "fact", "body": "project item", "scope": "project"})

	const (
		notLive       = "invalid: memory %d is not a live item in work_item:1, project:1, bot:2"
		notOnTheShelf = "invalid: memory %d is not a live item in work_item:1"
	)
	tests := []struct {
		name string
		give map[string]any
		want string
	}{
		{"retire unknown id", map[string]any{"retire": []any{9}}, fmt.Sprintf(notLive, 9)},
		{"retire another work item's item", map[string]any{"retire": []any{1}}, fmt.Sprintf(notLive, 1)},
		{"retire a retired item", map[string]any{"retire": []any{2}}, fmt.Sprintf(notLive, 2)},
		{
			"replace unknown id",
			map[string]any{"items": []any{map[string]any{"kind": "fact", "body": "x", "replaces": []any{9}}}},
			fmt.Sprintf(notOnTheShelf, 9),
		},
		{
			"replace an item of the project shelf from a work item item",
			map[string]any{"items": []any{map[string]any{"kind": "fact", "body": "x", "replaces": []any{4}}}},
			fmt.Sprintf(notOnTheShelf, 4),
		},
		{
			"unknown kind",
			map[string]any{"items": []any{map[string]any{"kind": "idea", "body": "x"}}},
			`invalid: unknown item kind "idea"`,
		},
		{
			"unknown scope",
			map[string]any{"items": []any{map[string]any{"kind": "fact", "body": "x", "scope": "space"}}},
			`invalid: scope "space" is not one of work_item, project, bot`,
		},
		{
			"team scope on a solo space",
			map[string]any{"items": []any{map[string]any{"kind": "fact", "body": "x", "scope": "team"}}},
			`invalid: scope "team" is not one of work_item, project, bot`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.give["brief"] = "v2"
			got, isErr := call("propose_wrap", tt.give)
			if got != tt.want || !isErr {
				t.Errorf("propose_wrap = %q (error %v), want %q", got, isErr, tt.want)
			}
		})
	}
	if _, err := st.PendingProposal(context.Background(), 1); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("PendingProposal = %v, want ErrNotFound: nothing saved", err)
	}
}

// leadAndDeveloper adds team core (lead and developer) and a Space of work item 1 for it, and
// returns the tokens of the two.
func leadAndDeveloper(t *testing.T, st *store.Store) (lead, developer store.Token) {
	t.Helper()
	ctx := context.Background()
	if _, err := testServerCreateTeam(t, st, ctx, store.Team{Name: "core", Roles: []store.Role{
		{Name: "lead", Lead: true, Backend: "claude"},
		{Name: "developer", Backend: "claude"},
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
	if lead, err = testToken(t, st, ctx, ch.ID, "lead"); err != nil {
		t.Fatal(err)
	}
	if developer, err = testToken(t, st, ctx, ch.ID, "developer"); err != nil {
		t.Fatal(err)
	}
	return lead, developer
}

func TestMCPSubmitForReview(t *testing.T) {
	ctx := context.Background()
	st, _ := mcpFixture(t)
	lead, developer := leadAndDeveloper(t, st)
	if _, err := st.UpdateWorkItem(ctx, store.WorkItem{ID: 1, Title: "Auth", AcceptanceLines: []store.AcceptanceLine{
		{Text: "wrong password is refused"}, {Text: "session lasts a day"}, {Text: "logout clears it"},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB().Exec("UPDATE work_item SET status = 'in_progress' WHERE id = 1"); err != nil {
		t.Fatal(err)
	}
	dataDir := t.TempDir()
	shot := filepath.Join(dataDir, "login.png")
	if err := os.WriteFile(shot, []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	leadCall := callToolResult(t, st, lead, dataDir)
	devCall := callToolResult(t, st, developer, dataDir)
	if _, isErr := leadCall("create_sub_item", map[string]any{"title": "Schema"}); isErr {
		t.Fatal("create_sub_item failed")
	}
	args := map[string]any{
		"lines": []map[string]any{
			{"position": 1, "checked": true, "note": "401 on a bad password"},
			{"position": 2, "checked": true},
		},
		"commands":    []map[string]any{{"command": "go test ./...", "output": "ok  reef/server 1.2s"}},
		"screenshots": []string{shot},
	}

	got, isErr := devCall("submit_for_review", args)
	if want := "invalid: only the lead submits a work item for review"; got != want || !isErr {
		t.Errorf("developer submit = %q (error %v), want %q", got, isErr, want)
	}
	got, isErr = leadCall("submit_for_review", args)
	if want := `invalid: work item 1 cannot go to review while sub-item 3 "Schema" is backlog`; got != want || !isErr {
		t.Errorf("submit with an open sub-item = %q (error %v), want %q", got, isErr, want)
	}
	if _, isErr := leadCall("set_sub_item_status", map[string]any{"id": 3, "status": "done"}); isErr {
		t.Fatal("set_sub_item_status failed")
	}
	args["screenshots"] = []string{filepath.Join(dataDir, "..", "login.png")}
	got, isErr = leadCall("submit_for_review", args)
	if want := `invalid: screenshot "` + args["screenshots"].([]string)[0] + `" is not under the data folder ` + dataDir; got != want || !isErr {
		t.Errorf("submit with a screenshot outside the data folder = %q (error %v), want %q", got, isErr, want)
	}
	args["screenshots"] = []string{shot}
	got, isErr = leadCall("submit_for_review", args)
	if !strings.Contains(got, `"status":"review"`) || isErr {
		t.Fatalf("lead submit = %q (error %v), want review", got, isErr)
	}
	p, err := st.Proof(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	p.SubmittedAt = time.Time{}
	b, _ := json.Marshal(p)
	want := `{"spaceId":5,"acceptanceLines":[` +
		`{"position":1,"text":"wrong password is refused","checked":true,"note":"401 on a bad password"},` +
		`{"position":2,"text":"session lasts a day","checked":true,"note":""},` +
		`{"position":3,"text":"logout clears it","checked":false,"note":""}],` +
		`"commands":[{"command":"go test ./...","output":"ok  reef/server 1.2s"}],` +
		`"screenshots":["` + shot + `"],"submittedAt":"0001-01-01T00:00:00Z"}`
	if string(b) != want {
		t.Errorf("proof = %s, want %s", b, want)
	}
}

func TestMCPSubItems(t *testing.T) {
	st, _ := mcpFixture(t)
	lead, developer := leadAndDeveloper(t, st)
	leadCall, devCall := callTool(t, st, lead), callTool(t, st, developer)

	for _, title := range []string{"Schema", "Handlers"} {
		if got, isErr := leadCall("create_sub_item", map[string]any{"title": title}); isErr {
			t.Fatalf("create_sub_item %q = %q", title, got)
		}
	}
	got, isErr := leadCall("list_sub_items", nil)
	want := `{"subItems":[{"id":3,"status":"backlog","title":"Schema","type":"task"},` +
		`{"id":4,"status":"backlog","title":"Handlers","type":"task"}]}`
	if got != want || isErr {
		t.Errorf("list_sub_items = %q (error %v), want %q", got, isErr, want)
	}

	got, isErr = devCall("set_sub_item_status", map[string]any{"id": 3, "status": "done"})
	if want := "invalid: only the lead sets a sub-item's status"; got != want || !isErr {
		t.Errorf("developer set_sub_item_status = %q (error %v), want %q", got, isErr, want)
	}
	got, isErr = leadCall("set_sub_item_status", map[string]any{"id": 3, "status": "done"})
	if !strings.Contains(got, `"status":"done"`) || isErr {
		t.Errorf("lead set_sub_item_status = %q (error %v), want status done", got, isErr)
	}
	got, isErr = leadCall("set_sub_item_status", map[string]any{"id": 2, "status": "done"})
	if want := "sub-item 2: not found"; got != want || !isErr {
		t.Errorf("set_sub_item_status on another work item = %q (error %v), want %q", got, isErr, want)
	}
	got, isErr = devCall("read_sub_item", map[string]any{"id": 4})
	if !strings.Contains(got, `"title":"Handlers"`) || isErr {
		t.Errorf("read_sub_item = %q (error %v), want the Handlers sub-item", got, isErr)
	}
}

func TestMCPProposeWorkItem(t *testing.T) {
	ctx := context.Background()
	st, _ := mcpFixture(t)
	if _, err := testServerCreateTeam(t, st, ctx, store.Team{
		Name: "infra", Responsibility: "Deploys, CI pipelines and servers.",
		Roles: []store.Role{{Name: "lead", Lead: true, Backend: "claude"}},
	}); err != nil {
		t.Fatal(err)
	}
	chat, err := st.CreateBotSpace(ctx,
		store.Space{ProjectID: 1, Name: "chat", Kind: store.KindConversation}, 1)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := testToken(t, st, ctx, chat.ID, "lead")
	if err != nil {
		t.Fatal(err)
	}
	call := callTool(t, st, tok)

	got, isErr := call("propose_work_item", map[string]any{
		"type": "incident", "title": "Production deploy fails",
		"description": "The deploy job exits 1 after the build step.",
		"acceptance":  []string{"Deploy passes on main"},
	})
	if isErr {
		t.Fatalf("propose_work_item = %q, want a work item", got)
	}
	for _, want := range []string{
		`"id":3`, `"type":"incident"`, `"status":"backlog"`, `"proposedBy":"lead"`,
		`"suggestedTeamId":2`, `"teamId":null`, `"queuePosition":null`,
		`"acceptanceLines":[{"checked":false,"note":"","position":1,"text":"Deploy passes on main"}]`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("propose_work_item = %q, want it to contain %q", got, want)
		}
	}

	got, isErr = call("propose_work_item", map[string]any{
		"type": "task", "title": "Rename the export", "description": "Call it archive.",
	})
	if !strings.Contains(got, `"suggestedTeamId":1`) || isErr {
		t.Errorf("a proposal no team claims = %q (error %v), want the solo team 1", got, isErr)
	}

	items, err := st.Inbox(ctx)
	if err != nil || len(items) != 2 || items[1].Kind != "proposed" ||
		items[1].Text != `lead proposed work item 3: Production deploy fails. `+
			`It is in the Backlog until you queue it. Suggested team: infra.` {
		t.Errorf("inbox = %+v, %v, want the proposed FYI of item 3 second", items, err)
	}
	if msgs, err := st.Messages(ctx, chat.ID, 0); err != nil || len(msgs) != 0 {
		t.Errorf("messages = %+v, %v, want none", msgs, err)
	}

	got, isErr = call("propose_work_item", map[string]any{"type": "chore", "title": "x", "description": "y"})
	if want := `invalid: unknown type "chore"`; got != want || !isErr {
		t.Errorf("unknown type = %q (error %v), want %q", got, isErr, want)
	}
}

func TestMCPProposeWorkItemNeedsAProject(t *testing.T) {
	ctx := context.Background()
	st, _ := mcpFixture(t)
	chat, err := st.CreateBotSpace(ctx, store.Space{Name: "chat", Kind: store.KindConversation}, 1)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := testToken(t, st, ctx, chat.ID, "lead")
	if err != nil {
		t.Fatal(err)
	}

	got, isErr := callTool(t, st, tok)("propose_work_item", map[string]any{
		"type": "task", "title": "Rename the export", "description": "Call it archive.",
	})
	if want := "this Space has no project, so it cannot propose a work item"; got != want || !isErr {
		t.Errorf("propose_work_item = %q (error %v), want %q", got, isErr, want)
	}
}

func TestMCPProposeBriefRefusedWhileWrapPending(t *testing.T) {
	st, tok := mcpFixture(t)
	call := callTool(t, st, tok)
	if got, isErr := call("propose_wrap", map[string]any{"brief": "final"}); isErr {
		t.Fatalf("propose_wrap = %q", got)
	}

	got, isErr := call("propose_brief", map[string]any{"brief": "plan"})
	if want := "invalid: Space 1 has a wrap-up waiting for approval"; got != want || !isErr {
		t.Errorf("propose_brief = %q (error %v), want %q", got, isErr, want)
	}
	p, err := st.PendingProposal(context.Background(), 1)
	if err != nil || p.Kind != store.ProposalWrap || p.Brief != "final" {
		t.Errorf("pending = %+v, %v, want the wrap with brief final", p, err)
	}
}

func TestMCPDirectMessages(t *testing.T) {
	ctx := context.Background()
	st, _ := mcpFixture(t)
	_, err := testServerCreateTeam(t, st, ctx, store.Team{Name: "core", Roles: []store.Role{
		{Name: "lead", Lead: true, Backend: "claude"},
		{Name: "developer", Backend: "codex"},
		{Name: "tester", Backend: "claude"},
	}})
	if err != nil {
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
	lead, err := testToken(t, st, ctx, ch.ID, "lead")
	if err != nil {
		t.Fatal(err)
	}
	developer, err := testToken(t, st, ctx, ch.ID, "developer")
	if err != nil {
		t.Fatal(err)
	}
	callLead := callTool(t, st, lead)
	got, isErr := callLead("send_to_role", map[string]any{"to": "developer", "body": "build it"})
	if want := "invalid: lead has no running turn to send from"; got != want || !isErr {
		t.Fatalf("send_to_role without a turn = %q (error %v), want %q", got, isErr, want)
	}
	if _, err := st.CreateTurn(ctx, store.Turn{
		SpaceID: ch.ID, Role: "lead", BotID: testServerBot(t, st, "lead"), Backend: "claude", WakeText: "go",
	}); err != nil {
		t.Fatal(err)
	}
	got, isErr = callLead("send_to_role", map[string]any{"to": "developer", "body": "build it"})
	if want := `{"body":"build it","chainId":1,"createdAt":"T","from":"lead",` +
		`"fromBotId":1,"id":1,"replyToId":null,"sourceMessageId":null,"spaceId":5,"state":"waiting","status":"sent","to":"developer","toBotId":3}`; got != want || isErr {
		t.Fatalf("send_to_role = %q (error %v), want %q", got, isErr, want)
	}
	if msgs, err := st.Messages(ctx, ch.ID, 0); err != nil || len(msgs) != 0 {
		t.Errorf("public messages = %+v, %v, want none", msgs, err)
	}

	callDeveloper := callTool(t, st, developer)
	got, isErr = callDeveloper("read_direct_messages", nil)
	if want := `{"messages":[{"body":"build it","chainId":1,"createdAt":"T",` +
		`"from":"lead","fromBotId":1,"id":1,"kind":"request","replyToId":null,"sourceMessageId":null,"spaceId":5,"state":"waiting","status":"sent","to":"developer","toBotId":3}]}`; got != want || isErr {
		t.Errorf("developer read = %q (error %v), want %q", got, isErr, want)
	}
	tester, err := testToken(t, st, ctx, ch.ID, "tester")
	if err != nil {
		t.Fatal(err)
	}
	got, isErr = callTool(t, st, tester)("read_direct_messages", nil)
	if want := `{"messages":[]}`; got != want || isErr {
		t.Errorf("tester read = %q (error %v), want %q", got, isErr, want)
	}

	bad := []struct {
		name string
		give map[string]any
		want bool
	}{
		{"empty", map[string]any{"to": "developer", "body": "  "}, true},
		{"self", map[string]any{"to": "lead", "body": "x"}, true},
		{"unknown", map[string]any{"to": "ghost", "body": "x"}, true},
		{"several", map[string]any{"to": []any{"developer", "tester"}, "body": "x"}, true},
	}
	for _, tt := range bad {
		t.Run(tt.name, func(t *testing.T) {
			if _, got := callLead("send_to_role", tt.give); got != tt.want {
				t.Errorf("send_to_role error = %v, want %v", got, tt.want)
			}
		})
	}
	if direct, err := st.DirectMessages(ctx, ch.ID, 0); err != nil || len(direct) != 1 {
		t.Errorf("direct messages after invalid sends = %+v, %v, want original only", direct, err)
	}

	if _, err := st.CreateDirectMessage(ctx, store.DirectMessage{
		SpaceID: ch.ID, From: "developer", To: "tester", FromBotID: testServerBot(t, st, "developer"), ToBotID: testServerBot(t, st, "tester"), Body: "keep it quiet",
	}); err != nil {
		t.Fatal(err)
	}
	if got, _ := callLead("read_direct_messages", nil); strings.Contains(got, "keep it quiet") {
		t.Errorf("lead read = %q, want no exchange of the developer and tester", got)
	}
	if err := st.SetActiveTurn(ctx, ch.ID, store.ActiveTurnWrap); err != nil {
		t.Fatal(err)
	}
	if got, _ := callLead("read_direct_messages", nil); !strings.Contains(got, "keep it quiet") {
		t.Errorf("lead read in a wrap-up = %q, want every exchange", got)
	}
}

func TestMCPQuickSession(t *testing.T) {
	st, _ := mcpFixture(t)
	tok, err := testToken(t, st, context.Background(), 4, "architect")
	if err != nil {
		t.Fatal(err)
	}
	call := callTool(t, st, tok)
	tests := []struct {
		tool string
		give map[string]any
	}{
		{"get_brief", nil},
		{"search_sessions", map[string]any{"query": "q"}},
		{"read_thread", map[string]any{"spaceId": 1}},
		{"propose_wrap", map[string]any{"brief": "b"}},
		{"propose_brief", map[string]any{"brief": "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			got, isErr := call(tt.tool, tt.give)
			if got != "this Space has no work item" || !isErr {
				t.Errorf("%s = %q (error %v), want the no-work-item error", tt.tool, got, isErr)
			}
		})
	}
}

func TestMCPPostRefusedOnClosedAndMovedSpaces(t *testing.T) {
	st, _ := mcpFixture(t)
	ctx := context.Background()
	tok, err := testToken(t, st, ctx, 4, "architect")
	if err != nil {
		t.Fatal(err)
	}
	call := callTool(t, st, tok)

	closeSpace(t, st, 4)
	got, isErr := call("post_to_space", map[string]any{"body": "late"})
	if got != "space 4 is closed: conflict" || !isErr {
		t.Errorf("post to a closed Space = %q (error %v), want the closed error", got, isErr)
	}

	if _, err := st.DB().Exec("UPDATE space SET closed_at = NULL, moved_to = 3 WHERE id = 4"); err != nil {
		t.Fatal(err)
	}
	got, isErr = call("post_to_space", map[string]any{"body": "late"})
	if got != "conversation 4 moved to Space 3: conflict" || !isErr {
		t.Errorf("post to a moved Space = %q (error %v), want the moved error", got, isErr)
	}
	msgs, err := st.Messages(ctx, 4, 0)
	if err != nil || len(msgs) != 0 {
		t.Errorf("messages of Space 4 = %+v, %v, want none", msgs, err)
	}
}

func TestMCPQuickSessionReadsItsOwnThread(t *testing.T) {
	st, _ := mcpFixture(t)
	ctx := context.Background()
	tok, err := testToken(t, st, ctx, 4, "architect")
	if err != nil {
		t.Fatal(err)
	}
	root, err := st.CreateMessage(ctx, store.Message{SpaceID: 4, Author: "user", Body: "plan it"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.CreateMessage(ctx, store.Message{
		SpaceID: 4, ParentID: &root.ID, Author: "architect", Body: "ok",
	}); err != nil {
		t.Fatal(err)
	}
	got, isErr := callTool(t, st, tok)("read_thread", map[string]any{"spaceId": 4, "root": root.ID})
	want := `{"messages":[{"author":"user","body":"plan it","createdAt":"T","id":5,` +
		`"parentId":null,"spaceId":4},{"author":"architect","body":"ok","createdAt":"T","id":6,` +
		`"parentId":5,"spaceId":4}]}`
	if got != want || isErr {
		t.Errorf("read_thread = %q (error %v), want %q", got, isErr, want)
	}
}

func TestMCPBrowserNavigate(t *testing.T) {
	st, tok := mcpFixture(t)
	dataDir := t.TempDir()
	fakeBrowserSocket(t, dataDir, `{"text":"navigated to x"}`+"\n")

	got, isErr := callToolResult(t, st, tok, dataDir)("browser_navigate", map[string]any{"url": "x"})
	if want := `{"text":"navigated to x"}`; got != want || isErr {
		t.Errorf("browser_navigate = %q (error %v), want %q", got, isErr, want)
	}
}

func TestMCPBrowserTakenOver(t *testing.T) {
	st, tok := mcpFixture(t)
	dataDir := t.TempDir()
	fakeBrowserSocket(t, dataDir, `{"error":"the user has the browser"}`+"\n")

	got, isErr := callToolResult(t, st, tok, dataDir)("browser_navigate", map[string]any{"url": "x"})
	if want := "the user has the browser"; got != want || !isErr {
		t.Errorf("browser_navigate = %q (error %v), want %q", got, isErr, want)
	}
}

func TestMCPBrowserScreenshot(t *testing.T) {
	// A 1x1 transparent PNG.
	const tinyPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="
	st, tok := mcpFixture(t)
	dataDir := t.TempDir()
	fakeBrowserSocket(t, dataDir, `{"image":"`+tinyPNG+`"}`+"\n")

	ctx := context.Background()
	ct, st2 := mcp.NewInMemoryTransports()
	ss, err := NewMCP(st, openMemory(t, st), tok, dataDir).Connect(ctx, st2, nil)
	if err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close(); _ = ss.Wait() })

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "browser_screenshot"})
	if err != nil {
		t.Fatal(err)
	}
	img, ok := res.Content[0].(*mcp.ImageContent)
	if !ok {
		t.Fatalf("content is %T, want an image", res.Content[0])
	}
	want, err := base64.StdEncoding.DecodeString(tinyPNG)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(img.Data, want) {
		t.Errorf("image bytes = %x, want %x", img.Data, want)
	}
	if img.MIMEType != "image/png" {
		t.Errorf("mime type = %q, want image/png", img.MIMEType)
	}
}

func TestMCPFindBots(t *testing.T) {
	st, _ := mcpFixture(t)
	lead, developer := leadAndDeveloper(t, st)
	ctx := context.Background()
	call := callTool(t, st, lead)
	pw, err := st.CreateConnection(ctx, store.Connection{Name: "pw", Command: "pw-mcp", Env: map[string]string{"KEY": "sk-secret"}})
	if err != nil {
		t.Fatal(err)
	}
	tester, err := st.CreateBot(ctx, store.Bot{
		Name: "tester", Backend: "claude", Description: "Breaks code with real browsers", Skills: []string{"claude:verify"},
		Tools: []string{"Read"}, Connections: []int64{pw.ID},
	})
	if err != nil {
		t.Fatal(err)
	}

	got, isErr := call("find_bots", map[string]any{"query": "browsers"})
	want := fmt.Sprintf(`{"bots":[{"capabilities":{"backend":"claude","mcp":[{"label":"pw-mcp","name":"pw"}],`+
		`"skills":["claude:verify"],"tools":["Read"],"write":false},"description":"Breaks code with real browsers",`+
		`"id":%d,"name":"tester"}],"note":"Capabilities are what each Bot is configured with, not permission for `+
		`you or anyone to use them."}`, tester.ID)
	if got != want || isErr || strings.Contains(got, "sk-secret") {
		t.Errorf("find_bots browsers = %q (error %v), want %q", got, isErr, want)
	}

	if got, _ := call("find_bots", map[string]any{"query": "lead"}); strings.Contains(got, `"name":"lead"`) {
		t.Errorf("find_bots returned the caller: %q", got)
	}

	if _, err := st.UpdateBot(ctx, store.Bot{ID: tester.ID, Name: "qa", Backend: "claude", Description: "Breaks code with real browsers"}); err != nil {
		t.Fatal(err)
	}
	got, _ = call("find_bots", map[string]any{"ids": []int64{tester.ID, 99}})
	if !strings.Contains(got, fmt.Sprintf(`"id":%d,"name":"qa"`, tester.ID)) || !strings.Contains(got, `{"id":99,"missing":true}`) {
		t.Errorf("find_bots ids = %q, want the renamed Bot by id and 99 missing", got)
	}
	if members, err := st.SpaceTeam(ctx, lead.SpaceID); err != nil || len(members.Roles) != 2 {
		t.Errorf("Space members after discovery = %+v, %v, want unchanged (2)", members, err)
	}
	_ = developer
}
