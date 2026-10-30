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

func TestTeammateAndMembership(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.ApplyTeam(context.Background(), store.Team{Name: "core",
		Roles: []store.Role{{Name: "boss", Lead: true, Backend: "claude"}}}); err != nil {
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
		{"create", http.MethodPost, "/teammates",
			`{"name":"sam","persona":"You test.","skills":["how"],"backend":"claude","face":"otter","colour":"violet","tools":["Read"],"write":true,"permissionMode":"acceptEdits"}`,
			201, `{"id":3,"name":"sam","persona":"You test.","skills":["how"],"backend":"claude","face":"otter","colour":"violet","tools":["Read"],"mcp":[],"write":true,"permissionMode":"acceptEdits"}`},
		{"unknown colour", http.MethodPost, "/teammates",
			`{"name":"kit","backend":"claude","colour":"mauve"}`,
			400, `{"error":"invalid: role kit: colour \"mauve\" must be one of orange, red, yellow, green, teal, blue, violet, pink"}`},
		{"join", http.MethodPost, "/teams/core/members", `{"teammateId":3}`,
			201, `{"teammateId":3,"name":"sam","lead":false,"write":true,"backend":"claude","permissionMode":"acceptEdits","persona":"You test.","skills":["how"],"face":"otter","colour":"violet","tools":["Read"],"mcp":[]}`},
		{"no conversations", http.MethodGet, "/teammates/3/conversations", "", 200, `[]`},
		{"conversations of nobody", http.MethodGet, "/teammates/99/conversations", "", 404,
			`{"error":"teammate 99: not found"}`},
		{"edit defaults", http.MethodPut, "/teammates/3",
			`{"name":"sam","persona":"You test.","skills":["how"],"backend":"claude","face":"otter","colour":"violet","tools":["Read","Grep"],"write":false}`,
			200, `{"id":3,"name":"sam","persona":"You test.","skills":["how"],"backend":"claude","face":"otter","colour":"violet","tools":["Read","Grep"],"mcp":[],"write":false,"permissionMode":""}`},
		{"membership uses profile", http.MethodGet, "/teams/core", "", 200,
			`{"id":2,"name":"core","responsibility":"","roles":[{"teammateId":2,"name":"boss","lead":true,"write":false,"backend":"claude","permissionMode":"","persona":"","skills":[],"tools":[],"mcp":[]},` +
				`{"teammateId":3,"name":"sam","lead":false,"write":false,"backend":"claude","permissionMode":"","persona":"You test.","skills":["how"],"face":"otter","colour":"violet","tools":["Read","Grep"],"mcp":[]}]}`},
		{"reject membership settings", http.MethodPut, "/teams/core/members/3",
			`{"lead":false,"tools":["Read","Edit"]}`,
			400, `{"error":"invalid: bad JSON: json: unknown field \"tools\""}`},
		{"second lead", http.MethodPut, "/teams/core/members/3", `{"lead":true}`,
			400, `{"error":"invalid: the team already has a lead, boss"}`},
		{"delete while in a team", http.MethodDelete, "/teammates/3", "",
			400, `{"error":"invalid: teammate sam is in team core; remove it from the team first"}`},
		{"leave", http.MethodDelete, "/teams/core/members/3", "", 204, ""},
		{"delete", http.MethodDelete, "/teammates/3", "", 204, ""},
		{"gone", http.MethodGet, "/teammates/3", "", 404, `{"error":"teammate 3: not found"}`},
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
	bot, err := st.CreateTeammate(ctx, store.Teammate{Name: "sam", Backend: "claude", Tools: []string{"Read"}})
	if err != nil {
		t.Fatal(err)
	}
	space, err := st.CreateTeammateSpace(ctx, store.Space{ProjectID: project.ID, Name: "chat"}, bot.ID)
	if err != nil {
		t.Fatal(err)
	}
	h := New(st, testMemory(t, st), t.TempDir(), "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)
	status, body := do(t, http.MethodPut, ts.URL+"/teammates/"+strconv.FormatInt(bot.ID, 10), `{"name":"sam","persona":"Checks examples.","backend":"claude","tools":["Read","Grep"],"write":true}`)
	if status != http.StatusOK {
		t.Fatalf("update profile = %d %s", status, body)
	}
	status, body = do(t, http.MethodGet, ts.URL+"/spaces/"+strconv.FormatInt(space.ID, 10)+"/members", "")
	if status != http.StatusOK || !strings.Contains(body, `"tools":["Read","Grep"]`) || !strings.Contains(body, `"write":true`) || !strings.Contains(body, `"persona":"Checks examples."`) {
		t.Errorf("Space members = %d %s, want updated profile", status, body)
	}
}
