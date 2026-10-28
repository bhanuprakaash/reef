package space

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/bhanuprakaash/reef/store"
)

// AskedTogether starts the line that names every role one message woke.
const AskedTogether = "Asked together: "

// _firstWake is the most unread messages a scope the session has read nothing of shows.
const _firstWake = 20

// wakeText is the text of a turn of role in sessionID, and the scopes it sent: for each of
// scopes, the threads (0 is the top level) the wake was routed in, a block of the messages
// there that the session was not sent in an earlier turn, oldest first, and never the role's
// own or reef's. The role's turns in the session that sent say how far it has read: the
// newest message each sent for a scope, kept on the turn. A turn from before that was kept
// read nothing. A block's header names its newest message, so a message stored after the wake
// was routed is in it. A scope with nothing read shows only its last _firstWake messages and
// says the rest are in read_thread. A scope with no unread message is left out, so the text
// is empty when nothing is unread.
func wakeText(
	ctx context.Context, st *store.Store, spaceID int64, role, sessionID string,
	scopes store.Scopes,
) (string, store.Scopes, error) {
	read, err := readThrough(ctx, st, spaceID, role, sessionID)
	if err != nil {
		return "", nil, err
	}
	var blocks []string
	var sent store.Scopes
	for _, sc := range scopes {
		b, newest, err := block(ctx, st, spaceID, role, sc, read)
		if err != nil {
			return "", nil, err
		}
		if b != "" {
			blocks = append(blocks, b)
			sc.Read = newest
			sent = append(sent, sc)
		}
	}
	return strings.Join(blocks, "\n\n"), sent, nil
}

func block(
	ctx context.Context, st *store.Store, spaceID int64, role string, sc store.Scope,
	read map[int64]int64,
) (text string, newest int64, err error) {
	var msgs []store.Message
	if sc.Thread != 0 {
		msgs, err = st.Thread(ctx, spaceID, sc.Thread)
	} else {
		msgs, err = st.Messages(ctx, spaceID, read[0])
	}
	if err != nil {
		return "", 0, err
	}
	unread := slices.DeleteFunc(msgs, func(m store.Message) bool {
		switch {
		case m.Author == role || m.Author == store.ReefAuthor:
			return true
		case m.ParentID != nil:
			return sc.Thread == 0 || m.ID <= read[*m.ParentID]
		default:
			return m.ID <= read[0] || read[m.ID] > 0
		}
	})
	if len(unread) == 0 {
		return "", 0, nil
	}
	omitted := 0
	if read[sc.Thread] == 0 && len(unread) > _firstWake {
		omitted = len(unread) - _firstWake
		unread = unread[omitted:]
	}

	last := unread[len(unread)-1]
	var b strings.Builder
	fmt.Fprintf(&b, "[Space %d, message %d", spaceID, last.ID)
	label := "New in the Space:"
	if sc.Thread != 0 {
		fmt.Fprintf(&b, ", thread %d", sc.Thread)
		label = "New in the thread:"
	}
	b.WriteString("]\n")
	for _, a := range sc.Asked {
		b.WriteString(a + "\n")
	}
	if omitted > 0 {
		fmt.Fprintf(&b, "%d older messages are not shown; read them with read_thread.\n", omitted)
	}
	if len(unread) > 1 {
		b.WriteString(label + "\n")
		for _, m := range unread[:len(unread)-1] {
			fmt.Fprintf(&b, "%s: %s\n", m.Author, m.Body)
		}
		b.WriteString("\n")
	}
	if last.Author == _userAuthor {
		b.WriteString(last.Body)
	} else {
		b.WriteString(last.Author + " wrote: " + last.Body)
	}
	return b.String(), last.ID, nil
}

// readThrough maps a thread root, or 0 for the top level, to the newest message that role's
// turns in sessionID sent for it. A turn that failed before it sent did not tell the agent.
func readThrough(
	ctx context.Context, st *store.Store, spaceID int64, role, sessionID string,
) (map[int64]int64, error) {
	read := map[int64]int64{}
	if sessionID == "" {
		return read, nil
	}
	sent, err := st.SentScopes(ctx, spaceID, role, sessionID)
	if err != nil {
		return nil, err
	}
	for _, sc := range sent {
		read[sc.Thread] = max(read[sc.Thread], sc.Read)
	}
	return read, nil
}
