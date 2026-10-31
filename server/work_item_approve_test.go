package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

func TestMCPProposeWorkItemNamesAnotherOwner(t *testing.T) {
	ctx := context.Background()
	st, _ := mcpFixture(t)
	if _, err := st.CreateProject(ctx, store.Project{Name: "web", Path: "/tmp/web"}); err != nil {
		t.Fatal(err)
	}
	lead, err := st.CreateBot(ctx, store.Bot{Name: "proposer", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	bob, err := st.CreateBot(ctx, store.Bot{Name: "bob", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	cy, err := st.CreateBot(ctx, store.Bot{Name: "cy", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	chat, err := st.CreateBotSpace(ctx, store.Space{ProjectID: 1, Name: "chat", Kind: store.KindConversation}, lead.ID)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := st.CreateToken(ctx, chat.ID, lead.ID)
	if err != nil {
		t.Fatal(err)
	}
	call := callTool(t, st, tok)

	got, isErr := call("propose_work_item", map[string]any{
		"type": "task", "title": "Rename the export", "description": "Call it archive.",
		"owner": "Bob", "participants": []string{fmt.Sprint(cy.ID)}, "targets": []int64{2},
	})
	if isErr {
		t.Fatalf("propose_work_item = %q", got)
	}
	w, err := st.WorkItem(ctx, 3)
	if err != nil || w.Status != "backlog" || w.ApprovedAt != nil || *w.OwnerBotID != bob.ID ||
		fmt.Sprint(w.ParticipantBotIDs) != fmt.Sprint([]int64{bob.ID, cy.ID}) || len(w.Targets) != 2 ||
		w.Targets[1].AuthorizedBy != "proposed by proposer" {
		t.Errorf("proposed item = %+v, %v, want backlog, owner bob, participants [bob cy], a proposed second target", w, err)
	}
	items, err := st.Inbox(ctx)
	if err != nil || len(items) == 0 ||
		items[0].Text != "proposer proposed work item 3: Rename the export (proposed owner bob). It is in the Backlog until you approve it." {
		t.Errorf("inbox = %+v, %v, want the proposal naming the owner", items, err)
	}
	if got, isErr := call("propose_work_item", map[string]any{
		"type": "task", "title": "x", "description": "y", "owner": "nobody",
	}); !isErr || !strings.Contains(got, `no Bot "nobody"`) {
		t.Errorf("unknown owner = %q (error %v), want it refused", got, isErr)
	}
}

func TestApproveQueuesOnceAndNothingRunsBefore(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	ann, err := env.st.CreateBot(ctx, store.Bot{Name: "ann", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	sam, err := env.st.CreateBot(ctx, store.Bot{Name: "sam", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	do(t, http.MethodPost, env.url+"/work-items", fmt.Sprintf(`{"projectId":1,"title":"Login","ownerBotId":%d}`, sam.ID))
	if _, body := do(t, http.MethodGet, env.url+"/spaces?workItem=1", ""); body != "[]" {
		t.Fatalf("Spaces of a backlog item = %s, want none", body)
	}
	if status, body := do(t, http.MethodPatch, env.url+"/work-items/1", `{"status":"queued"}`); status != 400 ||
		!strings.Contains(body, "approve it to queue it") {
		t.Fatalf("user queue = %d %s, want 400 asking to approve", status, body)
	}

	approve := fmt.Sprintf(`{"ownerBotId":%d,"participantBotIds":[%d],"key":"first"}`, ann.ID, sam.ID)
	status, body := do(t, http.MethodPost, env.url+"/work-items/1/approve", approve)
	if status != 200 || !strings.Contains(body, `"status":"in_progress"`) || !strings.Contains(body, `"approvalKey":"first"`) ||
		!strings.Contains(body, fmt.Sprintf(`"ownerBotId":%d,"pendingOwnerBotId":null,"participantBotIds":[%d,%d]`, ann.ID, ann.ID, sam.ID)) {
		t.Fatalf("approve = %d %s, want in_progress, owner ann, participants [ann sam]", status, body)
	}
	env.waitIdle(t, 2)
	if status, again := do(t, http.MethodPost, env.url+"/work-items/1/approve", approve); status != 200 || again != body {
		t.Errorf("approve again = %d %s, want the same item", status, again)
	}
	if _, spaces := do(t, http.MethodGet, env.url+"/spaces?workItem=1", ""); strings.Count(spaces, `"workItemId":1`) != 1 {
		t.Errorf("Spaces after two approvals = %s, want one", spaces)
	}
	if status, body := do(t, http.MethodPost, env.url+"/work-items/1/approve", `{"key":"second"}`); status != 409 {
		t.Errorf("approve with another key = %d %s, want 409", status, body)
	}
}

func TestApprovedMemberProposalMakesAParticipant(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	ann, err := env.st.CreateBot(ctx, store.Bot{Name: "ann", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	sam, err := env.st.CreateBot(ctx, store.Bot{Name: "sam", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	do(t, http.MethodPost, env.url+"/work-items", fmt.Sprintf(`{"projectId":1,"title":"Login","ownerBotId":%d}`, ann.ID))
	do(t, http.MethodPost, env.url+"/work-items/1/approve", `{"key":"k"}`)
	env.waitIdle(t, 2)
	tok, err := env.st.CreateToken(ctx, 2, ann.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got, isErr := callTool(t, env.st, tok)("propose_member", map[string]any{"botId": sam.ID, "reason": "reviews"}); isErr {
		t.Fatalf("propose_member = %q", got)
	}
	if status, body := do(t, http.MethodPost, env.url+"/member-proposals/1/approve", ""); status != 200 {
		t.Fatalf("approve member = %d %s", status, body)
	}
	if _, body := do(t, http.MethodGet, env.url+"/work-items/1", ""); !strings.Contains(body,
		fmt.Sprintf(`"participantBotIds":[%d,%d]`, ann.ID, sam.ID)) {
		t.Errorf("work item = %s, want sam a participant", body)
	}
}
