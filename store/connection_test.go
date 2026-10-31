package store

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestConnection_Validate(t *testing.T) {
	ctx := context.Background()
	s := spaceMemberStore(t)
	tests := []struct {
		name string
		give Connection
		want string
	}{
		{"both", Connection{Name: "a", Command: "x", URL: "http://h/mcp"}, "needs a command or a url, not both"},
		{"neither", Connection{Name: "a"}, "needs a command or a url, not both"},
		{"reserved name", Connection{Name: "reef", Command: "x"}, "must be letters, digits, _ or -, and not reef"},
		{"bad name", Connection{Name: "a b", Command: "x"}, "must be letters, digits, _ or -, and not reef"},
		{"bad url", Connection{Name: "a", URL: "ftp://h"}, `url "ftp://h" must be http or https`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.CreateConnection(ctx, tt.give)
			if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("CreateConnection = %v, want ErrInvalid containing %q", err, tt.want)
			}
		})
	}
	if _, err := s.CreateConnection(ctx, Connection{Name: "a", Command: "x"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateConnection(ctx, Connection{Name: "a", URL: "http://h/mcp"}); !errors.Is(err, ErrConflict) {
		t.Errorf("duplicate name = %v, want ErrConflict", err)
	}
	if _, err := s.CreateBot(ctx, Bot{Name: "sam", Backend: "claude", Connections: []int64{99}}); !errors.Is(err, ErrInvalid) {
		t.Errorf("Bot with unknown connection = %v, want ErrInvalid", err)
	}
}

func TestConnection_SessionKeepsWhatItAdopted(t *testing.T) {
	ctx := context.Background()
	s := spaceMemberStore(t)
	files, err := s.CreateConnection(ctx, Connection{Name: "files", Command: "files-mcp", Args: []string{"--root", "/"}})
	if err != nil {
		t.Fatal(err)
	}
	web, err := s.CreateConnection(ctx, Connection{Name: "web", URL: "http://h/mcp"})
	if err != nil {
		t.Fatal(err)
	}
	sam, err := s.CreateBot(ctx, Bot{Name: "sam", Backend: "claude", Connections: []int64{files.ID, web.ID}})
	if err != nil {
		t.Fatal(err)
	}
	kim, err := s.CreateBot(ctx, Bot{Name: "kim", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	chat, err := s.CreateBotSpace(ctx, Space{ProjectID: 1, Name: "chat", Kind: KindConversation, Role: "sam"}, sam.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdoptBotSession(ctx, chat.ID, sam.ID); err != nil {
		t.Fatal(err)
	}

	files.Command = "other-mcp"
	if _, err := s.UpdateConnection(ctx, files); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteConnection(ctx, web.ID); err != nil {
		t.Fatal(err)
	}

	got, err := s.ActiveBotSession(ctx, chat.ID, sam.ID)
	if err != nil {
		t.Fatal(err)
	}
	names := func(cs []Connection) []string {
		var out []string
		for _, c := range cs {
			out = append(out, c.Name+" "+c.Command+c.URL)
		}
		return out
	}
	if want := []string{"files files-mcp", "web http://h/mcp"}; !reflect.DeepEqual(names(got.Connections), want) {
		t.Errorf("active session connections = %v, want %v", names(got.Connections), want)
	}
	bot, err := s.Bot(ctx, sam.ID)
	if err != nil || !reflect.DeepEqual(bot.Connections, []int64{files.ID}) {
		t.Errorf("bot = %+v, %v, want the deleted connection out of its selection", bot, err)
	}
	next, err := s.AdoptBotSession(ctx, chat.ID, sam.ID)
	if err != nil || !reflect.DeepEqual(names(next.Connections), []string{"files other-mcp"}) {
		t.Errorf("new session = %+v, %v, want only files, as edited", next, err)
	}
	if other, err := s.AdoptBotSession(ctx, chat.ID, kim.ID); err != nil || len(other.Connections) != 0 {
		t.Errorf("unselected bot session = %+v, %v, want no connections", other, err)
	}
}
