// Package channel builds what a teammate is told at the start of a channel.
package channel

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bhanuprakaash/reef/store"
)

const (
	_dateLayout    = "2006-01-02"
	_maxIndexRunes = 100
	_closing       = "Details are on demand through the reef MCP tools read_decision, " +
		"read_thread and search_sessions. Record decisions with record_decision."
)

// Context is the first prompt of a role on a feature: the brief, the role's own
// notes and an index of decisions, channels and open tasks. It never holds a
// message body.
func Context(ctx context.Context, st *store.Store, featureID int64, role string) (string, error) {
	f, err := st.Feature(ctx, featureID)
	if err != nil {
		return "", err
	}
	p, err := st.Project(ctx, f.ProjectID)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "You are the %s on feature %q (%s) in project %q.\n",
		role, f.Name, f.Slug, p.Name)
	if f.Goal != "" {
		fmt.Fprintf(&b, "Goal: %s\n", f.Goal)
	}
	fmt.Fprintf(&b, "Stage: %s\n", f.Stage)

	brief, err := st.Brief(ctx, f.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return "", err
	}
	section(&b, "Brief", brief.Body, "No brief yet.")

	notes, err := st.RoleNotes(ctx, f.ID, role)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return "", err
	}
	section(&b, "Your notes", notes.Body, "No notes yet.")

	decisions, err := st.Decisions(ctx, f.ID)
	if err != nil {
		return "", err
	}
	channels, err := st.Channels(ctx, store.ChannelFilter{FeatureID: &f.ID})
	if err != nil {
		return "", err
	}
	tasks, err := st.Tasks(ctx, store.TaskFilter{FeatureID: &f.ID})
	if err != nil {
		return "", err
	}

	b.WriteString("\n## Index\n")
	var lines []string
	for _, d := range decisions {
		lines = append(lines, fmt.Sprintf("decision %d: %s (%s, %s)",
			d.ID, firstLine(d.Body), d.Author, d.CreatedAt.Format(_dateLayout)))
	}
	group(&b, "Decisions", lines)
	lines = nil
	for _, c := range channels {
		n, err := st.CountMessages(ctx, c.ID)
		if err != nil {
			return "", err
		}
		lines = append(lines, fmt.Sprintf("channel %d: %s, %s, %d messages",
			c.ID, c.Name, c.CreatedAt.Format(_dateLayout), n))
	}
	group(&b, "Channels", lines)
	lines = nil
	for _, t := range tasks {
		if t.Status != "done" {
			lines = append(lines, fmt.Sprintf("task %d [%s]: %s", t.ID, t.Status, t.Title))
		}
	}
	group(&b, "Open tasks", lines)

	b.WriteString("\n" + _closing + "\n")
	return b.String(), nil
}

// projectContext is the first prompt of a role in a quick session: it names the
// project and says there is no feature yet.
func projectContext(p store.Project, role string) string {
	return fmt.Sprintf("You are the %s in project %q (%s).\n"+
		"This is a quick session: it has no feature yet, so there is no brief, notes or decisions.\n",
		role, p.Name, p.Path)
}

// firstLine is the first line of body, cut to _maxIndexRunes characters.
func firstLine(body string) string {
	line := strings.TrimSpace(body)
	if i := strings.IndexAny(line, "\r\n"); i >= 0 {
		line = line[:i]
	}
	if r := []rune(line); len(r) > _maxIndexRunes {
		return string(r[:_maxIndexRunes]) + "…"
	}
	return line
}

func section(b *strings.Builder, title, body, empty string) {
	if body == "" {
		body = empty
	}
	fmt.Fprintf(b, "\n## %s\n%s\n", title, strings.TrimSpace(body))
}

func group(b *strings.Builder, title string, lines []string) {
	if len(lines) == 0 {
		lines = []string{"none"}
	}
	fmt.Fprintf(b, "\n### %s\n%s\n", title, strings.Join(lines, "\n"))
}
