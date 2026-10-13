package server

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/bhanuprakaash/reef/channel"
)

// wake is a turn to run: role answers text, and its reply goes in the thread of parent.
// from is the author of the message messageID that woke it; gated is set when the team
// holds wakes from that author to role for approval.
type wake struct {
	from      string
	messageID int64
	gated     bool
	role      string
	text      string
	parent    *int64
}

// wakes routes the messages of channel id stored after the message with id after, in
// order, into the turns they wake. It also returns the id of the last message routed, or
// after when there is none. A user message runs as its own text; an agent message as
// "<author> wrote: <body>". Each starts with a line naming the channel, the message and its
// thread root; a message in a thread comes with the thread before it. A role woken by
// several messages gets one wake, their texts in id order.
func (s *server) wakes(ctx context.Context, id, after int64) ([]wake, int64, error) {
	ch, err := s.st.Channel(ctx, id)
	if err != nil {
		return nil, after, err
	}
	team, err := s.st.Team(ctx, ch.Team)
	if err != nil {
		return nil, after, err
	}
	roles := make([]string, len(team.Roles))
	for i, r := range team.Roles {
		roles[i] = r.Name
	}
	msgs, err := s.st.Messages(ctx, id, after)
	if err != nil {
		return nil, after, err
	}
	var ws []wake
	for _, m := range msgs {
		after = m.ID
		var owner string
		if m.ParentID != nil {
			if owner, err = s.st.ThreadOwner(ctx, m.ID); err != nil {
				return nil, after, err
			}
		}
		woken := channel.Route(m, owner, roles, ch.Role)
		if len(woken) == 0 {
			continue
		}
		text := m.Body
		if m.Author != "user" {
			text = m.Author + " wrote: " + m.Body
		}
		root := m.ID
		if m.ParentID != nil {
			root = *m.ParentID
			thread, err := s.st.Thread(ctx, id, root)
			if err != nil {
				return nil, after, err
			}
			var b strings.Builder
			b.WriteString("Thread so far:\n")
			for _, t := range thread {
				if t.ID < m.ID {
					fmt.Fprintf(&b, "%s: %s\n", t.Author, t.Body)
				}
			}
			text = b.String() + "\n" + text
		}
		text = fmt.Sprintf("[channel %d, message %d, thread %d]\n%s", id, m.ID, root, text)
		for _, role := range woken {
			gated := team.Gated(m.Author, role)
			if i := slices.IndexFunc(ws, func(w wake) bool { return w.role == role }); i >= 0 {
				ws[i].text += "\n\n" + text
				ws[i].gated = ws[i].gated || gated
				continue
			}
			ws = append(ws, wake{
				from: m.Author, messageID: m.ID, gated: gated,
				role: role, text: text, parent: m.ParentID,
			})
		}
	}
	return ws, after, nil
}
