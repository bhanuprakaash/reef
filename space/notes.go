package space

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bhanuprakaash/reef/store"
)

// withNotes puts what reef posted in the Space since the role's last turn that sent its
// prompt, before turn began, into text, one "reef wrote:" line each, after the header line of
// a wake. A rollback changes the files under a session that cannot be rewound, so this is how
// the role learns of it. A turn that failed before it sent does not count: the agent never
// saw it. A role with no earlier sent turn gets none: it starts from the files as they are.
func (r *Runner) withNotes(ctx context.Context, turn store.Turn, text string) (string, error) {
	turns, err := r.st.Turns(ctx, store.TurnFilter{SpaceID: &r.spaceID})
	if err != nil {
		return "", err
	}
	var since time.Time
	for _, t := range turns {
		if t.Role == r.role && t.ID < turn.ID && t.Sent {
			since = t.StartedAt
		}
	}
	if since.IsZero() {
		return text, nil
	}
	msgs, err := r.st.MessagesBy(ctx, r.spaceID, store.ReefAuthor, since)
	if err != nil || len(msgs) == 0 {
		return text, err
	}
	var b strings.Builder
	for _, m := range msgs {
		fmt.Fprintf(&b, "%s wrote: %s\n", m.Author, m.Body)
	}
	b.WriteString("\n")
	return afterHeader(text, b.String()), nil
}

// afterHeader puts add after the header line of a wake, or at the top of text with none.
func afterHeader(text, add string) string {
	if header, rest, ok := strings.Cut(text, "\n"); ok && strings.HasPrefix(header, "[Space ") {
		return header + "\n" + add + rest
	}
	return add + text
}
