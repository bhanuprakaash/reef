package store

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/bhanuprakaash/reef/skill"
)

func TestSkill_KeepsExecuteBit(t *testing.T) {
	ctx := context.Background()
	s := spaceMemberStore(t)
	k, err := s.ImportSkill(ctx, "folder", "/src/deploy", "", "", skill.Package{
		Name: "deploy", Description: "Ship it.",
		Files: []skill.File{
			{Path: "SKILL.md", Body: []byte("---\nname: deploy\n---\n")},
			{Path: "ship.sh", Body: []byte("echo"), Executable: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	files, err := s.SkillFiles(ctx, k.ID, 1)
	if err != nil || len(files) != 2 || files[0].Executable || !files[1].Executable {
		t.Errorf("files = %+v, %v, want only ship.sh executable", files, err)
	}
}

func TestSaveSkillDraft_ConcurrentSavesWriteOnce(t *testing.T) {
	ctx := context.Background()
	s := spaceMemberStore(t)
	scribe, err := s.CreateBot(ctx, Bot{Name: "scribe", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	chat, err := s.CreateBotSpace(ctx, Space{ProjectID: 1, Name: "chat", Kind: KindConversation, Role: "scribe"}, scribe.ID)
	if err != nil {
		t.Fatal(err)
	}
	p := skill.Package{Name: "triage", Files: []skill.File{{Path: "SKILL.md", Body: []byte("---\nname: triage\ndescription: Sort.\n---\n")}}}
	if _, err := s.ImportSkill(ctx, "reef", "", "", "", p); err != nil {
		t.Fatal(err)
	}
	draft, err := s.CreateSkillDraft(ctx, chat.ID, scribe.ID, p)
	if err != nil {
		t.Fatal(err)
	}

	errs := make([]error, 4)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = s.SaveSkillDraft(ctx, draft.ID, true)
		}()
	}
	wg.Wait()
	var saved int
	for _, err := range errs {
		if err == nil {
			saved++
		} else if !errors.Is(err, ErrConflict) {
			t.Errorf("save = %v, want nil or ErrConflict", err)
		}
	}
	if k, err := s.Skill(ctx, 1); err != nil || saved != 1 || k.Version != 2 {
		t.Errorf("saves ok = %d, skill = %+v, %v, want one save and version 2", saved, k, err)
	}
}
