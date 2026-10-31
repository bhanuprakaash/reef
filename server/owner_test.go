package server

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

// ownedItemSpace adds Work item 1, owned by lead with architect taking part, and its Space 3 of the
// core team, which lead coordinates.
func ownedItemSpace(t *testing.T) *liveEnv {
	t.Helper()
	env := exchangeEnv(t)
	lead, architect := *testServerBot(t, env.st, "lead"), *testServerBot(t, env.st, "architect")
	body := fmt.Sprintf(`{"projectId":1,"title":"Login","ownerBotId":%d,"participantBotIds":[%d]}`, lead, architect)
	if status, got := do(t, http.MethodPost, env.url+"/work-items", body); status != http.StatusCreated {
		t.Fatalf("create item = %d %s", status, got)
	}
	if status, got := do(t, http.MethodPost, env.url+"/spaces",
		`{"projectId":1,"workItemId":1,"name":"Login","team":"core"}`); status != http.StatusCreated {
		t.Fatalf("create Space = %d %s", status, got)
	}
	return env
}

func (e *liveEnv) transfer(t *testing.T, bot string) (int, string) {
	t.Helper()
	return do(t, http.MethodPost, e.url+"/work-items/1/owner", fmt.Sprintf(`{"botId":%d}`, *testServerBot(t, e.st, bot)))
}

func (e *liveEnv) owner(t *testing.T) string {
	t.Helper()
	item, err := e.st.WorkItem(context.Background(), 1)
	if err != nil || item.OwnerBotID == nil {
		t.Fatalf("item = %+v, %v", item, err)
	}
	bot, err := e.st.Bot(context.Background(), *item.OwnerBotID)
	if err != nil {
		t.Fatal(err)
	}
	return bot.Name
}

func (e *liveEnv) handovers(t *testing.T) int {
	t.Helper()
	return e.count(t, "SELECT COUNT(*) FROM direct_message WHERE request_key LIKE 'owner:%'")
}

func TestTransferOwner_IdleSpaceChangesOwnerAndCoordinatorAndWakesOnlyTheNewOwner(t *testing.T) {
	env := ownedItemSpace(t)
	lead, architect := *testServerBot(t, env.st, "lead"), *testServerBot(t, env.st, "architect")
	origin := int64(1)
	key := "ask"
	ex, _, err := env.st.CreateExchange(context.Background(), store.DirectMessage{
		SpaceID: 3, OriginSpaceID: &origin, From: "architect", To: "lead",
		FromBotID: &architect, ToBotID: &lead, Body: "review the plan", RequestKey: &key,
	}, "Asked lead: review the plan")
	if err != nil {
		t.Fatal(err)
	}

	status, body := env.transfer(t, "architect")
	if status != http.StatusOK || !strings.Contains(body, `"pending":false`) {
		t.Fatalf("transfer = %d %s, want 200 not pending", status, body)
	}
	env.waitCount(t, "SELECT COUNT(*) FROM turn WHERE space_id = 3 AND status = 'done'", 2)
	env.waitIdle(t, 3)

	if got := env.owner(t); got != "architect" {
		t.Errorf("owner = %s, want architect", got)
	}
	if got := env.roster(t, 3); !slices.Equal(got, []string{"architect*", "lead"}) {
		t.Errorf("roster = %v, want architect coordinating, lead a member", got)
	}
	ran := env.ranTexts()
	handed := slices.DeleteFunc(slices.Clone(ran), func(r string) bool {
		return !strings.Contains(r, "You are now the owner and coordinator of Work item 1, Login")
	})
	if len(handed) != 1 || !strings.Contains(ran[0], "You are now the owner") || !strings.Contains(ran[1], "review the plan") {
		t.Errorf("ran %q, want the handover once, then the request waiting for lead", ran)
	}
	if env.handovers(t) != 1 {
		t.Errorf("handovers = %d, want 1", env.handovers(t))
	}
	if dm, err := env.st.DirectMessage(context.Background(), ex.ID); err != nil || *dm.ToBotID != lead ||
		dm.State != store.DirectAnswered {
		t.Errorf("request to lead = %+v, %v, want it answered by lead", dm, err)
	}
	if status, body := env.transfer(t, "architect"); status != http.StatusOK || env.handovers(t) != 1 {
		t.Errorf("transfer to the owner again = %d %s with %d handovers, want 200 and 1", status, body, env.handovers(t))
	}
}

func TestTransferOwner_RunningTurnFinishesFirstThenHandoverRunsOnce(t *testing.T) {
	env := ownedItemSpace(t)
	env.sendTo(t, 3, "go block")
	<-env.fake.started

	status, body := env.transfer(t, "architect")
	if status != http.StatusAccepted || !strings.Contains(body, `"note":"applies after the current turn"`) {
		t.Fatalf("transfer during a turn = %d %s, want 202", status, body)
	}
	if status, _ := env.transfer(t, "architect"); status != http.StatusAccepted {
		t.Errorf("same transfer again = %d, want 202", status)
	}
	if got := env.owner(t); got != "lead" || env.handovers(t) != 0 {
		t.Errorf("during the turn owner = %s with %d handovers, want lead and none", got, env.handovers(t))
	}

	env.fake.release <- struct{}{}
	env.waitCount(t, "SELECT COUNT(*) FROM direct_message WHERE request_key LIKE 'owner:%' AND state = 'answered'", 1)
	env.waitIdle(t, 3)

	if got := env.owner(t); got != "architect" {
		t.Errorf("owner after the turn = %s, want architect", got)
	}
	ran := env.ranTexts()
	if len(ran) != 2 || !strings.HasSuffix(ran[0], "go block") || !strings.Contains(ran[1], "You are now the owner") {
		t.Errorf("ran %q, want the running turn, then one handover", ran)
	}
}

func TestTransferOwner_StopThenTransferAppliesAtOnce(t *testing.T) {
	env := ownedItemSpace(t)
	env.sendTo(t, 3, "go block")
	<-env.fake.started
	if status, body := do(t, http.MethodPost, env.url+"/spaces/3/stop", ""); status >= 300 {
		t.Fatalf("stop = %d %s", status, body)
	}
	env.waitIdle(t, 3)

	if status, body := env.transfer(t, "architect"); status != http.StatusOK || !strings.Contains(body, `"pending":false`) {
		t.Fatalf("transfer after stop = %d %s, want 200 not pending", status, body)
	}
	if got := env.owner(t); got != "architect" {
		t.Errorf("owner = %s, want architect", got)
	}
}

func TestSetSpaceCoordinator_LeavesTheOwnerAlone(t *testing.T) {
	env := ownedItemSpace(t)
	architect := *testServerBot(t, env.st, "architect")
	if status, body := do(t, http.MethodPut, env.url+"/spaces/3/coordinator",
		fmt.Sprintf(`{"botId":%d}`, architect)); status != http.StatusOK {
		t.Fatalf("set coordinator = %d %s", status, body)
	}
	if got := env.roster(t, 3); !slices.Equal(got, []string{"architect*", "lead"}) {
		t.Errorf("roster = %v, want architect coordinating", got)
	}
	if got := env.owner(t); got != "lead" || env.handovers(t) != 0 {
		t.Errorf("owner = %s with %d handovers, want lead and none", got, env.handovers(t))
	}
}
