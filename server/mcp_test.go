package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"net"
	"path/filepath"
	"reflect"
	"regexp"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/bhanuprakaash/reef/store"
)

var _timestamp = regexp.MustCompile(`"(createdAt|updatedAt|retiredAt)":"[^"]*"`)

// mcpFixture has features 1 (auth) and 2 (billing). The returned token is the
// architect in channel 1; channel 2 is a second channel of auth, channel 3 is
// billing's and channel 4 is a quick session.
func mcpFixture(t *testing.T) (*store.Store, store.Token) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(t.TempDir())
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
	_, err = st.CreateFeature(ctx, store.Feature{ProjectID: 1, Slug: "auth", Name: "Auth"})
	must(err)
	_, err = st.CreateFeature(ctx, store.Feature{ProjectID: 1, Slug: "billing", Name: "Billing"})
	must(err)
	for _, c := range []store.Channel{
		{ProjectID: 1, FeatureID: &one, Name: "architect"},
		{ProjectID: 1, FeatureID: &one, Name: "developer"},
		{ProjectID: 1, FeatureID: &two, Name: "architect"},
		{ProjectID: 1, Name: "quick"},
	} {
		_, err = st.CreateChannel(ctx, c)
		must(err)
	}
	for _, m := range []store.Message{
		{ChannelID: 1, Author: "user", Body: "use sessions for auth"},
		{ChannelID: 2, Author: "developer", Body: "100% sure, use_sessions"},
		{ChannelID: 3, Author: "user", Body: "use sessions for billing"},
		{ChannelID: 2, ParentID: &two, Author: "architect", Body: "agreed"},
	} {
		_, err = st.CreateMessage(ctx, m)
		must(err)
	}
	_, err = st.PutBrief(ctx, store.Brief{FeatureID: 1, Body: "Build login", Author: "user"})
	must(err)
	_, err = st.PutRoleNotes(ctx, store.RoleNotes{FeatureID: 1, Role: "architect", Body: "keep it small"})
	must(err)
	_, err = st.CreateDecision(ctx, store.Decision{FeatureID: 2, Body: "use stripe", Author: "user"})
	must(err)
	tok, err := st.CreateToken(ctx, 1, "architect")
	must(err)
	return st, tok
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
	ctx := context.Background()
	ct, st2 := mcp.NewInMemoryTransports()
	ss, err := NewMCP(st, tok, dataDir).Connect(ctx, st2, nil)
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
			name: "get brief and notes",
			tool: "get_brief",
			want: `{"brief":"Build login","notes":"keep it small"}`,
		},
		{
			name: "record decision as the token's role",
			tool: "record_decision",
			give: map[string]any{"body": "use sessions", "why": "simple", "messageId": 1},
			want: `{"author":"architect","body":"use sessions","createdAt":"T","featureId":1,` +
				`"id":2,"messageId":1,"replacedBy":null,"retiredAt":null,"why":"simple"}`,
		},
		{
			name: "add task to the feature",
			tool: "add_task",
			give: map[string]any{"title": "write tests"},
			want: `{"body":"","createdAt":"T","featureId":1,"id":1,"projectId":1,` +
				`"status":"todo","title":"write tests","updatedAt":"T"}`,
		},
		{
			name: "search skips another feature",
			tool: "search_sessions",
			give: map[string]any{"query": "SESSIONS"},
			want: `{"hits":[{"author":"developer","channelId":2,"messageId":2,` +
				`"snippet":"100% sure, use_sessions"},{"author":"user","channelId":1,` +
				`"messageId":1,"snippet":"use sessions for auth"}]}`,
		},
		{
			name: "search treats underscore as text",
			tool: "search_sessions",
			give: map[string]any{"query": "use_s"},
			want: `{"hits":[{"author":"developer","channelId":2,"messageId":2,` +
				`"snippet":"100% sure, use_sessions"}]}`,
		},
		{
			name: "search limit",
			tool: "search_sessions",
			give: map[string]any{"query": "sessions", "limit": 1},
			want: `{"hits":[{"author":"developer","channelId":2,"messageId":2,` +
				`"snippet":"100% sure, use_sessions"}]}`,
		},
		{
			name: "read own decision",
			tool: "read_decision",
			give: map[string]any{"id": 2},
			want: `{"author":"architect","body":"use sessions","createdAt":"T","featureId":1,` +
				`"id":2,"messageId":1,"replacedBy":null,"retiredAt":null,"why":"simple"}`,
		},
		{
			name:    "read another feature's decision",
			tool:    "read_decision",
			give:    map[string]any{"id": 1},
			want:    "decision 1: not found",
			wantErr: true,
		},
		{
			name: "read thread after an id",
			tool: "read_thread",
			give: map[string]any{"channelId": 2, "after": 2},
			want: `{"messages":[{"author":"architect","body":"agreed","channelId":2,` +
				`"createdAt":"T","id":4,"parentId":2}]}`,
		},
		{
			name: "read thread of a root",
			tool: "read_thread",
			give: map[string]any{"channelId": 2, "root": 2},
			want: `{"messages":[{"author":"developer","body":"100% sure, use_sessions",` +
				`"channelId":2,"createdAt":"T","id":2,"parentId":null},` +
				`{"author":"architect","body":"agreed","channelId":2,"createdAt":"T","id":4,` +
				`"parentId":2}]}`,
		},
		{
			name:    "read thread of a reply",
			tool:    "read_thread",
			give:    map[string]any{"channelId": 2, "root": 4},
			want:    "thread 4: not found",
			wantErr: true,
		},
		{
			name: "post to channel as the token's role",
			tool: "post_to_channel",
			give: map[string]any{"body": "@developer take it"},
			want: `{"author":"architect","body":"@developer take it","channelId":1,` +
				`"createdAt":"T","id":5,"parentId":null}`,
		},
		{
			name: "post to channel in a thread",
			tool: "post_to_channel",
			give: map[string]any{"body": "done", "parent": 1},
			want: `{"author":"architect","body":"done","channelId":1,"createdAt":"T","id":6,` +
				`"parentId":1}`,
		},
		{
			name:    "post to channel under a reply",
			tool:    "post_to_channel",
			give:    map[string]any{"body": "x", "parent": 6},
			want:    "invalid: parent 6 is not a top-level message of channel 1",
			wantErr: true,
		},
		{
			name:    "read another feature's thread",
			tool:    "read_thread",
			give:    map[string]any{"channelId": 3},
			want:    "channel 3: not found",
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
		"brief": "v2", "notes": "mine",
		"decisions": []any{map[string]any{"body": "use sessions", "why": "simple"}},
	})
	if want := `{"id":1,"message":"proposal saved; the user will approve it"}`; got != want || isErr {
		t.Errorf("propose_wrap = %q (error %v), want %q", got, isErr, want)
	}
	p, err := st.PendingProposal(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	want := store.Proposal{
		ID: 1, ChannelID: 1, Role: "architect", Kind: store.ProposalWrap, Brief: "v2", Notes: "mine",
		Decisions: []store.ProposedDecision{{Body: "use sessions", Why: "simple"}},
		Retire:    []int64{},
		Status:    store.ProposalPending,
	}
	p.CreatedAt = time.Time{}
	if !reflect.DeepEqual(p, want) {
		t.Errorf("proposal = %+v, want %+v", p, want)
	}

	got, isErr = call("propose_wrap", map[string]any{"brief": "", "notes": "n", "decisions": []any{}})
	if want := "invalid: proposal brief is required"; got != want || !isErr {
		t.Errorf("propose_wrap with empty brief = %q (error %v), want %q", got, isErr, want)
	}
}

func TestMCPRecordDecisionReplaces(t *testing.T) {
	st, tok := mcpFixture(t)
	call := callTool(t, st, tok)
	call("record_decision", map[string]any{"body": "use sessions", "why": "simple"})

	got, isErr := call("record_decision", map[string]any{"body": "use tokens", "replaces": []int{2}})
	want := `{"author":"architect","body":"use tokens","createdAt":"T","featureId":1,` +
		`"id":3,"messageId":null,"replacedBy":null,"replaces":[2],"retiredAt":null,"why":""}`
	if got != want || isErr {
		t.Errorf("record_decision with replaces = %q (error %v), want %q", got, isErr, want)
	}
	got, isErr = call("read_decision", map[string]any{"id": 2})
	want = `{"author":"architect","body":"use sessions","createdAt":"T","featureId":1,` +
		`"id":2,"messageId":null,"replacedBy":3,"retiredAt":"T","why":"simple"}`
	if got != want || isErr {
		t.Errorf("read_decision of the replaced = %q (error %v), want %q", got, isErr, want)
	}
	got, isErr = call("record_decision", map[string]any{"body": "use jwt", "replaces": []int{1}})
	want = "invalid: decision 1 is not a live decision of feature 1"
	if got != want || !isErr {
		t.Errorf("record_decision replacing another feature's = %q (error %v), want %q", got, isErr, want)
	}
}

func TestMCPSetStage(t *testing.T) {
	st, tok := mcpFixture(t)
	ctx := context.Background()
	if _, err := st.CreateFeature(ctx, store.Feature{ProjectID: 1, Slug: "login", Name: "Login", Kind: "fixes"}); err != nil {
		t.Fatal(err)
	}
	three := int64(3)
	ch, err := st.CreateChannel(ctx, store.Channel{ProjectID: 1, FeatureID: &three, Name: "architect"})
	if err != nil {
		t.Fatal(err)
	}
	fixesTok, err := st.CreateToken(ctx, ch.ID, "architect")
	if err != nil {
		t.Fatal(err)
	}

	got, isErr := callTool(t, st, tok)("set_stage", map[string]any{"stage": "plan"})
	if want := `{"closedAt":null,"createdAt":"T","goal":"","id":1,"kind":"journey",` +
		`"name":"Auth","projectId":1,"slug":"auth","stage":"plan"}`; got != want || isErr {
		t.Errorf("set_stage plan = %q (error %v), want %q", got, isErr, want)
	}
	msgs, err := st.Messages(ctx, 1, 4)
	if err != nil || len(msgs) != 1 || msgs[0].Author != "reef" ||
		msgs[0].Body != "architect moved the stage to plan" {
		t.Errorf("messages after id 4 = %+v, %v, want one reef note", msgs, err)
	}

	tests := []struct {
		name string
		tok  store.Token
		give string
		want string
	}{
		{"unknown stage", tok, "x", `invalid: unknown stage "x"`},
		{"fixes feature", fixesTok, "plan", "invalid: feature login is a list of fixes and has no stage"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, isErr := callTool(t, st, tt.tok)("set_stage", map[string]any{"stage": tt.give})
			if got != tt.want || !isErr {
				t.Errorf("set_stage %q = %q (error %v), want %q", tt.give, got, isErr, tt.want)
			}
		})
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
		ID: 1, ChannelID: 1, Role: "architect", Kind: store.ProposalBrief,
		Brief: "plan: ship login first", Decisions: []store.ProposedDecision{},
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

func TestMCPProposeBriefRefusedWhileWrapPending(t *testing.T) {
	st, tok := mcpFixture(t)
	call := callTool(t, st, tok)
	if got, isErr := call("propose_wrap", map[string]any{"brief": "final", "notes": "n", "decisions": []any{}}); isErr {
		t.Fatalf("propose_wrap = %q", got)
	}

	got, isErr := call("propose_brief", map[string]any{"brief": "plan"})
	if want := "invalid: channel 1 has a wrap-up waiting for approval"; got != want || !isErr {
		t.Errorf("propose_brief = %q (error %v), want %q", got, isErr, want)
	}
	p, err := st.PendingProposal(context.Background(), 1)
	if err != nil || p.Kind != store.ProposalWrap || p.Brief != "final" {
		t.Errorf("pending = %+v, %v, want the wrap with brief final", p, err)
	}
}

func TestMCPListTasks(t *testing.T) {
	st, tok := mcpFixture(t)
	ctx := context.Background()
	one, two := int64(1), int64(2)
	for _, task := range []store.Task{
		{ProjectID: 1, FeatureID: &one, Title: "login form"},
		{ProjectID: 1, FeatureID: &two, Title: "invoice"},
		{ProjectID: 1, FeatureID: &one, Title: "logout", Status: "done"},
		{ProjectID: 1, Title: "no feature"},
	} {
		if _, err := st.CreateTask(ctx, task); err != nil {
			t.Fatal(err)
		}
	}
	call := callTool(t, st, tok)
	tests := []struct {
		name    string
		give    map[string]any
		want    string
		wantErr bool
	}{
		{
			name: "only this feature's tasks",
			want: `{"tasks":[{"body":"","createdAt":"T","featureId":1,"id":1,"projectId":1,` +
				`"status":"todo","title":"login form","updatedAt":"T"},{"body":"","createdAt":"T",` +
				`"featureId":1,"id":3,"projectId":1,"status":"done","title":"logout","updatedAt":"T"}]}`,
		},
		{
			name: "status filter",
			give: map[string]any{"status": "done"},
			want: `{"tasks":[{"body":"","createdAt":"T","featureId":1,"id":3,"projectId":1,` +
				`"status":"done","title":"logout","updatedAt":"T"}]}`,
		},
		{
			name: "no match",
			give: map[string]any{"status": "doing"},
			want: `{"tasks":[]}`,
		},
		{
			name:    "unknown status",
			give:    map[string]any{"status": "later"},
			want:    `invalid: unknown status "later"`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, isErr := call("list_tasks", tt.give)
			if got != tt.want || isErr != tt.wantErr {
				t.Errorf("list_tasks = %q (error %v), want %q (error %v)", got, isErr, tt.want, tt.wantErr)
			}
		})
	}
}

func TestMCPReadAndUpdateTask(t *testing.T) {
	st, tok := mcpFixture(t)
	ctx := context.Background()
	one, two := int64(1), int64(2)
	for _, task := range []store.Task{
		{ProjectID: 1, FeatureID: &one, Title: "login form", Body: "email and password"},
		{ProjectID: 1, FeatureID: &two, Title: "invoice"},
	} {
		if _, err := st.CreateTask(ctx, task); err != nil {
			t.Fatal(err)
		}
	}
	call := callTool(t, st, tok)
	tests := []struct {
		name    string
		tool    string
		give    map[string]any
		want    string
		wantErr bool
	}{
		{
			name: "read own task",
			tool: "read_task",
			give: map[string]any{"id": 1},
			want: `{"body":"email and password","createdAt":"T","featureId":1,"id":1,"projectId":1,` +
				`"status":"todo","title":"login form","updatedAt":"T"}`,
		},
		{
			name:    "read another feature's task",
			tool:    "read_task",
			give:    map[string]any{"id": 2},
			want:    "task 2: not found",
			wantErr: true,
		},
		{
			name:    "read a missing task",
			tool:    "read_task",
			give:    map[string]any{"id": 99},
			want:    "task 99: not found",
			wantErr: true,
		},
		{
			name: "update only the status",
			tool: "update_task",
			give: map[string]any{"id": 1, "status": "done"},
			want: `{"body":"email and password","createdAt":"T","featureId":1,"id":1,"projectId":1,` +
				`"status":"done","title":"login form","updatedAt":"T"}`,
		},
		{
			name: "update title and body",
			tool: "update_task",
			give: map[string]any{"id": 1, "title": "sign in form", "body": ""},
			want: `{"body":"","createdAt":"T","featureId":1,"id":1,"projectId":1,` +
				`"status":"done","title":"sign in form","updatedAt":"T"}`,
		},
		{
			name:    "update with an unknown status",
			tool:    "update_task",
			give:    map[string]any{"id": 1, "status": "later"},
			want:    `invalid: unknown status "later"`,
			wantErr: true,
		},
		{
			name:    "update with an empty title",
			tool:    "update_task",
			give:    map[string]any{"id": 1, "title": ""},
			want:    "invalid: task title is required",
			wantErr: true,
		},
		{
			name:    "update another feature's task",
			tool:    "update_task",
			give:    map[string]any{"id": 2, "status": "done"},
			want:    "task 2: not found",
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
	other, err := st.Task(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	if other.Status != "todo" {
		t.Errorf("another feature's task status = %q, want todo", other.Status)
	}
}

func TestMCPQuickSession(t *testing.T) {
	st, _ := mcpFixture(t)
	tok, err := st.CreateToken(context.Background(), 4, "architect")
	if err != nil {
		t.Fatal(err)
	}
	call := callTool(t, st, tok)
	tests := []struct {
		tool string
		give map[string]any
	}{
		{"get_brief", nil},
		{"record_decision", map[string]any{"body": "d"}},
		{"add_task", map[string]any{"title": "t"}},
		{"list_tasks", nil},
		{"read_task", map[string]any{"id": 1}},
		{"update_task", map[string]any{"id": 1, "status": "done"}},
		{"search_sessions", map[string]any{"query": "q"}},
		{"read_decision", map[string]any{"id": 1}},
		{"read_thread", map[string]any{"channelId": 1}},
		{"propose_wrap", map[string]any{"brief": "b", "notes": "n", "decisions": []any{}}},
		{"set_stage", map[string]any{"stage": "plan"}},
		{"propose_brief", map[string]any{"brief": "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			got, isErr := call(tt.tool, tt.give)
			if got != "this channel has no feature" || !isErr {
				t.Errorf("%s = %q (error %v), want the no-feature error", tt.tool, got, isErr)
			}
		})
	}
}

func TestMCPQuickSessionReadsItsOwnThread(t *testing.T) {
	st, _ := mcpFixture(t)
	ctx := context.Background()
	tok, err := st.CreateToken(ctx, 4, "architect")
	if err != nil {
		t.Fatal(err)
	}
	root, err := st.CreateMessage(ctx, store.Message{ChannelID: 4, Author: "user", Body: "plan it"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.CreateMessage(ctx, store.Message{
		ChannelID: 4, ParentID: &root.ID, Author: "architect", Body: "ok",
	}); err != nil {
		t.Fatal(err)
	}
	got, isErr := callTool(t, st, tok)("read_thread", map[string]any{"channelId": 4, "root": root.ID})
	want := `{"messages":[{"author":"user","body":"plan it","channelId":4,"createdAt":"T","id":5,` +
		`"parentId":null},{"author":"architect","body":"ok","channelId":4,"createdAt":"T","id":6,` +
		`"parentId":5}]}`
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
	ss, err := NewMCP(st, tok, dataDir).Connect(ctx, st2, nil)
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
