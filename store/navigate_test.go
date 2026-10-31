package store

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestNavigate_PinsLatestChatPagesAndFind(t *testing.T) {
	ctx := context.Background()
	s := spaceMemberStore(t)
	if _, err := s.CreateProject(ctx, Project{Name: "api", Path: "/tmp/api"}); err != nil {
		t.Fatal(err)
	}
	sam, err := s.CreateBot(ctx, Bot{Name: "sam", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	var chats []Space
	for i := 1; i <= 5; i++ {
		c, err := s.CreateBotSpace(ctx,
			Space{Name: fmt.Sprintf("chat %d", i), Kind: KindConversation, Role: "sam"}, sam.ID)
		if err != nil {
			t.Fatal(err)
		}
		chats = append(chats, c)
	}
	free, err := s.CreateBotSpace(ctx, Space{Name: "plan 100%", Kind: KindFree, Role: "sam"}, sam.ID)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	for _, c := range chats[:2] {
		if err := s.AddSpaceProject(ctx, c.ID, 1, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.AddSpaceProject(ctx, chats[4].ID, 2, nil); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := s.CreateMessage(ctx, Message{SpaceID: chats[0].ID, Author: "user", Body: "hi"}); err != nil {
		t.Fatal(err)
	}
	names := func(ps []SpacePreview) []string {
		var out []string
		for _, p := range ps {
			out = append(out, p.Name)
		}
		return out
	}
	one, two := int64(1), int64(2)

	got, err := s.BotConversationsPage(ctx, sam.ID, ConversationPage{ProjectID: &one, Limit: 1})
	if err != nil || !reflect.DeepEqual(names(got), []string{"chat 1"}) {
		t.Errorf("latest under project 1 = %v, %v, want [chat 1]", names(got), err)
	}
	got, err = s.BotConversationsPage(ctx, sam.ID, ConversationPage{ProjectID: &two, Limit: 1})
	if err != nil || !reflect.DeepEqual(names(got), []string{"chat 5"}) {
		t.Errorf("latest under project 2 = %v, %v, want [chat 5]", names(got), err)
	}
	var seen []string
	var before *int64
	for {
		page, err := s.BotConversationsPage(ctx, sam.ID, ConversationPage{Before: before, Limit: 2})
		if err != nil {
			t.Fatal(err)
		}
		if len(page) == 0 {
			break
		}
		seen = append(seen, names(page)...)
		before = &page[len(page)-1].ID
	}
	if want := []string{"chat 1", "chat 5", "chat 2", "chat 4", "chat 3"}; !reflect.DeepEqual(seen, want) {
		t.Errorf("pages = %v, want %v", seen, want)
	}

	for _, p := range []struct {
		kind string
		id   int64
	}{{PinBot, sam.ID}, {PinChat, chats[1].ID}, {PinSpace, free.ID}} {
		if err := s.SetPin(ctx, p.kind, p.id); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SetPin(ctx, PinSpace, chats[1].ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("pin a chat as a space = %v, want not found", err)
	}
	pins, err := s.Pins(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, p := range pins {
		titles = append(titles, p.Kind+":"+p.Title)
	}
	if want := []string{"space:plan 100%", "chat:chat 2", "bot:sam"}; !reflect.DeepEqual(titles, want) {
		t.Errorf("pins = %v, want %v", titles, want)
	}
	if err := s.DeleteSpace(ctx, free.ID); err != nil {
		t.Fatal(err)
	}
	if pins, _ = s.Pins(ctx); len(pins) != 2 {
		t.Errorf("pins after deleting a pinned Space = %d, want 2", len(pins))
	}

	var msgs []Message
	for i := 1; i <= 5; i++ {
		m, err := s.CreateMessage(ctx, Message{SpaceID: chats[2].ID, Author: "user", Body: fmt.Sprint("m", i)})
		if err != nil {
			t.Fatal(err)
		}
		msgs = append(msgs, m)
	}
	page, err := s.MessagesPage(ctx, chats[2].ID, msgs[3].ID, 2)
	if err != nil || len(page) != 2 || page[0].Body != "m2" || page[1].Body != "m3" {
		t.Errorf("messages before m4, 2 = %+v, %v, want m2 m3", page, err)
	}

	found, err := s.Find(ctx, "CHAT 1", 5)
	if err != nil || !reflect.DeepEqual(found, []FoundItem{{Kind: "chat", ID: chats[0].ID, Title: "chat 1"}}) {
		t.Errorf("find = %+v, %v", found, err)
	}
	if found, _ = s.Find(ctx, "sam", 5); len(found) != 1 || found[0].Kind != "bot" {
		t.Errorf("find sam = %+v, want the bot", found)
	}
}
