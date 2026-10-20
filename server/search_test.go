package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

func TestSearchDirectVisibility(t *testing.T) {
	ctx := context.Background()
	st, _ := mcpFixture(t)
	if _, err := st.CreateTeam(ctx, store.Team{Name: "core", Roles: []store.Role{
		{Name: "lead", Lead: true, Backend: "claude"},
		{Name: "developer", Backend: "claude"},
		{Name: "tester", Backend: "claude"},
	}}); err != nil {
		t.Fatal(err)
	}
	one := int64(1)
	ch, err := st.CreateChannel(ctx, store.Channel{
		ProjectID: 1, FeatureID: &one, Name: "team", Team: "core", Role: "lead", Backend: "claude",
	})
	if err != nil {
		t.Fatal(err)
	}
	callAs := func(role string) func(string, map[string]any) (string, bool) {
		tok, err := st.CreateToken(ctx, ch.ID, role)
		if err != nil {
			t.Fatal(err)
		}
		return callTool(t, st, tok)
	}
	callLead, callDeveloper, callTester := callAs("lead"), callAs("developer"), callAs("tester")

	req, err := st.CreateDirectMessage(ctx, store.DirectMessage{
		ChannelID: ch.ID, From: "lead", To: "developer", Body: "build the handoff"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateDirectResponse(ctx, req.ID, "developer", "handoff done"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateMessage(ctx, store.Message{
		ChannelID: ch.ID, Author: "lead", Body: "handoff plan for @developer"}); err != nil {
		t.Fatal(err)
	}

	const (
		public = `{"author":"lead","channelId":5,"kind":"public","messageId":5,` +
			`"snippet":"handoff plan for @developer"}`
		response = `{"author":"developer","chainId":1,"channelId":5,"from":"developer",` +
			`"kind":"direct","messageId":2,"snippet":"handoff done","to":"lead"}`
		request = `{"author":"lead","chainId":1,"channelId":5,"from":"lead","kind":"direct",` +
			`"messageId":1,"snippet":"build the handoff","to":"developer"}`
	)
	args := map[string]any{"query": "HANDOFF"}
	both := `{"hits":[` + public + `,` + response + `,` + request + `]}`
	for name, call := range map[string]func(string, map[string]any) (string, bool){
		"lead": callLead, "developer": callDeveloper,
	} {
		if got, isErr := call("search_sessions", args); got != both || isErr {
			t.Errorf("%s search = %q (error %v), want %q", name, got, isErr, both)
		}
	}
	if got, _ := callLead("search_sessions", map[string]any{"query": "HANDOFF", "limit": 2}); got !=
		`{"hits":[`+public+`,`+response+`]}` {
		t.Errorf("limited search = %q, want the newest two", got)
	}
	if got, isErr := callTester("search_sessions", args); got != `{"hits":[`+public+`]}` || isErr {
		t.Errorf("tester search = %q (error %v), want the public hit only", got, isErr)
	}
	if got, _ := callTester("read_direct_messages", nil); got != `{"messages":[]}` {
		t.Errorf("tester read_direct_messages = %q, want none", got)
	}
	got, _ := callTester("read_thread", map[string]any{"channelId": ch.ID})
	if strings.Contains(got, "build the handoff") || strings.Contains(got, "handoff done") {
		t.Errorf("tester read_thread = %q, want no direct body", got)
	}

	if n, err := st.CountMessages(ctx, ch.ID); err != nil || n != 1 {
		t.Errorf("public messages after the chain finished = %d, %v, want only the one posted", n, err)
	}

	h := New(st, t.TempDir(), "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)
	tests := []struct {
		name, path string
		wantStatus int
		want       string
	}{
		{
			name: "user sees both kinds", path: "/search?feature=1&q=handoff", wantStatus: 200,
			want: `[{"channelId":5,"messageId":5,"kind":"public","author":"lead",` +
				`"snippet":"handoff plan for @developer"},` +
				`{"channelId":5,"messageId":2,"kind":"direct","author":"developer",` +
				`"from":"developer","to":"lead","chainId":1,"snippet":"handoff done"},` +
				`{"channelId":5,"messageId":1,"kind":"direct","author":"lead",` +
				`"from":"lead","to":"developer","chainId":1,"snippet":"build the handoff"}]`,
		},
		{name: "limit", path: "/search?feature=1&q=handoff&limit=1", wantStatus: 200,
			want: `[{"channelId":5,"messageId":5,"kind":"public","author":"lead",` +
				`"snippet":"handoff plan for @developer"}]`},
		{name: "no match", path: "/search?feature=1&q=zebra", wantStatus: 200, want: `[]`},
		{name: "feature is required", path: "/search?q=handoff", wantStatus: 400,
			want: `{"error":"invalid: feature is required"}`},
		{name: "query is required", path: "/search?feature=1", wantStatus: 400,
			want: `{"error":"invalid: q is required"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := do(t, http.MethodGet, ts.URL+tt.path, "")
			if status != tt.wantStatus || body != tt.want {
				t.Errorf("GET %s = %d %s, want %d %s", tt.path, status, body, tt.wantStatus, tt.want)
			}
		})
	}
	if status, _ := do(t, http.MethodPost, ts.URL+"/search?feature=1&q=handoff", ""); status != 405 {
		t.Errorf("POST /search = %d, want 405", status)
	}
}
