package channel

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/memory"
)

func TestContext_DecisionsAreTheLiveAcceptedOnes(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ms := testMemory(t, st)
	w := memory.Writer{Author: "user", Role: "user", Scope: workScope(f)}
	note := func(body string, n memory.Note) memory.Item {
		t.Helper()
		n.Kind, n.Body = memory.KindDecision, body
		r, err := ms.Remember(ctx, w, n)
		if err != nil {
			t.Fatal(err)
		}
		return r.Item
	}
	sessions := note("use sessions", memory.Note{})
	stale := note("use basic auth", memory.Note{})
	for i := 3; i <= 9; i++ {
		note(fmt.Sprintf("live %d", i), memory.Note{})
	}
	note("use tokens", memory.Note{Replaces: []int64{sessions.ID}})
	if err := ms.Retire(ctx, stale.ID); err != nil {
		t.Fatal(err)
	}
	note("maybe use cookies", memory.Note{NeedsApproval: true})
	rejected := note("use ftp", memory.Note{NeedsApproval: true})
	if err := ms.Reject(ctx, rejected.ID); err != nil {
		t.Fatal(err)
	}

	got, err := Context(ctx, st, ms, f.ID, "developer")
	if err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{"use sessions", "use basic auth", "maybe use cookies", "use ftp"} {
		if strings.Contains(got, gone) {
			t.Errorf("Context lists %q, which is not live:\n%s", gone, got)
		}
	}
	if n := strings.Count(got, "\ndecision "); n != 8 {
		t.Errorf("decision lines = %d, want 8 live decisions:\n%s", n, got)
	}
	for _, want := range []string{"decision 10: use tokens", "decision 3: live 3"} {
		if !strings.Contains(got, want) {
			t.Errorf("Context does not contain %q:\n%s", want, got)
		}
	}
}
