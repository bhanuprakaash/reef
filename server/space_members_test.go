package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/skill"
	"github.com/bhanuprakaash/reef/space"
	"github.com/bhanuprakaash/reef/store"
)

func TestSpaceMembers_ShowTheAdoptedSession(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	team, err := env.st.SpaceTeam(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	botID := team.Roles[0].BotID
	conn, err := env.st.CreateConnection(ctx, store.Connection{Name: "files", Command: "files-mcp", Env: map[string]string{"KEY": "s3cret"}})
	if err != nil {
		t.Fatal(err)
	}
	bot, err := env.st.Bot(ctx, botID)
	if err != nil {
		t.Fatal(err)
	}
	bot.Model, bot.Connections = "sonnet", []int64{conn.ID}
	if _, err := env.st.UpdateBot(ctx, bot); err != nil {
		t.Fatal(err)
	}
	if _, err := env.st.AdoptBotSession(ctx, 1, botID); err != nil {
		t.Fatal(err)
	}

	_, body := do(t, http.MethodGet, env.url+"/spaces/1/members", "")
	for _, want := range []string{`"model":"sonnet"`, `"connections":["files"]`, `"refreshPending":false,"stale":false`} {
		if !strings.Contains(body, want) {
			t.Errorf("members = %s, want %s", body, want)
		}
	}
	if strings.Contains(body, "s3cret") {
		t.Errorf("members = %s, want no connection secret", body)
	}

	kim, err := env.st.CreateBot(ctx, store.Bot{Name: "kim", Backend: "claude", Connections: []int64{conn.ID}})
	if err != nil {
		t.Fatal(err)
	}
	status, body := do(t, http.MethodPost, env.url+"/spaces/1/members", fmt.Sprintf(`{"botId":%d}`, kim.ID))
	if status != 201 || strings.Contains(body, "s3cret") {
		t.Errorf("add member = %d %s, want 201 and no connection secret", status, body)
	}

	bot.Model = "opus"
	if _, err := env.st.UpdateBot(ctx, bot); err != nil {
		t.Fatal(err)
	}
	_, body = do(t, http.MethodGet, env.url+"/spaces/1/members", "")
	for _, want := range []string{`"model":"opus"`, `"model":"sonnet"`, `"stale":true`} {
		if !strings.Contains(body, want) {
			t.Errorf("members after edit = %s, want %s", body, want)
		}
	}
}

func TestSpaceMembers_StaleWhenAConnectionOrReefSkillChanges(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	team, err := env.st.SpaceTeam(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	botID := team.Roles[0].BotID
	conn, err := env.st.CreateConnection(ctx, store.Connection{Name: "files", Command: "files-mcp"})
	if err != nil {
		t.Fatal(err)
	}
	pkg := skill.Package{Name: "deploy", Description: "Ship it.",
		Files: []skill.File{{Path: "SKILL.md", Body: []byte("---\nname: deploy\n---\nv1")}}}
	lib, err := env.st.ImportSkill(ctx, "folder", "/src/deploy", "", "", pkg)
	if err != nil {
		t.Fatal(err)
	}
	bot, err := env.st.Bot(ctx, botID)
	if err != nil {
		t.Fatal(err)
	}
	bot.Connections, bot.Skills = []int64{conn.ID}, []string{"reef:deploy"}
	if _, err := env.st.UpdateBot(ctx, bot); err != nil {
		t.Fatal(err)
	}
	if _, err := space.AdoptBotSession(ctx, env.st, env.dir, 1, botID); err != nil {
		t.Fatal(err)
	}
	stale := func() bool {
		t.Helper()
		_, body := do(t, http.MethodGet, env.url+"/spaces/1/members", "")
		return strings.Contains(body, `"stale":true`)
	}
	if stale() {
		t.Fatal("stale right after adopting, want false")
	}
	conn.Command = "files-mcp-2"
	if _, err := env.st.UpdateConnection(ctx, conn); err != nil {
		t.Fatal(err)
	}
	if !stale() {
		t.Error("stale after editing the selected connection = false, want true")
	}
	if _, err := space.AdoptBotSession(ctx, env.st, env.dir, 1, botID); err != nil {
		t.Fatal(err)
	}
	pkg.Files[0].Body = []byte("---\nname: deploy\n---\nv2")
	if _, err := env.st.UpdateSkill(ctx, lib.ID, pkg); err != nil {
		t.Fatal(err)
	}
	if !stale() {
		t.Error("stale after a new library version of a picked skill = false, want true")
	}
}
