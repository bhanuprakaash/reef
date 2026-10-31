package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSkillEditorAndDrafts(t *testing.T) {
	ctx := context.Background()
	st, tok := mcpFixture(t)
	h := New(st, testMemory(t, st), t.TempDir(), "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)

	skillMD := func(name, body string) string {
		return `{"path":"SKILL.md","body":"---\nname: ` + name + `\ndescription: Ship it.\n---\n` + body + `"}`
	}
	code, body := do(t, http.MethodPost, ts.URL+"/skills",
		`{"name":"deploy","files":[`+skillMD("deploy", "v1")+`,{"path":"notes/a.md","body":"note"}]}`)
	if code != http.StatusCreated {
		t.Fatalf("POST /skills = %d %s", code, body)
	}
	for _, bad := range []string{
		`{"name":"other","files":[` + skillMD("deploy", "") + `]}`,
		`{"name":"x","files":[` + skillMD("x", "") + `,{"path":"../escape","body":""}]}`,
		`{"name":"x","files":[{"path":"notes.md","body":""}]}`,
	} {
		if code, body := do(t, http.MethodPost, ts.URL+"/skills", bad); code != http.StatusBadRequest {
			t.Errorf("POST /skills %s = %d %s, want 400", bad, code, body)
		}
	}

	code, body = do(t, http.MethodPut, ts.URL+"/skills/1", `{"files":[`+skillMD("deploy", "v2")+`]}`)
	if code != http.StatusOK || !strings.Contains(body, `"version":2`) {
		t.Fatalf("PUT /skills/1 = %d %s, want version 2", code, body)
	}
	if code, body := do(t, http.MethodPut, ts.URL+"/skills/1", `{"files":[`+skillMD("renamed", "")+`]}`); code != http.StatusBadRequest {
		t.Errorf("PUT with another name = %d %s, want 400", code, body)
	}
	v1, err := st.SkillFiles(ctx, 1, 1)
	if err != nil || len(v1) != 2 || !strings.HasSuffix(string(v1[0].Body), "---\nv1") || string(v1[1].Body) != "note" {
		t.Errorf("version 1 files after the edit = %v, %v, want both files as saved", v1, err)
	}

	call := callTool(t, st, tok)
	got, isErr := call("propose_skill", map[string]any{
		"name": "triage",
		"files": []map[string]string{{"path": "SKILL.md", "body": "---\nname: triage\ndescription: Sort bugs.\n---\nSteps."}},
	})
	if want := `{"id":1,"message":"draft saved; the user will review it"}`; got != want || isErr {
		t.Fatalf("propose_skill = %q (error %v), want %q", got, isErr, want)
	}
	got, isErr = call("propose_skill", map[string]any{
		"name": "triage", "files": []map[string]string{{"path": "notes.md", "body": "x"}},
	})
	if !isErr || !strings.Contains(got, "no SKILL.md") {
		t.Errorf("propose_skill without SKILL.md = %q (error %v), want an error", got, isErr)
	}
	call("propose_skill", map[string]any{
		"name": "deploy", "files": []map[string]string{{"path": "SKILL.md", "body": "---\nname: deploy\ndescription: Ship it.\n---\nv3"}},
	})

	var library []struct{ Name string }
	_, body = do(t, http.MethodGet, ts.URL+"/skills/library", "")
	if err := json.Unmarshal([]byte(body), &library); err != nil || len(library) != 1 {
		t.Fatalf("library with drafts pending = %s, want only deploy", body)
	}
	code, body = do(t, http.MethodGet, ts.URL+"/skill-drafts?space=999", "")
	if code != http.StatusOK || body != "[]" {
		t.Errorf("drafts of another Space = %d %q, want []", code, body)
	}
	_, body = do(t, http.MethodGet, ts.URL+"/skill-drafts", "")
	if !strings.Contains(body, `"name":"triage"`) || !strings.Contains(body, `"body":"---\nname: triage`) {
		t.Errorf("GET /skill-drafts = %s, want the triage draft with its file contents", body)
	}

	code, body = do(t, http.MethodPost, ts.URL+"/skill-drafts/1/save", `{}`)
	if code != http.StatusOK || !strings.Contains(body, `"name":"triage"`) || !strings.Contains(body, `"sourceKind":"reef"`) {
		t.Fatalf("save draft 1 = %d %s, want the new skill", code, body)
	}
	if code, body := do(t, http.MethodPost, ts.URL+"/skill-drafts/1/save", `{}`); code != http.StatusConflict {
		t.Errorf("save draft 1 again = %d %s, want 409", code, body)
	}
	if code, body := do(t, http.MethodPost, ts.URL+"/skill-drafts/2/save", `{}`); code != http.StatusConflict {
		t.Errorf("save a draft named like a library skill = %d %s, want 409", code, body)
	}
	code, body = do(t, http.MethodPost, ts.URL+"/skill-drafts/2/save", `{"update":true}`)
	if code != http.StatusOK || !strings.Contains(body, `"version":3`) {
		t.Errorf("save draft 2 as an update = %d %s, want version 3", code, body)
	}

	call("propose_skill", map[string]any{
		"name": "gone", "files": []map[string]string{{"path": "SKILL.md", "body": "---\nname: gone\ndescription: x\n---\n"}},
	})
	if code, body := do(t, http.MethodPost, ts.URL+"/skill-drafts/3/reject", ``); code != http.StatusNoContent {
		t.Fatalf("reject draft 3 = %d %s, want 204", code, body)
	}
	_, body = do(t, http.MethodGet, ts.URL+"/skills/library", "")
	if err := json.Unmarshal([]byte(body), &library); err != nil || len(library) != 2 {
		t.Errorf("library after a rejected draft = %s, want deploy and triage", body)
	}
	if code, body := do(t, http.MethodGet, ts.URL+"/skill-drafts", ""); code != http.StatusOK || body != "[]" {
		t.Errorf("pending drafts after all decided = %d %q, want []", code, body)
	}
	bot, err := st.Bot(ctx, *tok.BotID)
	if err != nil || len(bot.Skills) != 0 {
		t.Errorf("Bot skills after saving drafts = %v, %v, want none", bot.Skills, err)
	}
}
