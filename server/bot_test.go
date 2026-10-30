package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

func TestBotAndMembership(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := testServerTeam(t, st, context.Background(), store.Team{Name: "core",
		Roles: []store.Role{{Name: "core-boss", Lead: true, Backend: "claude"}}}); err != nil {
		t.Fatal(err)
	}
	h := New(st, testMemory(t, st), t.TempDir(), "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)

	steps := []struct {
		name   string
		method string
		path   string
		body   string
		status int
		want   string
	}{
		{"create", http.MethodPost, "/bots",
			`{"name":"sam","description":"You test.","skills":["how"],"backend":"claude","face":"otter","colour":"violet","tools":["Read"],"write":true,"permissionMode":"acceptEdits"}`,
			201, `{"id":3,"name":"sam","description":"You test.","skills":["how"],"backend":"claude","face":"otter","colour":"violet","tools":["Read"],"mcp":[],"write":true,"permissionMode":"acceptEdits"}`},
		{"unknown colour", http.MethodPost, "/bots",
			`{"name":"kit","backend":"claude","colour":"mauve"}`,
			400, `{"error":"invalid: role kit: colour \"mauve\" must be one of orange, red, yellow, green, teal, blue, violet, pink"}`},
		{"join", http.MethodPost, "/teams/core/members", `{"botId":3}`,
			201, `{"botId":3,"name":"sam","lead":false,"write":true,"backend":"claude","permissionMode":"acceptEdits","description":"You test.","skills":["how"],"face":"otter","colour":"violet","tools":["Read"],"mcp":[]}`},
		{"no conversations", http.MethodGet, "/bots/3/conversations", "", 200, `[]`},
		{"conversations of nobody", http.MethodGet, "/bots/99/conversations", "", 404,
			`{"error":"bot 99: not found"}`},
		{"edit defaults", http.MethodPut, "/bots/3",
			`{"name":"sam","description":"You test.","skills":["how"],"backend":"claude","face":"otter","colour":"violet","tools":["Read","Grep"],"write":false}`,
			200, `{"id":3,"name":"sam","description":"You test.","skills":["how"],"backend":"claude","face":"otter","colour":"violet","tools":["Read","Grep"],"mcp":[],"write":false,"permissionMode":""}`},
		{"membership uses profile", http.MethodGet, "/teams/core", "", 200,
			`{"id":2,"name":"core","responsibility":"","roles":[{"botId":2,"name":"core-boss","lead":true,"write":false,"backend":"claude","permissionMode":"","description":"","skills":[],"tools":[],"mcp":[]},` +
				`{"botId":3,"name":"sam","lead":false,"write":false,"backend":"claude","permissionMode":"","description":"You test.","skills":["how"],"face":"otter","colour":"violet","tools":["Read","Grep"],"mcp":[]}]}`},
		{"reject membership settings", http.MethodPut, "/teams/core/members/3",
			`{"lead":false,"tools":["Read","Edit"]}`,
			400, `{"error":"invalid: bad JSON: json: unknown field \"tools\""}`},
		{"second lead", http.MethodPut, "/teams/core/members/3", `{"lead":true}`,
			400, `{"error":"invalid: the team already has a lead, core-boss"}`},
		{"delete while in a team", http.MethodDelete, "/bots/3", "",
			400, `{"error":"invalid: bot sam is in team core; remove it from the team first"}`},
		{"leave", http.MethodDelete, "/teams/core/members/3", "", 204, ""},
		{"delete", http.MethodDelete, "/bots/3", "", 204, ""},
		{"gone", http.MethodGet, "/bots/3", "", 404, `{"error":"bot 3: not found"}`},
	}
	for _, tt := range steps {
		t.Run(tt.name, func(t *testing.T) {
			status, body := do(t, tt.method, ts.URL+tt.path, tt.body)
			if status != tt.status || body != tt.want {
				t.Errorf("%s %s = %d %q, want %d %q", tt.method, tt.path, status, body, tt.status, tt.want)
			}
		})
	}
}

func TestProfileUpdateReachesSpaceMembersREST(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	project, err := st.CreateProject(ctx, store.Project{Name: "shop", Path: "/tmp/shop"})
	if err != nil {
		t.Fatal(err)
	}
	bot, err := st.CreateBot(ctx, store.Bot{Name: "sam", Backend: "claude", Tools: []string{"Read"}})
	if err != nil {
		t.Fatal(err)
	}
	space, err := st.CreateBotSpace(ctx, store.Space{ProjectID: project.ID, Name: "chat"}, bot.ID)
	if err != nil {
		t.Fatal(err)
	}
	h := New(st, testMemory(t, st), t.TempDir(), "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)
	status, body := do(t, http.MethodPut, ts.URL+"/bots/"+strconv.FormatInt(bot.ID, 10), `{"name":"sam","description":"Checks examples.","backend":"claude","tools":["Read","Grep"],"write":true}`)
	if status != http.StatusOK {
		t.Fatalf("update profile = %d %s", status, body)
	}
	status, body = do(t, http.MethodGet, ts.URL+"/spaces/"+strconv.FormatInt(space.ID, 10)+"/members", "")
	if status != http.StatusOK || !strings.Contains(body, `"tools":["Read","Grep"]`) || !strings.Contains(body, `"write":true`) || !strings.Contains(body, `"description":"Checks examples."`) {
		t.Errorf("Space members = %d %s, want updated profile", status, body)
	}
}
