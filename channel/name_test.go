package channel

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/bhanuprakaash/reef/agent"
)

// nameAgent answers a prompt with its scripted events; a nil script closes the stream, like
// an agent that exited.
type nameAgent struct {
	script []agent.Event
	events chan agent.Event
	once   sync.Once
	cfg    agent.Config
	prompt string
}

func newNameAgent(script ...agent.Event) *nameAgent {
	return &nameAgent{script: script, events: make(chan agent.Event, 8)}
}

func (a *nameAgent) Start(_ context.Context, cfg agent.Config) error {
	a.cfg = cfg
	return nil
}

func (a *nameAgent) Resume(context.Context, agent.Config, string) error { return nil }
func (a *nameAgent) Events() <-chan agent.Event                         { return a.events }

func (a *nameAgent) Interrupt() error {
	a.once.Do(func() { close(a.events) })
	return nil
}

func (a *nameAgent) Send(_ context.Context, prompt string) error {
	a.prompt = prompt
	for _, ev := range a.script {
		a.events <- ev
	}
	if a.script == nil {
		return a.Interrupt()
	}
	return nil
}

func text(s string) agent.Event { return agent.Event{Kind: agent.KindText, Text: s} }

func TestProposeName(t *testing.T) {
	turnEnd := agent.Event{Kind: agent.KindTurnEnd}
	tests := []struct {
		name    string
		give    *nameAgent
		want    string
		wantErr string
	}{
		{"name", newNameAgent(text("Login redirect"), text(" loop.\n"), turnEnd), "login redirect loop", ""},
		{"empty reply", newNameAgent(text("  \n"), turnEnd), "", "empty name"},
		{"agent error", newNameAgent(agent.Event{Kind: agent.KindError, Text: "not logged in"}), "",
			"agent: not logged in"},
		{"agent exits", newNameAgent(), "", "agent exited"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mk := func(string) (agent.Agent, error) { return tt.give, nil }
			got, err := proposeName(context.Background(), mk, "claude", "fix the login redirect")
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("name = %q, %v, want %q", got, err, tt.want)
			}
			if !strings.HasSuffix(tt.give.prompt, "fix the login redirect") {
				t.Errorf("prompt = %q, want it to end with the message", tt.give.prompt)
			}
			if tt.give.cfg.Model != "haiku" || tt.give.cfg.Cwd == "" || len(tt.give.cfg.DisallowedTools) == 0 {
				t.Errorf("cfg = %+v, want haiku, an empty folder and denied tools", tt.give.cfg)
			}
		})
	}
}

func TestCleanName(t *testing.T) {
	tests := []struct {
		name string
		give string
		want string
	}{
		{"plain", "login redirect", "login redirect"},
		{"quotes and punctuation", `"Fix: the Login, redirect!"`, "fix the login redirect"},
		{"first line only", "login redirect\nBecause the user asked", "login redirect"},
		{"four words", "one two three four five six", "one two three four"},
		{"forty characters", "internationalization configuration localization framework", "internationalization configuration"},
		{"hyphens split", "login-redirect-loop-fix", "login redirect loop fix"},
		{"nothing left", "...", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cleanName(tt.give); got != tt.want {
				t.Errorf("cleanName(%q) = %q, want %q", tt.give, got, tt.want)
			}
		})
	}
}
