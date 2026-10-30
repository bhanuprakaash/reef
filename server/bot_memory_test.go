package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/memory"
	"github.com/bhanuprakaash/reef/store"
)

func TestRenamingABotKeepsItsMemoryItems(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.ApplyTeam(ctx, store.Team{Name: "core",
		Roles: []store.Role{{Name: "boss", Lead: true, Backend: "claude"}}}); err != nil {
		t.Fatal(err)
	}
	sam, err := st.CreateBot(ctx, store.Bot{Name: "sam", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddMember(ctx, "core", sam.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateProject(ctx, store.Project{Name: "reef", Path: "/tmp/reef"}); err != nil {
		t.Fatal(err)
	}
	f, err := st.CreateWorkItem(ctx, store.WorkItem{ProjectID: 1, Title: "Auth"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := st.CreateSpace(ctx, store.Space{
		ProjectID: 1, WorkItemID: &f.ID, Name: "auth", Role: "boss", Backend: "claude", Team: "core",
	})
	if err != nil {
		t.Fatal(err)
	}
	tok, err := testToken(t, st, ctx, c.ID, "sam")
	if err != nil {
		t.Fatal(err)
	}
	ms := openMemory(t, st)
	h := New(st, ms, t.TempDir(), "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)
	call := callToolMemory(t, st, ms, tok, t.TempDir())

	remember := func(body string) {
		t.Helper()
		out, isErr := call("remember", map[string]any{"kind": "lesson", "body": body, "scope": "bot"})
		if isErr || !strings.Contains(out, `"scope":"bot:3"`) {
			t.Fatalf("remember %q = %q, error %v, want scope bot:3", body, out, isErr)
		}
	}
	remember("before the rename")
	status, body := do(t, http.MethodPut, ts.URL+"/bots/3",
		`{"name":"samuel","backend":"claude"}`)
	if status != http.StatusOK || !strings.Contains(body, `"name":"samuel"`) {
		t.Fatalf("rename = %d %q, want 200 with samuel", status, body)
	}
	remember("after the rename")

	_, total, err := ms.Newest(ctx, memory.Ref{Kind: memory.ScopeBot, ID: "3"}, nil, 0)
	if err != nil || total != 2 {
		t.Errorf("items in bot:3 = %d, %v, want 2", total, err)
	}
}
