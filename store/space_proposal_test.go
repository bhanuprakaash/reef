package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestSpaceProposal_ListedInInboxAndDismissed(t *testing.T) {
	ctx := context.Background()
	s := spaceMemberStore(t)
	boss, dev := int64(1), int64(2)

	if _, err := s.CreateSpaceProposal(ctx, SpaceProposal{
		ProposedBy: "boss", Name: "review", BotIDs: []int64{dev, boss}, CoordinatorBotID: 9,
	}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("proposal with a coordinator outside the Bots = %v, want ErrInvalid", err)
	}
	p, err := s.CreateSpaceProposal(ctx, SpaceProposal{
		ProposedBy: "boss", Name: "review", BotIDs: []int64{dev, boss, dev},
	})
	if err != nil || !reflect.DeepEqual(p.BotIDs, []int64{dev, boss}) || p.CoordinatorBotID != dev {
		t.Fatalf("proposal = %+v, %v, want Bots [2 1] led by 2", p, err)
	}

	items, err := s.Inbox(ctx)
	if err != nil || len(items) != 1 || items[0].Kind != KindSpaceProposed ||
		items[0].Text != "boss proposed a Space: review" || !reflect.DeepEqual(*items[0].Proposal, p) {
		t.Fatalf("inbox = %+v, %v, want the proposal", items, err)
	}
	if err := s.DismissInbox(ctx, items[0]); err != nil {
		t.Fatal(err)
	}
	if items, err = s.Inbox(ctx); err != nil || len(items) != 0 {
		t.Errorf("inbox after dismiss = %+v, %v, want empty", items, err)
	}
}
