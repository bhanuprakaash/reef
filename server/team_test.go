package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

func TestChannelTeam(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	_, err = st.ApplyTeam(context.Background(), store.Team{Name: "core", Roles: []store.Role{
		{Name: "lead", Lead: true, Backend: "codex", PermissionMode: "read-only"},
		{Name: "architect", Backend: "claude"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.ApplyTeam(context.Background(), store.Team{Name: "crew", Roles: []store.Role{
		{Name: "boss", Lead: true, Backend: "claude"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	h := New(st, t.TempDir(), "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)
	do(t, http.MethodPost, ts.URL+"/projects", `{"name":"reef","path":"/tmp"}`)

	tests := []struct {
		name       string
		give       string
		wantStatus int
		wantIn     string
	}{
		{"no team is solo", `{"projectId":1,"name":"a"}`, 201,
			`"backend":"claude","permissionMode":"","paused":"","tokenBudget":1000000,"team":"solo"`},
		{"team lead sets defaults", `{"projectId":1,"name":"b","team":"core"}`, 201,
			`"backend":"codex","permissionMode":"read-only","paused":"","tokenBudget":1000000,"team":"core"`},
		{"request overrides lead", `{"projectId":1,"name":"c","team":"core","backend":"claude"}`, 201,
			`"backend":"claude","permissionMode":"","paused":"","tokenBudget":1000000,"team":"core"`},
		{"lead role names the channel", `{"projectId":1,"name":"e","team":"crew"}`, 201,
			`"name":"e","role":"boss"`},
		{"unknown team", `{"projectId":1,"name":"d","team":"nope"}`, 400,
			`{"error":"invalid: unknown team nope"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := do(t, http.MethodPost, ts.URL+"/channels", tt.give)
			if status != tt.wantStatus || !strings.Contains(body, tt.wantIn) {
				t.Errorf("POST /channels = %d %s, want %d containing %s",
					status, body, tt.wantStatus, tt.wantIn)
			}
		})
	}

	status, body := do(t, http.MethodGet, ts.URL+"/teams/core", "")
	want := `{"name":"core","roles":[{"name":"lead","lead":true,"backend":"codex",` +
		`"permission_mode":"read-only","persona":"","skills":"","tools":[],"mcp_servers":[]},` +
		`{"name":"architect","lead":false,"backend":"claude","permission_mode":"","persona":"",` +
		`"skills":"","tools":[],"mcp_servers":[]}]}`
	if status != 200 || body != want {
		t.Errorf("GET /teams/core = %d %s, want %s", status, body, want)
	}
	if status, _ := do(t, http.MethodGet, ts.URL+"/teams/nope", ""); status != 404 {
		t.Errorf("GET /teams/nope = %d, want 404", status)
	}
}
