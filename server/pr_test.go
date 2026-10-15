package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

// writePRGh writes a fake `gh` for the PR panel. `pr view` answers with $GH_RESP, or fails
// with $GH_ERR when that file has text. `pr merge` appends its arguments to $GH_MERGES, fails
// with $GH_MERGE_ERR when that file has text, and otherwise turns $GH_RESP's OPEN into MERGED.
func writePRGh(t *testing.T, binDir string) {
	t.Helper()
	script := `#!/bin/sh
if [ "$1 $2" = "pr view" ]; then
	if [ -s "$GH_ERR" ]; then cat "$GH_ERR" >&2; exit 1; fi
	cat "$GH_RESP"
elif [ "$1 $2" = "pr merge" ]; then
	echo "$*" >>"$GH_MERGES"
	if [ -s "$GH_MERGE_ERR" ]; then cat "$GH_MERGE_ERR" >&2; exit 1; fi
	sed 's/"OPEN"/"MERGED"/' "$GH_RESP" >"$GH_RESP.new" && mv "$GH_RESP.new" "$GH_RESP"
else
	exit 1
fi
`
	if err := os.WriteFile(filepath.Join(binDir, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestFeaturePR(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	binDir, files := t.TempDir(), t.TempDir()
	writePRGh(t, binDir)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	for env, name := range map[string]string{
		"GH_RESP": "resp.json", "GH_ERR": "err.txt", "GH_MERGE_ERR": "merge-err.txt",
		"GH_MERGES": "merges.txt",
	} {
		t.Setenv(env, filepath.Join(files, name))
	}
	set := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(files, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	h := New(st, t.TempDir(), "/bin/reef")
	t.Cleanup(h.Close)
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	do(t, http.MethodPost, ts.URL+"/projects", `{"name":"app","path":"`+t.TempDir()+`"}`)
	do(t, http.MethodPost, ts.URL+"/features", `{"projectId":1,"slug":"demo","name":"Demo"}`)

	set("err.txt", "no pull requests found for branch \"feature/demo\"\n")
	if code, body := do(t, http.MethodGet, ts.URL+"/features/1/pr", ""); code != 200 ||
		body != `{"pr":null}` {
		t.Errorf("GET without a PR = %d %s, want 200 {\"pr\":null}", code, body)
	}
	if code, _ := do(t, http.MethodPost, ts.URL+"/features/1/pr/merge", ""); code != 404 {
		t.Errorf("merge without a PR = %d, want 404", code)
	}

	set("err.txt", "gh: not logged in")
	if code, body := do(t, http.MethodGet, ts.URL+"/features/1/pr", ""); code != 502 ||
		body != `{"error":"gh pr view: gh: not logged in"}` {
		t.Errorf("GET with gh failing = %d %s, want 502 and gh's text", code, body)
	}
	set("err.txt", "")

	set("resp.json", `{"number":12,"title":"Add login","url":"https://github.com/x/y/pull/12",`+
		`"state":"OPEN","isDraft":false,"mergeable":"MERGEABLE","reviewDecision":"APPROVED",`+
		`"headRefName":"feature/demo","baseRefName":"main","statusCheckRollup":[`+
		`{"__typename":"CheckRun","name":"build","status":"COMPLETED","conclusion":"SUCCESS"},`+
		`{"__typename":"CheckRun","name":"lint","status":"COMPLETED","conclusion":"FAILURE"},`+
		`{"__typename":"StatusContext","context":"deploy","state":"PENDING"}],`+
		`"body":"Line one\nLine two","additions":10,"deletions":3,"changedFiles":2,"files":[`+
		`{"path":"a.go","additions":7,"deletions":1},{"path":"b.go","additions":3,"deletions":2}]}`)
	const open = `{"pr":{"number":12,"title":"Add login","state":"OPEN",` +
		`"url":"https://github.com/x/y/pull/12","isDraft":false,"reviewDecision":"APPROVED",` +
		`"base":"main","head":"feature/demo","canMerge":true,"checks":[` +
		`{"name":"build","state":"pass"},{"name":"lint","state":"fail"},` +
		`{"name":"deploy","state":"pending"}],"body":"Line one\nLine two","additions":10,` +
		`"deletions":3,"files":[{"path":"a.go","additions":7,"deletions":1},` +
		`{"path":"b.go","additions":3,"deletions":2}]}}`
	if code, body := do(t, http.MethodGet, ts.URL+"/features/1/pr", ""); code != 200 || body != open {
		t.Errorf("GET open PR = %d %s, want 200 %s", code, body, open)
	}

	set("merge-err.txt", "Pull request is not mergeable: the base branch policy prohibits the merge.\n")
	if code, body := do(t, http.MethodPost, ts.URL+"/features/1/pr/merge", ""); code != 409 ||
		!strings.Contains(body, "base branch policy prohibits the merge") {
		t.Errorf("merge refused by gh = %d %s, want 409 with gh's text", code, body)
	}
	set("merge-err.txt", "")

	code, body := do(t, http.MethodPost, ts.URL+"/features/1/pr/merge", "")
	if code != 200 || !strings.Contains(body, `"state":"MERGED"`) ||
		!strings.Contains(body, `"canMerge":false`) {
		t.Errorf("merge = %d %s, want 200 with state MERGED and canMerge false", code, body)
	}
	merges, err := os.ReadFile(filepath.Join(files, "merges.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "pr merge 12 --merge\npr merge 12 --merge\n"; string(merges) != want {
		t.Errorf("gh merge calls = %q, want %q", merges, want)
	}
}
