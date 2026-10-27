package memory

import (
	"errors"
	"testing"
)

func TestParseRef(t *testing.T) {
	tests := []struct {
		name    string
		give    string
		want    Ref
		wantErr string
	}{
		{name: "kind and id", give: "work_item:7", want: Ref{Kind: "work_item", ID: "7"}},
		{name: "id holding a colon", give: "message:a:b", want: Ref{Kind: "message", ID: "a:b"}},
		{name: "no colon", give: "project", wantErr: `invalid: "project" is not kind:id`},
		{name: "no kind", give: ":7", wantErr: `invalid: ":7" is not kind:id`},
		{name: "no id", give: "project:", wantErr: `invalid: "project:" is not kind:id`},
		{name: "empty", give: "", wantErr: `invalid: "" is not kind:id`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRef(tt.give)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr || !errors.Is(err, ErrInvalid) {
					t.Fatalf("ParseRef(%q) error = %v, want %q wrapping ErrInvalid", tt.give, err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("ParseRef(%q) = %v, %v, want %v", tt.give, got, err, tt.want)
			}
		})
	}
}
