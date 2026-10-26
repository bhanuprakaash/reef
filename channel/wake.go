package channel

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/bhanuprakaash/reef/store"
)

// AskedTogether starts the line that names every role one message woke.
const AskedTogether = "Asked together: "

var _header = regexp.MustCompile(
	`(?m)^\[channel \d+, message (\d+)(?:, thread (\d+))?\](?:\n(` + AskedTogether + `[^\n]*))?`)

// scope is the thread a message the wake was routed with is in, 0 for the top level, and
// the Asked together lines of those messages.
type scope struct {
	thread int64
	asked  []string
}

// wakeText is the text of a turn of role in sessionID: for each thread, or the top level,
// that routed, the text the wake was routed with, has a message in, a block of the messages
// there that the session was not sent in an earlier turn, oldest first, and never the role's
// own or reef's. The role's turns that sent a wake say how far it has read: the message each
// header names. A block's header names its newest message, so a message stored after the wake
// was routed is in it. A block with no unread message is left out, so the text is empty when
// nothing is unread.
func wakeText(
	ctx context.Context, st *store.Store, channelID int64, role, sessionID, routed string,
) (string, error) {
	read, err := readThrough(ctx, st, channelID, role, sessionID)
	if err != nil {
		return "", err
	}
	var blocks []string
	for _, sc := range scopes(routed) {
		b, err := block(ctx, st, channelID, role, sc, read)
		if err != nil {
			return "", err
		}
		if b != "" {
			blocks = append(blocks, b)
		}
	}
	return strings.Join(blocks, "\n\n"), nil
}

func scopes(routed string) []scope {
	var out []scope
	for _, g := range _header.FindAllStringSubmatch(routed, -1) {
		thread, _ := strconv.ParseInt(g[2], 10, 64)
		i := slices.IndexFunc(out, func(s scope) bool { return s.thread == thread })
		if i < 0 {
			out = append(out, scope{thread: thread})
			i = len(out) - 1
		}
		if g[3] != "" && !slices.Contains(out[i].asked, g[3]) {
			out[i].asked = append(out[i].asked, g[3])
		}
	}
	return out
}

func block(
	ctx context.Context, st *store.Store, channelID int64, role string, sc scope,
	read map[int64]int64,
) (string, error) {
	var msgs []store.Message
	var err error
	if sc.thread != 0 {
		msgs, err = st.Thread(ctx, channelID, sc.thread)
	} else {
		msgs, err = st.Messages(ctx, channelID, read[0])
	}
	if err != nil {
		return "", err
	}
	unread := slices.DeleteFunc(msgs, func(m store.Message) bool {
		switch {
		case m.Author == role || m.Author == store.ReefAuthor:
			return true
		case m.ParentID != nil:
			return sc.thread == 0 || m.ID <= read[*m.ParentID]
		default:
			return m.ID <= read[0] || read[m.ID] > 0
		}
	})
	if len(unread) == 0 {
		return "", nil
	}

	last := unread[len(unread)-1]
	var b strings.Builder
	fmt.Fprintf(&b, "[channel %d, message %d", channelID, last.ID)
	label := "New in the channel:"
	if sc.thread != 0 {
		fmt.Fprintf(&b, ", thread %d", sc.thread)
		label = "New in the thread:"
	}
	b.WriteString("]\n")
	for _, a := range sc.asked {
		b.WriteString(a + "\n")
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
	return b.String(), nil
}

// readThrough maps a thread root, or 0 for the top level, to the newest message that role's
// turns in sessionID sent for it. A turn that failed before it sent did not tell the agent.
func readThrough(
	ctx context.Context, st *store.Store, channelID int64, role, sessionID string,
) (map[int64]int64, error) {
	read := map[int64]int64{}
	if sessionID == "" {
		return read, nil
	}
	turns, err := st.Turns(ctx, store.TurnFilter{ChannelID: &channelID})
	if err != nil {
		return nil, err
	}
	for _, t := range turns {
		if t.Role != role || t.SessionID != sessionID || !t.Sent {
			continue
		}
		for _, g := range _header.FindAllStringSubmatch(t.WakeText, -1) {
			id, _ := strconv.ParseInt(g[1], 10, 64)
			thread, _ := strconv.ParseInt(g[2], 10, 64)
			read[thread] = max(read[thread], id)
		}
	}
	return read, nil
}
