package server

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
}

func treeSum(t *testing.T, dir string) string {
	t.Helper()
	h := sha256.New()
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(path)
		fmt.Fprintf(h, "%s\x00%s\x00", path, b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

type skillFiles struct {
	Name     string `json:"name"`
	Version  int    `json:"version"`
	Revision string `json:"revision"`
	Files    []struct{ Path, Body string }
}

func TestSkillImport(t *testing.T) {
	st, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h := New(st, testMemory(t, st), t.TempDir(), "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)

	marker := filepath.Join(t.TempDir(), "ran")
	src := filepath.Join(t.TempDir(), "deploy")
	writeFile(t, filepath.Join(src, "SKILL.md"), "---\nname: deploy\ndescription: Ship it.\n---\nSteps.\n", 0o644)
	writeFile(t, filepath.Join(src, "scripts/run.sh"), "#!/bin/sh\ntouch "+marker+"\n", 0o755)
	writeFile(t, filepath.Join(src, "notes/ref.md"), "reference\n", 0o644)
	before := treeSum(t, src)

	code, body := do(t, http.MethodPost, ts.URL+"/skills/import", `{"path":"`+src+`"}`)
	if code != http.StatusCreated {
		t.Fatalf("import folder = %d %s", code, body)
	}
	var got skillFiles
	code, body = do(t, http.MethodGet, ts.URL+"/skills/1", "")
	if err := json.Unmarshal([]byte(body), &got); err != nil || code != http.StatusOK {
		t.Fatalf("GET /skills/1 = %d %s", code, body)
	}
	want := `deploy 1 [SKILL.md=---
name: deploy
description: Ship it.
---
Steps.
 notes/ref.md=reference
 scripts/run.sh=#!/bin/sh
touch ` + marker + `
]`
	var files []string
	for _, f := range got.Files {
		files = append(files, f.Path+"="+f.Body)
	}
	if line := fmt.Sprintf("%s %d %v", got.Name, got.Version, files); line != want {
		t.Errorf("stored skill = %q, want %q", line, want)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("the import ran a script")
	}
	if after := treeSum(t, src); after != before {
		t.Error("the import changed the source folder")
	}

	if code, body := do(t, http.MethodGet, ts.URL+"/skills/library", ""); code != http.StatusOK ||
		!strings.Contains(body, `"name":"deploy","description":"Ship it.","sourceKind":"folder","source":"`+src+`"`) {
		t.Errorf("GET /skills/library = %d %s, want deploy from the folder", code, body)
	}
	if code, _ := do(t, http.MethodPost, ts.URL+"/skills/import", `{"path":"`+src+`"}`); code != http.StatusConflict {
		t.Errorf("import the same name again = %d, want 409", code)
	}

	writeFile(t, filepath.Join(src, "notes/ref.md"), "reference v2\n", 0o644)
	if code, body := do(t, http.MethodPost, ts.URL+"/skills/1/update", ""); code != http.StatusOK ||
		!strings.Contains(body, `"version":2`) {
		t.Errorf("update = %d %s, want version 2", code, body)
	}
	old, err := st.SkillFiles(t.Context(), 1, 1)
	if err != nil || string(old[1].Body) != "reference\n" {
		t.Errorf("version 1 notes/ref.md after update = %q %v, want %q", old[1].Body, err, "reference\n")
	}
}

func TestSkillImportGit(t *testing.T) {
	st, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h := New(st, testMemory(t, st), t.TempDir(), "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)

	work := t.TempDir()
	gitIn(t, work, "init", "-q", "-b", "main")
	writeFile(t, filepath.Join(work, "pkg/review/SKILL.md"), "---\nname: review\ndescription: Look hard.\n---\n", 0o644)
	writeFile(t, filepath.Join(work, "pkg/review/checks.md"), "list\n", 0o644)
	writeFile(t, filepath.Join(work, "bad/SKILL.md"), "no frontmatter\n", 0o644)
	gitIn(t, work, "add", ".")
	gitIn(t, work, "commit", "-q", "-m", "skills")
	sha := gitIn(t, work, "rev-parse", "HEAD")
	repo := "file://" + work

	code, body := do(t, http.MethodPost, ts.URL+"/skills/import", `{"repo":"`+repo+`","ref":"main","dir":"pkg/review"}`)
	if code != http.StatusCreated || !strings.Contains(body, `"revision":"`+sha+`"`) ||
		!strings.Contains(body, `"source":"`+repo+`"`) {
		t.Errorf("import repo = %d %s, want revision %s", code, body, sha)
	}

	for name, in := range map[string]string{
		"no SKILL.md":     `{"repo":"` + repo + `","dir":"pkg"}`,
		"bad frontmatter": `{"repo":"` + repo + `","dir":"bad"}`,
		"missing dir":     `{"repo":"` + repo + `","dir":"nope"}`,
		"escaping dir":    `{"repo":"` + repo + `","dir":"../x"}`,
		"bad repo":        `{"repo":"file://` + work + `/missing"}`,
		"missing folder":  `{"path":"` + work + `/missing"}`,
		"both":            `{"path":"x","repo":"y"}`,
	} {
		if code, body := do(t, http.MethodPost, ts.URL+"/skills/import", in); code != http.StatusBadRequest {
			t.Errorf("import with %s = %d %s, want 400", name, code, body)
		}
	}
	if skills, err := st.Skills(t.Context()); err != nil || len(skills) != 1 {
		t.Errorf("library after failed imports = %v %v, want only review", skills, err)
	}
}
