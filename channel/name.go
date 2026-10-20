package channel

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/bhanuprakaash/reef/agent"
)

const (
	_nameWords   = 4
	_nameRunes   = 40
	_nameMessage = 1000
	_nameClaude  = "haiku"
	_namePrompt  = "Name this chat channel from its first message. Reply with only the name: " +
		"two to four lowercase words separated by spaces that say what the work is, no hyphens, " +
		"no punctuation, no quotes. Do not use tools.\n\nFirst message:\n%s"
)

// _nameDenied are the claude tools a naming call may not use; the call also runs in an empty
// folder, so it has nothing to read.
var _nameDenied = []string{"Bash", "Edit", "Write", "MultiEdit", "NotebookEdit", "WebFetch", "WebSearch"}

// ProposeName asks the backend for a short name for a channel whose first user message is
// message. It is one throwaway turn: no tools, no stored turn or message.
func ProposeName(ctx context.Context, backend, message string) (string, error) {
	return proposeName(ctx, newAgent, backend, message)
}

func proposeName(
	ctx context.Context, mk func(backend string) (agent.Agent, error), backend, message string,
) (string, error) {
	ag, err := mk(backend)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(os.TempDir(), "reef-name")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("name folder: %w", err)
	}
	defer func() {
		_ = ag.Interrupt()
		for range ag.Events() {
		}
	}()

	cfg := agent.Config{Cwd: dir, DisallowedTools: _nameDenied}
	switch backend {
	case "claude":
		cfg.Model = _nameClaude
	case "codex":
		cfg.PermissionMode = "read-only"
	}
	if err := ag.Start(ctx, cfg); err != nil {
		return "", err
	}
	if r := []rune(message); len(r) > _nameMessage {
		message = string(r[:_nameMessage])
	}
	if err := ag.Send(ctx, fmt.Sprintf(_namePrompt, message)); err != nil {
		return "", err
	}
	var reply strings.Builder
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case ev, ok := <-ag.Events():
			if !ok {
				return "", errors.New("agent exited")
			}
			switch ev.Kind {
			case agent.KindText:
				reply.WriteString(ev.Text)
			case agent.KindError:
				return "", fmt.Errorf("agent: %s", ev.Text)
			case agent.KindTurnEnd:
				name := cleanName(reply.String())
				if name == "" {
					return "", errors.New("empty name")
				}
				return name, nil
			}
		}
	}
}

// cleanName cuts a reply down to the channel naming convention: the first line, lowercase
// words of letters, digits and hyphens, at most four words and 40 characters.
func cleanName(reply string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(reply), "\n")
	words := strings.FieldsFunc(strings.ToLower(line), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	var name string
	for _, w := range words[:min(len(words), _nameWords)] {
		next := strings.TrimSpace(name + " " + w)
		if len([]rune(next)) > _nameRunes {
			break
		}
		name = next
	}
	return name
}
