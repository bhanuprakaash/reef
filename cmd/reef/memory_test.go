package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCLI_Memory(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(t.TempDir())
	mustReef(t, dir, "project", "add", "--name", "shop")

	file := filepath.Join(t.TempDir(), "memory.md")
	if err := os.WriteFile(file, []byte("Use Go, not Python.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, want := mustReef(t, dir, "memory", "set", file),
		"saved memory for project shop\n"; got != want {
		t.Fatalf("memory set = %q, want %q", got, want)
	}
	if got, want := mustReef(t, dir, "memory", "show"), "Use Go, not Python.\n"; got != want {
		t.Fatalf("memory show = %q, want %q", got, want)
	}
	if got, want := mustReef(t, dir, "memory"), "Use Go, not Python.\n"; got != want {
		t.Fatalf("bare memory = %q, want %q", got, want)
	}

	var out bytes.Buffer
	err := run(context.Background(), []string{"memory", "set", "-", "--data", dir},
		strings.NewReader("from stdin"), &out, &syncBuffer{})
	if err != nil {
		t.Fatalf("memory set -: %v", err)
	}
	if got, want := mustReef(t, dir, "memory", "show"), "from stdin"; got != want {
		t.Fatalf("memory show after stdin set = %q, want %q", got, want)
	}
	if got, want := mustReef(t, dir, "memory", "--project", "1", "show"), "from stdin"; got != want {
		t.Fatalf("memory --project 1 show = %q, want %q", got, want)
	}

	if _, err := reef(t, dir, "memory", "show", "--project", "9"); err == nil ||
		err.Error() != "project 9: not found" {
		t.Errorf("memory show --project 9: error = %v, want project 9: not found", err)
	}
	if _, err := reef(t, dir, "memory", "show", "--project", "x"); err == nil ||
		err.Error() != `invalid: --project "x" is not a project id` {
		t.Errorf("memory show --project x: error = %v", err)
	}
}

func TestCLI_MemoryItems(t *testing.T) {
	dir := t.TempDir()

	steps := []struct {
		name, want string
		args       []string
	}{
		{"add", "1  accepted  decision  project:1  Use JWT for sessions\n",
			[]string{"memory", "add", "decision", "Use JWT for sessions", "--scope", "project:1", "--why", "stateless"}},
		{"add replacing", "2  accepted  decision  project:1  Use cookie sessions, not JWT\n",
			[]string{"memory", "add", "decision", "Use cookie sessions, not JWT", "--scope", "project:1",
				"--as", "developer", "--about", "work_item:12", "--replaces", "1"}},
		{"add lookalike", "3  accepted  decision  project:1  Use cookie sessions, not JWT\n2  similar  Use cookie sessions, not JWT\n",
			[]string{"memory", "add", "decision", "Use cookie sessions, not JWT", "--scope", "project:1"}},
		{"recall", "3  accepted  decision  project:1  direct  Use cookie sessions, not JWT\n" +
			"2  accepted  decision  project:1  direct  Use cookie sessions, not JWT\n" +
			"1  retired  decision  project:1  replaces memory:1  Use JWT for sessions\n",
			[]string{"memory", "recall", "cookie"}},
		{"recall other scope", "", []string{"memory", "recall", "cookie", "--scope", "team:1"}},
		{"recall limit", "3  accepted  decision  project:1  direct  Use cookie sessions, not JWT\n",
			[]string{"memory", "recall", "cookie", "--limit", "1"}},
		{"show", "2  accepted  decision  project:1  Use cookie sessions, not JWT\n" +
			"author  user  developer\nlink  about  work_item:12\nlink  replaces  memory:1\n",
			[]string{"memory", "show", "2"}},
		{"add asking approval", "4  candidate  convention  project:1  Indent with tabs\n",
			[]string{"memory", "add", "convention", "Indent with tabs", "--scope", "project:1", "--needs-approval"}},
		{"pending", "4  candidate  convention  project:1  Indent with tabs\n", []string{"memory", "pending"}},
		{"retire", "3  retired\n", []string{"memory", "retire", "3"}},
		{"add a second candidate", "5  candidate  convention  project:1  Review in pairs\n",
			[]string{"memory", "add", "convention", "Review in pairs", "--scope", "project:1", "--needs-approval"}},
		{"reject", "5  rejected\n", []string{"memory", "reject", "5"}},
		{"recall a rejected item", "", []string{"memory", "recall", "pairs"}},
		{"pending after reject", "4  candidate  convention  project:1  Indent with tabs\n",
			[]string{"memory", "pending"}},
	}
	for _, s := range steps {
		if got := mustReef(t, dir, s.args...); got != s.want {
			t.Errorf("%s: reef %s =\n%q\nwant\n%q", s.name, strings.Join(s.args, " "), got, s.want)
		}
	}

	got := mustReef(t, dir, "memory", "show", "1")
	const head = "1  retired  decision  project:1  Use JWT for sessions\nwhy  stateless\nauthor  user\nreplaced_by  2\nretired_at  "
	if !strings.HasPrefix(got, head) || strings.Count(got, "\n") != 5 {
		t.Errorf("show of a retired item = %q, want it to start with %q and end with a time", got, head)
	}

	got = mustReef(t, dir, "memory", "show", "5")
	const rejectedHead = "5  rejected  convention  project:1  Review in pairs\nauthor  user\nretired_at  "
	if !strings.HasPrefix(got, rejectedHead) || strings.Count(got, "\n") != 3 {
		t.Errorf("show of a rejected item = %q, want it to start with %q and end with a time", got, rejectedHead)
	}

	failures := []struct {
		args []string
		want string
	}{
		{[]string{"memory", "approve", "2"}, "invalid: memory 2 is not a candidate"},
		{[]string{"memory", "retire", "3"}, "invalid: memory 3 is already retired"},
		{[]string{"memory", "retire", "4"}, "invalid: memory 4 is a candidate"},
		{[]string{"memory", "retire", "5"}, "invalid: memory 5 is rejected"},
		{[]string{"memory", "reject", "2"}, "invalid: memory 2 is not a candidate"},
		{[]string{"memory", "reject", "5"}, "invalid: memory 5 is not a candidate"},
		{[]string{"memory", "show", "99"}, "memory 99: not found"},
		{[]string{"memory", "show", "x"}, `invalid: "x" is not a memory id`},
		{[]string{"memory", "add", "decision", "x", "--scope", "project"}, `invalid: --scope "project" is not kind:id`},
		{[]string{"memory", "add", "decision", "x", "--scope", "shelf:1"}, `invalid: unknown scope kind "shelf"`},
		{[]string{"memory", "add", "idea", "x", "--scope", "project:1"}, `invalid: unknown kind "idea"`},
		{[]string{"memory", "add", "decision", "x", "--scope", "project:1", "--replaces", "3"},
			"replace 3: invalid: memory 3 is already retired"},
	}
	for _, f := range failures {
		if _, err := reef(t, dir, f.args...); err == nil || err.Error() != f.want {
			t.Errorf("reef %s: error = %v, want %q", strings.Join(f.args, " "), err, f.want)
		}
	}
}

func TestCLI_MemoryEachCommandHasItsOwnFlags(t *testing.T) {
	const (
		undefined = "flag provided but not defined: -"
		retire    = "usage: reef memory retire <id> [--data DIR]"
		approve   = "usage: reef memory approve <id> [--data DIR]"
		reject    = "usage: reef memory reject <id> [--data DIR]"
		pending   = "usage: reef memory pending [--data DIR]"
		show      = "usage: reef memory [--project P] show [<id>] [--data DIR]"
		set       = "usage: reef memory [--project P] set FILE|- [--data DIR]"
		add       = `usage: reef memory add <kind> "<body>" --scope S [--as ROLE] [--about X]... [--replaces ID]... [--why W] [--needs-approval] [--data DIR]`
		recall    = `usage: reef memory recall "<q>" [--scope S] [--limit N] [--data DIR]`
		embed     = "usage: reef memory embed [none|openai|voyage|gemini [--key K]] [--data DIR]"
	)
	tests := []struct {
		name string
		give []string
		want string
	}{
		{"retire with a stray flag",
			[]string{"memory", "retire", "1", "--why", "x", "--key", "zzz"},
			undefined + "why\n" + retire},
		{"approve with --scope", []string{"memory", "approve", "1", "--scope", "project:1"},
			undefined + "scope\n" + approve},
		{"reject with --scope", []string{"memory", "reject", "1", "--scope", "project:1"},
			undefined + "scope\n" + reject},
		{"pending with --limit", []string{"memory", "pending", "--limit", "3"},
			undefined + "limit\n" + pending},
		{"show with --scope", []string{"memory", "show", "--scope", "project:1"},
			undefined + "scope\n" + show},
		{"set with --key", []string{"memory", "set", "-", "--key", "k"},
			undefined + "key\n" + set},
		{"add with --limit",
			[]string{"memory", "add", "fact", "x", "--scope", "project:1", "--limit", "3"},
			undefined + "limit\n" + add},
		{"add with --key",
			[]string{"memory", "add", "fact", "x", "--scope", "project:1", "--key", "k"},
			undefined + "key\n" + add},
		{"recall with --why", []string{"memory", "recall", "x", "--why", "y"},
			undefined + "why\n" + recall},
		{"recall with a negative limit",
			[]string{"memory", "recall", "x", "--limit", "-1"}, recall},
		{"embed with --scope", []string{"memory", "embed", "openai", "--scope", "project:1"},
			undefined + "scope\n" + embed},
		{"embed --key without a provider", []string{"memory", "embed", "--key", "k"}, embed},
		{"bare with a stray flag", []string{"memory", "--scope=project:1"},
			undefined + "scope\n" + show},
		{"unknown word", []string{"memory", "forget", "1"}, _memoryUsage},
	}
	dir := t.TempDir()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := reef(t, dir, tt.give...); err == nil || err.Error() != tt.want {
				t.Fatalf("reef %s: error = %v, want %q", strings.Join(tt.give, " "), err, tt.want)
			}
		})
	}
}

// Not parallel: it swaps http.DefaultTransport.
func TestCLI_MemoryEmbed(t *testing.T) {
	type call struct{ host, auth string }
	var calls []call
	fail := false
	old := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		auth := r.Header.Get("Authorization")
		if r.URL.Host == "generativelanguage.googleapis.com" {
			auth = r.Header.Get("x-goog-api-key")
		}
		calls = append(calls, call{r.URL.Host, auth})
		if fail {
			return &http.Response{
				StatusCode: http.StatusInternalServerError, Body: http.NoBody,
			}, nil
		}
		body := `{"data":[{"embedding":[0.5,0.25]}]}`
		switch r.URL.Host {
		case "api.voyageai.com":
			body = `{"data":[{"embedding":[0.5,0.25,0.125]}]}`
		case "generativelanguage.googleapis.com":
			body = `{"embedding":{"values":[0.5,0.25,0.125,1]}}`
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = old })

	dir := t.TempDir()
	var outputs []string
	do := func(args ...string) string {
		t.Helper()
		out, err := reef(t, dir, args...)
		if err != nil {
			t.Fatalf("reef %s: %v", strings.Join(args, " "), err)
		}
		outputs = append(outputs, out)
		return out
	}
	fails := func(want string, args ...string) {
		t.Helper()
		_, err := reef(t, dir, args...)
		if err == nil || err.Error() != want {
			t.Errorf("reef %s: error = %v, want %q", strings.Join(args, " "), err, want)
		}
		if err != nil {
			outputs = append(outputs, err.Error())
		}
	}
	query := func(q string) string {
		t.Helper()
		st, err := store.Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = st.Close() }()
		var got string
		if err := st.DB().QueryRow(q).Scan(&got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	const vectors = "SELECT count(*) FROM memory WHERE vector IS NOT NULL"
	const sizes = "SELECT min(length(vector)) || '-' || max(length(vector)) FROM memory"

	if got, want := do("memory", "embed"), "none\n"; got != want {
		t.Fatalf("embed = %q, want %q", got, want)
	}
	do("memory", "add", "decision", "Use JWT for sessions", "--scope", "project:1")
	if n := query(vectors); n != "0" {
		t.Fatalf("vectors with no provider = %s, want 0", n)
	}
	if got, want := do("memory", "embed", "openai", "--key", "sk-test-SECRET"),
		"embedding provider openai; embedded 1 items\n"; got != want {
		t.Fatalf("embed openai = %q, want %q", got, want)
	}
	if n := query(vectors); n != "1" {
		t.Fatalf("vectors after backfill = %s, want 1", n)
	}
	do("memory", "add", "decision", "Use cookie sessions", "--scope", "project:1")
	if n := query(vectors); n != "2" {
		t.Fatalf("vectors after add = %s, want 2", n)
	}
	if got, want := do("memory", "embed"), "openai\n"; got != want {
		t.Fatalf("embed = %q, want %q", got, want)
	}
	if got, want := do("memory", "embed", "openai"),
		"embedding provider openai; embedded 0 items\n"; got != want {
		t.Fatalf("embed openai again = %q, want %q", got, want)
	}

	fails("invalid: embedding provider voyage needs --key", "memory", "embed", "voyage")
	if got, want := do("memory", "embed"), "openai\n"; got != want {
		t.Fatalf("embed after refused switch = %q, want %q", got, want)
	}
	if got, want := do("memory", "embed", "voyage", "--key", "vk-test-SECRET"),
		"embedding provider voyage; embedded 2 items\n"; got != want {
		t.Fatalf("embed voyage = %q, want %q", got, want)
	}
	if got, want := query(sizes), "12-12"; got != want {
		t.Fatalf("vector bytes after switch to voyage = %s, want %s", got, want)
	}
	if got, want := do("memory", "embed", "gemini", "--key", "gk-test-SECRET"),
		"embedding provider gemini; embedded 2 items\n"; got != want {
		t.Fatalf("embed gemini = %q, want %q", got, want)
	}
	if got, want := query(sizes), "16-16"; got != want {
		t.Fatalf("vector bytes after switch to gemini = %s, want %s", got, want)
	}

	fail = true
	fails("embed 1: openai embeddings: status 500",
		"memory", "embed", "openai", "--key", "sk-2-SECRET")
	if n := query(vectors); n != "0" {
		t.Fatalf("vectors after failed switch = %s, want 0", n)
	}
	fail = false
	if got, want := do("memory", "embed", "openai"),
		"embedding provider openai; embedded 2 items\n"; got != want {
		t.Fatalf("embed openai retry = %q, want %q", got, want)
	}
	if got, want := query(sizes), "8-8"; got != want {
		t.Fatalf("vector bytes after retry = %s, want %s", got, want)
	}

	wantAuth := map[string][]string{
		"api.openai.com":                    {"Bearer sk-test-SECRET", "Bearer sk-2-SECRET"},
		"api.voyageai.com":                  {"Bearer vk-test-SECRET"},
		"generativelanguage.googleapis.com": {"gk-test-SECRET"},
	}
	for i, c := range calls {
		if !slices.Contains(wantAuth[c.host], c.auth) {
			t.Errorf("request %d to %s: Authorization = %q, want one of %q",
				i, c.host, c.auth, wantAuth[c.host])
		}
	}

	fails(`invalid: embedding provider "bogus" is not none, openai, voyage or gemini`,
		"memory", "embed", "bogus")
	if got, want := do("memory", "embed", "none"), "embedding provider none\n"; got != want {
		t.Fatalf("embed none = %q, want %q", got, want)
	}
	fails("invalid: embedding provider voyage needs --key", "memory", "embed", "voyage")
	for _, out := range outputs {
		if strings.Contains(out, "SECRET") {
			t.Errorf("output %q shows a key", out)
		}
	}
}

// Not parallel: it swaps http.DefaultTransport.
func TestCLI_MemoryAddWarnsWhenVectorNotStored(t *testing.T) {
	old := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusUnauthorized,
			Body: io.NopCloser(strings.NewReader(
				`{"error":{"code":"invalid_api_key","message":"Incorrect API key provided: sk-fake"}}`)),
		}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = old })

	dir := t.TempDir()
	mustReef(t, dir, "memory", "embed", "openai", "--key", "sk-fake")
	var stdout, stderr bytes.Buffer
	args := []string{"memory", "add", "decision", "Use JWT", "--scope", "project:1", "--data", dir}
	if err := run(context.Background(), args, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("add: %v", err)
	}
	if want := "1  accepted  decision  project:1  Use JWT\n"; stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
	want := "vector not stored: openai embeddings: status 401 invalid_api_key; run reef memory embed to retry\n"
	if stderr.String() != want {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
	if strings.Contains(stdout.String()+stderr.String(), "sk-fake") {
		t.Error("output contains the key")
	}
}

func TestCLI_MemoryEmbedSwitchRecovers(t *testing.T) {
	fail := false
	old := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if fail {
			return &http.Response{StatusCode: http.StatusInternalServerError, Body: http.NoBody}, nil
		}
		body := `{"data":[{"embedding":[0.5,0.25]}]}`
		if r.URL.Host == "api.voyageai.com" {
			body = `{"data":[{"embedding":[0.5,0.25,0.125]}]}`
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = old })

	dir := t.TempDir()
	query := func(q string) string {
		t.Helper()
		st, err := store.Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = st.Close() }()
		var got string
		if err := st.DB().QueryRow(q).Scan(&got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	exec := func(q string) {
		t.Helper()
		st, err := store.Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = st.Close() }()
		if _, err := st.DB().Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	const sizes = "SELECT min(length(vector)) || '-' || max(length(vector)) FROM memory"

	mustReef(t, dir, "memory", "add", "decision", "Use JWT for sessions", "--scope", "project:1")
	mustReef(t, dir, "memory", "add", "decision", "Use cookie sessions", "--scope", "project:1")
	mustReef(t, dir, "memory", "embed", "openai", "--key", "sk-test")
	if got, want := query(sizes), "8-8"; got != want {
		t.Fatalf("vector bytes on openai = %s, want %s", got, want)
	}

	fail = true
	if _, err := reef(t, dir, "memory", "embed", "voyage", "--key", "vk-test"); err == nil {
		t.Fatal("embed voyage with a failing backfill succeeded")
	}
	fail = false
	if got, want := mustReef(t, dir, "memory", "embed", "voyage"),
		"embedding provider voyage; embedded 2 items\n"; got != want {
		t.Fatalf("embed voyage after failed backfill = %q, want %q", got, want)
	}
	if got, want := query(sizes), "12-12"; got != want {
		t.Fatalf("vector bytes after rerun = %s, want %s", got, want)
	}

	exec(`CREATE TRIGGER no_clear BEFORE UPDATE OF vector ON memory
		BEGIN SELECT RAISE(ABORT, 'locked'); END`)
	if _, err := reef(t, dir, "memory", "embed", "openai", "--key", "sk-test"); err == nil ||
		!strings.Contains(err.Error(), "clear vectors") {
		t.Fatalf("embed openai with a failing clear: error = %v, want clear vectors", err)
	}
	if got, want := mustReef(t, dir, "memory", "embed"), "voyage\n"; got != want {
		t.Fatalf("provider after failed clear = %q, want %q", got, want)
	}
	exec(`DROP TRIGGER no_clear`)
	if got, want := mustReef(t, dir, "memory", "embed", "openai", "--key", "sk-test"),
		"embedding provider openai; embedded 2 items\n"; got != want {
		t.Fatalf("embed openai after failed clear = %q, want %q", got, want)
	}
	if got, want := query(sizes), "8-8"; got != want {
		t.Fatalf("vector bytes after rerun = %s, want %s", got, want)
	}
}

func TestCLI_MemoryEmbedKeyMakesDBOwnerOnly(t *testing.T) {
	old := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = old })

	dir := t.TempDir()
	if _, err := reef(t, dir, "project", "add", "--name", "x"); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(dir, "reef.db"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := reef(t, dir, "memory", "embed", "openai", "--key", "sk-test-SECRET"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"reef.db", "reef.db-wal", "reef.db-shm"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Errorf("%s mode = %o, want 600", name, got)
		}
	}
}
