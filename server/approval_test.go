package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bhanuprakaash/reef/space"
	"github.com/bhanuprakaash/reef/store"
)

func TestApproval_HoldsAreNotApprovals(t *testing.T) {
	env := newLiveEnv(t)
	id, _ := env.pingPong(t)
	approvals := fmt.Sprintf("%s/spaces/%d/approvals", env.url, id)

	if status, body := do(t, http.MethodGet, approvals, ""); status != 200 || body != "[]" {
		t.Errorf("approvals = %d %s, want none: a held wake is not an approval", status, body)
	}
	status, body := do(t, http.MethodPost, approvals+"/1", `{"approve":true}`)
	if status != 404 || body != `{"error":"approval 1 of Space 2: not found"}` {
		t.Errorf("approve a hold = %d %s, want 404", status, body)
	}
	if got := len(env.ranTexts()); got != 7 {
		t.Errorf("turns run = %d, want 7: the hold did not run", got)
	}
}

func TestApproval_GateHoldsWake(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	id, post := env.teamSpace(t)
	roles := []store.Role{
		{Name: "lead", Lead: true, Backend: "claude"}, {Name: "architect", Backend: "claude"},
	}
	_, err := env.st.ApplyTeam(ctx, store.Team{Name: "core", Roles: roles,
		Gates: []store.Gate{{From: "lead", To: "architect", On: true}}})
	if err != nil {
		t.Fatal(err)
	}
	env.fake.onRun = func(role, _ string, _ *int64) {
		if role == "lead" {
			post("lead", "@architect plan it", nil)
		}
	}
	sub := subscribeTo(t, env.url, id)
	approvals := fmt.Sprintf("%s/spaces/%d/approvals", env.url, id)
	const pending = `{"id":1,"spaceId":2,"from":"lead","to":"architect","messageId":2,` +
		`"text":"[Space 2, message 2]\nlead wrote: @architect plan it","status":`

	env.sendTo(t, id, "go")
	sub.want(t, _idleFrame,
		`turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"hi"}`)
	if got := sub.next(t); !strings.HasPrefix(got, "approval "+pending+`"pending","createdAt":`) {
		t.Errorf("frame = %s, want the pending approval", got)
	}
	sub.want(t, `turn {"role":"lead","status":"done"}`)
	status, body := do(t, http.MethodGet, approvals, "")
	if status != 200 || !strings.HasPrefix(body, "["+pending+`"pending","createdAt":`) {
		t.Errorf("approvals = %d %s, want the pending approval", status, body)
	}
	if got := env.ranTexts(); !slices.Equal(got, []string{"[Space 2, message 1]\ngo"}) {
		t.Errorf("ran = %q, want only go: no architect turn before approval", got)
	}

	if status, _ := do(t, http.MethodPost, approvals+"/1", `{"approve":true}`); status != 204 {
		t.Fatalf("approve = %d, want 204", status)
	}
	var turn []string
	approved := false
	for range 4 {
		f := sub.next(t)
		if strings.HasPrefix(f, "approval "+pending+`"approved","createdAt":`) {
			approved = true
			continue
		}
		turn = append(turn, f)
	}
	wantTurn := []string{
		`turn {"role":"architect","status":"running"}`, `agent {"kind":"text","text":"hi"}`,
		`turn {"role":"architect","status":"done"}`,
	}
	if !approved || !slices.Equal(turn, wantTurn) {
		t.Errorf("frames after approve = %q (approved %v), want the architect turn and the approved approval", turn, approved)
	}
	if got := env.ranTexts(); !slices.Equal(got, []string{
		"[Space 2, message 1]\ngo",
		"[Space 2, message 2]\nlead wrote: @architect plan it",
	}) {
		t.Errorf("ran = %q, want the architect to run the held wake", got)
	}

	env.sendTo(t, id, "again")
	sub.want(t, `turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"hi"}`)
	sub.next(t)
	sub.want(t, `turn {"role":"lead","status":"done"}`)
	if status, _ := do(t, http.MethodPost, approvals+"/2", `{"approve":false}`); status != 204 {
		t.Fatalf("reject = %d, want 204", status)
	}
	if got := sub.next(t); !strings.Contains(got, `"id":2,`) || !strings.Contains(got, `"status":"rejected"`) {
		t.Errorf("frame = %s, want the rejected approval 2", got)
	}
	if got := len(env.ranTexts()); got != 3 {
		t.Errorf("turns run = %d, want 3: a rejected wake does not run", got)
	}
	if status, body := do(t, http.MethodGet, approvals, ""); status != 200 || body != "[]" {
		t.Errorf("approvals = %d %s, want none pending", status, body)
	}
	status, body = do(t, http.MethodPost, approvals+"/2", `{"approve":true}`)
	if status != 409 || body != `{"error":"approval 2 is rejected: conflict"}` {
		t.Errorf("approve a rejected approval = %d %s, want 409", status, body)
	}
}

func TestApproval_LeadWakeOfDeveloperIsHeld(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	id, post := env.teamSpace(t)
	roles := []store.Role{
		{Name: "lead", Lead: true, Backend: "claude"}, {Name: "architect", Backend: "claude"},
		{Name: "developer", Write: true, Backend: "claude"},
	}
	if _, err := env.st.ApplyTeam(ctx, store.Team{Name: "core", Roles: roles}); err != nil {
		t.Fatal(err)
	}
	env.fake.onRun = func(role, text string, _ *int64) {
		if role == "lead" && strings.HasSuffix(text, "\nask developer") {
			post("lead", "@developer please add a README line", nil)
		}
		if role == "lead" && strings.HasSuffix(text, "\nask architect") {
			post("lead", "@architect plan it", nil)
		}
	}
	approvals := fmt.Sprintf("%s/spaces/%d/approvals", env.url, id)
	sub := subscribeTo(t, env.url, id)

	env.sendTo(t, id, "ask developer")
	sub.want(t, _idleFrame,
		`turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"hi"}`)
	if got := sub.next(t); !strings.HasPrefix(got, `approval {"id":1,"spaceId":2,"from":"lead","to":"developer",`) {
		t.Errorf("frame = %s, want the pending approval", got)
	}
	sub.want(t, `turn {"role":"lead","status":"done"}`)
	status, body := do(t, http.MethodGet, approvals, "")
	if status != 200 || !strings.HasPrefix(body, `[{"id":1,"spaceId":2,"from":"lead","to":"developer",`) ||
		!strings.Contains(body, `"status":"pending"`) {
		t.Errorf("approvals = %d %s, want lead to developer pending", status, body)
	}
	if got := env.ranTexts(); !slices.Equal(got, []string{"[Space 2, message 1]\nask developer"}) {
		t.Errorf("ran = %q, want only the lead: no developer turn before approval", got)
	}

	env.sendTo(t, id, "ask architect")
	sub.want(t, `turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"hi"}`,
		`turn {"role":"lead","status":"done"}`, `turn {"role":"architect","status":"running"}`,
		`agent {"kind":"text","text":"hi"}`, `turn {"role":"architect","status":"done"}`)
	if got := env.ranTexts(); len(got) != 3 || !strings.HasSuffix(got[2], "lead wrote: @architect plan it") {
		t.Errorf("ran = %q, want the architect to run at once", got)
	}
	if _, body := do(t, http.MethodGet, approvals, ""); strings.Count(body, `"status":"pending"`) != 1 {
		t.Errorf("approvals = %s, want only the developer one pending", body)
	}
}

// gatedSpace gates lead to architect and makes the lead hold a wake for the architect
// when it runs "go". It returns the Space id, a stream of it and its approvals URL.
func (e *liveEnv) gatedSpace(t *testing.T) (int, *stream, string) {
	t.Helper()
	id, post := e.teamSpace(t)
	roles := []store.Role{
		{Name: "lead", Lead: true, Backend: "claude"}, {Name: "architect", Backend: "claude"},
	}
	_, err := e.st.ApplyTeam(context.Background(), store.Team{Name: "core", Roles: roles,
		Gates: []store.Gate{{From: "lead", To: "architect", On: true}}})
	if err != nil {
		t.Fatal(err)
	}
	e.fake.onRun = func(role, text string, _ *int64) {
		if role == "lead" && strings.HasSuffix(text, "\ngo") {
			post("lead", "@architect plan it", nil)
		}
	}
	sub := subscribeTo(t, e.url, id)
	e.sendTo(t, id, "go")
	sub.want(t, _idleFrame,
		`turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"hi"}`)
	sub.next(t)
	sub.want(t, `turn {"role":"lead","status":"done"}`)
	return id, sub, fmt.Sprintf("%s/spaces/%d/approvals", e.url, id)
}

func TestApproval_TwoApprovesRunOneTurn(t *testing.T) {
	env := newLiveEnv(t)
	_, sub, approvals := env.gatedSpace(t)

	statuses := make(chan int, 2)
	for range 2 {
		go func() {
			res, err := http.Post(approvals+"/1", "application/json", strings.NewReader(`{"approve":true}`))
			if err != nil {
				statuses <- 0
				return
			}
			_ = res.Body.Close()
			statuses <- res.StatusCode
		}()
	}
	got := []int{<-statuses, <-statuses}
	slices.Sort(got)
	if !slices.Equal(got, []int{204, 409}) {
		t.Errorf("statuses = %v, want one 204 and one 409", got)
	}
	for range 4 {
		sub.next(t)
	}
	if got := env.ranTexts(); !slices.Equal(got, []string{
		"[Space 2, message 1]\ngo",
		"[Space 2, message 2]\nlead wrote: @architect plan it",
	}) {
		t.Errorf("ran = %q, want one architect turn", got)
	}
	status, body := do(t, http.MethodPost, approvals+"/1", `{"approve":true}`)
	if status != 409 || body != `{"error":"approval 1 is approved: conflict"}` {
		t.Errorf("approve again = %d %s, want 409", status, body)
	}
}

func TestApproval_ApproveSurvivesClientAbort(t *testing.T) {
	env := newLiveEnv(t)
	id, sub, approvals := env.gatedSpace(t)
	do(t, http.MethodPost, fmt.Sprintf("%s/spaces/%d/stop", env.url, id), "")
	ctx, abort := context.WithCancel(context.Background())
	env.open = func(context.Context) (session, error) {
		abort()
		return env.fake, nil
	}
	req := httptest.NewRequest(http.MethodPost, env.url+"/spaces/2/approvals/1",
		strings.NewReader(`{"approve":true}`)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	env.h.ServeHTTP(rec, req)
	if rec.Code != 204 {
		t.Fatalf("approve = %d %s, want 204", rec.Code, rec.Body)
	}
	for range 4 {
		sub.next(t)
	}
	if got := env.ranTexts(); !slices.Equal(got, []string{
		"[Space 2, message 1]\ngo",
		"[Space 2, message 2]\nlead wrote: @architect plan it",
	}) {
		t.Errorf("ran = %q, want the architect to run the held wake", got)
	}
	status, body := do(t, http.MethodGet, approvals, "")
	if status != 200 || body != "[]" {
		t.Errorf("approvals = %d %s, want none pending", status, body)
	}
}

// TestApproval_ApprovedHopsDoNotCountToTheCap: every wake is gated, so each of the 7 hops
// is approved by the user; none pauses the Space.
func TestApproval_ApprovedHopsDoNotCountToTheCap(t *testing.T) {
	env := newLiveEnv(t)
	id, post := env.teamSpace(t)
	roles := []store.Role{
		{Name: "lead", Lead: true, Backend: "claude"}, {Name: "architect", Backend: "claude"},
	}
	_, err := env.st.ApplyTeam(context.Background(), store.Team{Name: "core", Roles: roles,
		Gates: []store.Gate{{From: "lead", To: "architect", On: true}, {From: "architect", To: "lead", On: true}}})
	if err != nil {
		t.Fatal(err)
	}
	env.fake.onRun = func(role, _ string, _ *int64) {
		if role == "lead" {
			post("lead", "@architect again", nil)
		} else {
			post("architect", "@lead again", nil)
		}
	}
	sub := subscribeTo(t, env.url, id)
	approvals := fmt.Sprintf("%s/spaces/%d/approvals", env.url, id)
	env.sendTo(t, id, "go")
	sub.want(t, _idleFrame)
	for done := `turn {"role":"lead","status":"done"}`; sub.next(t) != done; {
	}

	for i := 1; i <= 7; i++ {
		role := "architect"
		if i%2 == 0 {
			role = "lead"
		}
		if status, body := do(t, http.MethodPost, fmt.Sprintf("%s/%d", approvals, i), `{"approve":true}`); status != 204 {
			t.Fatalf("approve %d = %d %s, want 204", i, status, body)
		}
		for done := `turn {"role":"` + role + `","status":"done"}`; sub.next(t) != done; {
		}
	}
	if got := len(env.ranTexts()); got != 8 {
		t.Errorf("turns run = %d, want 8: the user's turn and 7 approved hops", got)
	}
	if ch, err := env.st.Space(context.Background(), int64(id)); err != nil || ch.Paused != "" {
		t.Errorf("space = %+v, %v, want not paused", ch, err)
	}
}

// TestApproval_ApproveWhileBusyRunsAfter: the lead wakes the architect and, gated, the
// developer; approving the developer while the architect runs queues it behind that turn.
func TestApproval_ApproveWhileBusyRunsAfter(t *testing.T) {
	env := newLiveEnv(t)
	id, post := env.teamSpace(t)
	roles := []store.Role{
		{Name: "lead", Lead: true, Backend: "claude"}, {Name: "architect", Backend: "claude"},
		{Name: "developer", Backend: "claude"},
	}
	_, err := env.st.ApplyTeam(context.Background(), store.Team{Name: "core", Roles: roles,
		Gates: []store.Gate{{From: "lead", To: "developer", On: true}}})
	if err != nil {
		t.Fatal(err)
	}
	env.fake.onRun = func(role, _ string, _ *int64) {
		if role == "lead" {
			post("lead", "@developer build it", nil)
			post("lead", "@architect block", nil)
		}
	}
	sub := subscribeTo(t, env.url, id)
	approvals := fmt.Sprintf("%s/spaces/%d/approvals", env.url, id)

	env.sendTo(t, id, "go")
	<-env.fake.started
	sub.want(t, _idleFrame,
		`turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"hi"}`)
	sub.next(t)
	sub.want(t, `turn {"role":"lead","status":"done"}`,
		`turn {"role":"architect","status":"running"}`, `agent {"kind":"text","text":"part1"}`)

	if status, body := do(t, http.MethodPost, approvals+"/1", `{"approve":true}`); status != 204 || body != "" {
		t.Fatalf("approve while busy = %d %q, want 204", status, body)
	}
	if got := sub.next(t); !strings.Contains(got, `"id":1,`) || !strings.Contains(got, `"status":"approved"`) {
		t.Errorf("frame = %s, want the approved approval", got)
	}
	if got := len(env.ranTexts()); got != 2 {
		t.Errorf("turns run = %d, want 2: the developer waits for the architect", got)
	}
	env.fake.release <- struct{}{}
	sub.want(t, `agent {"kind":"text","text":"part2"}`, `turn {"role":"architect","status":"done"}`)
	sub.wantTurns(t, "developer")
	want := []string{
		"[Space 2, message 1]\ngo",
		"[Space 2, message 3]\nlead wrote: @architect block",
		"[Space 2, message 2]\nlead wrote: @developer build it",
	}
	if got := env.ranTexts(); !slices.Equal(got, want) {
		t.Errorf("ran = %q, want %q", got, want)
	}
	if a, err := env.st.Approval(context.Background(), int64(id), 1); err != nil || a.Status != store.ApprovalApproved {
		t.Errorf("approval = %+v, %v, want approved", a, err)
	}
}

func TestApproval_StopReturnsQueuedWakes(t *testing.T) {
	env := newLiveEnv(t)
	id, sub, approvals := env.gatedSpace(t)
	env.sendTo(t, id, "block")
	<-env.fake.started
	sub.want(t, `turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"part1"}`)
	if status, _ := do(t, http.MethodPost, approvals+"/1", `{"approve":true}`); status != 204 {
		t.Fatalf("approve while busy = %d, want 204", status)
	}
	if status, _ := env.sendTo(t, id, "again"); status != 202 {
		t.Fatalf("send while busy = %d, want 202", status)
	}

	do(t, http.MethodPost, fmt.Sprintf("%s/spaces/%d/stop", env.url, id), "")

	status, body := do(t, http.MethodGet, approvals, "")
	if status != 200 || !strings.Contains(body, `"id":1,`) || !strings.Contains(body, `"status":"pending"`) {
		t.Errorf("approvals = %d %s, want approval 1 pending again", status, body)
	}
	l := env.h.s.lookup(int64(id))
	l.mu.Lock()
	held := slices.Clone(l.held)
	l.mu.Unlock()
	if len(held) != 1 || held[0].text != "[Space 2, message 4]\nagain" {
		t.Errorf("held = %+v, want the wake of again", held)
	}
	if got := len(env.ranTexts()); got != 2 {
		t.Errorf("turns run = %d, want 2: go and block only", got)
	}
}

func TestApproval_DefaultTeamHasNoGateToArchitect(t *testing.T) {
	env := newLiveEnv(t)
	id, post := env.teamSpace(t)
	env.fake.onRun = func(role, _ string, _ *int64) {
		if role == "lead" {
			post("lead", "@architect plan it", nil)
		}
	}
	sub := subscribeTo(t, env.url, id)
	env.sendTo(t, id, "go")
	sub.want(t, _idleFrame,
		`turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"hi"}`,
		`turn {"role":"lead","status":"done"}`,
		`turn {"role":"architect","status":"running"}`, `agent {"kind":"text","text":"hi"}`,
		`turn {"role":"architect","status":"done"}`)
	if status, body := do(t, http.MethodGet, fmt.Sprintf("%s/spaces/%d/approvals", env.url, id), ""); status != 200 || body != "[]" {
		t.Errorf("approvals = %d %s, want none", status, body)
	}
}

func TestApproval_StoreFailurePausesAndContinueRetries(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	id, post := env.teamSpace(t)
	roles := []store.Role{
		{Name: "lead", Lead: true, Backend: "claude"}, {Name: "architect", Backend: "claude"},
	}
	_, err := env.st.ApplyTeam(ctx, store.Team{Name: "core", Roles: roles,
		Gates: []store.Gate{{From: "lead", To: "architect", On: true}}})
	if err != nil {
		t.Fatal(err)
	}
	env.fake.onRun = func(role, _ string, _ *int64) {
		if role == "lead" {
			post("lead", "@architect plan it", nil)
		}
	}
	env.exec(t, "CREATE TRIGGER no_approval BEFORE INSERT ON approval "+
		"BEGIN SELECT RAISE(ABORT, 'disk full'); END")
	sub := subscribeTo(t, env.url, id)
	approvals := fmt.Sprintf("%s/spaces/%d/approvals", env.url, id)

	env.sendTo(t, id, "go")
	const reason = "hold wake for architect: insert approval: constraint failed: disk full (1811)"
	sub.want(t, _idleFrame, `turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"hi"}`,
		`turn {"role":"lead","status":"done"}`, `pause {"reason":"`+reason+`"}`)
	if ch, err := env.st.Space(ctx, int64(id)); err != nil || ch.Paused != reason {
		t.Errorf("space = %+v, %v, want paused with %q", ch, err, reason)
	}
	if status, body := do(t, http.MethodGet, approvals, ""); status != 200 || body != "[]" {
		t.Errorf("approvals = %d %s, want none", status, body)
	}

	env.exec(t, "DROP TRIGGER no_approval")
	if status, _ := do(t, http.MethodPost, fmt.Sprintf("%s/spaces/%d/continue", env.url, id), ""); status != 204 {
		t.Fatalf("continue = %d, want 204", status)
	}
	if got := sub.next(t); !strings.HasPrefix(got, `approval {"id":1,"spaceId":2,"from":"lead","to":"architect",`) {
		t.Errorf("frame = %s, want the pending approval", got)
	}
	sub.want(t, `pause {"reason":""}`)
	status, body := do(t, http.MethodGet, approvals, "")
	if status != 200 || !strings.Contains(body, `"text":"[Space 2, message 2]\nlead wrote: @architect plan it","status":"pending"`) {
		t.Errorf("approvals = %d %s, want the pending approval", status, body)
	}
	if got := env.ranTexts(); !slices.Equal(got, []string{"[Space 2, message 1]\ngo"}) {
		t.Errorf("ran = %q, want only go: the wake waits for approval", got)
	}
}

func TestApproval_ApproveWhileStartingRunsAfter(t *testing.T) {
	env := newLiveEnv(t)
	id, _, approvals := env.gatedSpace(t)
	do(t, http.MethodPost, fmt.Sprintf("%s/spaces/%d/stop", env.url, id), "")
	ran := make(chan struct{})
	env.fake.onRun = func(role, _ string, _ *int64) {
		if role == "architect" {
			close(ran)
		}
	}
	var status int
	var body string
	env.open = func(context.Context) (session, error) {
		status, body = do(t, http.MethodPost, approvals+"/1", `{"approve":true}`)
		return env.fake, nil
	}
	if s, _ := env.sendTo(t, id, "again"); s != 202 {
		t.Fatalf("send = %d, want 202", s)
	}
	if status != 204 || body != "" {
		t.Fatalf("approve while starting = %d %q, want 204", status, body)
	}
	<-ran
	want := []string{
		"[Space 2, message 1]\ngo",
		"[Space 2, message 3]\nagain",
		"[Space 2, message 2]\nlead wrote: @architect plan it",
	}
	if got := env.ranTexts(); !slices.Equal(got, want) {
		t.Errorf("ran = %q, want %q", got, want)
	}
}

func TestApproval_ApproveWhileStartingStaysPendingWhenStartFails(t *testing.T) {
	env := newLiveEnv(t)
	id, _, approvals := env.gatedSpace(t)
	do(t, http.MethodPost, fmt.Sprintf("%s/spaces/%d/stop", env.url, id), "")
	var status int
	env.open = func(context.Context) (session, error) {
		status, _ = do(t, http.MethodPost, approvals+"/1", `{"approve":true}`)
		return nil, errors.New("boom")
	}
	if s, _ := env.sendTo(t, id, "again"); s != 500 {
		t.Fatalf("send = %d, want 500", s)
	}
	if status != 204 {
		t.Fatalf("approve while starting = %d, want 204", status)
	}
	s, body := do(t, http.MethodGet, approvals, "")
	if s != 200 || !strings.Contains(body, `"id":1,`) || !strings.Contains(body, `"status":"pending"`) {
		t.Errorf("approvals = %d %s, want approval 1 pending again", s, body)
	}
}

func TestApproval_ApproveWhileWrapRunsAfter(t *testing.T) {
	env := newLiveEnv(t)
	env.teamSpace(t)
	_, err := env.st.ApplyTeam(context.Background(), store.Team{Name: "core", Roles: []store.Role{
		{Name: "lead", Lead: true, Backend: "claude"}, {Name: "architect", Backend: "claude"},
	}, Gates: []store.Gate{{From: "lead", To: "architect", On: true}}})
	if err != nil {
		t.Fatal(err)
	}
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":1,"title":"Login"}`)
	do(t, http.MethodPost, env.url+"/spaces", `{"projectId":1,"workItemId":1,"name":"w","team":"core"}`)
	approvals := env.url + "/spaces/3/approvals"
	ran := make(chan struct{})
	var status int
	env.fake.onRun = func(role, text string, _ *int64) {
		switch {
		case role == "architect":
			close(ran)
		case strings.HasSuffix(text, "\ngo"):
			if _, err := env.st.CreateMessage(context.Background(),
				store.Message{SpaceID: 3, Author: "lead", Body: "@architect plan it"}); err != nil {
				t.Error(err)
			}
		case text == space.WrapCommand:
			status, _ = do(t, http.MethodPost, approvals+"/1", `{"approve":true}`)
		}
	}
	sub := subscribeTo(t, env.url, 3)
	env.sendTo(t, 3, "go")
	sub.want(t, _idleFrame,
		`turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"hi"}`)
	sub.next(t)
	sub.want(t, `turn {"role":"lead","status":"done"}`)
	env.sendTo(t, 3, "/wrap")
	<-ran
	if status != 204 {
		t.Fatalf("approve while wrapping = %d, want 204", status)
	}
	want := []string{"[Space 3, message 1]\ngo", "[Space 3, message 2]\nlead wrote: @architect plan it"}
	if got := env.ranTexts(); !slices.Equal(got, want) {
		t.Errorf("ran = %q, want %q", got, want)
	}
}

func TestApproval_TwoGatedWakesBothRunWhenBothApproved(t *testing.T) {
	env := newLiveEnv(t)
	id, post := env.teamSpace(t)
	roles := []store.Role{
		{Name: "lead", Lead: true, Backend: "claude"}, {Name: "architect", Backend: "claude"},
		{Name: "developer", Backend: "claude"},
	}
	_, err := env.st.ApplyTeam(context.Background(), store.Team{Name: "core", Roles: roles,
		Gates: []store.Gate{{From: "lead", To: "architect", On: true}, {From: "lead", To: "developer", On: true}}})
	if err != nil {
		t.Fatal(err)
	}
	env.fake.onRun = func(role, text string, _ *int64) {
		if role == "lead" && strings.HasSuffix(text, "\ngo") {
			post("lead", "@architect plan it", nil)
			post("lead", "@developer build it", nil)
		}
	}
	sub := subscribeTo(t, env.url, id)
	approvals := fmt.Sprintf("%s/spaces/%d/approvals", env.url, id)
	env.sendTo(t, id, "go")
	sub.want(t, _idleFrame,
		`turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"hi"}`)
	sub.next(t)
	sub.next(t)
	sub.want(t, `turn {"role":"lead","status":"done"}`)

	for _, aid := range []string{"1", "2"} {
		if status, body := do(t, http.MethodPost, approvals+"/"+aid, `{"approve":true}`); status != 204 {
			t.Fatalf("approve %s = %d %s, want 204", aid, status, body)
		}
	}
	waitRan(t, env, 3)
	want := []string{
		"[Space 2, message 1]\ngo",
		"[Space 2, message 2]\nlead wrote: @architect plan it",
		"[Space 2, message 3]\nlead wrote: @developer build it",
	}
	if got := env.ranTexts(); !slices.Equal(got, want) {
		t.Errorf("ran = %q, want %q", got, want)
	}
}

func waitRan(t *testing.T, env *liveEnv, n int) {
	t.Helper()
	for range 200 {
		if len(env.ranTexts()) >= n {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestApproval_ThreadAndTopLevelMentionHoldOneWake(t *testing.T) {
	env := newLiveEnv(t)
	id, post := env.teamSpace(t)
	roles := []store.Role{
		{Name: "lead", Lead: true, Backend: "claude"}, {Name: "developer", Backend: "claude"},
	}
	if _, err := env.st.ApplyTeam(context.Background(), store.Team{Name: "core", Roles: roles}); err != nil {
		t.Fatal(err)
	}
	env.fake.onRun = func(role, text string, _ *int64) {
		if role == "lead" && strings.HasSuffix(text, "\ngo") {
			one := int64(1)
			post("lead", "@developer build it", &one)
			post("lead", "@developer handed over", nil)
		}
	}
	approvals := fmt.Sprintf("%s/spaces/%d/approvals", env.url, id)
	env.sendTo(t, id, "go")
	waitRan(t, env, 1)
	waitIdle(t, env, id)

	_, body := do(t, http.MethodGet, approvals, "")
	if n := strings.Count(body, `"to":"developer"`); n != 1 {
		t.Fatalf("approvals = %s, want one for the developer, got %d", body, n)
	}
	if status, _ := do(t, http.MethodPost, approvals+"/1", `{"approve":true}`); status != 204 {
		t.Fatalf("approve = %d, want 204", status)
	}
	waitRan(t, env, 2)
	waitIdle(t, env, id)
	if got := len(env.ranTexts()); got != 2 {
		t.Errorf("turns run = %d, want 2: the lead and one developer turn", got)
	}

	env.sendTo(t, id, "go")
	waitRan(t, env, 3)
	env.wantInbox(t, "approval 2 lead wants to wake developer")
	if status, _ := do(t, http.MethodPost, approvals+"/2", `{"approve":false}`); status != 204 {
		t.Fatalf("reject = %d, want 204", status)
	}
	env.wantInbox(t)
}

func waitIdle(t *testing.T, env *liveEnv, id int) {
	t.Helper()
	for range 200 {
		if l := env.h.s.lookup(int64(id)); l == nil || !l.running() && func() bool {
			l.mu.Lock()
			defer l.mu.Unlock()
			return l.state == _idle
		}() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("space did not go idle")
}

func TestApproval_StopTellsSubscribersQueuedApprovalIsPendingAgain(t *testing.T) {
	env := newLiveEnv(t)
	id, sub, approvals := env.gatedSpace(t)
	env.sendTo(t, id, "block")
	<-env.fake.started
	sub.want(t, `turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"part1"}`)
	if status, _ := do(t, http.MethodPost, approvals+"/1", `{"approve":true}`); status != 204 {
		t.Fatalf("approve while busy = %d, want 204", status)
	}
	sub.next(t)

	do(t, http.MethodPost, fmt.Sprintf("%s/spaces/%d/stop", env.url, id), "")

	for range 4 {
		if got := sub.next(t); strings.HasPrefix(got, "approval ") && strings.Contains(got, `"status":"pending"`) {
			return
		}
	}
	t.Error("no pending approval frame after the stop returned the queued approval")
}
