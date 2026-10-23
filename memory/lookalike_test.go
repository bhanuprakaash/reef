package memory

import (
	"context"
	"reflect"
	"testing"
)

func TestLookalike_NearDuplicateWithoutVectors(t *testing.T) {
	s, db := openTest(t)
	remember(t, s, writer(), "retry the upload three times", "")

	res, err := s.Remember(context.Background(), writer(),
		Note{Kind: KindFact, Body: "retry the upload three times on failure"})
	if err != nil {
		t.Fatal(err)
	}
	want := []Similar{{ID: 1, Body: "retry the upload three times"}}
	if !reflect.DeepEqual(res.Similar, want) {
		t.Fatalf("Similar = %v, want %v", res.Similar, want)
	}
	if res.Item.ID != 2 {
		t.Fatalf("new item id = %d, want 2", res.Item.ID)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM memory`); n != 2 {
		t.Fatalf("%d items stored, want 2", n)
	}
}

func TestLookalike_UnrelatedWithoutVectors(t *testing.T) {
	tests := []struct {
		name string
		give string
	}{
		{"no shared words", "release notes live in the wiki"},
		{"under the term share", "retry the download twice on failure today"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := openTest(t)
			remember(t, s, writer(), "retry the upload three times", "")

			res, err := s.Remember(context.Background(), writer(), Note{Kind: KindFact, Body: tt.give})
			if err != nil {
				t.Fatal(err)
			}
			if res.Similar != nil {
				t.Fatalf("Similar = %v, want nil", res.Similar)
			}
		})
	}
}

func TestLookalike_OtherScopeIgnored(t *testing.T) {
	s, _ := openTest(t)
	other := writer()
	other.Scope = Ref{Kind: ScopeWorkItem, ID: "8"}
	remember(t, s, other, "retry the upload three times", "")

	res, err := s.Remember(context.Background(), writer(),
		Note{Kind: KindFact, Body: "retry the upload three times"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Similar != nil {
		t.Fatalf("Similar = %v, want nil", res.Similar)
	}
}

func TestLookalike_CosineThreshold(t *testing.T) {
	s, db := openTest(t)
	s.embed = &fakeEmbedder{vectors: map[string][]float32{
		"uses sqlite":       {1, 0},
		"uses sqlite again": {0.95, 0.3122499},
		"sqlite is local":   {0.9, -0.4358899},
	}}
	ctx := context.Background()
	remember(t, s, writer(), "uses sqlite", "")

	res, err := s.Remember(ctx, writer(), Note{Kind: KindFact, Body: "uses sqlite again"})
	if err != nil {
		t.Fatal(err)
	}
	want := []Similar{{ID: 1, Body: "uses sqlite"}}
	if !reflect.DeepEqual(res.Similar, want) {
		t.Fatalf("Similar = %v, want %v", res.Similar, want)
	}

	res, err = s.Remember(ctx, writer(), Note{Kind: KindFact, Body: "sqlite is local"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Similar != nil {
		t.Fatalf("Similar = %v, want nil", res.Similar)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM memory WHERE vector IS NOT NULL`); n != 3 {
		t.Fatalf("%d vectors stored, want 3", n)
	}
}

func TestLookalike_ReplacedItemNotReported(t *testing.T) {
	tests := []struct {
		name string
		give map[string][]float32
	}{
		{"words", nil},
		{"vectors", map[string][]float32{"uses sqlite": {1, 0}, "uses sqlite now": {1, 0}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := openTest(t)
			s.embed = &fakeEmbedder{vectors: tt.give}
			remember(t, s, writer(), "uses sqlite", "")

			res, err := s.Remember(context.Background(), writer(),
				Note{Kind: KindFact, Body: "uses sqlite now", Replaces: []int64{1}})
			if err != nil {
				t.Fatal(err)
			}
			if res.Similar != nil {
				t.Fatalf("Similar = %v, want nil", res.Similar)
			}
		})
	}
}
