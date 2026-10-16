package server

import (
	"net/http"
	"testing"
)

func TestStatus_CountsRunningChannels(t *testing.T) {
	env := newLiveEnv(t)
	if status, body := do(t, http.MethodGet, env.url+"/status", ""); status != 200 || body != `{"running":0}` {
		t.Errorf("idle GET /status = %d %s, want 200 {\"running\":0}", status, body)
	}
	env.send(t, "block")
	<-env.fake.started
	if status, body := do(t, http.MethodGet, env.url+"/status", ""); status != 200 || body != `{"running":1}` {
		t.Errorf("running GET /status = %d %s, want 200 {\"running\":1}", status, body)
	}
	env.fake.release <- struct{}{}
	if status, body := do(t, http.MethodPost, env.url+"/status", `{}`); status != 405 {
		t.Errorf("POST /status = %d %s, want 405", status, body)
	}
}
