package server

import (
	"context"
	"testing"

	"github.com/bhanuprakaash/reef/memory"
	"github.com/bhanuprakaash/reef/space"
	"github.com/bhanuprakaash/reef/store"
)

func TestRenamedBotKeepsThreadAndDirectRouting(t *testing.T) {
	ctx := context.Background()
	st, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err = st.CreateProject(ctx, store.Project{Name: "p", Path: "/tmp/p"}); err != nil {
		t.Fatal(err)
	}
	ch, err := st.CreateSpace(ctx, store.Space{ProjectID: 1, Name: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	bot, err := st.CreateBot(ctx, store.Bot{Name: "Prakash", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.AddSpaceMember(ctx, ch.ID, bot.ID); err != nil {
		t.Fatal(err)
	}
	msg, err := st.CreateMessage(ctx, store.Message{SpaceID: ch.ID, BotID: &bot.ID, Author: bot.Name, Body: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	bot.Name = "Parker"
	if _, err = st.UpdateBot(ctx, bot); err != nil {
		t.Fatal(err)
	}
	other, err := st.CreateBot(ctx, store.Bot{Name: "Prakash", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.AddSpaceMember(ctx, ch.ID, other.ID); err != nil {
		t.Fatal(err)
	}
	reply, err := st.CreateMessage(ctx, store.Message{SpaceID: ch.ID, ParentID: &msg.ID, Author: "user", Body: "reply"})
	if err != nil {
		t.Fatal(err)
	}
	srv := &server{st: st}
	wakes, err := srv.route(ctx, ch.ID, []store.Message{reply}, nil)
	if err != nil || len(wakes) != 1 || wakes[0].botID == nil || *wakes[0].botID != bot.ID || wakes[0].role != "Parker" {
		t.Fatalf("thread wakes = %+v, %v", wakes, err)
	}
	direct, err := st.CreateDirectMessage(ctx, store.DirectMessage{SpaceID: ch.ID, FromBotID: &other.ID, ToBotID: &bot.ID, From: "Prakash", To: "Parker", Body: "help"})
	if err != nil {
		t.Fatal(err)
	}
	wakes, _, err = srv.directWakes(ctx, ch.ID, 0, &other.ID)
	if err != nil || len(wakes) != 1 || wakes[0].botID == nil || *wakes[0].botID != bot.ID || wakes[0].direct.ID != direct.ID {
		t.Fatalf("direct wakes = %+v, %v", wakes, err)
	}
	history, err := st.Messages(ctx, ch.ID, 0)
	if err != nil || history[0].Author != "Prakash" || *history[0].BotID != bot.ID {
		t.Fatalf("history = %+v, %v", history, err)
	}
}

func TestRenamedBotMCPMemoryUsesItsIdentity(t *testing.T) {
	ctx := context.Background()
	st, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err = st.CreateProject(ctx, store.Project{Name: "p", Path: "/tmp/p"}); err != nil {
		t.Fatal(err)
	}
	ch, err := st.CreateSpace(ctx, store.Space{ProjectID: 1, Name: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	bot, err := st.CreateBot(ctx, store.Bot{Name: "sam", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.AddSpaceMember(ctx, ch.ID, bot.ID); err != nil {
		t.Fatal(err)
	}
	tok, err := st.CreateToken(ctx, ch.ID, bot.ID)
	if err != nil {
		t.Fatal(err)
	}
	ms, err := memory.Open(st.DB(), nil)
	if err != nil {
		t.Fatal(err)
	}
	m := &mcpServer{st: st, ms: ms, tok: tok}
	bot.Name = "sally"
	if _, err = st.UpdateBot(ctx, bot); err != nil {
		t.Fatal(err)
	}
	_, out, err := m.remember(ctx, nil, rememberIn{Kind: "fact", Body: "use sqlite", Scope: memory.ScopeBot})
	if err != nil {
		t.Fatal(err)
	}
	item, err := ms.Read(ctx, out.Item.ID)
	if err != nil || item.Author != "sally" {
		t.Fatalf("item = %+v, %v", item, err)
	}
	_, recalled, err := m.recall(ctx, nil, recallIn{Query: "sqlite", Scope: memory.ScopeBot})
	if err != nil || len(recalled.Hits) != 1 {
		t.Fatalf("recall = %+v, %v", recalled, err)
	}
	outsider, err := st.CreateBot(ctx, store.Bot{Name: "outsider", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	m.tok.BotID = &outsider.ID
	if _, _, err = m.remember(ctx, nil, rememberIn{Kind: "fact", Body: "wrong", Scope: memory.ScopeBot}); err == nil {
		t.Fatal("non-member token accepted")
	}
}

func TestProposalKeepsBotMemoryAfterRenameAndNameReuse(t *testing.T) {
	ctx := context.Background()
	st, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err = st.CreateProject(ctx, store.Project{Name: "p", Path: "/tmp/p"}); err != nil {
		t.Fatal(err)
	}
	item, err := st.CreateWorkItem(ctx, store.WorkItem{ProjectID: 1, Title: "work"})
	if err != nil {
		t.Fatal(err)
	}
	bot, err := st.CreateBot(ctx, store.Bot{Name: "sam", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	ch, err := st.CreateBotSpace(ctx, store.Space{ProjectID: 1, WorkItemID: &item.ID, Name: "chat"}, bot.ID)
	if err != nil {
		t.Fatal(err)
	}
	ms, err := memory.Open(st.DB(), nil)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := st.CreateToken(ctx, ch.ID, bot.ID)
	if err != nil {
		t.Fatal(err)
	}
	m := &mcpServer{st: st, ms: ms, tok: tok}
	_, proposal, err := m.proposeWrap(ctx, nil, wrapIn{Brief: "done", Items: []store.ProposedItem{{Kind: "fact", Body: "keep this", Scope: memory.ScopeBot}, {Kind: "convention", Body: "use tabs", Scope: memory.ScopeProject}}})
	if err != nil {
		t.Fatal(err)
	}
	bot.Name = "sally"
	if _, err = st.UpdateBot(ctx, bot); err != nil {
		t.Fatal(err)
	}
	other, err := st.CreateBot(ctx, store.Bot{Name: "sam", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.AddSpaceMember(ctx, ch.ID, other.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = ApproveProposal(ctx, st, ms, proposal.ID); err != nil {
		t.Fatal(err)
	}
	hits, err := ms.Recall(ctx, memory.Query{Scopes: space.BotScopes(ch, bot.ID), Text: "keep this"})
	if err != nil || len(hits) != 1 {
		t.Fatalf("original Bot memory = %+v, %v", hits, err)
	}
	saved, err := ms.Read(ctx, hits[0].ID)
	if err != nil || saved.Author != "sam" {
		t.Fatalf("saved = %+v, %v", saved, err)
	}
	conventions, err := ms.Recall(ctx, memory.Query{Scopes: []memory.Ref{{Kind: memory.ScopeProject, ID: "1"}}, Text: "use tabs"})
	if err != nil || len(conventions) != 1 {
		t.Fatalf("conventions = %+v, %v", conventions, err)
	}
	convention, err := ms.Read(ctx, conventions[0].ID)
	if err != nil || convention.Status != memory.StatusAccepted || convention.Author != "sam" {
		t.Fatalf("convention = %+v, %v", convention, err)
	}
	hits, err = ms.Recall(ctx, memory.Query{Scopes: space.BotScopes(ch, other.ID), Text: "keep this"})
	if err != nil || len(hits) != 0 {
		t.Fatalf("other Bot memory = %+v, %v", hits, err)
	}
}

func TestBotNameDoesNotGrantConventionAuthority(t *testing.T) {
	ctx := context.Background()
	st, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err = st.CreateProject(ctx, store.Project{Name: "p", Path: "/tmp/p"}); err != nil {
		t.Fatal(err)
	}
	ch, err := st.CreateSpace(ctx, store.Space{ProjectID: 1, Name: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	bot, err := st.CreateBot(ctx, store.Bot{Name: "sam", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.AddSpaceMember(ctx, ch.ID, bot.ID); err != nil {
		t.Fatal(err)
	}
	tok, err := st.CreateToken(ctx, ch.ID, bot.ID)
	if err != nil {
		t.Fatal(err)
	}
	ms, err := memory.Open(st.DB(), nil)
	if err != nil {
		t.Fatal(err)
	}
	bot.Name = "architect"
	if _, err = st.UpdateBot(ctx, bot); err != nil {
		t.Fatal(err)
	}
	m := &mcpServer{st: st, ms: ms, tok: tok}
	_, out, err := m.remember(ctx, nil, rememberIn{Kind: "convention", Body: "Use tabs", Scope: memory.ScopeProject})
	if err != nil {
		t.Fatal(err)
	}
	if out.Item.Status != memory.StatusCandidate {
		t.Fatalf("status = %s, want candidate", out.Item.Status)
	}
}

func TestCanceledBotLookupClosesSessionAndStopsDirect(t *testing.T) {
	ctx := context.Background()
	st, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err = st.CreateProject(ctx, store.Project{Name: "p", Path: "/tmp/p"}); err != nil {
		t.Fatal(err)
	}
	ch, err := st.CreateSpace(ctx, store.Space{ProjectID: 1, Name: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	from, err := st.CreateBot(ctx, store.Bot{Name: "sender", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	to, err := st.CreateBot(ctx, store.Bot{Name: "recipient", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	dm, err := st.CreateDirectMessage(ctx, store.DirectMessage{SpaceID: ch.ID, From: from.Name, To: to.Name, FromBotID: &from.ID, ToBotID: &to.ID, Body: "private"})
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	sess := &fakeSession{st: st}
	srv := &server{st: st}
	end, ok, _ := srv.runOne(canceled, ch.ID, &live{}, sess, _message, wake{botID: &to.ID, role: to.Name, direct: &dm})
	if ok || end.Error != "interrupted" || sess.closed.Load() != 1 {
		t.Fatalf("end = %+v, ok = %v, closed = %d", end, ok, sess.closed.Load())
	}
	got, err := st.DirectMessage(ctx, dm.ID)
	if err != nil || got.State != store.DirectStopped {
		t.Fatalf("direct = %+v, %v", got, err)
	}
}
