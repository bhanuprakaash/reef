package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
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
		{"pending", "", []string{"memory", "pending"}},
		{"retire", "3  retired\n", []string{"memory", "retire", "3"}},
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

	failures := []struct {
		args []string
		want string
	}{
		{[]string{"memory", "approve", "2"}, "invalid: memory 2 is not a candidate"},
		{[]string{"memory", "retire", "3"}, "invalid: memory 3 is already retired"},
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

// Not parallel: it swaps http.DefaultClient's transport.
func TestCLI_MemoryEmbed(t *testing.T) {
	var auth []string
	old := http.DefaultClient.Transport
	http.DefaultClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		auth = append(auth, r.Header.Get("Authorization"))
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"data":[{"embedding":[0.5,0.25]}]}`)),
		}, nil
	})
	t.Cleanup(func() { http.DefaultClient.Transport = old })

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
	vectors := func() int {
		t.Helper()
		st, err := store.Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = st.Close() }()
		var n int
		if err := st.DB().QueryRow("SELECT count(*) FROM memory WHERE vector IS NOT NULL").Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	if got, want := do("memory", "embed"), "none\n"; got != want {
		t.Fatalf("embed = %q, want %q", got, want)
	}
	do("memory", "add", "decision", "Use JWT for sessions", "--scope", "project:1")
	if n := vectors(); n != 0 {
		t.Fatalf("vectors with no provider = %d, want 0", n)
	}
	if got, want := do("memory", "embed", "openai", "--key", "sk-test-SECRET"),
		"embedding provider openai; embedded 1 items\n"; got != want {
		t.Fatalf("embed openai = %q, want %q", got, want)
	}
	if n := vectors(); n != 1 {
		t.Fatalf("vectors after backfill = %d, want 1", n)
	}
	do("memory", "add", "decision", "Use cookie sessions", "--scope", "project:1")
	if n := vectors(); n != 2 {
		t.Fatalf("vectors after add = %d, want 2", n)
	}
	if got, want := do("memory", "embed"), "openai\n"; got != want {
		t.Fatalf("embed = %q, want %q", got, want)
	}
	if got, want := do("memory", "embed", "openai"), "embedding provider openai\n"; got != want {
		t.Fatalf("embed openai again = %q, want %q", got, want)
	}
	if len(auth) == 0 {
		t.Error("the embedding service was never called")
	}
	for i, a := range auth {
		if a != "Bearer sk-test-SECRET" {
			t.Errorf("request %d Authorization = %q, want %q", i, a, "Bearer sk-test-SECRET")
		}
	}

	fails(`invalid: embedding provider "bogus" is not none, openai or voyage`, "memory", "embed", "bogus")
	if got, want := do("memory", "embed", "none"), "embedding provider none\n"; got != want {
		t.Fatalf("embed none = %q, want %q", got, want)
	}
	fails("invalid: embedding provider voyage needs --key", "memory", "embed", "voyage")
	for _, out := range outputs {
		if strings.Contains(out, "sk-test-SECRET") {
			t.Errorf("output %q shows the key", out)
		}
	}
}
