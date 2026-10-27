// Package channel builds what a teammate is told at the start of a channel.
package channel

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bhanuprakaash/reef/memory"
	"github.com/bhanuprakaash/reef/store"
)

const (
	_dateLayout    = "2006-01-02"
	_maxIndexRunes = 100
	_youHeader     = "## You\n"
	_findingsLimit = 5
	_projectLimit  = 10
	_teamLimit     = 5
	_yourLimit     = 5
	_toolsLine     = "Details are on demand through the reef MCP tools recall, read_memory, " +
		"read_thread, list_tasks and search_sessions. Record decisions with remember. When the " +
		"user and the team agree on the goal, scope or plan, call propose_brief."
)

var _findingKinds = []string{
	memory.KindFinding, memory.KindFact, memory.KindConvention, memory.KindLesson,
}

type shelfPart struct {
	label string
	kinds []string
	limit int
}

var _shelfParts = map[string][]shelfPart{
	memory.ScopeWorkItem: {
		{label: "Decisions", kinds: []string{memory.KindDecision}},
		{label: "Findings", kinds: _findingKinds, limit: _findingsLimit},
	},
	memory.ScopeProject:  {{label: "Project", limit: _projectLimit}},
	memory.ScopeTeam:     {{label: "Team", limit: _teamLimit}},
	memory.ScopeTeammate: {{label: "Your items", limit: _yourLimit}},
}

// Context is the first prompt of a role on a feature without a team: what `reef context`
// prints. It never holds a message body.
func Context(
	ctx context.Context, st *store.Store, ms *memory.Store, featureID int64, role string,
) (string, error) {
	f, err := st.Feature(ctx, featureID)
	if err != nil {
		return "", err
	}
	return firstPrompt(ctx, st, ms, f.ProjectID, &f.ID, 0, store.Team{}, store.Role{Name: role})
}

// Scopes are the shelves of memory a role reads and writes in ch: the feature's when it has
// one, the project's, the team's unless it is the solo team, and the role's own.
func Scopes(ch store.Channel, role string) []memory.Ref {
	var scopes []memory.Ref
	if ch.FeatureID != nil {
		scopes = append(scopes,
			memory.Ref{Kind: memory.ScopeWorkItem, ID: strconv.FormatInt(*ch.FeatureID, 10)})
	}
	scopes = append(scopes,
		memory.Ref{Kind: memory.ScopeProject, ID: strconv.FormatInt(ch.ProjectID, 10)})
	if ch.Team != "" && ch.Team != store.SoloTeam {
		scopes = append(scopes, memory.Ref{Kind: memory.ScopeTeam, ID: ch.Team})
	}
	return append(scopes, memory.Ref{Kind: memory.ScopeTeammate, ID: role})
}

// firstPrompt is the first prompt of a role in three sections: You, How to work and What we
// know. A part with nothing to say is left out, and so is a section with no part left. A quick
// session has no featureID. channelID is the channel the prompt is for, or 0 for none; it is not
// counted as an earlier channel.
func firstPrompt(
	ctx context.Context, st *store.Store, ms *memory.Store, projectID int64, featureID *int64,
	channelID int64, team store.Team, r store.Role,
) (string, error) {
	p, err := st.Project(ctx, projectID)
	if err != nil {
		return "", err
	}
	who, err := profileLine(ctx, st)
	if err != nil {
		return "", err
	}

	ch := store.Channel{ID: channelID, ProjectID: projectID, FeatureID: featureID, Team: team.Name}
	var b strings.Builder
	b.WriteString(_youHeader + roleIntro(r))
	var know []string
	how := teamIntro(team, r)
	if featureID == nil {
		fmt.Fprintf(&b, "You are the %s in project %q (%s).\n"+
			"This is a quick session: it has no feature yet, so there is no brief or feature items.\n",
			r.Name, p.Name, p.Path)
		b.WriteString(who)
		if know, err = itemParts(ctx, ms, nil, ch, r.Name); err != nil {
			return "", err
		}
	} else {
		f, err := st.Feature(ctx, *featureID)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "You are the %s on feature %q (%s) in project %q.\n",
			r.Name, f.Name, f.Slug, p.Name)
		how += _toolsLine + "\n"
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
		if know, err = featureKnow(ctx, st, ms, f, ch, r.Name); err != nil {
			return "", err
		}
	}
	if p.BaseBranch != "" {
		fmt.Fprintf(&b, "Base branch: %s. A pull request you open targets it.\n", p.BaseBranch)
	}

	if how != "" {
		b.WriteString("\n## How to work\n" + how)
	}
	if len(know) > 0 {
		b.WriteString("\n## What we know\n" + strings.Join(know, "\n"))
	}
	return b.String(), nil
}

// featureKnow is the parts of What we know for a role on f. Open tasks and channels with
// messages, other than ch, are only counted: the role looks them up with tools.
func featureKnow(
	ctx context.Context, st *store.Store, ms *memory.Store, f store.Feature, ch store.Channel,
	role string,
) ([]string, error) {
	brief, err := st.Brief(ctx, f.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	channels, err := st.Channels(ctx, store.ChannelFilter{FeatureID: &f.ID})
	if err != nil {
		return nil, err
	}
	tasks, err := st.Tasks(ctx, store.TaskFilter{FeatureID: &f.ID})
	if err != nil {
		return nil, err
	}

	var doing, counts []string
	open := 0
	for _, t := range tasks {
		if t.Status == "done" {
			continue
		}
		open++
		if t.Status == "doing" {
			doing = append(doing, fmt.Sprintf("task %d: %s", t.ID, t.Title))
		}
	}
	if open > 0 {
		counts = append(counts, fmt.Sprintf("%d open %s (list_tasks).", open, plural(open, "task")))
	}
	earlier := 0
	for _, c := range channels {
		if c.ID == ch.ID {
			continue
		}
		n, err := st.CountMessages(ctx, c.ID)
		if err != nil {
			return nil, err
		}
		if n > 0 {
			earlier++
		}
	}
	if earlier > 0 {
		counts = append(counts, fmt.Sprintf("%d earlier %s (search_sessions).", earlier, plural(earlier, "channel")))
	}

	parts := appendPart(nil, "Brief", brief.Body)
	if parts, err = itemParts(ctx, ms, parts, ch, role); err != nil {
		return nil, err
	}
	parts = appendPart(parts, "Doing", strings.Join(doing, "\n"))
	return appendPart(parts, "On demand", strings.Join(counts, " ")), nil
}

// itemParts adds the parts that read memory items to parts, one shelf of Scopes at a time. Only
// accepted items are read, the newest first. A part with a limit ends with how many of how many
// it shows.
func itemParts(
	ctx context.Context, ms *memory.Store, parts []string, ch store.Channel, role string,
) ([]string, error) {
	for _, scope := range Scopes(ch, role) {
		for _, sp := range _shelfParts[scope.Kind] {
			items, total, err := ms.Newest(ctx, scope, sp.kinds, sp.limit)
			if err != nil {
				return nil, err
			}
			lines := make([]string, len(items), len(items)+1)
			for i, it := range items {
				lines[i] = itemLine(it)
			}
			if len(items) > 0 && sp.limit > 0 {
				lines = append(lines,
					fmt.Sprintf("%d of %d; recall(..., scope=%s)", len(items), total, scope.Kind))
			}
			parts = appendPart(parts, sp.label, strings.Join(lines, "\n"))
		}
	}
	return parts, nil
}

func itemLine(it memory.Item) string {
	return fmt.Sprintf("%s %d: %s (%s, %s)",
		it.Kind, it.ID, firstLine(it.Body), it.Author, it.CreatedAt.Format(_dateLayout))
}

// changedSince is what the feature gained after since that a role whose session was told of
// everything before it has not seen: the accepted memory items added in the scopes the role
// reads, the items retired in them and the brief in full when a version is newer. An item added
// and retired since is named as retired only, as the role may have seen it in its previous
// turn. It is empty when nothing changed. Tasks are left out.
func changedSince(
	ctx context.Context, st *store.Store, ms *memory.Store, ch store.Channel, role string,
	since time.Time,
) (string, error) {
	added, retired, err := ms.Changed(ctx, Scopes(ch, role), since)
	if err != nil {
		return "", err
	}
	var lines []string
	for _, it := range added {
		lines = append(lines, itemLine(it))
	}
	for _, it := range retired {
		line := fmt.Sprintf("%s %d is retired", it.Kind, it.ID)
		if it.ReplacedBy != nil {
			by, err := ms.Read(ctx, *it.ReplacedBy)
			if err != nil {
				return "", err
			}
			line += fmt.Sprintf(", replaced by %s %d", by.Kind, by.ID)
		}
		lines = append(lines, line+".")
	}
	brief, err := st.Brief(ctx, *ch.FeatureID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return "", err
	}
	if !brief.CreatedAt.After(since) {
		brief.Body = ""
	}
	var parts []string
	parts = appendPart(parts, "Changed since your last turn", strings.Join(lines, "\n"))
	parts = appendPart(parts, "New brief", brief.Body)
	return strings.Join(parts, "\n"), nil
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

// teamIntro tells a teammate who is on its team and how public and direct delivery differ. A
// team of one gets none.
func teamIntro(team store.Team, r store.Role) string {
	if len(team.Roles) < 2 {
		return ""
	}
	names := make([]string, len(team.Roles))
	for i, t := range team.Roles {
		names[i] = "@" + t.Name
	}
	s := fmt.Sprintf("Your team: %s. Use post_to_channel and @mentions for work "+
		"the whole team needs: decisions, blockers, questions for the user and outcomes belong in the "+
		"channel. Only a line that starts with @name wakes that teammate; an @name inside a sentence "+
		"wakes nobody. Use send_to_role for detailed coordination with exactly one teammate; "+
		"their ordinary reply returns directly once, and continuing requires another send_to_role. "+
		"When a direct chain completes with information the team or the user needs, publish "+
		"one concise outcome with post_to_channel; raw logs and routine acknowledgements stay "+
		"private. "+
		"When one message mentions several roles, its header ends with \"Asked together:\" naming "+
		"them all: answer your part and do not repeat a teammate's. "+
		"A public message that wakes you starts with "+
		"[channel N, message N] or [channel N, message N, thread N]: your plain reply goes where "+
		"the header says, a top-level one starting your own thread; pass thread as the parent of "+
		"post_to_channel when the header has one. A turn that posts with post_to_channel has "+
		"answered: its plain reply is not posted.\n", strings.Join(names, ", "))
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

func appendPart(parts []string, label, body string) []string {
	if body = strings.TrimSpace(body); body == "" {
		return parts
	}
	return append(parts, "### "+label+"\n"+body+"\n")
}

func plural(n int, noun string) string {
	if n == 1 {
		return noun
	}
	return noun + "s"
}
