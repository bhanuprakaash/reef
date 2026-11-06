package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/bhanuprakaash/reef/store"
)

const (
	_adaChat = 2
	_benChat = 3
)

// consultEnv has Ada's direct conversation (Space 2) and Ben's (Space 3). A user message of
// "consult:" and a JSON list of tool arguments makes Ada call consult with each.
type consultEnv struct {
	*liveEnv
	ada, ben int64

	mu      sync.Mutex
	results []string
}

func newConsultEnv(t *testing.T) *consultEnv {
	t.Helper()
	e := &consultEnv{liveEnv: newLiveEnv(t)}
	for _, name := range []string{"ada", "ben"} {
		if status, body := do(t, http.MethodPost, e.url+"/bots",
			`{"name":"`+name+`","backend":"claude"}`); status != 201 {
			t.Fatalf("create %s = %d %s", name, status, body)
		}
		do(t, http.MethodPost, e.url+"/spaces",
			fmt.Sprintf(`{"projectId":1,"name":"%s chat","botId":%d,"kind":"conversation"}`,
				name, *testServerBot(t, e.st, name)))
	}
	e.ada, e.ben = *testServerBot(t, e.st, "ada"), *testServerBot(t, e.st, "ben")
	e.fake.onRun = func(role, text string, _ *int64) {
		tool := "consult"
		_, args, ok := strings.Cut(text, "consult:")
		if !ok {
			tool = "send_to_space"
			_, args, ok = strings.Cut(text, "send:")
		}
		if role != "ada" || !ok {
			return
		}
		turn, err := e.st.CreateTurn(context.Background(), store.Turn{
			SpaceID: _adaChat, Role: role, BotID: &e.ada, Backend: "claude", WakeText: text})
		if err != nil {
			t.Error(err)
			return
		}
		defer func() {
			turn.Status = "done"
			_, _ = e.st.EndTurn(context.Background(), turn)
		}()
		tok, err := e.st.CreateToken(context.Background(), _adaChat, e.ada)
		if err != nil {
			t.Error(err)
			return
		}
		var calls []map[string]any
		if err := json.Unmarshal([]byte(args), &calls); err != nil {
			t.Error(err)
			return
		}
		call := callTool(t, e.st, tok)
		for _, c := range calls {
			got, _ := call(tool, c)
			e.mu.Lock()
			e.results = append(e.results, got)
			e.mu.Unlock()
		}
	}
	return e
}

func (e *consultEnv) say(t *testing.T, text string) {
	t.Helper()
	body, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		t.Fatal(err)
	}
	if status, got := do(t, http.MethodPost, e.url+"/spaces/2/send", string(body)); status >= 300 {
		t.Fatalf("send = %d %s", status, got)
	}
}

// lines is the messages of the Space as "author: body (exchange id)".
func (e *consultEnv) lines(t *testing.T, spaceID int64) []string {
	t.Helper()
	msgs, err := e.st.Messages(context.Background(), spaceID, 0)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, m := range msgs {
		id := int64(0)
		if m.ExchangeID != nil {
			id = *m.ExchangeID
		}
		lines = append(lines, fmt.Sprintf("%s: %s (exchange %d)", m.Author, m.Body, id))
	}
	return lines
}

// consult sends the calls and waits until the Space has replies to all of them, returning the
// tool results.
func (e *consultEnv) consult(t *testing.T, replies int, calls string) []string {
	t.Helper()
	e.mu.Lock()
	e.results = nil
	e.mu.Unlock()
	e.say(t, "consult:"+calls)
	for {
		n := 0
		for _, l := range e.lines(t, _adaChat) {
			if strings.HasPrefix(l, "ben: ") {
				n++
			}
		}
		if n >= replies {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	for id := range int64(8) {
		e.waitIdle(t, int(id))
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return slices.Clone(e.results)
}

func TestConsultKeepsTheChatAndLinksEveryExchange(t *testing.T) {
	ctx := context.Background()
	e := newConsultEnv(t)

	got := e.consult(t, 1, `[{"bot":"ben","body":"q1"}]`)
	if want := []string{`{"consultation":4,"exchange":1,"state":"waiting"}`}; !slices.Equal(got, want) {
		t.Fatalf("first consult = %q, want %q", got, want)
	}
	chat, err := e.st.Space(ctx, _adaChat)
	team, _ := e.st.SpaceTeam(ctx, _adaChat)
	if err != nil || chat.Kind != store.KindConversation || chat.MovedTo != nil || len(team.Roles) != 1 {
		t.Errorf("origin chat = %+v with %d members, %v, want the same conversation with one member",
			chat, len(team.Roles), err)
	}
	if lines := e.lines(t, _benChat); len(lines) != 0 {
		t.Errorf("Ben's own chat = %q, want nothing written", lines)
	}
	consultation, err := e.st.Space(ctx, 4)
	if err != nil || consultation.Kind != store.KindConsultation || consultation.Name != "Consultation from ada · q1 #4" ||
		consultation.BotID == nil || *consultation.BotID != e.ben {
		t.Errorf("consultation = %+v, %v", consultation, err)
	}
	wantRan := "[Space 2, consultation 4, exchange 1]\nben replied: hi"
	if ran := e.ranTexts(); !slices.Contains(ran, wantRan) {
		t.Errorf("ran = %q, want the wake of Ada %q", ran, wantRan)
	}

	got = e.consult(t, 2, `[{"bot":"ben","body":"q2","consultation":4}]`)
	if want := []string{`{"consultation":4,"exchange":3,"state":"waiting"}`}; !slices.Equal(got, want) {
		t.Fatalf("follow-up = %q, want %q", got, want)
	}
	got = e.consult(t, 3, `[{"bot":"ben","body":"q3"}]`)
	if want := []string{`{"consultation":5,"exchange":5,"state":"waiting"}`}; !slices.Equal(got, want) {
		t.Fatalf("unrelated question = %q, want %q", got, want)
	}
	got = e.consult(t, 4, `[{"bot":"ben","body":"q4"},{"bot":"ben","body":"q4"}]`)
	if want := slices.Repeat([]string{`{"consultation":6,"exchange":7,"state":"waiting"}`}, 2); !slices.Equal(got, want) {
		t.Fatalf("repeated call = %q, want one exchange %q", got, want)
	}

	var exchanges []string
	for _, l := range e.lines(t, _adaChat) {
		if !strings.HasPrefix(l, "user:") {
			exchanges = append(exchanges, l)
		}
	}
	want := []string{
		"reef: Asked ben: q1 (exchange 1)", "ben: hi (exchange 1)",
		"reef: Asked ben: q2 (exchange 3)", "ben: hi (exchange 3)",
		"reef: Asked ben: q3 (exchange 5)", "ben: hi (exchange 5)",
		"reef: Asked ben: q4 (exchange 7)", "ben: hi (exchange 7)",
	}
	if !slices.Equal(exchanges, want) {
		t.Errorf("chat = %q, want %q", exchanges, want)
	}
	for space, n := range map[int64]int{4: 2, 5: 1, 6: 1} {
		if got := e.turnCount(t, space); got != n {
			t.Errorf("turns in consultation %d = %d, want %d", space, got, n)
		}
	}
}

func TestConsultationsWithTheSameOpeningHaveDistinctNames(t *testing.T) {
	e := newConsultEnv(t)
	e.consult(t, 1, `[{"bot":"ben","body":"same words"}]`)
	e.consult(t, 2, `[{"bot":"ben","body":"same words"}]`)
	var names []string
	for _, id := range []int64{4, 5} {
		c, err := e.st.Space(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, c.Name)
	}
	if want := []string{"Consultation from ada · same words #4", "Consultation from ada · same words #5"}; !slices.Equal(names, want) {
		t.Errorf("names = %q, want %q", names, want)
	}
}

func TestConsultRefusals(t *testing.T) {
	ctx := context.Background()
	e := newConsultEnv(t)
	e.consult(t, 1, `[{"bot":"ben","body":"q1"}]`)

	tools := func(spaceID, botID int64) []string {
		tok, err := e.st.CreateToken(ctx, spaceID, botID)
		if err != nil {
			t.Fatal(err)
		}
		ct, st2 := mcp.NewInMemoryTransports()
		if _, err := NewMCP(e.st, testMemory(t, e.st), tok, t.TempDir()).Connect(ctx, st2, nil); err != nil {
			t.Fatal(err)
		}
		cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0"}, nil).Connect(ctx, ct, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = cs.Close() }()
		var names []string
		for tool, err := range cs.Tools(ctx, nil) {
			if err != nil {
				t.Fatal(err)
			}
			names = append(names, tool.Name)
		}
		return names
	}
	if !slices.Contains(tools(_adaChat, e.ada), "consult") {
		t.Error("a direct conversation has no consult tool")
	}
	if slices.Contains(tools(4, e.ben), "consult") {
		t.Error("the consulted Bot has the consult tool: nested consultations are off")
	}

	if status, body := do(t, http.MethodPost, e.url+"/spaces/4/members", fmt.Sprintf(`{"botId":%d}`, e.ada)); status != 400 {
		t.Errorf("add a member to a consultation = %d %s, want 400", status, body)
	}
	if team, err := e.st.SpaceTeam(ctx, 4); err != nil || len(team.Roles) != 1 {
		t.Errorf("consultation members = %+v, %v, want only Ben", team.Roles, err)
	}

	key := "wrong"
	for _, in := range []store.DirectMessage{
		{SpaceID: 1, OriginSpaceID: ptr(int64(_adaChat))},
		{SpaceID: _benChat, OriginSpaceID: ptr(int64(_adaChat))},
		{SpaceID: 4, OriginSpaceID: ptr(int64(_benChat))},
		{OriginSpaceID: ptr(int64(1))},
	} {
		in.From, in.To, in.FromBotID, in.ToBotID, in.Body, in.RequestKey = "ada", "ben", &e.ada, &e.ben, "q", &key
		if _, _, err := e.st.Consult(ctx, in, "t", "n"); err == nil {
			t.Errorf("consult %+v succeeded, want a refusal", in)
		}
	}
}

func ptr[T any](v T) *T { return &v }

func TestConsultSessionsAndRestart(t *testing.T) {
	ctx := context.Background()
	e := newConsultEnv(t)
	e.consult(t, 1, `[{"bot":"ben","body":"q1"}]`)

	if _, err := e.st.AdoptBotSession(ctx, 4, e.ben); err != nil {
		t.Fatal(err)
	}
	status, body := do(t, http.MethodPost, fmt.Sprintf("%s/spaces/4/bots/%d/refresh", e.url, e.ben), "")
	if status != 200 || body != `{"pending":false}` {
		t.Errorf("refresh of Ben in the consultation = %d %s", status, body)
	}
	status, body = do(t, http.MethodPost, fmt.Sprintf("%s/bots/%d/revoke", e.url, e.ben), "")
	if status != 200 || !strings.Contains(body, `"sessions":1`) {
		t.Errorf("revoke of Ben = %d %s, want his consultation session ended", status, body)
	}
	lines := e.lines(t, 4)
	if last := lines[len(lines)-1]; !strings.HasPrefix(last, "reef: ben was revoked") {
		t.Errorf("last line of the consultation = %q, want the revoke note", last)
	}

	key := "cut"
	cut, _, err := e.st.Consult(ctx, store.DirectMessage{
		SpaceID: 4, OriginSpaceID: ptr(int64(_adaChat)), From: "ada", To: "ben", FromBotID: &e.ada,
		ToBotID: &e.ben, Body: "q2", RequestKey: &key}, "", "Asked ben: q2")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.st.CreateTurn(ctx, store.Turn{
		SpaceID: 4, Role: "ben", Backend: "claude", BotID: &e.ben, DirectMessageID: &cut.ID}); err != nil {
		t.Fatal(err)
	}
	e.restart(t)
	if _, err := e.st.FailRunningTurns(ctx, nil); err != nil {
		t.Fatal(err)
	}
	ex := e.exchange(t, cut.ID)
	if ex.Request.State != store.DirectFailed || ex.Response == nil || ex.Response.Body != store.ExchangeInterrupted {
		t.Errorf("cut exchange = %+v, want failed with the interruption note", ex)
	}
	lines = e.lines(t, _adaChat)
	want := fmt.Sprintf("ben: No reply: failed, %s (exchange %d)", store.ExchangeInterrupted, cut.ID)
	if last := lines[len(lines)-1]; last != want {
		t.Errorf("last line of the chat = %q, want %q", last, want)
	}
	if n := e.turnCount(t, 4); n != 2 {
		t.Errorf("turns in the consultation = %d, want 2: the cut one is not replayed", n)
	}
}
