package server

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/bhanuprakaash/reef/space"
	"github.com/bhanuprakaash/reef/store"
)

const (
	_userAuthor = "user"
)

var _wakeHeader = regexp.MustCompile(`^\[Space \d+, message (\d+)[,\]]`)

// wakeSource returns the id of the public message a wake text starts with, from the header
// route writes, or nil when text is not a public wake.
func wakeSource(text string) *int64 {
	m := _wakeHeader.FindStringSubmatch(text)
	if m == nil {
		return nil
	}
	id, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return nil
	}
	return &id
}

// wake is a turn to run: role answers text, and its reply goes in the thread of parent.
// from is the author of the message messageID that woke it.
type wake struct {
	from      string
	messageID int64
	role      string
	text      string
	parent    *int64
	direct    *store.DirectMessage
	// scopes are set on a wake routed from a message: text is what it was routed with, and the
	// turn's own text is built from the unread messages of scopes when it starts.
	scopes store.Scopes
	// hold is the id of the cap hold a wake was released by, when it was.
	hold int64
}

func (s *server) directWakes(
	ctx context.Context, spaceID, after int64, author string,
) ([]wake, int64, error) {
	msgs, err := s.st.DirectMessages(ctx, spaceID, after)
	if err != nil {
		return nil, after, err
	}
	if len(msgs) > 0 {
		after = msgs[len(msgs)-1].ID
	}
	ws := make([]wake, 0, len(msgs))
	for i := range msgs {
		m := msgs[i]
		if m.From != author {
			continue
		}
		if m.Status == store.DirectFailed {
			continue
		}
		source := "direct request"
		if m.ReplyToID != nil {
			source = "direct response"
		}
		text := fmt.Sprintf("[Space %d, direct message %d, chain %d, source: %s]\n%s wrote privately: %s",
			spaceID, m.ID, m.ChainID, source, m.From, m.Body)
		ws = append(ws, wake{
			from: m.From, messageID: m.ID,
			role: m.To, text: text, direct: &msgs[i],
		})
	}
	return ws, after, nil
}

// wakes routes the messages of Space id stored after the message with id after. It also
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

// route routes msgs of Space id, in order, into the turns they wake. Only the messages
// written by author are routed. A user message runs as its own text; an agent message as
// "<author> wrote: <body>". Each starts with a line naming the Space and the message, and
// its thread root when it is a reply. That text is what the wake was routed with: the turn's
// own text is built when it starts, from the messages the role has not been sent. A role woken
// by several messages gets one wake, their texts in id order; it replies where the first one did.
func (s *server) route(ctx context.Context, id int64, msgs []store.Message, author string) ([]wake, error) {
	ch, err := s.st.Space(ctx, id)
	if err != nil {
		return nil, err
	}
	team, err := s.st.SpaceTeam(ctx, ch.ID)
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
		woken := space.Route(m, owner, roles, ch.Role)
		if len(woken) == 0 {
			continue
		}
		text := m.Body
		if m.Author != _userAuthor {
			text = m.Author + " wrote: " + m.Body
		}
		header := fmt.Sprintf("[Space %d, message %d]", id, m.ID)
		var thread int64
		if m.ParentID != nil {
			thread = *m.ParentID
			header = fmt.Sprintf("[Space %d, message %d, thread %d]", id, m.ID, thread)
		}
		var asked string
		if len(woken) > 1 {
			names := make([]string, len(woken))
			for i, role := range woken {
				names[i] = "@" + role
			}
			asked = space.AskedTogether + strings.Join(names, ", ")
			header += "\n" + asked
		}
		for _, role := range woken {
			if i := slices.IndexFunc(ws, func(w wake) bool { return w.role == role }); i >= 0 {
				ws[i].text += "\n\n" + header + "\n" + text
				ws[i].scopes = addScope(ws[i].scopes, thread, asked)
				ws[i].messageID = m.ID
				continue
			}
			ws = append(ws, wake{
				from: m.Author, messageID: m.ID,
				role: role, text: header + "\n" + text, parent: m.ParentID,
				scopes: addScope(nil, thread, asked),
			})
		}
	}
	return ws, nil
}

// addScope adds the thread of a routed message, 0 for the top level, to scopes with the line
// that names the roles it woke together, if it has one. A thread is in scopes once.
func addScope(scopes store.Scopes, thread int64, asked string) store.Scopes {
	i := slices.IndexFunc(scopes, func(sc store.Scope) bool { return sc.Thread == thread })
	if i < 0 {
		scopes = append(scopes, store.Scope{Thread: thread})
		i = len(scopes) - 1
	}
	if asked != "" && !slices.Contains(scopes[i].Asked, asked) {
		scopes[i].Asked = append(scopes[i].Asked, asked)
	}
	return scopes
}
