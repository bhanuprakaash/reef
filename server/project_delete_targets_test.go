package server

import (
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/space"
)

func TestDeleteProject_PausesAnAssignmentThatTargetsItAndKeepsTheRecord(t *testing.T) {
	env, app, infra := twoTargetEnv(t)
	env.exec(t, "UPDATE work_item SET status = 'in_progress' WHERE id = 1")

	if status, body := do(t, http.MethodDelete, env.url+"/projects/3", ""); status != 204 || body != "" {
		t.Fatalf("delete infra = %d %q, want 204", status, body)
	}

	if _, err := os.Stat(infra); !os.IsNotExist(err) {
		t.Errorf("infra workspace after delete: err = %v, want not exist", err)
	}
	if _, err := os.Stat(app); err != nil {
		t.Errorf("app workspace after delete: %v, want kept", err)
	}
	if _, body := do(t, http.MethodGet, env.url+"/work-items/1", ""); !strings.Contains(body, `"status":"needs_you"`) {
		t.Errorf("item = %s, want needs_you", body)
	}
	if _, body := do(t, http.MethodGet, env.url+"/messages?space=2", ""); !strings.Contains(body, "Project infra was deleted.") {
		t.Errorf("messages = %s, want a note naming infra", body)
	}
	if n := env.count(t, "SELECT COUNT(*) FROM workspace WHERE project_id = 3 AND state = 'removed'"); n != 1 {
		t.Errorf("removed infra workspace rows = %d, want 1", n)
	}
	if _, body := do(t, http.MethodGet, env.url+"/work-items/1/targets", ""); !strings.Contains(body, `"projectName":"deleted project 3"`) {
		t.Errorf("targets = %s, want the deleted target kept", body)
	}
}

func TestDeleteProject_RefusedWhileAnAssignmentThatTargetsItRuns(t *testing.T) {
	env, _, infra := twoTargetEnv(t)
	root := space.WorkRoot(env.dir, "1")
	unlock, err := env.h.s.trees.Lock(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()

	status, body := do(t, http.MethodDelete, env.url+"/projects/3", "")
	if want := `{"error":"` + root + ` has a turn running: conflict"}`; status != 409 || body != want {
		t.Errorf("delete = %d %s, want 409 %s", status, body, want)
	}
	if _, err := os.Stat(infra); err != nil {
		t.Errorf("infra workspace after refused delete: %v, want kept", err)
	}
}
