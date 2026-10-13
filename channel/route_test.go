package channel

import (
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
		want  string
	}{
		{"user, no mention", store.Message{Author: "user", Body: "build login"}, "", "lead"},
		{"user mentions architect", store.Message{Author: "user", Body: "@architect plan it"}, "", "architect"},
		{"first mention wins", store.Message{Author: "lead", Body: "@developer then @architect"}, "", "developer"},
		{"mention must end at a word boundary", store.Message{Author: "user", Body: "@architects?"}, "", "lead"},
		{"unknown mention", store.Message{Author: "user", Body: "@tester hi"}, "", "lead"},
		{"email is not a mention", store.Message{Author: "user", Body: "me@architect.dev"}, "", "lead"},
		{"agent without mention wakes nobody", store.Message{Author: "architect", Body: "plan: ..."}, "", ""},
		{"agent mentions another", store.Message{Author: "lead", Body: "ask @architect"}, "", "architect"},
		{"agent mentions itself", store.Message{Author: "lead", Body: "I am @lead"}, "", ""},
		{"reply goes to thread owner", store.Message{Author: "user", ParentID: &root, Body: "why?"}, "architect", "architect"},
		{"thread beats mention", store.Message{Author: "user", ParentID: &root, Body: "@developer why?"}, "architect", "architect"},
		{"reply in a thread the user owns goes to the lead", store.Message{Author: "user", ParentID: &root, Body: "more"}, "user", "lead"},
		{"owner replying in own thread wakes nobody", store.Message{Author: "architect", ParentID: &root, Body: "because"}, "architect", ""},
		{"owner replying in own thread can still mention", store.Message{Author: "architect", ParentID: &root, Body: "@developer go"}, "architect", "developer"},
		{"lead replying in a user thread can mention", store.Message{Author: "lead", ParentID: &root, Body: "@architect plan"}, "user", "architect"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Route(tt.give, tt.owner, roles, "lead"); got != tt.want {
				t.Errorf("Route = %q, want %q", got, tt.want)
			}
		})
	}
}
