package server

import (
	"context"
	"reflect"
	"regexp"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/bhanuprakaash/reef/store"
)

var _timestamp = regexp.MustCompile(`"(createdAt|updatedAt)":"[^"]*"`)

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
	ctx := context.Background()
	ct, st2 := mcp.NewInMemoryTransports()
	ss, err := NewMCP(st, tok).Connect(ctx, st2, nil)
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
				`"id":2,"messageId":1,"why":"simple"}`,
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
				`"id":2,"messageId":1,"why":"simple"}`,
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
		ID: 1, ChannelID: 1, Role: "architect", Brief: "v2", Notes: "mine",
		Decisions: []store.ProposedDecision{{Body: "use sessions", Why: "simple"}},
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
		{"search_sessions", map[string]any{"query": "q"}},
		{"read_decision", map[string]any{"id": 1}},
		{"read_thread", map[string]any{"channelId": 1}},
		{"propose_wrap", map[string]any{"brief": "b", "notes": "n", "decisions": []any{}}},
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
