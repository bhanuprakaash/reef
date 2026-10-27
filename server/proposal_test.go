package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

func TestWrap_RunsWrapTurn(t *testing.T) {
	env := newLiveEnv(t)
	id := env.workItemSpace(t)
	sub := subscribeTo(t, env.url, id)
	status, body := do(t, http.MethodPost, fmt.Sprintf("%s/spaces/%d/wrap", env.url, id), "")
	if status != 202 || body != "" {
		t.Fatalf("wrap = %d %q, want 202", status, body)
	}
	sub.want(t,
		_idleFrame,
		`turn {"role":"lead","status":"running"}`,
		`agent {"kind":"text","text":"wrapped"}`,
		`turn {"role":"lead","status":"done"}`,
	)
}

// gitProject makes the folder of project id a git repo with one commit, so a promote of one
// of its Spaces can run.
func gitProject(t *testing.T, env *liveEnv, id int64) {
	t.Helper()
	p, err := env.st.Project(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	gitIn(t, p.Path, "init", "-b", "main")
	writeIn(t, p.Path, "a.txt", "a")
	gitIn(t, p.Path, "add", ".")
	gitIn(t, p.Path, "commit", "-m", "first")
}

func TestPromote_NewWorkItem(t *testing.T) {
	env := newLiveEnv(t)
	gitProject(t, env, 1)
	sub := subscribe(t, env.url)
	status, body := do(t, http.MethodPost, env.url+"/spaces/1/promote",
		`{"title":"Login"}`)
	if status != 202 || !strings.Contains(body, `"workItemId":1`) {
		t.Fatalf("promote = %d %s, want 202 with workItemId 1", status, body)
	}
	sub.want(t,
		_idleFrame,
		`turn {"role":"lead","status":"running"}`,
		`agent {"kind":"text","text":"promoted"}`,
		`turn {"role":"lead","status":"done"}`,
	)
	status, body = do(t, http.MethodGet, env.url+"/work-items/1", "")
	if status != 200 || !strings.Contains(body, `"projectId":1,"parentId":null,"type":"feature","title":"Login"`) {
		t.Errorf("work item = %d %s, want title Login", status, body)
	}
	status, body = do(t, http.MethodGet, env.url+"/spaces/1", "")
	if status != 200 || !strings.Contains(body, `"workItemId":1`) {
		t.Errorf("space = %d %s, want workItemId 1", status, body)
	}
}

func TestPromote_NotAGitRepo(t *testing.T) {
	env := newLiveEnv(t)
	p, err := env.st.Project(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	status, body := do(t, http.MethodPost, env.url+"/spaces/1/promote", `{"title":"Login"}`)
	want := `{"error":"invalid: project ` + p.Path + ` is not a git repo with a commit"}`
	if status != 400 || body != want {
		t.Errorf("promote = %d %s, want 400 %s", status, body, want)
	}
	if ws, err := env.st.WorkItems(context.Background(), store.WorkItemFilter{}); err != nil || len(ws) != 0 {
		t.Errorf("work items = %+v, %v, want none after a refused promote", ws, err)
	}
}

func TestPromote_ExistingWorkItem(t *testing.T) {
	env := newLiveEnv(t)
	gitProject(t, env, 1)
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":1,"title":"Login"}`)
	status, body := do(t, http.MethodPost, env.url+"/spaces/1/promote", `{"workItemId":1}`)
	if status != 202 || !strings.Contains(body, `"workItemId":1`) {
		t.Errorf("promote = %d %s, want 202 with workItemId 1", status, body)
	}
}

func TestPromote_Errors(t *testing.T) {
	env := newLiveEnv(t)
	do(t, http.MethodPost, env.url+"/projects", `{"name":"other","path":"`+t.TempDir()+`"}`)
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":2,"title":"Billing"}`)
	tests := []struct {
		name       string
		path       string
		give       string
		wantStatus int
		wantBody   string
	}{
		{"no body fields", "/spaces/1/promote", `{}`, 400,
			`{"error":"invalid: give title, or workItemId"}`},
		{"both forms", "/spaces/1/promote", `{"title":"A","workItemId":1}`, 400,
			`{"error":"invalid: give title, or workItemId"}`},
		{"unknown field", "/spaces/1/promote", `{"feature":1}`, 400,
			`{"error":"invalid: bad JSON: json: unknown field \"feature\""}`},
		{"work item of another project", "/spaces/1/promote", `{"workItemId":1}`, 400,
			`{"error":"invalid: work item 1 is not in project 1"}`},
		{"unknown work item", "/spaces/1/promote", `{"workItemId":9}`, 400,
			`{"error":"invalid: work item 9 is not in project 1"}`},
		{"unknown space", "/spaces/9/promote", `{"title":"A"}`, 404,
			`{"error":"space 9: not found"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := do(t, http.MethodPost, env.url+tt.path, tt.give)
			if status != tt.wantStatus || body != tt.wantBody {
				t.Errorf("%s = %d %s, want %d %s", tt.path, status, body, tt.wantStatus, tt.wantBody)
			}
		})
	}
	_, body := do(t, http.MethodGet, env.url+"/spaces/1", "")
	if !strings.Contains(body, `"workItemId":null`) {
		t.Errorf("space after errors = %s, want workItemId null", body)
	}
	if got := env.opened.Load(); got != 0 {
		t.Errorf("sessions opened = %d, want 0: a refused promote does not open the agent", got)
	}
}

func TestPromote_OnWorkItemSpace(t *testing.T) {
	env := newLiveEnv(t)
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":1,"title":"Login"}`)
	do(t, http.MethodPost, env.url+"/spaces", `{"projectId":1,"workItemId":1,"name":"c","backend":"claude"}`)
	status, body := do(t, http.MethodPost, env.url+"/spaces/2/promote", `{"title":"Other"}`)
	if status != 400 || body != `{"error":"invalid: Space 2 already has work item 1"}` {
		t.Errorf("promote = %d %s, want 400 already has work item", status, body)
	}
}

func TestPromote_WhileRunning(t *testing.T) {
	env := newLiveEnv(t)
	env.send(t, "block")
	<-env.fake.started
	status, body := do(t, http.MethodPost, env.url+"/spaces/1/promote", `{"title":"Login"}`)
	if status != 409 || body != `{"error":"space 1 has a turn running: conflict"}` {
		t.Errorf("promote = %d %s, want 409 turn running", status, body)
	}
	if ws, err := env.st.WorkItems(context.Background(), store.WorkItemFilter{}); err != nil || len(ws) != 0 {
		t.Errorf("work items = %+v, %v, want none after a refused promote", ws, err)
	}
	env.fake.release <- struct{}{}
}

func TestProposalRoutes(t *testing.T) {
	env := newLiveEnv(t)
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":1,"title":"Login"}`)
	do(t, http.MethodPost, env.url+"/spaces", `{"projectId":1,"workItemId":1,"name":"c","backend":"claude"}`)
	if status, body := do(t, http.MethodGet, env.url+"/spaces/2/proposal", ""); status != 404 ||
		body != `{"error":"pending proposal of Space 2: not found"}` {
		t.Errorf("proposal before = %d %s, want 404", status, body)
	}
	propose := func(brief string) {
		t.Helper()
		_, err := env.st.CreateProposal(context.Background(), store.Proposal{
			SpaceID: 2, Role: "lead", Brief: brief,
			Items: []store.ProposedItem{{Kind: "decision", Body: "use SQLite", Why: "simple", Scope: "work_item"}},
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	propose("first brief")
	status, body := do(t, http.MethodGet, env.url+"/spaces/2/proposal", "")
	if status != 200 || !strings.Contains(body, `"id":1,"spaceId":2,"role":"lead","kind":"wrap","brief":"first brief"`) {
		t.Fatalf("proposal = %d %s, want proposal 1", status, body)
	}
	status, body = do(t, http.MethodPost, env.url+"/proposals/1/approve", "")
	if status != 200 || !strings.Contains(body, `"status":"approved"`) || !strings.Contains(body, `"itemsAdded":1`) {
		t.Errorf("approve = %d %s, want approved with 1 item", status, body)
	}
	if _, body := do(t, http.MethodGet, env.url+"/work-items/1/brief", ""); !strings.Contains(body, `"body":"first brief"`) {
		t.Errorf("brief = %s, want first brief", body)
	}
	status, body = do(t, http.MethodPost, env.url+"/proposals/1/approve", "")
	if status != 400 || body != `{"error":"invalid: proposal 1 is already approved"}` {
		t.Errorf("approve again = %d %s, want 400 already approved", status, body)
	}

	propose("second brief")
	status, body = do(t, http.MethodPost, env.url+"/proposals/2/reject", "")
	if status != 200 || !strings.Contains(body, `"status":"rejected"`) {
		t.Errorf("reject = %d %s, want rejected", status, body)
	}
	if _, body := do(t, http.MethodGet, env.url+"/work-items/1/brief", ""); !strings.Contains(body, `"body":"first brief"`) {
		t.Errorf("brief after reject = %s, want first brief", body)
	}
	status, body = do(t, http.MethodPost, env.url+"/proposals/2/reject", "")
	if status != 400 || body != `{"error":"invalid: proposal 2 is already rejected"}` {
		t.Errorf("reject again = %d %s, want 400 already rejected", status, body)
	}
	status, body = do(t, http.MethodPost, env.url+"/proposals/9/approve", "")
	if status != 404 || body != `{"error":"proposal 9: not found"}` {
		t.Errorf("approve unknown = %d %s, want 404", status, body)
	}
}
