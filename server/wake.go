package server

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/bhanuprakaash/reef/space"
	"github.com/bhanuprakaash/reef/store"
)

const (
	_userAuthor = "user"
)

// wake is a turn to run: role answers text, and its reply goes in the thread of parent.
// from is the author of the message messageID that woke it.
type wake struct {
	from      string
	fromBotID *int64
	botID     *int64
	messageID int64
	role      string
	text      string
	parent    *int64
	direct    *store.DirectMessage
	// stored is when the message that woke it, the first one for a role woken by several, was stored.
	stored time.Time
	// scopes are set on a wake routed from a message: text is what it was routed with, and the
	// turn's own text is built from the unread messages of scopes when it starts.
	scopes store.Scopes
	// hold is the id of the cap hold a wake was released by, when it was.
	hold int64
	// moveTo is the id of the Bot a move turn adds: text asks the lead for the summary it
	// starts from, and messageID is the user's message that asked.
	moveTo int64
}

// wakeMessage is the public message that woke w, nil for a direct wake.
func (w wake) wakeMessage() *int64 {
	if w.direct != nil || w.messageID == 0 {
		return nil
	}
	return &w.messageID
}

func (s *server) directWakes(
	ctx context.Context, spaceID, after int64, author *int64,
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
		if m.FromBotID == nil || author == nil || *m.FromBotID != *author {
			continue
		}
		if m.Status == store.DirectFailed || m.OriginSpaceID != nil {
			continue
		}
		ws = append(ws, directWake(&msgs[i]))
	}
	return ws, after, nil
}

// directWake is the wake of direct message m for its recipient.
func directWake(m *store.DirectMessage) wake {
	source := "direct request"
	if m.ReplyToID != nil {
		source = "direct response"
	}
	text := fmt.Sprintf("[Space %d, direct message %d, chain %d, source: %s]\n%s wrote privately: %s",
		m.SpaceID, m.ID, m.ChainID, source, m.From, m.Body)
	if m.IsUserGuidance() && m.FromBotID != nil {
		text = fmt.Sprintf("User instruction relayed by %s from Space %d (sent at %s, message %d): %s",
			m.From, *m.OriginSpaceID, m.CreatedAt.Format(time.RFC3339), *m.OriginMessageID, m.Body)
	} else if m.IsUserGuidance() {
		text = fmt.Sprintf("Guidance from the user (sent at %s, message %d): %s",
			m.CreatedAt.Format(time.RFC3339), *m.OriginMessageID, m.Body)
	}
	return wake{
		from: m.From, fromBotID: m.FromBotID, botID: m.ToBotID, messageID: m.ID,
		role: m.To, text: text, direct: m, stored: m.CreatedAt,
	}
}

// guidanceFirst moves the wakes the user's words are in ahead of the wakes a Bot started, each
// group keeping its order, so guidance never jumps a message the user sent before it.
func guidanceFirst(ws []wake) []wake {
	slices.SortStableFunc(ws, func(a, b wake) int {
		return cmp.Compare(guidanceRank(a), guidanceRank(b))
	})
	return ws
}

func guidanceRank(w wake) int {
	if w.fromBotID == nil || (w.direct != nil && w.direct.IsUserGuidance()) {
		return 0
	}
	return 1
}

// inStoredOrder is the wakes of one routing read, the public ones and the direct ones, in the
// order their messages were stored.
func inStoredOrder(public, direct []wake) []wake {
	ws := append(public, direct...)
	slices.SortStableFunc(ws, func(a, b wake) int { return a.stored.Compare(b.stored) })
	return ws
}

// wakes routes the messages of Space id stored after the message with id after. It also
// returns the id of the last message read, or after when there is none. See route.
func (s *server) wakes(ctx context.Context, id, after int64, author *int64) ([]wake, int64, error) {
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
func (s *server) route(ctx context.Context, id int64, msgs []store.Message, author *int64) ([]wake, error) {
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
	}
	var ws []wake
	for _, m := range msgs {
		if author == nil {
			if m.BotID != nil || m.Author != _userAuthor {
				continue
			}
		} else if m.BotID == nil || *m.BotID != *author {
			continue
		}
		var owner string
		if m.ParentID != nil {
			ownerID, err := s.st.ThreadOwnerBotID(ctx, m.ID)
			if err != nil {
				return nil, err
			}
			if ownerID == nil {
				owner = _userAuthor
			} else {
				for _, r := range team.Roles {
					if r.BotID == *ownerID {
						owner = r.Name
						break
					}
				}
			}
		}
		routed := m
		if m.BotID != nil {
			for _, r := range team.Roles {
				if r.BotID == *m.BotID {
					routed.Author = r.Name
					break
				}
			}
		}
		if len(team.Roles) == 0 {
			return nil, fmt.Errorf("Space %d has no lead: %w", id, store.ErrConflict)
		}
		woken := space.Route(routed, owner, roles, team.Roles[0].Name)
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
			var botID int64
			for _, member := range team.Roles {
				if member.Name == role {
					botID = member.BotID
					break
				}
			}
			if i := slices.IndexFunc(ws, func(w wake) bool { return w.botID != nil && *w.botID == botID }); i >= 0 {
				ws[i].text += "\n\n" + header + "\n" + text
				ws[i].scopes = addScope(ws[i].scopes, thread, asked)
				ws[i].messageID = m.ID
				continue
			}
			ws = append(ws, wake{
				from: m.Author, fromBotID: m.BotID, botID: &botID, messageID: m.ID,
				role: role, text: header + "\n" + text, parent: m.ParentID,
				scopes: addScope(nil, thread, asked), stored: m.CreatedAt,
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
