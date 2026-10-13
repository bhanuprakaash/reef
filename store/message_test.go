package store

import (
	"context"
	"errors"
	"testing"
)

func TestMessages_Thread(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.CreateProject(ctx, Project{Name: "shop", Path: "/tmp/shop"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a", "b"} {
		if _, err := s.CreateChannel(ctx, Channel{ProjectID: 1, Name: name}); err != nil {
			t.Fatal(err)
		}
	}
	one := int64(1)
	for _, m := range []Message{
		{ChannelID: 1, Author: "user", Body: "root"},
		{ChannelID: 1, Author: "user", Body: "other"},
		{ChannelID: 1, ParentID: &one, Author: "lead", Body: "reply"},
		{ChannelID: 1, Author: "architect", Body: "plan"},
	} {
		if _, err := s.CreateMessage(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	three, four, nine := int64(3), int64(4), int64(9)
	reply := Message{ChannelID: 1, ParentID: &four, Author: "user", Body: "ok"}
	if _, err := s.CreateMessage(ctx, reply); err != nil {
		t.Fatal(err)
	}
	creates := []struct {
		name string
		give Message
	}{
		{"reply to a reply", Message{ChannelID: 1, ParentID: &three, Author: "user", Body: "x"}},
		{"parent in another one", Message{ChannelID: 2, ParentID: &one, Author: "user", Body: "x"}},
		{"unknown parent", Message{ChannelID: 1, ParentID: &nine, Author: "user", Body: "x"}},
	}
	for _, tt := range creates {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := s.CreateMessage(ctx, tt.give); !errors.Is(err, ErrInvalid) {
				t.Errorf("err = %v, want ErrInvalid", err)
			}
		})
	}

	msgs, err := s.Thread(ctx, 1, 1)
	if err != nil || len(msgs) != 2 || msgs[0].Body != "root" || msgs[0].ParentID != nil ||
		msgs[1].Body != "reply" || *msgs[1].ParentID != 1 {
		t.Errorf("Thread(1, 1) = %+v, %v, want root then its reply", msgs, err)
	}
	msgs, err = s.Thread(ctx, 1, 4)
	if err != nil || len(msgs) != 2 || msgs[0].Author != "architect" || msgs[1].Body != "ok" {
		t.Errorf("Thread(1, 4) = %+v, %v, want the architect's root then the user's reply", msgs, err)
	}
	if _, err := s.Thread(ctx, 1, 3); !errors.Is(err, ErrNotFound) {
		t.Errorf("Thread of a reply: err = %v, want ErrNotFound", err)
	}
	owners := []struct {
		name string
		give int64
		want string
	}{
		{"user root", 1, "user"},
		{"user root without replies", 2, "user"},
		{"reply in a user thread", 3, "user"},
		{"role root", 4, "architect"},
		{"user reply in a role thread", 5, "architect"},
	}
	for _, tt := range owners {
		t.Run("owner of "+tt.name, func(t *testing.T) {
			if got, err := s.ThreadOwner(ctx, tt.give); err != nil || got != tt.want {
				t.Errorf("ThreadOwner(%d) = %q, %v, want %q", tt.give, got, err, tt.want)
			}
		})
	}
	if _, err := s.ThreadOwner(ctx, 99); !errors.Is(err, ErrNotFound) {
		t.Errorf("ThreadOwner(99) err = %v, want ErrNotFound", err)
	}
}
