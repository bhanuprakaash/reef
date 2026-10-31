package server

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestRefreshBot_WaitsForTheRunningTurn(t *testing.T) {
	env := newLiveEnv(t)
	team, err := env.st.SpaceTeam(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	url := env.url + "/spaces/1/bots/" + strconv.FormatInt(team.Roles[0].BotID, 10) + "/refresh"

	env.send(t, "block")
	<-env.fake.started
	status, body := do(t, http.MethodPost, url, `{"model":"opus"}`)
	if status != 202 || body != `{"pending":true}` {
		t.Fatalf("refresh during a turn = %d %q, want 202 pending", status, body)
	}
	if got := env.ranTexts(); len(got) != 1 {
		t.Errorf("ran = %q, want the refresh to wait for the turn", got)
	}
	env.fake.release <- struct{}{}
	env.waitIdle(t, 1)

	env.send(t, "next")
	env.waitIdle(t, 1)
	want := []string{"[Space 1, message 1]\nblock", "refresh", "[Space 1, message 2]\nnext"}
	if got := env.ranTexts(); !slices.Equal(got, want) {
		t.Errorf("ran = %q, want the refresh applied before the next turn: %q", got, want)
	}
	if _, body := do(t, http.MethodGet, env.url+"/bots/"+strconv.FormatInt(team.Roles[0].BotID, 10), ""); !strings.Contains(body, `"model":"opus"`) {
		t.Errorf("bot = %s, want the model switch kept", body)
	}

	status, body = do(t, http.MethodPost, url, "")
	if status != 200 || body != `{"pending":false}` {
		t.Errorf("refresh while idle = %d %q, want 200 not pending", status, body)
	}
	if got := env.ranTexts(); got[len(got)-1] != "refresh" {
		t.Errorf("ran = %q, want the idle refresh applied at once", got)
	}
}
