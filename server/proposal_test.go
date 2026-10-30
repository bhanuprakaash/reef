package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/memory"
	"github.com/bhanuprakaash/reef/space"
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

// gitProject makes the folder of project id a git repo with one commit, so a Space of it can
// be attached to a work item.
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

func TestAttach_FreeSpaceToWorkItem(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	gitProject(t, env, 1)
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":1,"title":"Login"}`)
	ms := testMemory(t, env.st)
	if _, err := ms.Remember(ctx,
		memory.Writer{Author: "user", Role: "user", Scope: memory.Ref{Kind: memory.ScopeSpace, ID: "1"}},
		memory.Note{Kind: memory.KindDecision, Body: "use sessions, not JWT"}); err != nil {
		t.Fatal(err)
	}
	before, err := space.Context(ctx, env.st, ms, 1, "lead")
	if err != nil || strings.Contains(before, "use sessions, not JWT") {
		t.Fatalf("first prompt of the work item before the attach = %q, %v, want without the Space's item", before, err)
	}

	status, body := do(t, http.MethodPost, env.url+"/spaces/1/attach", `{"workItemId":1}`)
	if status != 200 || !strings.Contains(body, `"workItemId":1`) || !strings.Contains(body, `"kind":"work_item"`) {
		t.Fatalf("attach = %d %s, want 200 with workItemId 1 and kind work_item", status, body)
	}

	_, body = do(t, http.MethodGet, env.url+"/spaces?workItem=1", "")
	if !strings.Contains(body, `"id":1,`) {
		t.Errorf("Spaces of work item 1 = %s, want Space 1", body)
	}
	after, err := space.Context(ctx, env.st, ms, 1, "lead")
	if err != nil || !strings.Contains(after, "decision 1: use sessions, not JWT (user, ") {
		t.Errorf("first prompt of the work item after the attach =\n%s\n%v, want the decision in it", after, err)
	}
	_, body = do(t, http.MethodGet, env.url+"/messages?space=1", "")
	if !strings.Contains(body, `"author":"reef","body":"This Space is now attached to work item \"Login\" (1). It works in `) {
		t.Errorf("messages = %s, want the note from reef", body)
	}
}

func TestAttach_RefusedWhenTheWorkItemHasAnActiveSpace(t *testing.T) {
	env := newLiveEnv(t)
	gitProject(t, env, 1)
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":1,"title":"Login"}`)
	if status, body := do(t, http.MethodPost, env.url+"/spaces",
		`{"projectId":1,"workItemId":1,"name":"c"}`); status != 201 {
		t.Fatalf("create = %d %s, want 201", status, body)
	}
	const want = `{"error":"conflict: work item 1 already has an active Space"}`

	if status, body := do(t, http.MethodPost, env.url+"/spaces",
		`{"projectId":1,"workItemId":1,"name":"again"}`); status != 409 || body != want {
		t.Errorf("second Space of work item 1 = %d %s, want 409 %s", status, body, want)
	}
	if status, body := do(t, http.MethodPost, env.url+"/spaces/1/attach", `{"workItemId":1}`); status != 409 || body != want {
		t.Errorf("attach to work item 1 = %d %s, want 409 %s", status, body, want)
	}
	_, body := do(t, http.MethodGet, env.url+"/spaces/1", "")
	if !strings.Contains(body, `"workItemId":null`) {
		t.Errorf("Space 1 after the refused attach = %s, want workItemId null", body)
	}
}

func TestAttach_NotAGitRepo(t *testing.T) {
	env := newLiveEnv(t)
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":1,"title":"Login"}`)
	p, err := env.st.Project(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	status, body := do(t, http.MethodPost, env.url+"/spaces/1/attach", `{"workItemId":1}`)
	want := `{"error":"invalid: project ` + p.Path + ` is not a git repo with a commit"}`
	if status != 400 || body != want {
		t.Errorf("attach = %d %s, want 400 %s", status, body, want)
	}
}

func TestAttach_Errors(t *testing.T) {
	env := newLiveEnv(t)
	gitProject(t, env, 1)
	do(t, http.MethodPost, env.url+"/projects", `{"name":"other","path":"`+t.TempDir()+`"}`)
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":2,"title":"Billing"}`)
	tests := []struct {
		name       string
		path       string
		give       string
		wantStatus int
		wantBody   string
	}{
		{"no work item", "/spaces/1/attach", `{}`, 400,
			`{"error":"invalid: workItemId is required"}`},
		{"unknown field", "/spaces/1/attach", `{"feature":1}`, 400,
			`{"error":"invalid: bad JSON: json: unknown field \"feature\""}`},
		{"work item of another project", "/spaces/1/attach", `{"workItemId":1}`, 400,
			`{"error":"invalid: work item 1 is not in project 1"}`},
		{"unknown work item", "/spaces/1/attach", `{"workItemId":9}`, 400,
			`{"error":"invalid: unknown work item 9"}`},
		{"unknown space", "/spaces/9/attach", `{"workItemId":1}`, 404,
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
}

func TestAttach_AttachedSpaceAndConversation(t *testing.T) {
	env := newLiveEnv(t)
	gitProject(t, env, 1)
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":1,"title":"Login"}`)
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":1,"title":"Search"}`)
	do(t, http.MethodPost, env.url+"/spaces/1/attach", `{"workItemId":1}`)
	do(t, http.MethodPost, env.url+"/spaces",
		`{"projectId":1,"name":"talk","botId":1,"kind":"conversation"}`)

	status, body := do(t, http.MethodPost, env.url+"/spaces/1/attach", `{"workItemId":2}`)
	if status != 400 || body != `{"error":"invalid: Space 1 already has work item 1"}` {
		t.Errorf("attach an attached Space = %d %s, want 400 already has work item", status, body)
	}
	status, body = do(t, http.MethodPost, env.url+"/spaces/2/attach", `{"workItemId":2}`)
	if status != 400 || !strings.Contains(body, "only a free Space attaches to a work item") {
		t.Errorf("attach a conversation = %d %s, want 400 only a free Space attaches", status, body)
	}
}

func TestAttach_WhileRunning(t *testing.T) {
	env := newLiveEnv(t)
	gitProject(t, env, 1)
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":1,"title":"Login"}`)
	env.send(t, "block")
	<-env.fake.started
	status, body := do(t, http.MethodPost, env.url+"/spaces/1/attach", `{"workItemId":1}`)
	if status != 409 || body != `{"error":"space 1 has a turn running: conflict"}` {
		t.Errorf("attach = %d %s, want 409 turn running", status, body)
	}
	_, body = do(t, http.MethodGet, env.url+"/spaces/1", "")
	if !strings.Contains(body, `"workItemId":null`) {
		t.Errorf("Space 1 after the refused attach = %s, want workItemId null", body)
	}
	env.fake.release <- struct{}{}
}

func TestCreateSpace_FromABotAddsMembersOneByOne(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	sam, err := env.st.CreateBot(ctx, store.Bot{
		Name: "sam", Backend: "claude", Write: true, Tools: []string{"Read"}})
	if err != nil {
		t.Fatal(err)
	}

	status, body := do(t, http.MethodPost, env.url+"/spaces",
		`{"projectId":1,"name":"two","botId":1,"team":"solo"}`)
	if status != 400 || body != `{"error":"invalid: give botId or team, not both"}` {
		t.Errorf("create with both = %d %s, want 400 not both", status, body)
	}
	status, body = do(t, http.MethodPost, env.url+"/spaces", `{"projectId":1,"name":"two","botId":1}`)
	if status != 201 || !strings.Contains(body, `"kind":"free"`) || !strings.Contains(body, `"team":""`) ||
		!strings.Contains(body, `"role":"lead"`) {
		t.Fatalf("create from bot 1 = %d %s, want a free Space with no team led by lead", status, body)
	}

	status, body = do(t, http.MethodPost, env.url+"/spaces/2/members", fmt.Sprintf(`{"botId":%d}`, sam.ID))
	if status != 201 || !strings.Contains(body, `"name":"sam","lead":false,"write":true`) {
		t.Fatalf("add sam = %d %s, want 201 with sam, not lead, write", status, body)
	}
	status, body = do(t, http.MethodPost, env.url+"/spaces/2/members", fmt.Sprintf(`{"botId":%d}`, sam.ID))
	if status != 409 {
		t.Errorf("add sam again = %d %s, want 409", status, body)
	}
	_, body = do(t, http.MethodGet, env.url+"/spaces/2/members", "")
	if strings.Count(body, `"name":`) != 2 || !strings.Contains(body, `"name":"lead","lead":true`) {
		t.Errorf("members = %s, want lead then sam", body)
	}
}

func TestProposalRoutes(t *testing.T) {
	env := newLiveEnv(t)
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":1,"title":"Login"}`)
	do(t, http.MethodPost, env.url+"/spaces", `{"projectId":1,"workItemId":1,"name":"c"}`)
	if status, body := do(t, http.MethodGet, env.url+"/spaces/2/proposal", ""); status != 404 ||
		body != `{"error":"pending proposal of Space 2: not found"}` {
		t.Errorf("proposal before = %d %s, want 404", status, body)
	}
	propose := func(brief string) {
		t.Helper()
		_, err := env.st.CreateProposal(context.Background(), store.Proposal{
			SpaceID: 2, Role: "lead", BotID: testServerBot(t, env.st, "lead"), Brief: brief,
			Items: []store.ProposedItem{{Kind: "decision", Body: "use SQLite", Why: "simple", Scope: "work_item"}},
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	propose("first brief")
	status, body := do(t, http.MethodGet, env.url+"/spaces/2/proposal", "")
	if status != 200 || !strings.Contains(body, `"id":1,"spaceId":2,"botId":1,"role":"lead","kind":"wrap","brief":"first brief"`) {
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
