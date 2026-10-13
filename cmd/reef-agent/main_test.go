package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bhanuprakaash/reef/agent"
)

// fake replays one scripted batch of events per Send, then waits for Interrupt.
type fake struct {
	turns       [][]agent.Event
	closeAfter  bool   // close Events after the last turn, like a crashed backend
	afterTurn   func() // runs after a turn's events are delivered
	sent        []string
	resumed     string
	interrupted bool
	events      chan agent.Event
	done        chan struct{}
	wg          sync.WaitGroup
	closeOnce   sync.Once
}

func newFake(turns ...[]agent.Event) *fake {
	return &fake{turns: turns, events: make(chan agent.Event), done: make(chan struct{})}
}

func (f *fake) Start(context.Context, agent.Config) error { return nil }

func (f *fake) Resume(_ context.Context, _ agent.Config, id string) error {
	f.resumed = id
	return nil
}

func (f *fake) Send(_ context.Context, prompt string) error {
	turn := f.turns[len(f.sent)]
	f.sent = append(f.sent, prompt)
	f.wg.Add(1)
	go func() {
		defer f.wg.Done()
		for _, ev := range turn {
			select {
			case f.events <- ev:
			case <-f.done:
				return
			}
		}
		if f.afterTurn != nil {
			f.afterTurn()
		}
		if f.closeAfter {
			f.closeOnce.Do(func() { close(f.events) })
		}
	}()
	return nil
}

func (f *fake) Events() <-chan agent.Event { return f.events }

func (f *fake) Interrupt() error {
	f.interrupted = true
	close(f.done)
	f.wg.Wait()
	f.closeOnce.Do(func() { close(f.events) })
	return nil
}

func usage(in, out, cache int64) agent.Event {
	u := agent.Usage{InputTokens: in, OutputTokens: out, CacheReadTokens: cache}
	return agent.Event{Kind: agent.KindUsage, Usage: &u}
}

func TestRun(t *testing.T) {
	text := agent.Event{Kind: agent.KindText, Text: "hi"}
	end := agent.Event{Kind: agent.KindTurnEnd}
	tests := []struct {
		name       string
		give       [][]agent.Event
		stdin      string
		closeAfter bool
		wantOut    string
		wantErr    string
		wantSent   []string
	}{
		{
			name:  "turn end then stdin EOF",
			give:  [][]agent.Event{{{Kind: agent.KindSession, SessionID: "s1"}, text, usage(3, 4, 0), end}},
			stdin: "",
			wantOut: `{"kind":"session","sessionId":"s1"}` + "\n" +
				`{"kind":"text","text":"hi"}` + "\n" +
				`{"kind":"usage","usage":{"inputTokens":3,"outputTokens":4,"cacheReadTokens":0,"cacheWriteTokens":0}}` + "\n" +
				`{"kind":"turn_end"}` + "\n",
			wantSent: []string{"first"},
		},
		{
			name:  "follow-up prompt from stdin",
			give:  [][]agent.Event{{text, end}, {{Kind: agent.KindText, Text: "again"}, end}},
			stdin: "\nsecond\n",
			wantOut: `{"kind":"text","text":"hi"}` + "\n" +
				`{"kind":"turn_end"}` + "\n" +
				`{"kind":"text","text":"again"}` + "\n" +
				`{"kind":"turn_end"}` + "\n",
			wantSent: []string{"first", "second"},
		},
		{
			name:     "error event",
			give:     [][]agent.Event{{text, {Kind: agent.KindError, Text: "boom"}}},
			wantOut:  `{"kind":"text","text":"hi"}` + "\n" + `{"kind":"error","text":"boom"}` + "\n",
			wantErr:  "agent error: boom",
			wantSent: []string{"first"},
		},
		{
			name:       "events closed mid-turn",
			give:       [][]agent.Event{{text}},
			closeAfter: true,
			wantOut:    `{"kind":"text","text":"hi"}` + "\n",
			wantErr:    "agent events closed unexpectedly",
			wantSent:   []string{"first"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake(tt.give...)
			f.closeAfter = tt.closeAfter
			var out bytes.Buffer
			err := run(context.Background(), []string{"--backend", "claude", "--cwd", "/tmp", "first"},
				strings.NewReader(tt.stdin), &out, func(string) (agent.Agent, error) { return f, nil })
			if got := errString(err); got != tt.wantErr {
				t.Errorf("err = %q, want %q", got, tt.wantErr)
			}
			if out.String() != tt.wantOut {
				t.Errorf("stdout = %q, want %q", out.String(), tt.wantOut)
			}
			if strings.Join(f.sent, ",") != strings.Join(tt.wantSent, ",") {
				t.Errorf("sent = %q, want %q", f.sent, tt.wantSent)
			}
			if !f.interrupted {
				t.Error("agent was not interrupted")
			}
		})
	}
}

func TestRun_Resume(t *testing.T) {
	f := newFake([]agent.Event{{Kind: agent.KindTurnEnd}})
	err := run(context.Background(), []string{"--backend", "claude", "--resume", "abc", "go"},
		strings.NewReader(""), &bytes.Buffer{}, func(string) (agent.Agent, error) { return f, nil })
	if err != nil || f.resumed != "abc" {
		t.Errorf("err = %v, resumed = %q, want nil, abc", err, f.resumed)
	}
}

func TestRun_CtrlC(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := newFake([]agent.Event{usage(3, 4, 1), usage(2, 1, 0)})
	f.afterTurn = cancel // the turn never ends: only Interrupt unblocks the backend
	var out bytes.Buffer
	done := make(chan error)
	go func() {
		done <- run(ctx, []string{"--backend", "claude", "--cwd", "/tmp", "first"},
			strings.NewReader(""), &out, func(string) (agent.Agent, error) { return f, nil })
	}()
	select {
	case err := <-done:
		if !errors.Is(err, errInterrupted) {
			t.Errorf("err = %v, want errInterrupted", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("run did not return within 2s")
	}
	want := `{"kind":"usage","usage":{"inputTokens":3,"outputTokens":4,"cacheReadTokens":1,"cacheWriteTokens":0}}` + "\n" +
		`{"kind":"usage","usage":{"inputTokens":2,"outputTokens":1,"cacheReadTokens":0,"cacheWriteTokens":0}}` + "\n" +
		`{"kind":"usage","usage":{"inputTokens":5,"outputTokens":5,"cacheReadTokens":1,"cacheWriteTokens":0}}` + "\n"
	if out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
	if !f.interrupted {
		t.Error("agent was not interrupted")
	}
}

func TestNewAgent(t *testing.T) {
	tests := []struct{ give, want string }{
		{"claude", ""},
		{"codex", ""},
		{"vim", `unknown backend "vim"`},
	}
	for _, tt := range tests {
		_, err := newAgent(tt.give)
		if got := errString(err); got != tt.want {
			t.Errorf("newAgent(%q) err = %q, want %q", tt.give, got, tt.want)
		}
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
