package memory

import (
	"context"
	"testing"
)

func TestSaved(t *testing.T) {
	ctx := context.Background()
	s, _ := openTest(t)
	w := writer()
	for _, n := range []Note{
		{Kind: KindFact, Body: "accepted one"},
		{Kind: KindFact, Body: "candidate one", NeedsApproval: true},
		{Kind: KindFact, Body: "retired one"},
	} {
		if _, err := s.Remember(ctx, w, n); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Retire(ctx, 3); err != nil {
		t.Fatal(err)
	}
	other := Ref{Kind: ScopeProject, ID: "7"}

	tests := []struct {
		name   string
		scope  Ref
		body   string
		except []int64
		want   bool
	}{
		{"accepted", w.Scope, "accepted one", nil, true},
		{"candidate", w.Scope, "candidate one", nil, true},
		{"retired", w.Scope, "retired one", nil, false},
		{"other scope", other, "accepted one", nil, false},
		{"not exact", w.Scope, "accepted", nil, false},
		{"only the excepted one", w.Scope, "accepted one", []int64{1}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.Saved(ctx, tt.scope, tt.body, tt.except)
			if err != nil || got != tt.want {
				t.Errorf("Saved(%v, %q) = %v, %v, want %v", tt.scope, tt.body, got, err, tt.want)
			}
		})
	}
}
