package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

func getJSON[T any](t *testing.T, url string) T {
	t.Helper()
	status, body := do(t, http.MethodGet, url, "")
	if status != http.StatusOK {
		t.Fatalf("GET %s = %d %s", url, status, body)
	}
	var v T
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func chatLines(chats []store.BotChat) []string {
	var lines []string
	for _, c := range chats {
		origin := ""
		if c.Origin != nil {
			origin = fmt.Sprintf(" from %d %q", c.Origin.SpaceID, c.Origin.Label)
		}
		lines = append(lines, fmt.Sprintf("%s %d bot %d %q%s", c.Kind, c.SpaceID, c.BotID, c.Label, origin))
	}
	slices.Sort(lines)
	return lines
}

func TestBotChatsListBothSidesAndActivityReadsOnly(t *testing.T) {
	ctx := context.Background()
	e := newConsultEnv(t)
	e.consult(t, 1, `[{"bot":"ben","body":"q1"}]`)

	ada := chatLines(getJSON[[]store.BotChat](t, fmt.Sprintf("%s/bots/%d/chats", e.url, e.ada)))
	wantAda := []string{
		fmt.Sprintf(`chat 2 bot %d "ada chat"`, e.ada),
		fmt.Sprintf(`consultation 4 bot %d "Consultation with ben · q1" from 2 "ada · ada chat"`, e.ben),
	}
	if !slices.Equal(ada, wantAda) {
		t.Errorf("Ada's chats = %q, want %q", ada, wantAda)
	}
	ben := chatLines(getJSON[[]store.BotChat](t, fmt.Sprintf("%s/bots/%d/chats", e.url, e.ben)))
	wantBen := []string{
		fmt.Sprintf(`chat 3 bot %d "ben chat"`, e.ben),
		fmt.Sprintf(`consultation 4 bot %d "Consultation from ada · q1" from 2 "ada · ada chat"`, e.ben),
	}
	if !slices.Equal(ben, wantBen) {
		t.Errorf("Ben's chats = %q, want %q", ben, wantBen)
	}

	inProject := chatLines(getJSON[[]store.BotChat](t, fmt.Sprintf("%s/bots/%d/chats?project=1", e.url, e.ben)))
	if !slices.Equal(inProject, wantBen) {
		t.Errorf("Ben's chats under the Project = %q, want the consultation of its origin chat too: %q", inProject, wantBen)
	}

	count := func(table string) (n int) {
		if err := e.st.DB().QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	turns, sessions := count("turn"), count("bot_session")
	url := fmt.Sprintf("%s/spaces/4/bots/%d/activity", e.url, e.ben)
	if _, raw := do(t, http.MethodGet, url, ""); !strings.Contains(raw, `"toolCalls":[]`) {
		t.Errorf("activity = %s, want a turn with no tool calls to show []", raw)
	}
	a := getJSON[store.BotActivity](t, url)
	if a.Label != "Consultation from ada · q1" || a.Origin == nil || a.Origin.SpaceID != 2 || len(a.Turns) == 0 ||
		len(a.Exchanges) == 0 {
		t.Errorf("activity = %+v, want the consultation with its origin, turns and exchanges", a)
	}
	if err := e.st.CloseSpace(ctx, 4); err != nil {
		t.Fatal(err)
	}
	if again := getJSON[store.BotActivity](t, url); len(again.Turns) != len(a.Turns) || again.Space.ClosedAt == nil {
		t.Errorf("closed activity = %+v, want the same turns on a closed Space", again)
	}
	if n, m := count("turn"), count("bot_session"); n != turns || m != sessions {
		t.Errorf("turns %d -> %d, sessions %d -> %d, want a read to start none", turns, n, sessions, m)
	}
	if status, _ := do(t, http.MethodGet, fmt.Sprintf("%s/spaces/4/bots/%d/activity", e.url, e.ada), ""); status != http.StatusNotFound {
		t.Errorf("activity of a Bot outside the Space = %d, want 404", status)
	}
}
