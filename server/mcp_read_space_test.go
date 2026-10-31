package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

func rowCounts(t *testing.T, st *store.Store, spaceID int64) [4]int {
	t.Helper()
	var n [4]int
	for i, table := range []string{"turn", "bot_session", "space_member", "space_project"} {
		if err := st.DB().QueryRow("SELECT COUNT(*) FROM "+table+" WHERE space_id = ?", spaceID).
			Scan(&n[i]); err != nil {
			t.Fatal(err)
		}
	}
	return n
}

func TestMCPReadSpaceReadsReferencedSpaceWithoutWakingIt(t *testing.T) {
	st, _ := mcpFixture(t)
	ctx := context.Background()
	tok, err := testToken(t, st, ctx, 4, "architect")
	if err != nil {
		t.Fatal(err)
	}
	call := callTool(t, st, tok)

	// Spaces 1 and 3 are both named "architect": the ids tell them apart.
	msg, err := st.CreateMessage(ctx, store.Message{
		SpaceID: 4, Author: "user",
		Body: "Why sessions in #[architect](space:3) and #[architect](space:1)?",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveMessageRefs(ctx, msg.ID, store.ParseSpaceRefs(msg.Body)); err != nil {
		t.Fatal(err)
	}
	refs, err := st.MessageRefs(ctx, msg.ID)
	if err != nil || len(refs) != 2 || refs[0].SpaceID != 1 || refs[1].SpaceID != 3 {
		t.Fatalf("message refs = %+v, %v, want Spaces 1 and 3", refs, err)
	}

	if got, isErr := call("read_space", map[string]any{"spaceId": 2}); !isErr ||
		got != "space 2: not found" {
		t.Errorf("read_space of an unreferenced Space = %q (error %v), want not found", got, isErr)
	}

	closeSpace(t, st, 3)
	before := rowCounts(t, st, 3)
	got, isErr := call("read_space", map[string]any{"spaceId": 3, "query": "billing"})
	if isErr {
		t.Fatalf("read_space = %q, want a result", got)
	}
	for _, want := range []string{
		`"name":"architect"`, `"closed":true`, `"body":"use sessions for billing"`, `"id":3`,
		`"snapshot":{"asOf":`, `"running":false`, `"lastTurn":null`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("read_space = %s, missing %s", got, want)
		}
	}
	if after := rowCounts(t, st, 3); after != before {
		t.Errorf("rows of Space 3 = %v after read, %v before", after, before)
	}

	if err := st.DeleteSpace(ctx, 3); err != nil {
		t.Fatal(err)
	}
	got, isErr = call("read_space", map[string]any{"spaceId": 3})
	want := `{"memory":null,"messages":null,"spaceId":3,` +
		`"unavailable":"this Space was deleted; its messages and memory are gone"}`
	if got != want || isErr {
		t.Errorf("read_space of a deleted Space = %q (error %v), want %q", got, isErr, want)
	}
}

func TestMCPReadSpaceCreatesNoWorkspace(t *testing.T) {
	st, _ := mcpFixture(t)
	ctx := context.Background()
	tok, err := testToken(t, st, ctx, 4, "architect")
	if err != nil {
		t.Fatal(err)
	}
	dataDir := t.TempDir()
	msg, err := st.CreateMessage(ctx, store.Message{
		SpaceID: 4, Author: "user", Body: "see #[architect](space:1)",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveMessageRefs(ctx, msg.ID, []int64{1}); err != nil {
		t.Fatal(err)
	}
	if _, isErr := callToolResult(t, st, tok, dataDir)("read_space", map[string]any{"spaceId": 1}); isErr {
		t.Fatal("read_space failed")
	}
	entries, err := os.ReadDir(dataDir)
	if err != nil || len(entries) != 0 {
		t.Errorf("data dir = %v, %v, want empty (%s)", entries, err, filepath.Base(dataDir))
	}
}
