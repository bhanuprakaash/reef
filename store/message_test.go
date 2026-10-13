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
	} {
		if _, err := s.CreateMessage(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	three, nine := int64(3), int64(9)
	for name, give := range map[string]Message{
		"reply to a reply":      {ChannelID: 1, ParentID: &three, Author: "user", Body: "x"},
		"parent in another one": {ChannelID: 2, ParentID: &one, Author: "user", Body: "x"},
		"unknown parent":        {ChannelID: 1, ParentID: &nine, Author: "user", Body: "x"},
	} {
		if _, err := s.CreateMessage(ctx, give); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}

	msgs, err := s.Thread(ctx, 1, 1)
	if err != nil || len(msgs) != 2 || msgs[0].Body != "root" || msgs[0].ParentID != nil ||
		msgs[1].Body != "reply" || *msgs[1].ParentID != 1 {
		t.Errorf("Thread(1, 1) = %+v, %v, want root then its reply", msgs, err)
	}
	if _, err := s.Thread(ctx, 1, 3); !errors.Is(err, ErrNotFound) {
		t.Errorf("Thread of a reply: err = %v, want ErrNotFound", err)
	}
	for id, want := range map[int64]string{1: "user", 2: "user", 3: "user"} {
		if got, err := s.ThreadOwner(ctx, id); err != nil || got != want {
			t.Errorf("ThreadOwner(%d) = %q, %v, want %q", id, got, err, want)
		}
	}
}
