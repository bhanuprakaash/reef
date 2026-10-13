package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

func TestWrap_RunsWrapTurn(t *testing.T) {
	env := newLiveEnv(t)
	id := env.featureChannel(t)
	sub := subscribeTo(t, env.url, id)
	status, body := do(t, http.MethodPost, fmt.Sprintf("%s/channels/%d/wrap", env.url, id), "")
	if status != 202 || body != "" {
		t.Fatalf("wrap = %d %q, want 202", status, body)
	}
	sub.want(t,
		_idleFrame,
		`turn {"status":"running"}`,
		`agent {"kind":"text","text":"wrapped"}`,
		`turn {"status":"done"}`,
	)
}

func TestPromote_NewFeature(t *testing.T) {
	env := newLiveEnv(t)
	sub := subscribe(t, env.url)
	status, body := do(t, http.MethodPost, env.url+"/channels/1/promote",
		`{"slug":"login","name":"Login"}`)
	if status != 202 || !strings.Contains(body, `"featureId":1`) {
		t.Fatalf("promote = %d %s, want 202 with featureId 1", status, body)
	}
	sub.want(t,
		_idleFrame,
		`turn {"status":"running"}`,
		`agent {"kind":"text","text":"promoted"}`,
		`turn {"status":"done"}`,
	)
	status, body = do(t, http.MethodGet, env.url+"/features/1", "")
	if status != 200 || !strings.Contains(body, `"projectId":1,"slug":"login","name":"Login"`) {
		t.Errorf("feature = %d %s, want slug login", status, body)
	}
	status, body = do(t, http.MethodGet, env.url+"/channels/1", "")
	if status != 200 || !strings.Contains(body, `"featureId":1`) {
		t.Errorf("channel = %d %s, want featureId 1", status, body)
	}
}

func TestPromote_ExistingFeature(t *testing.T) {
	env := newLiveEnv(t)
	do(t, http.MethodPost, env.url+"/features", `{"projectId":1,"slug":"login","name":"Login"}`)
	status, body := do(t, http.MethodPost, env.url+"/channels/1/promote", `{"featureId":1}`)
	if status != 202 || !strings.Contains(body, `"featureId":1`) {
		t.Errorf("promote = %d %s, want 202 with featureId 1", status, body)
	}
}

func TestPromote_Errors(t *testing.T) {
	env := newLiveEnv(t)
	do(t, http.MethodPost, env.url+"/projects", `{"name":"other","path":"`+t.TempDir()+`"}`)
	do(t, http.MethodPost, env.url+"/features", `{"projectId":2,"slug":"billing","name":"Billing"}`)
	do(t, http.MethodPost, env.url+"/features", `{"projectId":1,"slug":"taken","name":"Taken"}`)
	tests := []struct {
		name       string
		path       string
		give       string
		wantStatus int
		wantBody   string
	}{
		{"no body fields", "/channels/1/promote", `{}`, 400,
			`{"error":"invalid: give slug and name, or featureId"}`},
		{"both forms", "/channels/1/promote", `{"slug":"a","name":"A","featureId":1}`, 400,
			`{"error":"invalid: give slug and name, or featureId"}`},
		{"unknown field", "/channels/1/promote", `{"feature":1}`, 400,
			`{"error":"invalid: bad JSON: json: unknown field \"feature\""}`},
		{"slug without name", "/channels/1/promote", `{"slug":"a"}`, 400,
			`{"error":"invalid: feature name is required"}`},
		{"bad slug", "/channels/1/promote", `{"slug":"A b","name":"A"}`, 400,
			`{"error":"invalid: feature slug \"A b\" must match ^[a-z0-9][a-z0-9-]*$"}`},
		{"feature of another project", "/channels/1/promote", `{"featureId":1}`, 400,
			`{"error":"invalid: feature 1 is not in project 1"}`},
		{"unknown feature", "/channels/1/promote", `{"featureId":9}`, 400,
			`{"error":"invalid: feature 9 is not in project 1"}`},
		{"slug taken", "/channels/1/promote", `{"slug":"taken","name":"T"}`, 409,
			`{"error":"feature slug \"taken\" is taken in its project: conflict"}`},
		{"unknown channel", "/channels/9/promote", `{"slug":"a","name":"A"}`, 404,
			`{"error":"channel 9: not found"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := do(t, http.MethodPost, env.url+tt.path, tt.give)
			if status != tt.wantStatus || body != tt.wantBody {
				t.Errorf("%s = %d %s, want %d %s", tt.path, status, body, tt.wantStatus, tt.wantBody)
			}
		})
	}
	_, body := do(t, http.MethodGet, env.url+"/channels/1", "")
	if !strings.Contains(body, `"featureId":null`) {
		t.Errorf("channel after errors = %s, want featureId null", body)
	}
	if got := env.opened.Load(); got != 0 {
		t.Errorf("sessions opened = %d, want 0: a refused promote does not open the agent", got)
	}
}

func TestPromote_OnFeatureChannel(t *testing.T) {
	env := newLiveEnv(t)
	do(t, http.MethodPost, env.url+"/features", `{"projectId":1,"slug":"login","name":"Login"}`)
	do(t, http.MethodPost, env.url+"/channels", `{"projectId":1,"featureId":1,"name":"c","backend":"claude"}`)
	status, body := do(t, http.MethodPost, env.url+"/channels/2/promote", `{"slug":"other","name":"Other"}`)
	if status != 400 || body != `{"error":"invalid: channel 2 already has feature 1"}` {
		t.Errorf("promote = %d %s, want 400 already has feature", status, body)
	}
}

func TestPromote_WhileRunning(t *testing.T) {
	env := newLiveEnv(t)
	env.send(t, "block")
	<-env.fake.started
	status, body := do(t, http.MethodPost, env.url+"/channels/1/promote", `{"slug":"login","name":"Login"}`)
	if status != 409 || body != `{"error":"channel 1 has a turn running: conflict"}` {
		t.Errorf("promote = %d %s, want 409 turn running", status, body)
	}
	if _, err := env.st.FeatureByRef(context.Background(), "login"); err == nil {
		t.Error("feature login exists, want none after a refused promote")
	}
	env.fake.release <- struct{}{}
}

func TestProposalRoutes(t *testing.T) {
	env := newLiveEnv(t)
	do(t, http.MethodPost, env.url+"/features", `{"projectId":1,"slug":"login","name":"Login"}`)
	do(t, http.MethodPost, env.url+"/channels", `{"projectId":1,"featureId":1,"name":"c","backend":"claude"}`)
	if status, body := do(t, http.MethodGet, env.url+"/channels/2/proposal", ""); status != 404 ||
		body != `{"error":"pending proposal of channel 2: not found"}` {
		t.Errorf("proposal before = %d %s, want 404", status, body)
	}
	propose := func(brief string) {
		t.Helper()
		_, err := env.st.CreateProposal(context.Background(), store.Proposal{
			ChannelID: 2, Role: "lead", Brief: brief, Notes: "n",
			Decisions: []store.ProposedDecision{{Body: "use SQLite", Why: "simple"}},
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	propose("first brief")
	status, body := do(t, http.MethodGet, env.url+"/channels/2/proposal", "")
	if status != 200 || !strings.Contains(body, `"id":1,"channelId":2,"role":"lead","brief":"first brief"`) {
		t.Fatalf("proposal = %d %s, want proposal 1", status, body)
	}
	status, body = do(t, http.MethodPost, env.url+"/proposals/1/approve", "")
	if status != 200 || !strings.Contains(body, `"status":"approved"`) || !strings.Contains(body, `"decisionsAdded":1`) {
		t.Errorf("approve = %d %s, want approved with 1 decision", status, body)
	}
	if _, body := do(t, http.MethodGet, env.url+"/features/1/brief", ""); !strings.Contains(body, `"body":"first brief"`) {
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
	if _, body := do(t, http.MethodGet, env.url+"/features/1/brief", ""); !strings.Contains(body, `"body":"first brief"`) {
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
