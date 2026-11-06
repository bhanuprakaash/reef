package store

import (
	"context"
	"reflect"
	"testing"
)

func TestSentScopes(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.CreateProject(ctx, Project{Name: "shop", Path: "/tmp/shop"}); err != nil {
		t.Fatal(err)
	}
	ch, err := s.CreateSpace(ctx, Space{ProjectID: 1, Name: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	bots := identityTestBots(t, s)
	for _, tt := range []struct {
		role, session string
		scopes        Scopes
		sent          bool
	}{
		{"developer", "s1", Scopes{{Thread: 0, Read: 5}}, true},
		{"developer", "s1", Scopes{{Thread: 3, Asked: []string{"Asked together: @a, @b"}, Read: 9}}, true},
		{"developer", "s1", Scopes{{Thread: 3, Read: 12}}, false},
		{"developer", "s2", Scopes{{Thread: 0, Read: 40}}, true},
		{"tester", "s1", Scopes{{Thread: 0, Read: 41}}, true},
		{"developer", "s1", nil, true},
	} {
		turn, err := s.CreateTurn(ctx, Turn{
			SpaceID: ch.ID, Role: tt.role, BotID: bots[tt.role], Backend: "claude", SessionID: tt.session,
			Scopes: tt.scopes,
		})
		if err != nil {
			t.Fatal(err)
		}
		if tt.sent {
			if _, err := s.SetTurnSent(ctx, turn.ID, "prompt"); err != nil {
				t.Fatal(err)
			}
		}
	}

	got, err := s.SentScopes(ctx, ch.ID, *bots["developer"], "s1")
	if err != nil {
		t.Fatal(err)
	}
	want := Scopes{
		{Thread: 0, Read: 5},
		{Thread: 3, Asked: []string{"Asked together: @a, @b"}, Read: 9},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SentScopes = %+v, want %+v: this role and session, turns that sent", got, want)
	}
}
