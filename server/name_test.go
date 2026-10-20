package server

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
)

type namer struct {
	calls   atomic.Int32
	started chan struct{}
	release chan struct{}
	name    string
	err     error
}

func (n *namer) propose(ctx context.Context, backend, message string) (string, error) {
	n.calls.Add(1)
	if n.release != nil {
		n.started <- struct{}{}
		select {
		case <-n.release:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return n.name + " " + backend + " " + message, n.err
}

// defaultNamed adds channel 2 with the name the new-channel form sends for an empty field.
func (e *liveEnv) defaultNamed(t *testing.T, n *namer) {
	t.Helper()
	e.h.s.name = n.propose
	if status, body := do(t, http.MethodPost, e.url+"/channels",
		`{"projectId":1,"name":"claude 02:32 PM","backend":"claude"}`); status != 201 {
		t.Fatalf("create channel = %d %s, want 201", status, body)
	}
}

func (e *liveEnv) channelName(t *testing.T, id int64) string {
	t.Helper()
	c, err := e.st.Channel(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return c.Name
}

func TestSend_NamesDefaultNamedChannel(t *testing.T) {
	env := newLiveEnv(t)
	env.defaultNamed(t, &namer{name: "login"})
	sub := subscribeTo(t, env.url, 2)

	if status, _ := env.sendTo(t, 2, "fix redirect"); status != 202 {
		t.Fatalf("send = %d, want 202", status)
	}
	for f := sub.next(t); f != `name {}`; f = sub.next(t) {
	}
	if got, want := env.channelName(t, 2), "login claude fix redirect"; got != want {
		t.Errorf("name = %q, want %q", got, want)
	}
}

func TestSend_NameAttemptedOnce(t *testing.T) {
	env := newLiveEnv(t)
	n := &namer{err: errors.New("provider down")}
	env.defaultNamed(t, n)

	for _, text := range []string{"one", "two"} {
		if status, _ := env.sendTo(t, 2, text); status != 202 {
			t.Fatalf("send %s = %d, want 202", text, status)
		}
	}
	env.h.Close()
	if got := env.channelName(t, 2); got != "claude 02:32 PM" {
		t.Errorf("name = %q, want the default name kept", got)
	}
	if got := n.calls.Load(); got != 1 {
		t.Errorf("naming calls = %d, want 1", got)
	}
	ms, err := env.st.Messages(context.Background(), 2, 0)
	if err != nil || len(ms) < 2 {
		t.Errorf("messages = %d, %v, want the channel to keep working", len(ms), err)
	}
}

func TestSend_RenameDuringProposalWins(t *testing.T) {
	env := newLiveEnv(t)
	n := &namer{name: "proposed", started: make(chan struct{}), release: make(chan struct{})}
	env.defaultNamed(t, n)

	if status, _ := env.sendTo(t, 2, "hello"); status != 202 {
		t.Fatalf("send = %d, want 202", status)
	}
	<-n.started
	if status, body := do(t, http.MethodPut, env.url+"/channels/2", `{"name":"mine"}`); status != 200 {
		t.Fatalf("rename = %d %s, want 200", status, body)
	}
	close(n.release)
	env.h.Close()
	if got := env.channelName(t, 2); got != "mine" {
		t.Errorf("name = %q, want mine", got)
	}
}

func TestSend_RenameBeforeFirstMessageWins(t *testing.T) {
	env := newLiveEnv(t)
	n := &namer{name: "proposed"}
	env.defaultNamed(t, n)

	do(t, http.MethodPut, env.url+"/channels/2", `{"name":"mine"}`)
	env.sendTo(t, 2, "hello")
	env.h.Close()
	if got := env.channelName(t, 2); got != "mine" {
		t.Errorf("name = %q, want mine", got)
	}
	if got := n.calls.Load(); got != 0 {
		t.Errorf("naming calls = %d, want 0", got)
	}
}

func TestSend_PermissionChangeKeepsNamePending(t *testing.T) {
	env := newLiveEnv(t)
	n := &namer{name: "proposed"}
	env.defaultNamed(t, n)

	do(t, http.MethodPut, env.url+"/channels/2", `{"name":"claude 02:32 PM","permissionMode":"acceptEdits"}`)
	env.sendTo(t, 2, "hello")
	env.h.Close()
	if got, want := env.channelName(t, 2), "proposed claude hello"; got != want {
		t.Errorf("name = %q, want %q", got, want)
	}
}

func TestSend_TypedNameIsNeverProposed(t *testing.T) {
	tests := []struct {
		name string
		give string
	}{
		{"plain", `{"projectId":1,"name":"build","backend":"claude"}`},
		{"time after another word", `{"projectId":1,"name":"standup 10:30","backend":"claude"}`},
		{"default of the other backend", `{"projectId":1,"name":"codex 02:32 PM","backend":"claude"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newLiveEnv(t)
			n := &namer{name: "proposed"}
			env.h.s.name = n.propose
			if status, body := do(t, http.MethodPost, env.url+"/channels", tt.give); status != 201 {
				t.Fatalf("create channel = %d %s, want 201", status, body)
			}
			env.sendTo(t, 2, "hello")
			env.h.Close()
			if got := n.calls.Load(); got != 0 {
				t.Errorf("naming calls = %d, want 0", got)
			}
		})
	}
}

func TestIsDefaultName(t *testing.T) {
	tests := []struct {
		name   string
		give   string
		prefix string
		want   bool
	}{
		{"24 hour", "claude 14:32", "claude", true},
		{"12 hour", "claude 02:32 PM", "claude", true},
		{"narrow no-break space", "claude 02:32 PM", "claude", true},
		{"short hour lowercase", "codex 9:05 am", "codex", true},
		{"team", "solo 14:32", "solo", true},
		{"team with a dot", "a.b 14:32", "a.b", true},
		{"typed", "build", "claude", false},
		{"text after the time", "claude 14:32 notes", "claude", false},
		{"other prefix", "standup 14:32", "claude", false},
		{"no time", "claude", "claude", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isDefaultName(tt.give, tt.prefix); got != tt.want {
				t.Errorf("isDefaultName(%q, %q) = %v, want %v", tt.give, tt.prefix, got, tt.want)
			}
		})
	}
}
