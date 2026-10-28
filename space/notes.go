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
// a wake, then a line for each direct request the role sent that has failed since. A rollback
// changes the files under a session that cannot be rewound, so this is how the role learns of
// it. A turn that failed before it sent does not count: the agent never saw it. A role with no
// earlier sent turn gets none: it starts from the files as they are.
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
	if err != nil {
		return "", err
	}
	failed, err := r.failedRequests(ctx, since)
	if err != nil || len(msgs)+len(failed) == 0 {
		return text, err
	}
	var b strings.Builder
	for _, m := range msgs {
		fmt.Fprintf(&b, "%s wrote: %s\n", m.Author, m.Body)
	}
	for _, m := range failed {
		fmt.Fprintf(&b, "%s wrote: Your direct request to %s failed: %s\n", store.ReefAuthor, m.To, firstLine(m.Body))
	}
	b.WriteString("\n")
	return afterHeader(text, b.String()), nil
}

// failedRequests is the direct requests the role sent that are failed and got their failed
// response after since, oldest first.
func (r *Runner) failedRequests(ctx context.Context, since time.Time) ([]store.DirectMessage, error) {
	msgs, err := r.st.DirectMessagesForRole(ctx, r.spaceID, r.role, 0)
	if err != nil {
		return nil, err
	}
	answered := map[int64]bool{}
	for _, m := range msgs {
		if m.ReplyToID != nil && m.CreatedAt.After(since) {
			answered[*m.ReplyToID] = true
		}
	}
	var failed []store.DirectMessage
	for _, m := range msgs {
		if answered[m.ID] && m.From == r.role && m.State == store.DirectFailed {
			failed = append(failed, m)
		}
	}
	return failed, nil
}

// afterHeader puts add after the header line of a wake, or at the top of text with none.
func afterHeader(text, add string) string {
	if header, rest, ok := strings.Cut(text, "\n"); ok && strings.HasPrefix(header, "[Space ") {
		return header + "\n" + add + rest
	}
	return add + text
}
