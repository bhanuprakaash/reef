package store

import (
	"context"
	"testing"
)

func TestStore_Setting(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()

	if got, err := st.Setting(ctx, "embedding.provider"); err != nil || got != "" {
		t.Fatalf("missing setting = %q, %v; want empty", got, err)
	}
	for _, v := range []string{"openai", "voyage"} {
		if err := st.SetSettings(ctx, map[string]string{"embedding.provider": v}); err != nil {
			t.Fatal(err)
		}
		if got, err := st.Setting(ctx, "embedding.provider"); err != nil || got != v {
			t.Fatalf("setting = %q, %v; want %q", got, err, v)
		}
	}
}
