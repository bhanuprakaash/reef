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
		"read_thread and search_sessions. Record decisions with record_decision. When the " +
		"user and the team agree on the goal, scope or plan, call propose_brief."
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

	who, err := profileLine(ctx, st)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "You are the %s on feature %q (%s) in project %q.\n",
		role, f.Name, f.Slug, p.Name)
	b.WriteString(who)
	if f.Goal != "" {
		fmt.Fprintf(&b, "Goal: %s\n", f.Goal)
	}
	if f.Kind == "fixes" {
		b.WriteString("This feature is a list of fixes tracked as tasks, not one piece of work with stages.\n")
	} else {
		fmt.Fprintf(&b, "Stage: %s\n", f.Stage)
		fmt.Fprintf(&b, "Stages in order: %s. Call set_stage when the work moves to the next one.\n",
			strings.Join(store.Stages(), ", "))
	}

	if p.BaseBranch != "" {
		fmt.Fprintf(&b, "Base branch: %s. A pull request you open targets it.\n", p.BaseBranch)
	}

	mem, err := st.ProjectMemory(ctx, f.ProjectID)
	if err != nil {
		return "", err
	}
	if mem.Body != "" {
		section(&b, "Project memory", mem.Body, "")
	}

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
		if n == 0 {
			continue
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

// roleIntro is the role's persona and skills, when it has any, to put before its context.
func roleIntro(r store.Role) string {
	var b strings.Builder
	if r.Persona != "" {
		fmt.Fprintf(&b, "%s\n\n", strings.TrimSpace(r.Persona))
	}
	if len(r.Skills) > 0 {
		fmt.Fprintf(&b, "Your skills:\n%s\n\n", strings.Join(r.Skills, ", "))
	}
	return b.String()
}

// _introChangedPrefix marks roleIntro text as a change to a role whose session is already
// running, so the agent knows this replaces what it was told before.
const _introChangedPrefix = "Your persona or skills changed:\n\n"

// updatedRoleIntro is roleIntro(r), marked as a change to a role whose session is already
// running, so the agent knows this replaces what it was told before.
func updatedRoleIntro(r store.Role) string {
	return _introChangedPrefix + roleIntro(r)
}

// teamIntro tells a teammate who is on its team and that they talk through the channel. A
// team of one gets none.
func teamIntro(team store.Team, r store.Role) string {
	if len(team.Roles) < 2 {
		return ""
	}
	names := make([]string, len(team.Roles))
	for i, t := range team.Roles {
		names[i] = "@" + t.Name
	}
	s := fmt.Sprintf("\n## Team\nYour team: %s. To hand work to a teammate, @mention them in a "+
		"message: that wakes them. Your reply, and anything you send with post_to_channel, is "+
		"posted to the channel; nobody sees anything else. A message that wakes you starts with "+
		"[channel N, message N] or [channel N, message N, thread N]: your plain reply goes where "+
		"the header says, a top-level one starting your own thread; pass thread as the parent of "+
		"post_to_channel when the header has one.\n", strings.Join(names, ", "))
	if r.Lead {
		s += "Do not summarize or repeat a specialist's post back to the user: they see it.\n"
	}
	return s
}

// profileLine says who the agent works with, or is empty before a profile exists.
func profileLine(ctx context.Context, st *store.Store) (string, error) {
	p, err := st.Profile(ctx)
	if errors.Is(err, store.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	s := "You work with " + p.Name
	if o := strings.ToLower(p.Occupation); o != "" && o != "other" {
		s += ", a " + o
	}
	s += "."
	if len(p.Style) > 0 {
		s += " They like: " + strings.ToLower(strings.Join(p.Style, ", ")) + "."
	}
	return s + "\n", nil
}

func projectContext(p store.Project, role, memory, who string) string {
	s := fmt.Sprintf("You are the %s in project %q (%s).\n"+
		"This is a quick session: it has no feature yet, so there is no brief, notes or decisions.\n",
		role, p.Name, p.Path) + who
	if memory != "" {
		var b strings.Builder
		b.WriteString(s)
		section(&b, "Project memory", memory, "")
		return b.String()
	}
	return s
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
