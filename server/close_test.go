package server

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

func TestApproveProposal_ClosesChannelAndStopsItsAgent(t *testing.T) {
	env := newLiveEnv(t)
	id := env.featureChannel(t)
	sub := subscribeTo(t, env.url, id)
	env.sendTo(t, id, "hi")
	sub.want(t, _idleFrame, `turn {"role":"lead","status":"running"}`,
		`agent {"kind":"text","text":"hi"}`, `turn {"role":"lead","status":"done"}`)
	_, err := env.st.CreateProposal(context.Background(), store.Proposal{
		ChannelID: int64(id), Role: "lead", Brief: "brief", Notes: "n",
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
	if _, body := do(t, http.MethodGet, env.url+"/channels/2", ""); !strings.Contains(body, `"closedAt":"`) {
		t.Errorf("channel = %s, want closedAt set", body)
	}
	const want = `{"error":"channel 2 is closed: conflict"}`
	if status, body := env.sendTo(t, id, "again"); status != 409 || body != want {
		t.Errorf("send = %d %s, want 409 %s", status, body, want)
	}
	if status, body := do(t, http.MethodPost, env.url+"/channels/2/wrap", ""); status != 409 || body != want {
		t.Errorf("wrap = %d %s, want 409 %s", status, body, want)
	}

	if status, _ := do(t, http.MethodPost, env.url+"/channels/2/reopen", ""); status != 204 {
		t.Fatalf("reopen = %d, want 204", status)
	}
	if _, body := do(t, http.MethodGet, env.url+"/channels/2", ""); !strings.Contains(body, `"closedAt":null`) {
		t.Errorf("channel after reopen = %s, want closedAt null", body)
	}
	if status, body := env.sendTo(t, id, "again"); status != 202 {
		t.Errorf("send after reopen = %d %s, want 202", status, body)
	}
}

func TestApproveBriefProposal_KeepsChannelAndItsAgent(t *testing.T) {
	env := newLiveEnv(t)
	id := env.featureChannel(t)
	sub := subscribeTo(t, env.url, id)
	env.sendTo(t, id, "hi")
	sub.want(t, _idleFrame, `turn {"role":"lead","status":"running"}`,
		`agent {"kind":"text","text":"hi"}`, `turn {"role":"lead","status":"done"}`)
	_, err := env.st.CreateProposal(context.Background(), store.Proposal{
		ChannelID: int64(id), Role: "lead", Kind: store.ProposalBrief, Brief: "brief",
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
	if _, body := do(t, http.MethodGet, env.url+"/channels/2", ""); !strings.Contains(body, `"closedAt":null`) {
		t.Errorf("channel = %s, want closedAt null", body)
	}
	if status, body := env.sendTo(t, id, "again"); status != 202 {
		t.Errorf("send = %d %s, want 202", status, body)
	}
}

func TestCloseFeature_ClosesChannelsAndRefusesARunningTurn(t *testing.T) {
	env := newLiveEnv(t)
	id := env.featureChannel(t)
	sub := subscribeTo(t, env.url, id)
	env.sendTo(t, id, "block")
	<-env.fake.started

	status, body := do(t, http.MethodPost, env.url+"/features/1/close", "")
	if want := `{"error":"channel 2 has a turn running: conflict"}`; status != 409 || body != want {
		t.Errorf("close while running = %d %s, want 409 %s", status, body, want)
	}
	if _, body := do(t, http.MethodGet, env.url+"/features/1", ""); !strings.Contains(body, `"closedAt":null`) {
		t.Errorf("feature after refused close = %s, want closedAt null", body)
	}

	env.fake.release <- struct{}{}
	sub.want(t, _idleFrame, `turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"part1"}`,
		`agent {"kind":"text","text":"part2"}`, `turn {"role":"lead","status":"done"}`)
	if status, body := do(t, http.MethodPost, env.url+"/features/1/close", ""); status != 204 {
		t.Fatalf("close = %d %s, want 204", status, body)
	}

	if got := env.fake.closed.Load(); got != 1 {
		t.Errorf("session closes = %d, want 1", got)
	}
	for _, path := range []string{"/features/1", "/channels/2"} {
		if _, body := do(t, http.MethodGet, env.url+path, ""); !strings.Contains(body, `"closedAt":"`) {
			t.Errorf("GET %s = %s, want closedAt set", path, body)
		}
	}
	if status, body := env.sendTo(t, id, "again"); status != 409 ||
		body != `{"error":"channel 2 is closed: conflict"}` {
		t.Errorf("send = %d %s, want 409 closed", status, body)
	}

	if status, _ := do(t, http.MethodPost, env.url+"/features/1/reopen", ""); status != 204 {
		t.Fatalf("reopen = %d, want 204", status)
	}
	if _, body := do(t, http.MethodGet, env.url+"/features/1", ""); !strings.Contains(body, `"closedAt":null`) {
		t.Errorf("feature after reopen = %s, want closedAt null", body)
	}
	if _, body := do(t, http.MethodGet, env.url+"/channels/2", ""); !strings.Contains(body, `"closedAt":"`) {
		t.Errorf("channel after feature reopen = %s, want closedAt still set", body)
	}
}

func TestWrap_RefusedWhileProposalPending(t *testing.T) {
	env := newLiveEnv(t)
	id := env.featureChannel(t)
	_, err := env.st.CreateProposal(context.Background(), store.Proposal{
		ChannelID: int64(id), Role: "lead", Brief: "brief", Notes: "n",
	})
	if err != nil {
		t.Fatal(err)
	}

	const want = `{"error":"channel 2 has a wrap-up waiting for approval: conflict"}`
	if status, body := do(t, http.MethodPost, env.url+"/channels/2/wrap", ""); status != 409 || body != want {
		t.Errorf("wrap = %d %s, want 409 %s", status, body, want)
	}
	if status, body := env.sendTo(t, id, "hi"); status != 202 {
		t.Errorf("send = %d %s, want 202: only a wrap waits on the proposal", status, body)
	}
}
