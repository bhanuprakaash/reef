package channel

import (
	"slices"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

func TestRoute(t *testing.T) {
	roles := []string{"lead", "architect", "developer"}
	root := int64(7)
	tests := []struct {
		name  string
		give  store.Message
		owner string
		want  []string
	}{
		{"user, no mention", store.Message{Author: "user", Body: "build login"}, "", []string{"lead"}},
		{"user mentions architect", store.Message{Author: "user", Body: "@architect plan it"}, "", []string{"architect"}},
		{"every mention wakes, once", store.Message{Author: "user", Body: "@architect @developer @architect"}, "", []string{"architect", "developer"}},
		{"mention must end at a word boundary", store.Message{Author: "user", Body: "@architects?"}, "", []string{"lead"}},
		{"unknown mention", store.Message{Author: "user", Body: "@tester hi"}, "", []string{"lead"}},
		{"email is not a mention", store.Message{Author: "user", Body: "me@architect.dev"}, "", []string{"lead"}},
		{"agent without mention wakes nobody", store.Message{Author: "architect", Body: "plan: ..."}, "", nil},
		{"agent mentions another", store.Message{Author: "lead", Body: "ask @architect"}, "", []string{"architect"}},
		{"agent mentions itself", store.Message{Author: "lead", Body: "I am @lead"}, "", nil},
		{"reply goes to thread owner", store.Message{Author: "user", ParentID: &root, Body: "why?"}, "architect", []string{"architect"}},
		{"thread beats mention", store.Message{Author: "user", ParentID: &root, Body: "@developer why?"}, "architect", []string{"architect"}},
		{"reply in a thread the user owns goes to the lead", store.Message{Author: "user", ParentID: &root, Body: "more"}, "user", []string{"lead"}},
		{"agent reply in a thread the user owns wakes nobody", store.Message{Author: "architect", ParentID: &root, Body: "done"}, "user", nil},
		{"owner replying in own thread wakes nobody", store.Message{Author: "architect", ParentID: &root, Body: "because"}, "architect", nil},
		{"owner replying in own thread can still mention", store.Message{Author: "architect", ParentID: &root, Body: "@developer go"}, "architect", []string{"developer"}},
		{"lead replying in a user thread can mention", store.Message{Author: "lead", ParentID: &root, Body: "@architect plan"}, "user", []string{"architect"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Route(tt.give, tt.owner, roles, "lead"); !slices.Equal(got, tt.want) {
				t.Errorf("Route = %q, want %q", got, tt.want)
			}
		})
	}
}
