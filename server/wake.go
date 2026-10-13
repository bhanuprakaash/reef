package server

import (
	"context"
	"fmt"
	"strings"

	"github.com/bhanuprakaash/reef/channel"
)

// wake is a turn to run: role answers text, and its reply goes in the thread of parent.
type wake struct {
	role   string
	text   string
	parent *int64
}

// wakes routes the messages of channel id stored after the message with id after, in
// order, into the turns they wake. It also returns the id of the last message routed, or
// after when there is none. A user message runs as its own text; an agent message as
// "<author> wrote: <body>". A message in a thread comes with the thread before it.
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
		role := channel.Route(m, owner, roles, ch.Role)
		if role == "" {
			continue
		}
		text := m.Body
		if m.Author != "user" {
			text = m.Author + " wrote: " + m.Body
		}
		if m.ParentID != nil {
			thread, err := s.st.Thread(ctx, id, *m.ParentID)
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
		ws = append(ws, wake{role: role, text: text, parent: m.ParentID})
	}
	return ws, after, nil
}
