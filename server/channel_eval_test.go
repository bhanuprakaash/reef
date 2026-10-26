package server

// The channel eval replays a scripted channel shaped like a real "browser issue" channel
// (team small, one long thread, double posts, plain-text @names, an app restart with a wake
// queued) through the real routing and the real channel.Session, once per backend. It also
// replays one real run of each backend, recorded in testdata/eval. Only the agent is fake. It
// lives in package server because the routing is unexported, and the server's session opener
// is the one place the real Session is built. Run it with -v to see its numbers.
//
// A check that fails today is listed in _expectedFail with the ticket that fixes it. The
// eval fails when a listed check passes, so that ticket removes the mark.

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

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
		"duplicate bodies": "#47",
		"wake size":        "#47",
		"double post wake": "#48",
	},
	"claude-recorded": {
		"duplicate bodies": "#47",
		"wake size":        "#47",
	},
	"codex": {
		"duplicate bodies": "#47",
		"wake size":        "#47",
		"double post wake": "#48",
	},
	"codex-recorded": {
		"duplicate bodies": "#47",
		"wake size":        "#47",
	},
}

var (
	_evalTag     = regexp.MustCompile(`\[c(\d+)\.([0-9a-z]+)\]`)
	_evalThread  = regexp.MustCompile(`, thread (\d+)\]`)
	_evalSources = regexp.MustCompile(`\[channel \d+, message (\d+)(?:, thread (\d+))?`)
)

type evalSend struct{ role, session, prompt string }

// evalRecording is one real channel run: the user's messages in order, each sent once the
// channel is idle, and every turn with the prompt Reef sent and the events the adapter emitted.
type evalRecording struct {
	Backend string `json:"backend"`
	Model   string `json:"model"`
	User    []struct {
		Text   string `json:"text"`
		Thread bool   `json:"thread"` // a reply in the thread of the first message
	} `json:"user"`
	// Cost is what each role's session really cost, read from claude's own totals.
	Cost  map[string]float64 `json:"cost,omitempty"`
	Turns []evalTurn         `json:"turns"`
}

type evalTurn struct {
	Role    string        `json:"role"`
	Session string        `json:"session"`
	Prompt  string        `json:"prompt"`
	Events  []agent.Event `json:"events"`
}

type evalEnv struct {
	t       *testing.T
	backend string
	dir     string
	url     string
	ts      *httptest.Server
	h       *Handler
	blocked chan struct{}
	block   atomic.Bool

	rec     *evalRecording // replayed instead of evalReply when set
	mu      sync.Mutex
	st      *store.Store
	sent    []evalSend
	totals  map[string]float64 // what a role's session has really cost
	turns   map[string]int
	replays map[string]int // recorded turns of each role replayed so far
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
				size := 8
				if e.rec != nil {
					for _, tn := range e.rec.Turns {
						size = max(size, len(tn.Events))
					}
				}
				// Send queues a whole turn before the Runner reads any of it.
				return &evalAgent{env: e, events: make(chan agent.Event, size)}, nil
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
// reporting matches the real one: claude reports its session's running cost, codex none.
// Send hands the Runner one []agent.Event per turn, the shape a recorded stream has.
type evalAgent struct {
	env     *evalEnv
	events  chan agent.Event
	closed  sync.Once
	role    string
	session string
	started bool
	mcp     *mcp.ClientSession // the real reef tools a replayed turn calls
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
	if a.env.rec == nil {
		return nil
	}
	client, server := mcp.NewInMemoryTransports()
	if _, err := NewMCP(a.env.store(), tok, a.env.dir).Connect(ctx, server, nil); err != nil {
		return err
	}
	a.mcp, err = mcp.NewClient(&mcp.Implementation{Name: "eval", Version: "v0"}, nil).
		Connect(ctx, client, nil)
	return err
}

func (a *evalAgent) Events() <-chan agent.Event { return a.events }

func (a *evalAgent) Interrupt() error {
	a.closed.Do(func() {
		close(a.events)
		if a.mcp != nil {
			_ = a.mcp.Close()
		}
	})
	return nil
}

func (a *evalAgent) Send(ctx context.Context, prompt string) error {
	e := a.env
	if e.rec != nil {
		return a.replay(ctx, prompt)
	}
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
		e.totals[a.role] += cost
		total := e.totals[a.role]
		e.mu.Unlock()
		end.CostUSD = &total
	}
	for _, ev := range append(evs, end) {
		a.events <- ev
	}
	return nil
}

// replay emits the role's next recorded turn. Each reef tool call that succeeded in the real
// run calls the real tool again, so its messages and direct messages are stored and routed. A
// role woken more often than it was recorded ends its turn with no reply.
func (a *evalAgent) replay(ctx context.Context, prompt string) error {
	e := a.env
	e.mu.Lock()
	e.sent = append(e.sent, evalSend{a.role, a.session, prompt})
	n := e.replays[a.role]
	e.replays[a.role]++
	e.mu.Unlock()
	evs := []agent.Event{{Kind: agent.KindTurnEnd}}
	for _, tn := range e.rec.Turns {
		if tn.Role != a.role {
			continue
		}
		if n == 0 {
			evs = tn.Events
			break
		}
		n--
	}
	failed := map[string]bool{}
	for _, ev := range evs {
		if ev.Kind == agent.KindToolResult && ev.IsError {
			failed[ev.ToolID] = true
		}
	}
	for _, ev := range evs {
		tool, ok := strings.CutPrefix(ev.ToolName, "mcp__reef__")
		if ok && ev.Kind == agent.KindToolCall && !failed[ev.ToolID] {
			var args map[string]any
			if err := json.Unmarshal(ev.Input, &args); err != nil {
				return err
			}
			// A post with a parent goes to the thread of the wake, as it did in the real run.
			if h := _evalSources.FindAllStringSubmatch(prompt, -1); args["parent"] != nil && len(h) > 0 {
				last := h[len(h)-1]
				args["parent"], _ = strconv.ParseInt(cmp.Or(last[2], last[1]), 10, 64)
			}
			if _, err := a.mcp.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args}); err != nil {
				return err
			}
		}
		if ev.CostUSD != nil {
			e.mu.Lock()
			e.totals[a.role] = *ev.CostUSD
			e.mu.Unlock()
		}
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
		t.Run(backend+"-recorded", func(t *testing.T) { replayEval(t, backend) })
	}
}

// newEval serves a new data folder holding team small on backend, a project and channel 1.
func newEval(t *testing.T, backend string) *evalEnv {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	e := &evalEnv{
		t: t, backend: backend, dir: dir, st: st, blocked: make(chan struct{}, 1),
		totals: map[string]float64{}, turns: map[string]int{}, replays: map[string]int{},
	}
	_, err = st.ApplyTeam(context.Background(), store.Team{
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
	return e
}

func runEval(t *testing.T, backend string) {
	e := newEval(t, backend)
	root := int64(1)
	e.say(evalBody("", 0, "0", ""), nil)
	for c := 1; c <= _evalChains; c++ {
		e.say(evalBody("", c, "0", ""), &root)
	}
	e.send(evalBody("", 6, "0", ""), &root)
	<-e.blocked
	e.restart()
	e.idle()

	ctx := context.Background()
	turns, err := e.store().Turns(ctx, store.TurnFilter{ChannelID: &root})
	if err != nil {
		t.Fatal(err)
	}
	approvals, err := e.store().PendingApprovals(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	lost := "the tester's wake for the lead's message to @developer @tester was lost"
	for _, tn := range turns {
		if tn.Role == "tester" && strings.Contains(tn.WakeText, "[c6.1]") {
			lost = ""
		}
	}
	holds, err := e.store().PendingHolds(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range append(approvals, holds...) {
		if a.To == "tester" {
			lost = ""
		}
	}
	e.check(backend)
	expect(t, backend, "restart", lost)
	if len(holds) == 0 {
		return
	}
	if status, body := do(t, http.MethodPost, e.url+"/channels/1/continue", ""); status != 204 {
		t.Fatalf("continue = %d %s, want 204", status, body)
	}
	e.idle()
	turns, err = e.store().Turns(ctx, store.TurnFilter{ChannelID: &root})
	if err != nil {
		t.Fatal(err)
	}
	ran := false
	for _, tn := range turns {
		ran = ran || tn.Role == "tester" && strings.Contains(tn.WakeText, "[c6.1]")
	}
	if !ran {
		t.Error("continue did not run the tester's held wake")
	}
}

// replayEval sends the user's messages of the backend's recording, each once the channel is
// idle, while the agents replay their recorded turns.
func replayEval(t *testing.T, backend string) {
	b, err := os.ReadFile(filepath.Join("testdata", "eval", backend+".json"))
	if err != nil {
		t.Fatal(err)
	}
	e := newEval(t, backend)
	e.rec = &evalRecording{}
	if err := json.Unmarshal(b, e.rec); err != nil {
		t.Fatal(err)
	}
	root := int64(1)
	for _, m := range e.rec.User {
		var parent *int64
		if m.Thread {
			parent = &root
		}
		e.say(m.Text, parent)
	}
	e.check(backend + "-recorded")
}

// check measures what channel 1 sent its agents, prints the numbers and runs the checks
// _expectedFail lists under name.
func (e *evalEnv) check(name string) {
	t := e.t
	ctx := context.Background()
	id := int64(1)
	turns, err := e.store().Turns(ctx, store.TurnFilter{ChannelID: &id})
	if err != nil {
		t.Fatal(err)
	}
	msgs, err := e.store().Messages(ctx, 1, 0)
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

	byID := map[int64]store.Message{}
	second := map[int64]bool{} // posted after another message of its author's same turn
	for _, m := range msgs {
		byID[m.ID] = m
	}
	for _, tn := range turns {
		posts := 0
		for _, m := range msgs {
			if m.Author == tn.Role && tn.EndedAt != nil &&
				!m.CreatedAt.Before(tn.StartedAt) && !m.CreatedAt.After(*tn.EndedAt) {
				posts++
				second[m.ID] = posts > 1
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
		var src []store.Message
		for _, g := range _evalSources.FindAllStringSubmatch(tn.WakeText, -1) {
			id, _ := strconv.ParseInt(g[1], 10, 64)
			if m, ok := byID[id]; ok {
				src = append(src, m)
			}
		}
		if len(src) == 0 {
			continue
		}
		newest := src[len(src)-1].ID
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
		if !slices.ContainsFunc(src, func(m store.Message) bool { return !second[m.ID] }) {
			doublePost++
		}
		handoff := regexp.MustCompile(`(?m)^\s*(?:@[\w-]+[ \t,]+)*@` + tn.Role + `\b`)
		if !slices.ContainsFunc(src, func(m store.Message) bool {
			return m.Author == "user" || handoff.MatchString(m.Body)
		}) {
			plainText++
		}
	}

	var costBad []string
	var tokens agent.Usage
	stored := map[string]float64{}
	for _, tn := range turns {
		if tn.CostUSD != nil {
			stored[tn.Role] += *tn.CostUSD
		}
		tokens.Add(tn.Usage)
	}
	for _, role := range []string{"lead", "developer", "tester"} {
		if real := e.totals[role]; math.Abs(stored[role]-real) > 1e-6 {
			costBad = append(costBad, fmt.Sprintf("%s stored %.2f, session cost %.2f", role, stored[role], real))
		}
	}

	t.Logf("%s: %d turns, %d messages, %d characters sent, %d duplicate characters, "+
		"largest wake %d, wake count %d, wasted wakes %d (double post) + %d (plain-text @name)",
		name, len(turns), len(msgs), total, dupChars, largest, wakes, doublePost, plainText)
	t.Logf("%s: stored turn cost %v, session cost %v, tokens %+v", name, stored, e.totals, tokens)

	failure := func(n int, what string) string {
		if n == 0 {
			return ""
		}
		return fmt.Sprintf("%d %s", n, what)
	}
	expect(t, name, "duplicate bodies", failure(dupBodies, "bodies sent twice to a role in one session"))
	expect(t, name, "duplicate in prompt", failure(inPrompt, "prompts hold a body twice"))
	expect(t, name, "wake size", failure(overSized, "wakes are larger than their new messages plus a header"))
	expect(t, name, "double post wake", failure(doublePost, "wakes from the second post of a turn"))
	expect(t, name, "plain-text mention wake", failure(plainText, "wakes from an @name inside a sentence"))
	expect(t, name, "cost sum", strings.Join(costBad, "; "))
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
