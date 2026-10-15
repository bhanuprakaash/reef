package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/store"
)

func TestTeamsOmitMCPServers(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	_, err = st.ApplyTeam(context.Background(), store.Team{Name: "keys", Roles: []store.Role{{
		Name: "lead", Lead: true, Backend: "claude", MCPServers: []agent.MCPServer{
			{Command: "my-mcp", Args: []string{"--key", "sk-secret-123"}},
		},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	h := New(st, t.TempDir(), "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)

	for _, path := range []string{"/teams", "/teams/keys"} {
		status, body := do(t, http.MethodGet, ts.URL+path, "")
		if status != 200 || !strings.Contains(body, `"mcp":[{"name":"mcp1","label":"my-mcp"}]`) ||
			strings.Contains(body, "mcpServers") || strings.Contains(body, "sk-secret-123") {
			t.Errorf("GET %s = %d %s, want the lead with an mcp label and no mcpServers", path, status, body)
		}
	}
	if got, err := st.Team(context.Background(), "keys"); err != nil ||
		got.Roles[0].MCPServers[0].Args[1] != "sk-secret-123" {
		t.Errorf("stored team = %+v, %v, want the mcp server kept for the agent", got, err)
	}
}

func TestTeamsReturnFace(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	_, err = st.ApplyTeam(context.Background(), store.Team{Name: "crew", Roles: []store.Role{
		{Name: "lead", Lead: true, Backend: "claude", Face: "otter"},
		{Name: "tester", Backend: "claude"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	h := New(st, t.TempDir(), "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)

	_, body := do(t, http.MethodGet, ts.URL+"/teams/crew", "")
	if !strings.Contains(body, `"face":"otter"`) || strings.Count(body, `"face"`) != 1 {
		t.Errorf("GET /teams/crew = %s, want face otter on the lead only", body)
	}
}

func TestPutTeamModel(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h := New(st, t.TempDir(), "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)

	code, body := do(t, http.MethodPut, ts.URL+"/teams/crew", `{"roles":[
		{"name":"lead","lead":true,"backend":"claude","model":"opus"},
		{"name":"developer","backend":"codex"}]}`)
	if code != http.StatusOK || !strings.Contains(body, `"model":"opus"`) || strings.Count(body, `"model"`) != 1 {
		t.Errorf("PUT /teams/crew = %d %s, want model opus on the lead only", code, body)
	}
	_, body = do(t, http.MethodGet, ts.URL+"/teams/crew", "")
	if !strings.Contains(body, `"model":"opus"`) || strings.Count(body, `"model"`) != 1 {
		t.Errorf("GET /teams/crew = %s, want model opus on the lead only", body)
	}
}

func TestChannelTeam(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	_, err = st.ApplyTeam(context.Background(), store.Team{Name: "core", Roles: []store.Role{
		{Name: "lead", Lead: true, Backend: "codex", PermissionMode: "read-only"},
		{Name: "architect", Write: true, Backend: "claude"},
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
	want := `{"name":"core","roles":[{"name":"lead","lead":true,"write":false,"backend":"codex",` +
		`"permissionMode":"read-only","persona":"","skills":[],"tools":[],"mcp":[]},` +
		`{"name":"architect","lead":false,"write":true,"backend":"claude","permissionMode":"","persona":"",` +
		`"skills":[],"tools":[],"mcp":[]}]}`
	if status != 200 || body != want {
		t.Errorf("GET /teams/core = %d %s, want %s", status, body, want)
	}
	if status, _ := do(t, http.MethodGet, ts.URL+"/teams/nope", ""); status != 404 {
		t.Errorf("GET /teams/nope = %d, want 404", status)
	}
}

func TestPutTeam(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	_, err = st.ApplyTeam(context.Background(), store.Team{Name: "crew", Roles: []store.Role{
		{Name: "lead", Lead: true, Backend: "claude", Persona: "old persona", MCPServers: []agent.MCPServer{
			{Command: "my-mcp", Args: []string{"--key", "sk-secret-456"}},
		}},
		{Name: "developer", Backend: "claude", Write: true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	h := New(st, t.TempDir(), "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)

	// A body that omits mcpServers keeps the lead's stored one and updates the persona.
	body := `{"name":"crew","roles":[` +
		`{"name":"lead","lead":true,"write":false,"backend":"claude","permissionMode":"","persona":"new persona","skills":[],"tools":[]},` +
		`{"name":"developer","lead":false,"write":true,"backend":"claude","permissionMode":"","persona":"","skills":[],"tools":[]}]}`
	status, resBody := do(t, http.MethodPut, ts.URL+"/teams/crew", body)
	if status != 200 || strings.Contains(resBody, "mcpServers") {
		t.Fatalf("PUT /teams/crew = %d %s, want 200 without mcpServers", status, resBody)
	}
	got, err := st.Team(context.Background(), "crew")
	if err != nil {
		t.Fatal(err)
	}
	if got.Roles[0].Persona != "new persona" {
		t.Errorf("lead persona = %q, want %q", got.Roles[0].Persona, "new persona")
	}
	if len(got.Roles[0].MCPServers) != 1 || got.Roles[0].MCPServers[0].Args[1] != "sk-secret-456" {
		t.Errorf("lead mcpServers = %+v, want the stored one kept", got.Roles[0].MCPServers)
	}

	// The same error as `reef team apply`: two leads.
	status, resBody = do(t, http.MethodPut, ts.URL+"/teams/crew", `{"roles":[`+
		`{"name":"lead","lead":true,"backend":"claude"},{"name":"developer","lead":true,"backend":"claude"}]}`)
	want := `{"error":"invalid: team crew needs exactly one lead role, has 2"}`
	if status != 400 || resBody != want {
		t.Errorf("PUT /teams/crew two leads = %d %s, want 400 %s", status, resBody, want)
	}

	// The same error as `reef team apply`: unknown backend.
	status, resBody = do(t, http.MethodPut, ts.URL+"/teams/crew", `{"roles":[{"name":"lead","lead":true,"backend":"gemini"}]}`)
	want = `{"error":"role lead: invalid: backend must be claude or codex"}`
	if status != 400 || resBody != want {
		t.Errorf("PUT /teams/crew bad backend = %d %s, want 400 %s", status, resBody, want)
	}

	// A body name that does not match the path is invalid.
	status, _ = do(t, http.MethodPut, ts.URL+"/teams/crew", `{"name":"other","roles":[{"name":"lead","lead":true,"backend":"claude"}]}`)
	if status != 400 {
		t.Errorf("PUT /teams/crew with mismatched name = %d, want 400", status)
	}

	// A name with a space or a slash, as the URL-encoded path sends it, is a clear invalid error.
	roleBody := `{"roles":[{"name":"lead","lead":true,"backend":"claude"}]}`
	for _, path := range []string{"/teams/a%20b", "/teams/a%2Fb"} {
		status, resBody = do(t, http.MethodPut, ts.URL+path, roleBody)
		if status != 400 || !strings.Contains(resBody, "must be letters, digits, _ or -") {
			t.Errorf("PUT %s = %d %s, want 400 naming the allowed characters", path, status, resBody)
		}
	}
}

func TestPutTeamRemoveMCP(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	servers := func() []agent.MCPServer {
		return []agent.MCPServer{
			{Command: "first-mcp", Args: []string{"--key", "sk-secret-1"}},
			{URL: "https://mcp.example.com/sse?key=sk-secret-2"},
		}
	}
	reset := func() {
		t.Helper()
		_, err := st.ApplyTeam(context.Background(), store.Team{Name: "crew", Roles: []store.Role{
			{Name: "lead", Lead: true, Backend: "claude", MCPServers: servers()},
		}})
		if err != nil {
			t.Fatal(err)
		}
	}
	stored := func() []agent.MCPServer {
		t.Helper()
		got, err := st.Team(context.Background(), "crew")
		if err != nil {
			t.Fatal(err)
		}
		return got.Roles[0].MCPServers
	}
	h := New(st, t.TempDir(), "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)
	put := func(remove string) (int, string) {
		return do(t, http.MethodPut, ts.URL+"/teams/crew",
			`{"roles":[{"name":"lead","lead":true,"backend":"claude","mcpRemove":`+remove+`}]}`)
	}

	// Removing mcp1 by name and label drops it and keeps the url server with its config.
	reset()
	status, resBody := put(`[{"name":"mcp1","label":"first-mcp"}]`)
	if status != 200 || !strings.Contains(resBody, `"mcp":[{"name":"mcp1","label":"mcp.example.com"}]`) ||
		strings.Contains(resBody, "sk-secret") {
		t.Fatalf("PUT /teams/crew = %d %s, want one mcp labelled mcp.example.com", status, resBody)
	}
	if got := stored(); len(got) != 1 || got[0].URL != "https://mcp.example.com/sse?key=sk-secret-2" {
		t.Errorf("lead mcpServers = %+v, want only the url server", got)
	}

	// A stale label is a conflict and changes nothing.
	reset()
	status, resBody = put(`[{"name":"mcp1","label":"other-mcp"}]`)
	want := `{"error":"conflict: MCP servers changed since you loaded the team; reload"}`
	if status != 409 || resBody != want {
		t.Errorf("PUT /teams/crew stale label = %d %s, want 409 %s", status, resBody, want)
	}
	if got := stored(); len(got) != 2 {
		t.Errorf("lead mcpServers = %+v, want both kept", got)
	}

	// A server added after the window loaded the team survives the removal.
	reset()
	_, resBody = do(t, http.MethodGet, ts.URL+"/teams/crew", "")
	if !strings.Contains(resBody, `"mcp":[{"name":"mcp1","label":"first-mcp"},{"name":"mcp2","label":"mcp.example.com"}]`) {
		t.Fatalf("GET /teams/crew = %s, want two mcp labels", resBody)
	}
	added := append(servers(), agent.MCPServer{Command: "third-mcp", Args: []string{"--key", "sk-secret-3"}})
	_, err = st.ApplyTeam(context.Background(), store.Team{Name: "crew", Roles: []store.Role{
		{Name: "lead", Lead: true, Backend: "claude", MCPServers: added},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if status, resBody = put(`[{"name":"mcp1","label":"first-mcp"}]`); status != 200 {
		t.Fatalf("PUT /teams/crew = %d %s, want 200", status, resBody)
	}
	if got := stored(); len(got) != 2 || got[0].URL != "https://mcp.example.com/sse?key=sk-secret-2" ||
		got[1].Args[1] != "sk-secret-3" {
		t.Errorf("lead mcpServers = %+v, want the url and third servers", got)
	}

	// Removing mcp1 and mcp2 together removes exactly those two.
	reset()
	_, err = st.ApplyTeam(context.Background(), store.Team{Name: "crew", Roles: []store.Role{
		{Name: "lead", Lead: true, Backend: "claude", MCPServers: added},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if status, resBody = put(`[{"name":"mcp1","label":"first-mcp"},{"name":"mcp2","label":"mcp.example.com"}]`); status != 200 {
		t.Fatalf("PUT /teams/crew = %d %s, want 200", status, resBody)
	}
	if got := stored(); len(got) != 1 || got[0].Command != "third-mcp" {
		t.Errorf("lead mcpServers = %+v, want only the third server", got)
	}
}

func TestPutTeamCopyFrom(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	_, err = st.ApplyTeam(context.Background(), store.Team{Name: "crew", Roles: []store.Role{
		{Name: "lead", Lead: true, Backend: "claude", MCPServers: []agent.MCPServer{
			{Command: "my-mcp", Args: []string{"--key", "sk-secret-789"}},
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	h := New(st, t.TempDir(), "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)

	body := `{"roles":[{"name":"lead","lead":true,"write":false,"backend":"claude","permissionMode":"",` +
		`"persona":"","skills":[],"tools":[]}]}`
	status, resBody := do(t, http.MethodPut, ts.URL+"/teams/crew2?from=crew", body)
	if status != 200 || strings.Contains(resBody, "mcpServers") {
		t.Fatalf("PUT /teams/crew2?from=crew = %d %s, want 200 without mcpServers", status, resBody)
	}
	got, err := st.Team(context.Background(), "crew2")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Roles[0].MCPServers) != 1 || got.Roles[0].MCPServers[0].Args[1] != "sk-secret-789" {
		t.Errorf("copy's lead mcpServers = %+v, want the source's kept", got.Roles[0].MCPServers)
	}

	// A copy over a name that already exists is a conflict, not a silent replace.
	status, resBody = do(t, http.MethodPut, ts.URL+"/teams/crew2?from=crew", body)
	want := `{"error":"conflict: a team named \"crew2\" already exists"}`
	if status != 409 || resBody != want {
		t.Errorf("PUT /teams/crew2?from=crew again = %d %s, want 409 %s", status, resBody, want)
	}

	// Copying from a team that does not exist is not found, not a team made without servers.
	status, resBody = do(t, http.MethodPut, ts.URL+"/teams/crew3?from=nope", body)
	want = `{"error":"not found: team \"nope\" not found"}`
	if status != 404 || resBody != want {
		t.Errorf("PUT /teams/crew3?from=nope = %d %s, want 404 %s", status, resBody, want)
	}
	if _, err := st.Team(context.Background(), "crew3"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("team crew3 = %v, want ErrNotFound (not created)", err)
	}
}

// Two concurrent copies to the same new name must not both succeed: the loser gets 409.
func TestPutTeamCopyFrom_Concurrent(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	_, err = st.ApplyTeam(context.Background(), store.Team{Name: "crew", Roles: []store.Role{
		{Name: "lead", Lead: true, Backend: "claude"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	h := New(st, t.TempDir(), "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)

	body := `{"roles":[{"name":"lead","lead":true,"write":false,"backend":"claude","permissionMode":"",` +
		`"persona":"","skills":[],"tools":[]}]}`
	put := func() (int, error) {
		req, err := http.NewRequest(http.MethodPut, ts.URL+"/teams/race?from=crew", strings.NewReader(body))
		if err != nil {
			return 0, err
		}
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return 0, err
		}
		defer func() { _ = res.Body.Close() }()
		return res.StatusCode, nil
	}
	statuses := make([]int, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range statuses {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			statuses[i], errs[i] = put()
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	oks, conflicts := 0, 0
	for _, status := range statuses {
		switch status {
		case 200:
			oks++
		case 409:
			conflicts++
		}
	}
	if oks != 1 || conflicts != 1 {
		t.Errorf("concurrent copy statuses = %v, want exactly one 200 and one 409", statuses)
	}
}
