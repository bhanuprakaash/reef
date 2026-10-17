package channel

import (
	"context"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

func TestContext_IndexOmitsRetiredDecisions(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	for _, body := range []string{"use sessions", "use cookies"} {
		if _, err := st.CreateDecision(ctx, store.Decision{
			FeatureID: f.ID, Body: body, Author: "user",
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.CreateDecision(ctx, store.Decision{
		FeatureID: f.ID, Body: "use tokens", Author: "user", Replaces: []int64{1},
	}); err != nil {
		t.Fatal(err)
	}

	got, err := Context(ctx, st, f.ID, "developer")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "use sessions") {
		t.Errorf("Context index lists retired decision 1:\n%s", got)
	}
	for _, want := range []string{"decision 2: use cookies", "decision 3: use tokens"} {
		if !strings.Contains(got, want) {
			t.Errorf("Context index does not contain %q:\n%s", want, got)
		}
	}
}
