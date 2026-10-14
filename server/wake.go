package server

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/bhanuprakaash/reef/channel"
	"github.com/bhanuprakaash/reef/store"
)

const (
	_userAuthor = "user"
	_reefAuthor = "reef"
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

// wakes routes the messages of channel id stored after the message with id after. It also
// returns the id of the last message read, or after when there is none. See route.
func (s *server) wakes(ctx context.Context, id, after int64, author string) ([]wake, int64, error) {
	msgs, err := s.st.Messages(ctx, id, after)
	if err != nil {
		return nil, after, err
	}
	if len(msgs) > 0 {
		after = msgs[len(msgs)-1].ID
	}
	ws, err := s.route(ctx, id, msgs, author)
	return ws, after, err
}

// route routes msgs of channel id, in order, into the turns they wake. Only the messages
// written by author are routed. A user message runs as its own text; an agent message as
// "<author> wrote: <body>". Each starts with a line naming the channel and the message, and
// its thread root when it is a reply; a message in a thread comes with the thread before it.
// A role woken by several messages in the same thread gets one wake, their texts in id order.
// The first wake of a role starts, after its header, with what reef posted since the role's
// last turn (see notes).
func (s *server) route(ctx context.Context, id int64, msgs []store.Message, author string) ([]wake, error) {
	ch, err := s.st.Channel(ctx, id)
	if err != nil {
		return nil, err
	}
	team, err := s.st.Team(ctx, ch.Team)
	if err != nil {
		return nil, err
	}
	roles := make([]string, len(team.Roles))
	for i, r := range team.Roles {
		roles[i] = r.Name
		if r.Lead {
			roles[i] = ch.Role
		}
	}
	var ws []wake
	told := map[string]bool{}
	for _, m := range msgs {
		if m.Author != author {
			continue
		}
		var owner string
		if m.ParentID != nil {
			if owner, err = s.st.ThreadOwner(ctx, m.ID); err != nil {
				return nil, err
			}
		}
		woken := channel.Route(m, owner, roles, ch.Role)
		if len(woken) == 0 {
			continue
		}
		text := m.Body
		if m.Author != _userAuthor {
			text = m.Author + " wrote: " + m.Body
		}
		header := fmt.Sprintf("[channel %d, message %d]", id, m.ID)
		if m.ParentID != nil {
			root := *m.ParentID
			header = fmt.Sprintf("[channel %d, message %d, thread %d]", id, m.ID, root)
			thread, err := s.st.Thread(ctx, id, root)
			if err != nil {
				return nil, err
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
		for _, role := range woken {
			gated := team.Gated(m.Author, role)
			if i := slices.IndexFunc(ws, func(w wake) bool {
				return w.role == role && sameParent(w.parent, m.ParentID)
			}); i >= 0 {
				ws[i].text += "\n\n" + header + "\n" + text
				ws[i].gated = ws[i].gated || gated
				continue
			}
			var notes string
			if !told[role] {
				told[role] = true
				if notes, err = s.notes(ctx, id, role); err != nil {
					return nil, err
				}
			}
			ws = append(ws, wake{
				from: m.Author, messageID: m.ID, gated: gated,
				role: role, text: header + "\n" + notes + text, parent: m.ParentID,
			})
		}
	}
	return ws, nil
}

// notes is what reef posted in channel id since role's last turn began, one "reef wrote:" line
// each, to go before the wake. A rollback changes the files under a session that cannot be
// rewound, so this is how the role learns of it. A role with no turn yet gets none: it starts
// from the files as they are.
func (s *server) notes(ctx context.Context, id int64, role string) (string, error) {
	turns, err := s.st.Turns(ctx, store.TurnFilter{ChannelID: &id})
	if err != nil {
		return "", err
	}
	var since time.Time
	for _, t := range turns {
		if t.Role == role {
			since = t.StartedAt
		}
	}
	if since.IsZero() {
		return "", nil
	}
	msgs, err := s.st.MessagesBy(ctx, id, _reefAuthor, since)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, m := range msgs {
		fmt.Fprintf(&b, "%s wrote: %s\n", m.Author, m.Body)
	}
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	return b.String(), nil
}

func sameParent(a, b *int64) bool {
	return a == b || a != nil && b != nil && *a == *b
}
