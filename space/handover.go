package space

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bhanuprakaash/reef/memory"
	"github.com/bhanuprakaash/reef/store"
	"github.com/bhanuprakaash/reef/worktree"
)

const (
	_handoverMessages  = 5
	_handoverDecisions = 10
)

// renewAt is the input tokens a backend session may have used before the Bot's next turn starts
// a new backend context in its place.
var renewAt int64 = 800_000

// renewed reports whether the backend session of last has used renewAt input tokens.
func renewed(last store.Turn) bool {
	return last.SessionUsage != nil && last.SessionUsage.InputTokens >= renewAt
}

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
// left, the workspace and the last messages in short.
func handover(
	ctx context.Context, st *store.Store, ms *memory.Store, ch store.Space, botID int64, dir string,
) (string, error) {
	var b strings.Builder
	b.WriteString("## Handover\nThis session replaces your earlier one. The Space keeps its history; " +
		"this is a short summary, not the transcript.\n")
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
		items, _, err := ms.Newest(ctx, BotScopes(ch, botID)[0], []string{memory.KindDecision}, _handoverDecisions)
		if err != nil {
			return "", err
		}
		lines := make([]string, len(items))
		for i, it := range items {
			lines[i] = itemLine(it)
		}
		b.WriteString(handoverList("Settled decisions", lines))
	}
	msgs, err := st.MessagesPage(ctx, ch.ID, 0, _handoverMessages)
	if err != nil {
		return "", err
	}
	lines := make([]string, len(msgs))
	for i, m := range msgs {
		lines[i] = fmt.Sprintf("%s: %s", m.Author, firstLine(m.Body))
	}
	b.WriteString(handoverList("Last messages", lines))
	b.WriteString("Read earlier messages with read_thread and search_sessions.\n")
	return b.String(), nil
}

func handoverList(label string, lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return label + ":\n- " + strings.Join(lines, "\n- ") + "\n"
}
