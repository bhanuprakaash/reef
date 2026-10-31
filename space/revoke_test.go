package space

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

func TestSession_RevokedBotStartsNewContextAfterRestart(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionSpace(t, st, f, false)
	architect := testBotID(t, st, "architect")

	old := newRecordingAgent()
	old.session = "s1"
	dir := t.TempDir()
	live := openRefreshSession(t, st, ch, dir, old.factory)
	if err := sendAs(ctx, st, ch.ID, live.Run, "before", discard); err != nil {
		t.Fatal(err)
	}
	token := old.cfg.MCPServers["reef"].Env[TokenEnv]
	if _, err := st.Token(ctx, token); err != nil {
		t.Fatalf("token before revoking: %v", err)
	}

	spaces, tokens, _, err := st.RevokeBot(ctx, *architect)
	if err != nil || !slices.Equal(spaces, []int64{ch.ID}) || tokens != 1 {
		t.Fatalf("RevokeBot = %v, %d tokens, %v; want Space %d and 1 token", spaces, tokens, err, ch.ID)
	}
	if _, err := st.Token(ctx, token); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("token after revoking = %v, want ErrNotFound", err)
	}
	if err := live.Close(); err != nil {
		t.Errorf("close of the revoked session = %v, want nil", err)
	}

	next := newRecordingAgent()
	next.session = "s2"
	restarted := openRefreshSession(t, st, ch, dir, next.factory)
	if err := sendAs(ctx, st, ch.ID, restarted.Run, "after", discard); err != nil {
		t.Fatal(err)
	}
	if !next.started || next.resumed != "" {
		t.Errorf("started = %v, resumed %q; want a new context and the revoked s1 never resumed", next.started, next.resumed)
	}
	if len(next.prompts) != 1 || !strings.Contains(next.prompts[0], "## Handover") {
		t.Errorf("prompts = %q, want a handover", next.prompts)
	}
	if notes := messageBodies(t, st, ch.ID); strings.Contains(strings.Join(notes, "\n"), "refreshed") {
		t.Errorf("messages = %q, want no refreshed notice after a revoke", notes)
	}
	got, err := st.ActiveBotSession(ctx, ch.ID, *architect)
	if err != nil || got.ID != 2 {
		t.Errorf("active bot session = %+v, %v, want the replacement with id 2", got, err)
	}
}
