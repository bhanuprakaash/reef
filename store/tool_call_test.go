package store

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCut_ASCIITruncatesToMax(t *testing.T) {
	s := strings.Repeat("a", _toolCallMax+10)
	if got := cut(s); len(got) != _toolCallMax {
		t.Fatalf("len(cut(s)) = %d, want %d", len(got), _toolCallMax)
	}
}

func TestCut_BacksOffToRuneBoundary(t *testing.T) {
	prefix := strings.Repeat("a", _toolCallMax-2)
	s := prefix + "世" + "more text after the cut"

	got := cut(s)

	if got != prefix {
		t.Fatalf("cut(s) = %q, want %q", got, prefix)
	}
	if !utf8.ValidString(got) {
		t.Fatalf("cut(s) = %q, not valid UTF-8", got)
	}
}
