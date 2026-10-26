package server

// The channel eval replays a scripted channel shaped like a real "browser issue" channel
// (team small, one long thread, double posts, plain-text @names, an app restart with a wake
// queued) through the real routing and the real channel.Session, once per backend. Only the
// agent is fake. It lives in package server because the routing is unexported, and the
// server's session opener is the one place the real Session is built. Run it with -v to see
// its numbers.
//
// A check that fails today is listed in _expectedFail with the ticket that fixes it. The
// eval fails when a listed check passes, so that ticket removes the mark.

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/channel"
	"github.com/bhanuprakaash/reef/store"
)

const (
	_evalChains = 5
	// _evalHeader is what a wake may hold beyond its new messages: its header lines.
	_evalHeader = 200
	_evalFiller = "the toolbar handler is registered before the page has loaded and is lost on reload. "
)

var _expectedFail = map[string]map[string]string{
	"claude": {
		"duplicate bodies":        "#47",
		"duplicate in prompt":     "#47",
		"wake size":               "#47",
		"double post wake":        "#48",
		"plain-text mention wake": "#49",
		"cost sum":                "#50",
		"restart":                 "#55",
	},
	"codex": {
		"duplicate bodies":        "#47",
		"duplicate in prompt":     "#47",
		"wake size":               "#47",
		"double post wake":        "#48",
		"plain-text mention wake": "#49",
		"restart":                 "#55",
	},
}

var (
	_evalTag     = regexp.MustCompile(`\[c(\d+)\.([0-9a-z]+)\]`)
	_evalThread  = regexp.MustCompile(`, thread (\d+)\]`)
	_evalSources = regexp.MustCompile(`\[channel \d+, message (\d+)`)
)

type evalSend struct{ role, session, prompt string }

type evalEnv struct {
	t       *testing.T
	backend string
	dir     string
	url     string
	ts      *httptest.Server
	h       *Handler
	blocked chan struct{}
	block   atomic.Bool

	mu     sync.Mutex
	st     *store.Store
	sent   []evalSend
	totals map[string]float64 // what a session has really cost, by session id
	turns  map[string]int
}

func (e *evalEnv) store() *store.Store {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.st
}

// serve starts a server on the data folder whose sessions are real and whose agents are fake.
func (e *evalEnv) serve() {
	st := e.store()
	trees := channel.NewTrees(e.dir)
	e.h = newHandler(st, e.dir, trees, func(ctx context.Context, id int64) (session, error) {
		return channel.OpenWith(ctx, st, e.dir, "reef", id, trees,
			func(string) (agent.Agent, error) {
				return &evalAgent{env: e, events: make(chan agent.Event, 8)}, nil
			})
	})
	e.ts = httptest.NewServer(e.h)
	e.url = e.ts.URL
	e.t.Cleanup(e.h.Close)
	e.t.Cleanup(e.ts.Close)
}

// restart stops the server and starts a new one on the same data folder.
func (e *evalEnv) restart() {
	e.h.Close()
	e.ts.Close()
	if err := e.store().Close(); err != nil {
		e.t.Fatal(err)
	}
	st, err := store.Open(e.dir)
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { _ = st.Close() })
	e.mu.Lock()
	e.st = st
	e.mu.Unlock()
	e.serve()
}

func (e *evalEnv) send(text string, parent *int64) {
	e.t.Helper()
	in, err := json.Marshal(map[string]any{"text": text, "parent": parent})
	if err != nil {
		e.t.Fatal(err)
	}
	if status, body := do(e.t, http.MethodPost, e.url+"/channels/1/send", string(in)); status != 202 {
		e.t.Fatalf("send = %d %s, want 202", status, body)
	}
}

// say sends a user message and waits for the chain it starts to end.
func (e *evalEnv) say(text string, parent *int64) {
	e.t.Helper()
	e.send(text, parent)
	e.idle()
}

func (e *evalEnv) idle() {
	e.t.Helper()
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		if _, body := do(e.t, http.MethodGet, e.url+"/status", ""); body == `{"running":0}` {
			return
		}
	}
	e.t.Fatal("channel still running after 20s")
}

// evalAgent answers a wake by the last message tag it holds, as a backend whose usage
// reporting matches the real one: claude reports its process's running cost, codex none.
// Send hands the Runner one []agent.Event per turn, the shape a recorded stream has.
type evalAgent struct {
	env     *evalEnv
	events  chan agent.Event
	closed  sync.Once
	role    string
	session string
	started bool
	process float64 // cost total this process has reported
}

func (a *evalAgent) Start(ctx context.Context, cfg agent.Config) error {
	return a.open(ctx, cfg, "")
}

func (a *evalAgent) Resume(ctx context.Context, cfg agent.Config, sessionID string) error {
	return a.open(ctx, cfg, sessionID)
}

func (a *evalAgent) open(ctx context.Context, cfg agent.Config, sessionID string) error {
	tok, err := a.env.store().Token(ctx, cfg.MCPServers["reef"].Env[channel.TokenEnv])
	if err != nil {
		return err
	}
	a.role, a.session = tok.Role, sessionID
	if sessionID == "" {
		a.session = "session-" + tok.Role
	}
	return nil
}

func (a *evalAgent) Events() <-chan agent.Event { return a.events }

func (a *evalAgent) Interrupt() error {
	a.closed.Do(func() { close(a.events) })
	return nil
}

func (a *evalAgent) Send(ctx context.Context, prompt string) error {
	e := a.env
	tags := _evalTag.FindAllStringSubmatch(prompt, -1)
	if len(tags) == 0 {
		return fmt.Errorf("prompt of %s holds no message tag", a.role)
	}
	chain, _ := strconv.Atoi(tags[len(tags)-1][1])
	step := tags[len(tags)-1][2]
	e.mu.Lock()
	e.sent = append(e.sent, evalSend{a.role, a.session, prompt})
	e.mu.Unlock()
	if a.role == "developer" && chain == 6 && step == "1" && e.block.CompareAndSwap(false, true) {
		e.blocked <- struct{}{}
		return nil
	}
	var parent *int64
	if th := _evalThread.FindAllStringSubmatch(prompt, -1); len(th) > 0 {
		id, _ := strconv.ParseInt(th[len(th)-1][1], 10, 64)
		parent = &id
	}
	tool, text := evalReply(a.role, chain, step)
	var evs []agent.Event
	if !a.started {
		a.started = true
		evs = append(evs, agent.Event{Kind: agent.KindSession, SessionID: a.session, Model: "model-" + e.backend})
	}
	if tool != "" {
		input, err := json.Marshal(map[string]any{"body": tool, "parent": parent})
		if err != nil {
			return err
		}
		_, err = e.store().CreateMessage(ctx, store.Message{
			ChannelID: 1, ParentID: parent, Author: a.role, Body: tool,
		})
		if err != nil {
			return err
		}
		evs = append(evs,
			agent.Event{Kind: agent.KindToolCall, ToolID: "t1", ToolName: "mcp__reef__post_to_channel", Input: input},
			agent.Event{Kind: agent.KindToolResult, ToolID: "t1", Output: "posted"})
	}
	evs = append(evs,
		agent.Event{Kind: agent.KindText, Text: text},
		agent.Event{Kind: agent.KindUsage, Usage: &agent.Usage{InputTokens: 100, OutputTokens: 50}})
	end := agent.Event{Kind: agent.KindTurnEnd}
	if e.backend == "claude" {
		e.mu.Lock()
		cost := 0.10
		if e.turns[a.session] == 0 {
			cost = 1.00
		}
		e.turns[a.session]++
		e.totals[a.session] += cost
		total := e.totals[a.session]
		e.mu.Unlock()
		reported := total - a.process
		a.process = total
		end.CostUSD = &reported
	}
	for _, ev := range append(evs, end) {
		a.events <- ev
	}
	return nil
}

func evalBody(mention string, chain int, step string, tail string) string {
	return fmt.Sprintf("%s[c%d.%s] %s%s", mention, chain, step, strings.Repeat(_evalFiller, 8), tail)
}

// evalReply is what role says to the last message of the wake, tagged chain and step: a
// post_to_channel call (tool, may be empty) and the turn's plain reply (text).
func evalReply(role string, chain int, step string) (tool, text string) {
	b := func(mention, step, tail string) string { return evalBody(mention, chain, step, tail) }
	switch {
	case role == "lead" && step == "0" && chain == 0:
		return "", b("", "1", "")
	case role == "lead" && step == "0" && chain == 6:
		return "", b("@developer @tester ", "1", "")
	case role == "lead" && step == "0":
		return "", b("@developer ", "1", "")
	case role == "lead" && step == "4":
		return "", b("", "5", "")
	case role == "lead":
		return "", b("", "z", "")
	case role == "developer" && step == "1" && chain == 2:
		return b("@tester ", "2", ""), b("@lead ", "x", "cc @tester")
	case role == "developer" && step == "1":
		return "", b("@tester ", "2", "")
	case role == "developer":
		return "", b("@lead ", "4", "")
	case chain == 4:
		return "", b("@developer ", "3", "I'll report to @lead later.")
	default:
		return "", b("@developer ", "3", "")
	}
}

func TestChannelEval(t *testing.T) {
	for _, backend := range []string{"claude", "codex"} {
		t.Run(backend, func(t *testing.T) { runEval(t, backend) })
	}
}

func runEval(t *testing.T, backend string) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	e := &evalEnv{
		t: t, backend: backend, dir: dir, st: st, blocked: make(chan struct{}, 1),
		totals: map[string]float64{}, turns: map[string]int{},
	}
	ctx := context.Background()
	_, err = st.ApplyTeam(ctx, store.Team{
		Name: "small",
		Roles: []store.Role{
			{Name: "lead", Lead: true, Backend: backend},
			{Name: "developer", Backend: backend},
			{Name: "tester", Backend: backend},
		},
		Gates: []store.Gate{
			{From: "lead", To: "developer"}, {From: "developer", To: "tester"},
			{From: "tester", To: "developer"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	e.serve()
	do(t, http.MethodPost, e.url+"/projects", `{"name":"reef","path":"`+t.TempDir()+`"}`)
	do(t, http.MethodPost, e.url+"/channels",
		`{"projectId":1,"name":"browser issue","backend":"`+backend+`","team":"small"}`)

	root := int64(1)
	e.say(evalBody("", 0, "0", ""), nil)
	for c := 1; c <= _evalChains; c++ {
		e.say(evalBody("", c, "0", ""), &root)
	}
	e.send(evalBody("", 6, "0", ""), &root)
	<-e.blocked
	e.restart()
	e.idle()

	st = e.store()
	turns, err := st.Turns(ctx, store.TurnFilter{ChannelID: &root})
	if err != nil {
		t.Fatal(err)
	}
	msgs, err := st.Messages(ctx, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	approvals, err := st.PendingApprovals(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}

	var total, dupChars, inPrompt int
	for _, s := range e.sent {
		total += len(s.prompt)
		for _, m := range msgs {
			if strings.Count(s.prompt, m.Body) > 1 {
				inPrompt++
				break
			}
		}
	}
	dupBodies := 0
	for _, role := range []string{"lead", "developer", "tester"} {
		for _, m := range msgs {
			seen := 0
			if m.Author == role {
				seen = 1
			}
			for _, s := range e.sent {
				if s.role == role {
					seen += strings.Count(s.prompt, m.Body)
				}
			}
			if seen > 1 {
				dupBodies++
				dupChars += len(m.Body) * (seen - 1)
			}
		}
	}

	var largest, wakes, overSized, doublePost, plainText int
	lastSeen := map[string]int64{}
	for _, tn := range turns {
		if tn.WakeText == "" {
			continue
		}
		wakes++
		largest = max(largest, len(tn.WakeText))
		src := _evalSources.FindAllStringSubmatch(tn.WakeText, -1)
		newest, _ := strconv.ParseInt(src[len(src)-1][1], 10, 64)
		fresh := 0
		for _, m := range msgs {
			if m.Author != tn.Role && m.ID > lastSeen[tn.Role] && m.ID <= newest {
				fresh += len(m.Body)
			}
		}
		lastSeen[tn.Role] = newest
		if len(tn.WakeText) > fresh+_evalHeader {
			overSized++
		}
		tags := _evalTag.FindAllString(tn.WakeText, -1)
		switch last := tags[len(tags)-1]; {
		case tn.Role == "lead" && last == "[c2.x]":
			doublePost++
		case tn.Role == "lead" && last == "[c4.3]":
			plainText++
		}
	}

	var costBad []string
	stored := map[string]float64{}
	for _, tn := range turns {
		if tn.CostUSD != nil {
			stored[tn.Role] += *tn.CostUSD
		}
	}
	for _, role := range []string{"lead", "developer", "tester"} {
		if real := e.totals["session-"+role]; math.Abs(stored[role]-real) > 1e-6 {
			costBad = append(costBad, fmt.Sprintf("%s stored %.2f, session cost %.2f", role, stored[role], real))
		}
	}

	lost := "the tester's wake for the lead's message to @developer @tester was lost"
	for _, tn := range turns {
		if tn.Role == "tester" && strings.Contains(tn.WakeText, "[c6.1]") {
			lost = ""
		}
	}
	for _, a := range approvals {
		if a.To == "tester" {
			lost = ""
		}
	}

	t.Logf("%s: %d turns, %d messages, %d characters sent, %d duplicate characters, "+
		"largest wake %d, wake count %d, wasted wakes %d (double post) + %d (plain-text @name)",
		backend, len(turns), len(msgs), total, dupChars, largest, wakes, doublePost, plainText)
	t.Logf("%s: stored turn cost %v, session cost %v", backend, stored, e.totals)

	failure := func(n int, what string) string {
		if n == 0 {
			return ""
		}
		return fmt.Sprintf("%d %s", n, what)
	}
	expect(t, backend, "duplicate bodies", failure(dupBodies, "bodies sent twice to a role in one session"))
	expect(t, backend, "duplicate in prompt", failure(inPrompt, "prompts hold a body twice"))
	expect(t, backend, "wake size", failure(overSized, "wakes are larger than their new messages plus a header"))
	expect(t, backend, "double post wake", failure(doublePost, "wakes from the second post of a turn"))
	expect(t, backend, "plain-text mention wake", failure(plainText, "wakes from an @name inside a sentence"))
	expect(t, backend, "cost sum", strings.Join(costBad, "; "))
	expect(t, backend, "restart", lost)
}

func expect(t *testing.T, backend, check, failure string) {
	t.Helper()
	ticket, marked := _expectedFail[backend][check]
	switch {
	case failure != "" && marked:
		t.Logf("%s: %q fails as expected, %s fixes it: %s", backend, check, ticket, failure)
	case failure != "":
		t.Errorf("%s: %q: %s", backend, check, failure)
	case marked:
		t.Errorf("%s: %q passes: remove its mark for %s from _expectedFail", backend, check, ticket)
	}
}
