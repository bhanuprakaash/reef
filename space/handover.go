package space

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/bhanuprakaash/reef/memory"
	"github.com/bhanuprakaash/reef/store"
	"github.com/bhanuprakaash/reef/worktree"
)

const (
	_handoverMessages     = 5
	_handoverMessageRunes = 2000
	_handoverDecisions    = 10
)

// _renewTokens is the context size at which the Bot's next turn starts a new backend context in
// place of the one that reached it.
const _renewTokens = 150_000

// replacement reports about the backend context that starts for the Bot's BotSession botSessionID
// when no session is left to resume: whether the Bot has finished turns in the Space, whose
// history the new context needs a handover for, and whether the BotSession replaces an earlier one
// and has not yet had a backend session of its own. A revoked BotSession counts as no earlier one:
// revoking posts its own notice.
func replacement(
	ctx context.Context, st *store.Store, spaceID, botID, botSessionID int64,
) (history, refreshed bool, err error) {
	turns, err := st.Turns(ctx, store.TurnFilter{SpaceID: &spaceID, BotID: &botID})
	if err != nil {
		return false, false, err
	}
	earlier, started := false, false
	for _, t := range turns {
		own := t.BotSessionID != nil && *t.BotSessionID == botSessionID
		started = started || own && t.SessionID != ""
		if t.Status == "done" && t.WakeText != "" {
			history = true
			revoked := false
			if !own && t.BotSessionID != nil {
				if revoked, err = st.BotSessionRevoked(ctx, *t.BotSessionID); err != nil {
					return false, false, err
				}
			}
			earlier = earlier || !own && !revoked
		}
	}
	return history, earlier && !started, nil
}

// handover is what a new backend context of the Bot reads in its first turn in place of the
// transcript, built from the stored records only: the goal, the decisions, the work done and
// left, the workspace and the last messages the Bot has already answered, which its first wake
// leaves out.
func handover(
	ctx context.Context, st *store.Store, ms *memory.Store, ch store.Space, bot store.Role, dir string,
	newcomer bool,
) (string, error) {
	var b strings.Builder
	if newcomer {
		b.WriteString("## Handover\nYou joined this Space after it started. The Space keeps its " +
			"history; this is a short summary of where it stands, not the transcript.\n")
	} else {
		b.WriteString("## Handover\nThis session replaces your earlier one. The Space keeps its history; " +
			"this is a short summary, not the transcript.\n")
	}
	if ch.WorkItemID == nil {
		fmt.Fprintf(&b, "Goal: %s\nWorkspace: %s\n", firstLine(ch.Name), dir)
	} else {
		f, err := st.WorkItem(ctx, *ch.WorkItemID)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "Goal: %s\n", firstLine(f.Title))
		if f.Description != "" {
			fmt.Fprintf(&b, "Description: %s\n", firstLine(f.Description))
		}
		brief, err := st.Brief(ctx, f.ID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return "", err
		}
		if brief.Body != "" {
			fmt.Fprintf(&b, "Brief: %s\n", firstLine(brief.Body))
		}
		subs, err := st.WorkItems(ctx, store.WorkItemFilter{ParentID: &f.ID})
		if err != nil {
			return "", err
		}
		var done, open []string
		for _, w := range subs {
			line := fmt.Sprintf("sub-item %d: %s", w.ID, firstLine(w.Title))
			switch w.Status {
			case "done":
				done = append(done, line)
			case "dropped":
			default:
				open = append(open, line+" ("+w.Status+")")
			}
		}
		b.WriteString(handoverList("Completed", done))
		b.WriteString(handoverList("Unfinished", open))
		fmt.Fprintf(&b, "Workspace: %s, branch %s\n", dir, worktree.Branch(f.TreeKey()))
		pr, ok, err := st.WorkItemGitHub(ctx, f.ID)
		if err != nil {
			return "", err
		}
		if ok && pr.PRURL != "" {
			fmt.Fprintf(&b, "Pull request: %s\n", pr.PRURL)
		}
	}
	if ms != nil {
		items, _, err := ms.Newest(ctx, BotScopes(ch, bot.BotID)[0], []string{memory.KindDecision}, _handoverDecisions)
		if err != nil {
			return "", err
		}
		lines := make([]string, len(items))
		for i, it := range items {
			lines[i] = itemLine(it)
		}
		b.WriteString(handoverList("Settled decisions", lines))
	}
	msgs, err := st.Messages(ctx, ch.ID, 0)
	if err != nil {
		return "", err
	}
	var posted int64
	for _, m := range msgs {
		if m.BotID != nil && *m.BotID == bot.BotID && m.ParentID == nil {
			posted = max(posted, m.ID)
		}
	}
	msgs = slices.DeleteFunc(msgs, func(m store.Message) bool { return m.ID > posted })
	msgs = msgs[max(0, len(msgs)-_handoverMessages):]
	lines := make([]string, len(msgs))
	for i, m := range msgs {
		lines[i] = fmt.Sprintf("%s: %s", m.Author, clip(m.Body, _handoverMessageRunes))
	}
	turns, err := st.Turns(ctx, store.TurnFilter{SpaceID: &ch.ID, BotID: &bot.BotID, Status: "done"})
	if err != nil {
		return "", err
	}
	for i := len(turns) - 1; i >= 0; i-- {
		if turns[i].WakeText == "" {
			continue
		}
		if strings.TrimSpace(turns[i].Reply) != "" {
			lines = append(lines, fmt.Sprintf("%s: %s", bot.Name, clip(turns[i].Reply, _handoverMessageRunes)))
		}
		break
	}
	b.WriteString(handoverList("Last messages", lines))
	b.WriteString("Read earlier messages with read_thread and search_sessions, and saved memory with recall.\n")
	return b.String(), nil
}

// newcomer reports whether the Bot joined the Space after it started and has not run in it yet:
// it has no turn here and a join notice names it.
func newcomer(ctx context.Context, st *store.Store, spaceID int64, bot store.Role) (bool, error) {
	turns, err := st.Turns(ctx, store.TurnFilter{SpaceID: &spaceID, BotID: &bot.BotID})
	if err != nil || len(turns) > 0 {
		return false, err
	}
	notices, err := st.MembershipNotices(ctx, spaceID, time.Time{})
	if err != nil {
		return false, err
	}
	return slices.Contains(notices, bot.Name+store.JoinedSuffix), nil
}

func clip(body string, limit int) string {
	r := []rune(strings.TrimSpace(body))
	if len(r) > limit {
		return string(r[:limit]) + "…"
	}
	return string(r)
}

func handoverList(label string, lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return label + ":\n- " + strings.Join(lines, "\n- ") + "\n"
}
