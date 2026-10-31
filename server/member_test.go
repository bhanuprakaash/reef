package server

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/bhanuprakaash/reef/store"
)

func (e *liveEnv) removeMember(t *testing.T, spaceID, botID int64, body string) (int, string) {
	t.Helper()
	return do(t, http.MethodDelete, fmt.Sprintf("%s/spaces/%d/members/%d", e.url, spaceID, botID), body)
}

func (e *liveEnv) roster(t *testing.T, spaceID int64) []string {
	t.Helper()
	team, err := e.st.SpaceTeam(context.Background(), spaceID)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, r := range team.Roles {
		name := r.Name
		if r.Lead {
			name += "*"
		}
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func TestRemoveMember_BusyBotFinishesItsTurnThenPendingRequestsAreCancelled(t *testing.T) {
	ctx := context.Background()
	env := exchangeEnv(t)
	lead, architect := *testServerBot(t, env.st, "lead"), *testServerBot(t, env.st, "architect")
	origin := int64(1)
	key := "k1"
	ex, _, err := env.st.CreateExchange(ctx, store.DirectMessage{
		SpaceID: 2, OriginSpaceID: &origin, From: "architect", To: "lead",
		FromBotID: &architect, ToBotID: &lead, Body: "review the plan", RequestKey: &key,
	}, "Asked lead: review the plan")
	if err != nil {
		t.Fatal(err)
	}
	plain, err := env.st.CreateDirectMessage(ctx, store.DirectMessage{
		SpaceID: 2, From: "architect", To: "lead", FromBotID: &architect, ToBotID: &lead, Body: "ping",
	})
	if err != nil {
		t.Fatal(err)
	}

	sub := subscribeTo(t, env.url, 2)
	sub.want(t, _idleFrame)
	env.sendTo(t, 2, "block")
	<-env.fake.started
	status, body := env.removeMember(t, 2, lead, fmt.Sprintf(`{"coordinator":%d}`, architect))
	if status != http.StatusAccepted || body != `{"pending":true}` {
		t.Fatalf("remove busy lead = %d %q, want 202 pending", status, body)
	}
	if got := env.roster(t, 2); !slices.Equal(got, []string{"architect", "lead*"}) {
		t.Errorf("roster while its turn runs = %v, want both", got)
	}
	env.fake.release <- struct{}{}
	if got := turnFrames(t, sub, 2); !slices.Equal(got, []string{"lead running ", "lead done "}) {
		t.Errorf("turns = %q, want the lead's turn to finish", got)
	}
	env.waitIdle(t, 2)

	if got := env.roster(t, 2); !slices.Equal(got, []string{"architect*"}) {
		t.Errorf("roster after = %v, want architect leading alone", got)
	}
	for _, id := range []int64{ex.ID, plain.ID} {
		dm, err := env.st.DirectMessage(ctx, id)
		if err != nil || dm.State != store.DirectCancelled || dm.Body == "" {
			t.Errorf("request %d = %+v, %v, want cancelled with its body kept", id, dm, err)
		}
	}
	if got := env.exchange(t, ex.ID).Response; got == nil || got.Status != store.DirectFailed ||
		got.Body != "cancelled: lead was removed from the Space" {
		t.Errorf("exchange response = %+v, want failed, cancelled: lead was removed from the Space", got)
	}
	if got := env.originLines(t); !slices.Contains(got,
		fmt.Sprintf("lead: No reply: cancelled, cancelled: lead was removed from the Space (exchange %d)", ex.ID)) {
		t.Errorf("origin chat = %q, want the cancellation visible", got)
	}
	msgs, err := env.st.Messages(ctx, 2, 0)
	if err != nil || msgs[len(msgs)-1].Body != "lead left the Space" {
		t.Errorf("messages = %+v, %v, want the leave notice last", msgs, err)
	}

	env.sendTo(t, 2, "@lead are you there")
	if got := turnFrames(t, sub, 2); !slices.Equal(got, []string{"architect running ", "architect done "}) {
		t.Errorf("turns after = %q, want only the coordinator's, none for the removed Bot", got)
	}
	env.waitIdle(t, 2)
}

func TestRemoveMember_RemovalDuringAClaimIsStoredAndAppliedWhenTheClaimReleases(t *testing.T) {
	ctx := context.Background()
	env := exchangeEnv(t)
	architect := *testServerBot(t, env.st, "architect")
	c, _, err := env.h.s.claim(ctx, 2, _noTurn)
	if err != nil {
		t.Fatal(err)
	}
	if status, body := env.removeMember(t, 2, architect, ""); status != http.StatusAccepted || body != `{"pending":true}` {
		t.Fatalf("remove during a claim = %d %q, want 202 pending", status, body)
	}
	if status, body := do(t, http.MethodGet, env.url+"/spaces/2/members", ""); status != 200 ||
		!strings.Contains(body, `"name":"architect","lead":false,"leaving":true`) {
		t.Errorf("members = %d %s, want architect leaving", status, body)
	}
	if err := c.release(nil); err != nil {
		t.Fatal(err)
	}
	if got := env.roster(t, 2); !slices.Equal(got, []string{"lead*"}) {
		t.Errorf("roster after the claim = %v, want architect gone", got)
	}
}

func TestRemoveMember_ContinueRunsNoWakeForARemovedBot(t *testing.T) {
	ctx := context.Background()
	env := exchangeEnv(t)
	architect := testServerBot(t, env.st, "architect")
	if err := env.st.PauseSpace(ctx, 2, "held"); err != nil {
		t.Fatal(err)
	}
	s := env.h.s
	s.mu.Lock()
	l := s.entryLocked(2)
	s.mu.Unlock()
	l.mu.Lock()
	l.held = []wake{{botID: architect, role: "architect", text: "review"}}
	l.mu.Unlock()
	if status, body := env.removeMember(t, 2, *architect, ""); status != http.StatusOK {
		t.Fatalf("remove architect = %d %s, want 200", status, body)
	}
	if status, body := do(t, http.MethodPost, env.url+"/spaces/2/continue", ""); status != http.StatusNoContent {
		t.Fatalf("continue = %d %s, want 204", status, body)
	}
	if n := env.turnCount(t, 2); n != 0 {
		t.Errorf("turns after continue = %d, want none for the removed Bot", n)
	}
}

func TestRemoveMember_CoordinatorAndLastBotNeedAReplacement(t *testing.T) {
	env := exchangeEnv(t)
	lead, architect := *testServerBot(t, env.st, "lead"), *testServerBot(t, env.st, "architect")

	for _, body := range []string{``, `{"coordinator":` + fmt.Sprint(lead) + `}`, `{"coordinator":999}`} {
		if status, resp := env.removeMember(t, 2, lead, body); status != http.StatusConflict ||
			!strings.Contains(resp, "name a remaining member as coordinator") {
			t.Errorf("remove coordinator with %q = %d %s, want 409", body, status, resp)
		}
	}
	if status, _ := env.removeMember(t, 2, architect, ""); status != http.StatusOK {
		t.Fatalf("remove architect = %d, want 200", status)
	}
	if got := env.roster(t, 2); !slices.Equal(got, []string{"lead*"}) {
		t.Errorf("roster = %v, want lead alone", got)
	}
	if status, resp := env.removeMember(t, 2, lead, ""); status != http.StatusConflict ||
		!strings.Contains(resp, "name a replacement") {
		t.Errorf("remove the last Bot = %d %s, want 409", status, resp)
	}
	if status, resp := env.removeMember(t, 2, lead, fmt.Sprintf(`{"replacement":%d}`, architect)); status != http.StatusOK {
		t.Errorf("remove the last Bot with a replacement = %d %s, want 200", status, resp)
	}
	if got := env.roster(t, 2); !slices.Equal(got, []string{"architect*"}) {
		t.Errorf("roster = %v, want the replacement leading", got)
	}
	env.wantMessages(t, 2, "reef: architect left the Space", "reef: architect joined the Space", "reef: lead left the Space")
}

func TestAddMember_UserAdditionNeedsNoApprovalBotProposalDoes(t *testing.T) {
	ctx := context.Background()
	env := exchangeEnv(t)
	kim, err := env.st.CreateBot(ctx, store.Bot{Name: "kim", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if status, body := do(t, http.MethodPost, env.url+"/spaces/2/members", fmt.Sprintf(`{"botId":%d}`, kim.ID)); status != 201 {
		t.Fatalf("add kim = %d %s, want 201", status, body)
	}
	env.wantMessages(t, 2, "reef: kim joined the Space")

	sam, err := env.st.CreateBot(ctx, store.Bot{Name: "sam", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	tok, err := testToken(t, env.st, ctx, 2, "lead")
	if err != nil {
		t.Fatal(err)
	}
	call := callTool(t, env.st, tok)
	if out, isErr := call("propose_member", map[string]any{"botId": sam.ID, "reason": "needs a reviewer"}); isErr ||
		!strings.Contains(out, `"botName":"sam"`) || !strings.Contains(out, `"proposedBy":"lead"`) {
		t.Fatalf("propose_member = %q, %v, want the pending proposal", out, isErr)
	}
	if got := env.roster(t, 2); slices.Contains(got, "sam") {
		t.Fatalf("roster = %v, want sam not added by a proposal", got)
	}
	status, body := do(t, http.MethodGet, env.url+"/spaces/2/member-proposals", "")
	if status != 200 || !strings.Contains(body, `"reason":"needs a reviewer"`) {
		t.Fatalf("proposals = %d %s, want the pending one", status, body)
	}
	if status, body := do(t, http.MethodPost, env.url+"/member-proposals/1/approve", ""); status != 200 {
		t.Fatalf("approve = %d %s, want 200", status, body)
	}
	if got := env.roster(t, 2); !slices.Contains(got, "sam") {
		t.Errorf("roster = %v, want sam after approval", got)
	}
	if status, _ := do(t, http.MethodPost, env.url+"/member-proposals/1/approve", ""); status != http.StatusConflict {
		t.Errorf("approve twice = %d, want 409", status)
	}
	if status, body := do(t, http.MethodGet, env.url+"/spaces/2/member-proposals", ""); status != 200 || body != "[]" {
		t.Errorf("proposals after = %d %q, want none", status, body)
	}
}

func TestProposeMemberIsOnlyOfferedWhereItCanWork(t *testing.T) {
	ctx := context.Background()
	e := newSendEnv(t)
	e.consult(t, 1, `[{"bot":"ben","body":"q1"}]`)
	offered := map[int64]bool{}
	for _, space := range []int64{_adaChat, _work, 5} {
		bot := e.ada
		if space == _work || space == 5 {
			bot = e.ben
		}
		tok, err := e.st.CreateToken(ctx, space, bot)
		if err != nil {
			t.Fatal(err)
		}
		ct, st := mcp.NewInMemoryTransports()
		ss, err := NewMCP(e.st, openMemory(t, e.st), tok, t.TempDir()).Connect(ctx, st, nil)
		if err != nil {
			t.Fatal(err)
		}
		cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0"}, nil).Connect(ctx, ct, nil)
		if err != nil {
			t.Fatal(err)
		}
		tools, err := cs.ListTools(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		offered[space] = slices.ContainsFunc(tools.Tools, func(tool *mcp.Tool) bool { return tool.Name == "propose_member" })
		_ = cs.Close()
		_ = ss.Wait()
	}
	if want := map[int64]bool{_adaChat: false, _work: true, 5: false}; !maps.Equal(offered, want) {
		t.Errorf("propose_member offered in Spaces %v, want %v: not in a conversation or a consultation", offered, want)
	}
}
