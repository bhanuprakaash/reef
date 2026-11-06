package server

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

func TestApproveProposal_ClosesSpaceAndStopsItsAgent(t *testing.T) {
	env := newLiveEnv(t)
	id := env.workItemSpace(t)
	sub := subscribeTo(t, env.url, id)
	env.sendTo(t, id, "hi")
	sub.want(t, _idleFrame, `turn {"role":"lead","status":"running"}`,
		`agent {"kind":"text","text":"hi"}`, `turn {"role":"lead","status":"done"}`)
	_, err := env.st.CreateProposal(context.Background(), store.Proposal{
		SpaceID: int64(id), Role: "lead", BotID: testServerBot(t, env.st, "lead"), Brief: "brief",
	})
	if err != nil {
		t.Fatal(err)
	}

	if status, body := do(t, http.MethodPost, env.url+"/proposals/1/approve", ""); status != 200 {
		t.Fatalf("approve = %d %s, want 200", status, body)
	}

	if got := env.fake.closed.Load(); got != 1 {
		t.Errorf("session closes = %d, want 1", got)
	}
	if _, body := do(t, http.MethodGet, env.url+"/spaces/2", ""); !strings.Contains(body, `"closedAt":"`) {
		t.Errorf("space = %s, want closedAt set", body)
	}
	const want = `{"error":"space 2 is closed: conflict"}`
	if status, body := env.sendTo(t, id, "again"); status != 409 || body != want {
		t.Errorf("send = %d %s, want 409 %s", status, body, want)
	}
	if status, body := do(t, http.MethodPost, env.url+"/spaces/2/wrap", ""); status != 409 || body != want {
		t.Errorf("wrap = %d %s, want 409 %s", status, body, want)
	}

	if status, _ := do(t, http.MethodPost, env.url+"/spaces/2/reopen", ""); status != 204 {
		t.Fatalf("reopen = %d, want 204", status)
	}
	if _, body := do(t, http.MethodGet, env.url+"/spaces/2", ""); !strings.Contains(body, `"closedAt":null`) {
		t.Errorf("space after reopen = %s, want closedAt null", body)
	}
	if status, body := env.sendTo(t, id, "again"); status != 202 {
		t.Errorf("send after reopen = %d %s, want 202", status, body)
	}
}

func TestApproveBriefProposal_KeepsSpaceAndItsAgent(t *testing.T) {
	env := newLiveEnv(t)
	id := env.workItemSpace(t)
	sub := subscribeTo(t, env.url, id)
	env.sendTo(t, id, "hi")
	sub.want(t, _idleFrame, `turn {"role":"lead","status":"running"}`,
		`agent {"kind":"text","text":"hi"}`, `turn {"role":"lead","status":"done"}`)
	_, err := env.st.CreateProposal(context.Background(), store.Proposal{
		SpaceID: int64(id), Role: "lead", Kind: store.ProposalBrief, Brief: "brief",
	})
	if err != nil {
		t.Fatal(err)
	}

	if status, body := do(t, http.MethodPost, env.url+"/proposals/1/approve", ""); status != 200 {
		t.Fatalf("approve = %d %s, want 200", status, body)
	}

	if got := env.fake.closed.Load(); got != 0 {
		t.Errorf("session closes = %d, want 0", got)
	}
	if _, body := do(t, http.MethodGet, env.url+"/spaces/2", ""); !strings.Contains(body, `"closedAt":null`) {
		t.Errorf("space = %s, want closedAt null", body)
	}
	if status, body := env.sendTo(t, id, "again"); status != 202 {
		t.Errorf("send = %d %s, want 202", status, body)
	}
}

func TestDropWorkItem_StopsARunningTurnAndClosesSpaces(t *testing.T) {
	env := newLiveEnv(t)
	id := env.workItemSpace(t)
	env.sendTo(t, id, "block")
	<-env.fake.started

	if status, body := do(t, http.MethodPatch, env.url+"/work-items/1", `{"status":"dropped"}`); status != 200 {
		t.Fatalf("drop = %d %s, want 200", status, body)
	}

	for _, path := range []string{"/work-items/1", "/spaces/2"} {
		if _, body := do(t, http.MethodGet, env.url+path, ""); !strings.Contains(body, `"closedAt":"`) {
			t.Errorf("GET %s = %s, want closedAt set", path, body)
		}
	}
	if status, body := env.sendTo(t, id, "again"); status != 409 ||
		body != `{"error":"space 2 is closed: conflict"}` {
		t.Errorf("send = %d %s, want 409 closed", status, body)
	}
}

func TestWrap_RefusedWhileProposalPending(t *testing.T) {
	env := newLiveEnv(t)
	id := env.workItemSpace(t)
	_, err := env.st.CreateProposal(context.Background(), store.Proposal{
		SpaceID: int64(id), Role: "lead", BotID: testServerBot(t, env.st, "lead"), Brief: "brief",
	})
	if err != nil {
		t.Fatal(err)
	}

	const want = `{"error":"space 2 has a wrap-up waiting for approval: conflict"}`
	if status, body := do(t, http.MethodPost, env.url+"/spaces/2/wrap", ""); status != 409 || body != want {
		t.Errorf("wrap = %d %s, want 409 %s", status, body, want)
	}
	if status, body := env.sendTo(t, id, "hi"); status != 202 {
		t.Errorf("send = %d %s, want 202: only a wrap waits on the proposal", status, body)
	}
}

func TestWrap_NotBlockedByPendingBrief(t *testing.T) {
	env := newLiveEnv(t)
	id := env.workItemSpace(t)
	_, err := env.st.CreateProposal(context.Background(), store.Proposal{
		SpaceID: int64(id), Role: "lead", Kind: store.ProposalBrief, Brief: "plan",
	})
	if err != nil {
		t.Fatal(err)
	}

	if status, body := do(t, http.MethodPost, env.url+"/spaces/2/wrap", ""); status != 202 {
		t.Errorf("wrap = %d %s, want 202: a pending brief does not block a wrap", status, body)
	}
}

func TestCloseSpace_FreeSpaceStopsItsAgentAndKeepsHistory(t *testing.T) {
	env := newLiveEnv(t)
	sub := subscribe(t, env.url)
	env.sendTo(t, 1, "hi")
	sub.want(t, _idleFrame, `turn {"role":"lead","status":"running"}`,
		`agent {"kind":"text","text":"hi"}`, `turn {"role":"lead","status":"done"}`)

	for range 2 {
		if status, body := do(t, http.MethodPost, env.url+"/spaces/1/close", ""); status != 204 {
			t.Fatalf("close = %d %s, want 204", status, body)
		}
	}

	if got := env.fake.closed.Load(); got != 1 {
		t.Errorf("session closes = %d, want 1", got)
	}
	if _, body := do(t, http.MethodGet, env.url+"/spaces/1", ""); !strings.Contains(body, `"closedAt":"`) {
		t.Errorf("space = %s, want closedAt set", body)
	}
	if _, body := do(t, http.MethodGet, env.url+"/messages?space=1", ""); !strings.Contains(body, `"body":"hi"`) {
		t.Errorf("messages = %s, want the history kept", body)
	}
	const want = `{"error":"space 1 is closed: conflict"}`
	if status, body := env.sendTo(t, 1, "again"); status != 409 || body != want {
		t.Errorf("send = %d %s, want 409 %s", status, body, want)
	}
}

func TestCloseSpace_RefusesWorkItemSpaceAndRunningTurn(t *testing.T) {
	env := newLiveEnv(t)
	env.workItemSpace(t)

	const want = `{"error":"invalid: space 2 belongs to work item 1, close the work item"}`
	if status, body := do(t, http.MethodPost, env.url+"/spaces/2/close", ""); status != 400 || body != want {
		t.Errorf("close work item Space = %d %s, want 400 %s", status, body, want)
	}

	env.sendTo(t, 1, "block")
	<-env.fake.started
	if status, body := do(t, http.MethodPost, env.url+"/spaces/1/close", ""); status != 409 {
		t.Errorf("close during a turn = %d %s, want 409", status, body)
	}
}
