package server

import (
	"net/http"
	"strings"
	"testing"
)

func TestAddTargetIsNotedInTheItemsSpace(t *testing.T) {
	env := newLiveEnv(t)
	api := t.TempDir()
	do(t, http.MethodPost, env.url+"/projects", `{"name":"api","path":"`+api+`"}`)
	gitIn(t, api, "init", "-b", "main")
	writeIn(t, api, "README", "x")
	gitIn(t, api, "add", ".")
	gitIn(t, api, "commit", "-m", "init")
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":1,"title":"Rename the field","ownerBotId":1}`)
	do(t, http.MethodPost, env.url+"/work-items/1/approve", `{"key":"k1"}`)
	env.waitCount(t, "SELECT COUNT(*) FROM message WHERE space_id = 2 AND author != 'reef'", 1)
	env.waitIdle(t, 2)

	status, body := do(t, http.MethodPost, env.url+"/work-items/1/targets", `{"projectId":2}`)
	if status != 201 || !strings.Contains(body, `"projectName":"api","authorizedBy":"user","originMessageId":null`) {
		t.Fatalf("add target = %d %s, want 201 authorized by user", status, body)
	}
	_, body = do(t, http.MethodGet, env.url+"/messages?space=2", "")
	if !strings.Contains(body, "Added repository api to this Work item, authorized by user.") {
		t.Errorf("messages of the item's Space = %s, want the note naming api and user", body)
	}
	env.waitCount(t, "SELECT COUNT(*) FROM message WHERE space_id = 2 AND body LIKE 'Repository api is ready: folder 2-api/ under your working folder, on branch work-item/1, base refs/heads/main.'", 1)
	if _, body = do(t, http.MethodGet, env.url+"/work-items/1/targets", ""); strings.Count(body, `"projectId"`) != 2 {
		t.Errorf("targets = %s, want the home project and api", body)
	}
	if status, _ = do(t, http.MethodPost, env.url+"/work-items/1/targets", `{"projectId":2}`); status != 409 {
		t.Errorf("repeat target = %d, want 409", status)
	}
	if status, _ = do(t, http.MethodPost, env.url+"/work-items/1/targets", `{"projectId":9}`); status != 400 {
		t.Errorf("unknown project = %d, want 400", status)
	}
}
