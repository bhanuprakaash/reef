package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/store"
)

func TestTeamsOmitMCPServers(t *testing.T) {
	st, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	_, err = testServerTeam(t, st, context.Background(), store.Team{Name: "keys", Roles: []store.Role{{
		Name: "lead", Lead: true, Backend: "claude", MCPServers: []agent.MCPServer{
			{Command: "my-mcp", Args: []string{"--key", "sk-secret-123"}},
		},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	h := New(st, testMemory(t, st), t.TempDir(), "/bin/reef")
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

func TestSpaceMembersOmitMCPServers(t *testing.T) {
	st, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	_, err = testServerTeam(t, st, ctx, store.Team{Name: "keys", Roles: []store.Role{{
		Name: "lead", Lead: true, Backend: "claude", MCPServers: []agent.MCPServer{
			{Command: "my-mcp", Env: map[string]string{"API_KEY": "sk-secret-123"}},
		},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateProject(ctx, store.Project{Name: "reef", Path: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateSpace(ctx, store.Space{ProjectID: 1, Name: "talk", Team: "keys"}); err != nil {
		t.Fatal(err)
	}
	h := New(st, testMemory(t, st), t.TempDir(), "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)

	status, body := do(t, http.MethodGet, ts.URL+"/spaces/1/members", "")
	if status != 200 || !strings.Contains(body, `"mcp":[{"name":"mcp1","label":"my-mcp"}]`) ||
		strings.Contains(body, "mcpServers") || strings.Contains(body, "sk-secret-123") {
		t.Errorf("GET members = %d %s, want the lead with an mcp label and no env", status, body)
	}
}

func TestTeamsReturnFace(t *testing.T) {
	st, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	_, err = testServerTeam(t, st, context.Background(), store.Team{Name: "crew", Roles: []store.Role{
		{Name: "lead", Lead: true, Backend: "claude", Face: "otter"},
		{Name: "tester", Backend: "claude"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	h := New(st, testMemory(t, st), t.TempDir(), "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)

	_, body := do(t, http.MethodGet, ts.URL+"/teams/crew", "")
	if !strings.Contains(body, `"face":"otter"`) || strings.Count(body, `"face"`) != 1 {
		t.Errorf("GET /teams/crew = %s, want face otter on the lead only", body)
	}
}

func TestCreateUpdateAndDeleteTeam(t *testing.T) {
	st, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	developer, err := st.CreateBot(context.Background(), store.Bot{
		Name: "developer", Backend: "claude", Write: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if developer.ID != 2 {
		t.Fatalf("developer id = %d, want 2", developer.ID)
	}
	h := New(st, testMemory(t, st), t.TempDir(), "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)

	status, body := do(t, http.MethodPost, ts.URL+"/teams", `{
		"name":"release","responsibility":"Ship Reef safely.",
		"roles":[{"botId":1,"lead":true},{"botId":2}]}`)
	if status != http.StatusCreated || !strings.Contains(body, `"responsibility":"Ship Reef safely."`) {
		t.Fatalf("POST /teams = %d %s, want 201 with responsibility", status, body)
	}
	status, body = do(t, http.MethodPut, ts.URL+"/teams/release", `{
		"responsibility":"Own releases and rollback.",
		"roles":[{"botId":1,"lead":true},{"botId":2}]}`)
	if status != http.StatusOK || !strings.Contains(body, `"responsibility":"Own releases and rollback."`) {
		t.Fatalf("PUT /teams/release = %d %s, want 200 with updated responsibility", status, body)
	}
	status, body = do(t, http.MethodGet, ts.URL+"/teams/release", "")
	if status != http.StatusOK || !strings.Contains(body, `"responsibility":"Own releases and rollback."`) {
		t.Errorf("GET /teams/release = %d %s, want updated responsibility", status, body)
	}
	status, body = do(t, http.MethodDelete, ts.URL+"/teams/release", "")
	if status != http.StatusNoContent || body != "" {
		t.Errorf("DELETE /teams/release = %d %s, want 204", status, body)
	}
}

func TestDeleteTeam_KeepsWorkItem(t *testing.T) {
	ctx := context.Background()
	st, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := testServerCreateTeam(t, st, ctx, store.Team{Name: "release", Roles: []store.Role{
		{Name: "lead", Lead: true, Backend: "claude"},
	}}); err != nil {
		t.Fatal(err)
	}
	project, err := st.CreateProject(ctx, store.Project{Name: "reef", Path: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	var teamID int64
	if err := st.DB().QueryRowContext(ctx, "SELECT id FROM team WHERE name = 'release'").
		Scan(&teamID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateWorkItem(ctx, store.WorkItem{
		ProjectID: project.ID,
		Title:     "Ship desktop updater",
		Status:    "queued",
		TeamID:    &teamID,
	}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodDelete, "/teams/release", nil)
	req.SetPathValue("name", "release")
	res := httptest.NewRecorder()
	(&server{st: st}).deleteTeam(res, req)
	status, body := res.Code, strings.TrimSpace(res.Body.String())
	if status != http.StatusNoContent || body != "" {
		t.Errorf("DELETE /teams/release = %d %s, want 204", status, body)
	}
}

func TestSpaceTeam(t *testing.T) {
	st, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	_, err = testServerTeam(t, st, context.Background(), store.Team{Name: "core", Roles: []store.Role{
		{Name: "core-lead", Lead: true, Backend: "codex", PermissionMode: "read-only"},
		{Name: "core-architect", Write: true, Backend: "claude"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = testServerTeam(t, st, context.Background(), store.Team{Name: "crew", Roles: []store.Role{
		{Name: "crew-boss", Lead: true, Backend: "claude"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	h := New(st, testMemory(t, st), t.TempDir(), "/bin/reef")
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
			`"role":"lead","botId":1,"paused":"","tokenBudget":1000000,"team":"solo"`},
		{"team lead leads the space", `{"projectId":1,"name":"b","team":"core"}`, 201,
			`"role":"core-lead","botId":2,"paused":"","tokenBudget":1000000,"team":"core"`},
		{"lead role names the space", `{"projectId":1,"name":"e","team":"crew"}`, 201,
			`"name":"e","role":"crew-boss"`},
		{"unknown team", `{"projectId":1,"name":"d","team":"nope"}`, 400,
			`{"error":"invalid: unknown team nope"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := do(t, http.MethodPost, ts.URL+"/spaces", tt.give)
			if status != tt.wantStatus || !strings.Contains(body, tt.wantIn) {
				t.Errorf("POST /spaces = %d %s, want %d containing %s",
					status, body, tt.wantStatus, tt.wantIn)
			}
		})
	}

	status, body := do(t, http.MethodGet, ts.URL+"/teams/core", "")
	want := `{"id":2,"name":"core","responsibility":"","roles":[{"botId":2,"name":"core-lead","lead":true,"write":false,"backend":"codex",` +
		`"permissionMode":"read-only","description":"","skills":[],"tools":[],"mcp":[]},` +
		`{"botId":3,"name":"core-architect","lead":false,"write":true,"backend":"claude","permissionMode":"","description":"",` +
		`"skills":[],"tools":[],"mcp":[]}]}`
	if status != 200 || body != want {
		t.Errorf("GET /teams/core = %d %s, want %s", status, body, want)
	}
	if status, _ := do(t, http.MethodGet, ts.URL+"/teams/nope", ""); status != 404 {
		t.Errorf("GET /teams/nope = %d, want 404", status)
	}
}

func TestPutTeam(t *testing.T) {
	st, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	_, err = testServerTeam(t, st, context.Background(), store.Team{Name: "crew", Roles: []store.Role{
		{Name: "crew-lead", Lead: true, Backend: "claude", Description: "old description", MCPServers: []agent.MCPServer{
			{Command: "my-mcp", Args: []string{"--key", "sk-secret-456"}},
		}},
		{Name: "crew-developer", Backend: "claude", Write: true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	h := New(st, testMemory(t, st), t.TempDir(), "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)

	// A team body names Bot ids and a lead; the Bots keep their profile.
	status, resBody := do(t, http.MethodPut, ts.URL+"/teams/crew",
		`{"name":"crew","roles":[{"botId":2,"lead":true}]}`)
	if status != 200 || strings.Contains(resBody, "mcpServers") || !strings.Contains(resBody, `"description":"old description"`) {
		t.Fatalf("PUT /teams/crew = %d %s, want 200 with the lead's own description and no mcpServers", status, resBody)
	}
	got, err := st.Team(context.Background(), "crew")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Roles) != 1 || len(got.Roles[0].MCPServers) != 1 || got.Roles[0].MCPServers[0].Args[1] != "sk-secret-456" {
		t.Errorf("crew roles = %+v, want the lead with its stored mcpServers", got.Roles)
	}

	// The same error as `reef team apply`: two leads.
	status, resBody = do(t, http.MethodPut, ts.URL+"/teams/crew", `{"roles":[{"botId":2,"lead":true},{"botId":3,"lead":true}]}`)
	want := `{"error":"invalid: team crew has 2 coordinator suggestions, at most one"}`
	if status != 400 || resBody != want {
		t.Errorf("PUT /teams/crew two leads = %d %s, want 400 %s", status, resBody, want)
	}

	// The same error as `reef team apply`: unknown bot.
	status, resBody = do(t, http.MethodPut, ts.URL+"/teams/crew", `{"roles":[{"botId":99,"lead":true}]}`)
	want = `{"error":"invalid: unknown bot 99"}`
	if status != 400 || resBody != want {
		t.Errorf("PUT /teams/crew unknown bot = %d %s, want 400 %s", status, resBody, want)
	}

	// Per-member configuration is refused.
	status, _ = do(t, http.MethodPut, ts.URL+"/teams/crew", `{"roles":[{"botId":2,"lead":true,"write":true}]}`)
	if status != 400 {
		t.Errorf("PUT /teams/crew with member config = %d, want 400", status)
	}

	// A body name that does not match the path is invalid.
	status, _ = do(t, http.MethodPut, ts.URL+"/teams/crew", `{"name":"other","roles":[{"botId":2,"lead":true}]}`)
	if status != 400 {
		t.Errorf("PUT /teams/crew with mismatched name = %d, want 400", status)
	}

	// A name with a space or a slash, as the URL-encoded path sends it, is a clear invalid error.
	roleBody := `{"roles":[{"botId":2,"lead":true}]}`
	for _, path := range []string{"/teams/a%20b", "/teams/a%2Fb"} {
		status, resBody = do(t, http.MethodPut, ts.URL+path, roleBody)
		if status != 400 || !strings.Contains(resBody, "must be letters, digits, _ or -") {
			t.Errorf("PUT %s = %d %s, want 400 naming the allowed characters", path, status, resBody)
		}
	}
}

func TestPutTeamRejectsMembershipMCPRemoval(t *testing.T) {
	st, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h := New(st, testMemory(t, st), t.TempDir(), "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)
	status, body := do(t, http.MethodPut, ts.URL+"/teams/crew", `{"roles":[{"botId":1,"lead":true,"mcpRemove":[]}]}`)
	if status != http.StatusBadRequest || !strings.Contains(body, "unknown field") {
		t.Errorf("membership MCP removal = %d %s, want bad request", status, body)
	}
}

func TestPutTeamCopyFrom(t *testing.T) {
	st, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	_, err = testServerTeam(t, st, context.Background(), store.Team{Name: "crew", Roles: []store.Role{
		{Name: "crew-lead", Lead: true, Backend: "claude", MCPServers: []agent.MCPServer{
			{Command: "my-mcp", Args: []string{"--key", "sk-secret-789"}},
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	h := New(st, testMemory(t, st), t.TempDir(), "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)

	body := `{"roles":[{"botId":2,"lead":true}]}`
	status, resBody := do(t, http.MethodPut, ts.URL+"/teams/crew2?from=crew", body)
	if status != 200 || strings.Contains(resBody, "mcpServers") {
		t.Fatalf("PUT /teams/crew2?from=crew = %d %s, want 200 without mcpServers", status, resBody)
	}
	got, err := st.Team(context.Background(), "crew2")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Roles[0].MCPServers) != 1 || got.Roles[0].MCPServers[0].Args[1] != "sk-secret-789" {
		t.Errorf("copy's lead mcpServers = %+v, want the Bot's kept", got.Roles[0].MCPServers)
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
	st, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	_, err = testServerTeam(t, st, context.Background(), store.Team{Name: "crew", Roles: []store.Role{
		{Name: "crew-lead", Lead: true, Backend: "claude"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	h := New(st, testMemory(t, st), t.TempDir(), "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)

	body := `{"roles":[{"botId":2,"lead":true}]}`
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

func TestPutBotSkills(t *testing.T) {
	st, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h := New(st, testMemory(t, st), t.TempDir(), "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)

	tests := []struct {
		name   string
		skills string
		want   int
	}{
		{"a string", `"x"`, http.StatusBadRequest},
		{"no source", `["how"]`, http.StatusBadRequest},
		{"an empty name", `["claude:how",""]`, http.StatusBadRequest},
		{"a blank name", `["claude:how"," "]`, http.StatusBadRequest},
		{"a duplicate", `["claude:how","claude:how"]`, http.StatusBadRequest},
		{"a duplicate after trimming", `["claude:how"," claude:how"]`, http.StatusBadRequest},
		{"a space inside", `["how why"]`, http.StatusBadRequest},
		{"a slash", `["a/b"]`, http.StatusBadRequest},
		{"a dot", `["v1.2"]`, http.StatusBadRequest},
		{"a parenthesis", `["Bash(rm:*)"]`, http.StatusBadRequest},
		{"clean names", `["claude:how","claude:plugin:skill","reef:a_b-1"]`, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := `{"name":"lead","backend":"claude","skills":` + tt.skills + `}`
			if status, resBody := do(t, http.MethodPut, ts.URL+"/bots/1", body); status != tt.want {
				t.Errorf("PUT skills %s = %d %s, want %d", tt.skills, status, resBody, tt.want)
			}
		})
	}

	body := `{"name":"lead","backend":"claude","skills":[" claude:how ","claude:why"]}`
	if status, resBody := do(t, http.MethodPut, ts.URL+"/bots/1", body); status != http.StatusOK ||
		!strings.Contains(resBody, `"skills":["claude:how","claude:why"]`) {
		t.Errorf("PUT skills with a padded name = %d %s, want 200 and the trimmed names", status, resBody)
	}
}

func TestCrewRosterOnly(t *testing.T) {
	ts, st := newEmbedServer(t, nil)
	ctx := context.Background()
	ann, bob, cy := *testServerBot(t, st, "ann"), *testServerBot(t, st, "bob"), *testServerBot(t, st, "cy")
	project, err := st.CreateProject(ctx, store.Project{Name: "reef", Path: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	post := func(path, body string) (int, string) { return do(t, http.MethodPost, ts.URL+path, body) }
	put := func(path, body string) (int, string) { return do(t, http.MethodPut, ts.URL+path, body) }

	// A Crew needs no coordinator suggestion.
	if status, body := post("/teams", fmt.Sprintf(
		`{"name":"duo","responsibility":"pair","roles":[{"botId":%d},{"botId":%d}]}`, ann, bob)); status != 201 {
		t.Fatalf("POST /teams = %d %s, want 201", status, body)
	}
	if status, body := post("/teams", fmt.Sprintf(
		`{"name":"trio","roles":[{"botId":%d,"lead":true},{"botId":%d},{"botId":%d}]}`, bob, ann, cy)); status != 201 {
		t.Fatalf("POST /teams = %d %s, want 201", status, body)
	}

	// Adding a Crew puts in only the missing Bots and keeps the coordinator.
	status, body := post("/spaces", fmt.Sprintf(`{"projectId":%d,"name":"s","team":"duo"}`, project.ID))
	if status != 201 {
		t.Fatalf("POST /spaces = %d %s, want 201", status, body)
	}
	status, body = post("/spaces/1/members", `{"crew":"trio"}`)
	if status != 200 || strings.Count(body, `"lead":true`) != 1 || strings.Count(body, `"botId"`) != 3 ||
		!strings.HasPrefix(body, fmt.Sprintf(`[{"botId":%d,"name":"ann","lead":true`, ann)) {
		t.Errorf("POST /spaces/1/members crew = %d %s, want ann still leading, three members", status, body)
	}

	// A start with a user-adjusted roster and coordinator.
	status, body = post("/spaces", fmt.Sprintf(
		`{"projectId":%d,"name":"t","team":"trio","botIds":[%d,%d,%d],"coordinatorBotId":%d}`,
		project.ID, bob, ann, bob, ann))
	if status != 201 {
		t.Fatalf("POST /spaces adjusted = %d %s, want 201", status, body)
	}
	status, body = do(t, http.MethodGet, ts.URL+"/spaces/2/members", "")
	if status != 200 || strings.Count(body, `"botId"`) != 2 ||
		!strings.HasPrefix(body, fmt.Sprintf(`[{"botId":%d,"name":"ann","lead":true`, ann)) {
		t.Errorf("GET /spaces/2/members = %d %s, want ann leading and bob", status, body)
	}

	// Changing and deleting a Crew leaves the Bots and the Space members.
	if status, body := put("/teams/trio", fmt.Sprintf(`{"roles":[{"botId":%d}]}`, cy)); status != 200 {
		t.Fatalf("PUT /teams/trio = %d %s, want 200", status, body)
	}
	if status, body := do(t, http.MethodDelete, ts.URL+"/teams/duo", ""); status != 204 {
		t.Fatalf("DELETE /teams/duo = %d %s, want 204", status, body)
	}
	status, body = do(t, http.MethodGet, ts.URL+"/spaces/1/members", "")
	if status != 200 || strings.Count(body, `"botId"`) != 3 {
		t.Errorf("GET /spaces/1/members after = %d %s, want three members", status, body)
	}
	if status, body = do(t, http.MethodGet, ts.URL+"/bots", ""); status != 200 || !strings.Contains(body, `"name":"ann"`) {
		t.Errorf("GET /bots after = %d %s, want ann kept", status, body)
	}
}

func TestSpaceCoordinatorChange(t *testing.T) {
	ts, st := newEmbedServer(t, nil)
	ctx := context.Background()
	ann, bob, cy := *testServerBot(t, st, "ann"), *testServerBot(t, st, "bob"), *testServerBot(t, st, "cy")
	post := func(path, body string) (int, string) { return do(t, http.MethodPost, ts.URL+path, body) }
	put := func(path, body string) (int, string) { return do(t, http.MethodPut, ts.URL+path, body) }
	status, body := post("/spaces", fmt.Sprintf(`{"name":"s","botIds":[%d,%d]}`, ann, bob))
	if status != 201 {
		t.Fatalf("POST /spaces = %d %s, want 201", status, body)
	}
	if !strings.Contains(body, `"team":""`) {
		t.Errorf("POST /spaces of Bots = %s, want no Crew", body)
	}

	status, body = put("/spaces/1/coordinator", fmt.Sprintf(`{"botId":%d}`, bob))
	if status != 200 || strings.Count(body, `"lead":true`) != 1 ||
		!strings.HasPrefix(body, fmt.Sprintf(`[{"botId":%d,"name":"bob","lead":true`, bob)) {
		t.Errorf("PUT coordinator = %d %s, want bob the only coordinator", status, body)
	}
	if status, body = put("/spaces/1/coordinator", fmt.Sprintf(`{"botId":%d}`, cy)); status != 400 {
		t.Errorf("PUT coordinator of a non-member = %d %s, want 400", status, body)
	}

	// A Bot added to a Space with no members becomes its coordinator.
	if _, err := st.CreateSpace(ctx, store.Space{Name: "empty"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB().ExecContext(ctx, "DELETE FROM space_member WHERE space_id = 2"); err != nil {
		t.Fatal(err)
	}
	if status, body = post("/spaces/2/members", fmt.Sprintf(`{"botId":%d}`, cy)); status != 201 ||
		!strings.Contains(body, `"lead":true`) {
		t.Errorf("POST first member = %d %s, want the coordinator", status, body)
	}
}
