package server

import (
	"context"
	"net/http"
	"net/http/httptest"
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
			`{"name":"sam","persona":"You test.","skills":["how"],"backend":"claude","face":"otter","tools":["Read"],"write":true,"permissionMode":"acceptEdits"}`,
			201, `{"id":3,"name":"sam","persona":"You test.","skills":["how"],"backend":"claude","face":"otter","tools":["Read"],"mcp":[],"write":true,"permissionMode":"acceptEdits"}`},
		{"join", http.MethodPost, "/teams/core/members", `{"teammateId":3}`,
			201, `{"teammateId":3,"name":"sam","lead":false,"write":true,"backend":"claude","permissionMode":"acceptEdits","persona":"You test.","skills":["how"],"face":"otter","tools":["Read"],"mcp":[]}`},
		{"no conversations", http.MethodGet, "/teammates/3/conversations", "", 200, `[]`},
		{"conversations of nobody", http.MethodGet, "/teammates/99/conversations", "", 404,
			`{"error":"teammate 99: not found"}`},
		{"edit defaults", http.MethodPut, "/teammates/3",
			`{"name":"sam","persona":"You test.","skills":["how"],"backend":"claude","face":"otter","tools":["Read","Grep"],"write":false}`,
			200, `{"id":3,"name":"sam","persona":"You test.","skills":["how"],"backend":"claude","face":"otter","tools":["Read","Grep"],"mcp":[],"write":false,"permissionMode":""}`},
		{"membership kept its copy", http.MethodGet, "/teams/core", "", 200,
			`{"name":"core","roles":[{"teammateId":2,"name":"boss","lead":true,"write":false,"backend":"claude","permissionMode":"","persona":"","skills":[],"tools":[],"mcp":[]},` +
				`{"teammateId":3,"name":"sam","lead":false,"write":true,"backend":"claude","permissionMode":"acceptEdits","persona":"You test.","skills":["how"],"face":"otter","tools":["Read"],"mcp":[]}]}`},
		{"edit membership", http.MethodPut, "/teams/core/members/3",
			`{"lead":false,"tools":["Read","Edit"],"write":true,"permissionMode":"acceptEdits"}`,
			200, `{"teammateId":3,"name":"sam","lead":false,"write":true,"backend":"claude","permissionMode":"acceptEdits","persona":"You test.","skills":["how"],"face":"otter","tools":["Read","Edit"],"mcp":[]}`},
		{"second lead", http.MethodPut, "/teams/core/members/3",
			`{"lead":true,"tools":[],"write":false,"permissionMode":""}`,
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
