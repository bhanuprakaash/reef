// Package space builds what a bot is told at the start of a Space.
package space

import (
	"context"
	"errors"
	"fmt"
	"slices"
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
	_spaceLimit    = 5
	_teamLimit     = 5
	_yourLimit     = 5
	_toolsLine     = "Details are on demand through the reef MCP tools recall, read_memory, " +
		"read_thread and search_sessions. Record decisions with remember. When the " +
		"user and the team agree on the goal, scope or plan, call propose_brief."
	_quickToolsLine = "Details are on demand through the reef MCP tools recall and read_memory. " +
		"remember saves to this %s by default; use scope %s."
	_quickProjectScopes = "project or bot for what holds across the project or for you"
	_quickBotScope      = "bot for what holds for you"
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
	memory.ScopeSpace:   {{label: "Space", limit: _spaceLimit}},
	memory.ScopeProject: {{label: "Project", limit: _projectLimit}},
	memory.ScopeTeam:    {{label: "Team", limit: _teamLimit}},
	memory.ScopeBot:     {{label: "Your items", limit: _yourLimit}},
}

// Context is the first prompt of a role on a work item without a team: what `reef context`
// prints. It never holds a message body.
func Context(
	ctx context.Context, st *store.Store, ms *memory.Store, workItemID int64, role string,
) (string, error) {
	f, err := st.WorkItem(ctx, workItemID)
	if err != nil {
		return "", err
	}
	return firstPrompt(ctx, st, ms, f.ProjectID, &f.ID, 0, store.Team{}, store.Role{Name: role})
}

// Scopes are the shelves of memory a role reads and writes in ch. The first is the Space's own:
// the work item's, the Space's for a free Space, or the conversation's. Then the project's, the
// team's unless it is the solo team, and the Bot's own, named by its id. A role the Space
// does not have, such as an agent attached by hand with reef token, has no Bot: its shelf
// is named by the role.
func Scopes(
	ctx context.Context, st *store.Store, ch store.Space, role string,
) ([]memory.Ref, error) {
	bot := role
	switch id, err := botID(ctx, st, ch, role); {
	case err == nil:
		bot = strconv.FormatInt(id, 10)
	case !errors.Is(err, store.ErrNotFound):
		return nil, err
	}
	return botScopes(ch, bot), nil
}

func BotScopes(ch store.Space, botID int64) []memory.Ref {
	return botScopes(ch, strconv.FormatInt(botID, 10))
}

func botScopes(ch store.Space, bot string) []memory.Ref {
	var scopes []memory.Ref
	switch {
	case ch.WorkItemID != nil:
		scopes = append(scopes,
			memory.Ref{Kind: memory.ScopeWorkItem, ID: strconv.FormatInt(*ch.WorkItemID, 10)})
	case ch.Kind == store.KindConversation:
		scopes = append(scopes,
			memory.Ref{Kind: memory.ScopeConversation, ID: strconv.FormatInt(ch.ID, 10)})
	default:
		scopes = append(scopes, memory.Ref{Kind: memory.ScopeSpace, ID: strconv.FormatInt(ch.ID, 10)})
	}
	if ch.ProjectID != 0 {
		scopes = append(scopes,
			memory.Ref{Kind: memory.ScopeProject, ID: strconv.FormatInt(ch.ProjectID, 10)})
	}
	if ch.Team != "" && ch.Team != store.SoloTeam {
		scopes = append(scopes, memory.Ref{Kind: memory.ScopeTeam, ID: ch.Team})
	}
	return append(scopes, memory.Ref{Kind: memory.ScopeBot, ID: bot})
}

// botID is the id of the Bot that holds role in ch. With no Space (ch.ID is 0, as for
// reef context) it is the one that holds role in the Team of the work item, or else the
// Bot named role.
func botID(ctx context.Context, st *store.Store, ch store.Space, role string) (int64, error) {
	if ch.ID != 0 {
		return st.SpaceMemberBotID(ctx, ch.ID, role)
	}
	if ch.WorkItemID != nil {
		w, err := st.WorkItem(ctx, *ch.WorkItemID)
		if err != nil {
			return 0, err
		}
		if w.TeamID != nil {
			t, err := st.TeamByID(ctx, *w.TeamID)
			if err != nil {
				return 0, err
			}
			for _, r := range t.Roles {
				if r.Name == role {
					return r.BotID, nil
				}
			}
		}
	}
	all, err := st.Bots(ctx)
	if err != nil {
		return 0, err
	}
	for _, t := range all {
		if t.Name == role {
			return t.ID, nil
		}
	}
	return 0, fmt.Errorf("bot %s: %w", role, store.ErrNotFound)
}

// ReadScopes are Scopes and the conversation shelves of every direct conversation the role's
// Bot has had. They are never loaded at wake: only recall and read_memory reach them.
func ReadScopes(
	ctx context.Context, st *store.Store, ch store.Space, role string,
) ([]memory.Ref, error) {
	scopes, err := Scopes(ctx, st, ch, role)
	if err != nil {
		return nil, err
	}
	id, err := st.SpaceMemberBotID(ctx, ch.ID, role)
	if errors.Is(err, store.ErrNotFound) {
		return scopes, nil
	}
	if err != nil {
		return nil, err
	}
	return readBotConversations(ctx, st, scopes, id)
}

func ReadBotScopes(ctx context.Context, st *store.Store, ch store.Space, id int64) ([]memory.Ref, error) {
	return readBotConversations(ctx, st, BotScopes(ch, id), id)
}

func readBotConversations(ctx context.Context, st *store.Store, scopes []memory.Ref, id int64) ([]memory.Ref, error) {
	convs, err := st.BotConversations(ctx, id)
	if err != nil {
		return nil, err
	}
	for _, c := range convs {
		r := memory.Ref{Kind: memory.ScopeConversation, ID: strconv.FormatInt(c.ID, 10)}
		if !slices.Contains(scopes, r) {
			scopes = append(scopes, r)
		}
	}
	return scopes, nil
}

// wakeScopes are the shelves of scopes loaded at wake. A conversation shelf never is, and a
// direct conversation loads only its Bot's.
func wakeScopes(ch store.Space, scopes []memory.Ref) []memory.Ref {
	return slices.DeleteFunc(slices.Clone(scopes), func(r memory.Ref) bool {
		return r.Kind == memory.ScopeConversation ||
			ch.Kind == store.KindConversation && r.Kind != memory.ScopeBot
	})
}

// firstPrompt is the first prompt of a role in three sections: You, How to work and What we
// know. A part with nothing to say is left out, and so is a section with no part left. A quick
// session has no workItemID. spaceID is the Space the prompt is for, or 0 for none; it is not
// counted as an earlier Space.
func firstPrompt(
	ctx context.Context, st *store.Store, ms *memory.Store, projectID int64, workItemID *int64,
	spaceID int64, team store.Team, r store.Role,
) (string, error) {
	who, err := profileLine(ctx, st)
	if err != nil {
		return "", err
	}

	ch := store.Space{ID: spaceID, ProjectID: projectID, WorkItemID: workItemID, Team: team.Name}
	if spaceID != 0 {
		sp, err := st.Space(ctx, spaceID)
		if err != nil {
			return "", err
		}
		ch.Kind = sp.Kind
	}
	var b strings.Builder
	b.WriteString(_youHeader + roleIntro(r))
	var know []string
	how := teamIntro(team, r)
	if workItemID == nil {
		kind, scope := "a free Space", "Space"
		if ch.Kind == store.KindConversation {
			kind, scope = "a direct conversation with the user", "conversation"
		}
		fmt.Fprintf(&b, "You are %s.\n"+
			"This is %s: it has no work item, so there is no brief or work item items.\n",
			r.Name, kind)
		scopes := _quickBotScope
		if projectID != 0 {
			scopes = _quickProjectScopes
		}
		how += fmt.Sprintf(_quickToolsLine, scope, scopes) + "\n"
		if spaceID != 0 {
			projects, err := st.SpaceProjects(ctx, spaceID)
			if err != nil {
				return "", err
			}
			if len(projects) > 0 {
				names := make([]string, len(projects))
				for i, p := range projects {
					names[i] = fmt.Sprintf("%s (%d)", p.Name, p.ID)
				}
				fmt.Fprintf(&b, "Associated projects, for context only; you do not work in them: %s.\n",
					strings.Join(names, ", "))
			}
		}
		b.WriteString(who)
		if know, err = itemParts(ctx, st, ms, nil, ch, r); err != nil {
			return "", err
		}
	} else {
		f, err := st.WorkItem(ctx, *workItemID)
		if err != nil {
			return "", err
		}
		p, err := st.Project(ctx, projectID)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "You are %s on work item %q (%d) in project %q.\n",
			r.Name, f.Title, f.ID, p.Name)
		how += _toolsLine + "\n"
		b.WriteString(who)
		if f.Description != "" {
			fmt.Fprintf(&b, "Description: %s\n", f.Description)
		}
		if len(f.AcceptanceLines) > 0 {
			b.WriteString("Acceptance lines:\n")
			for _, l := range f.AcceptanceLines {
				tick := " "
				if l.Checked {
					tick = "x"
				}
				fmt.Fprintf(&b, "%d. [%s] %s\n", l.Position, tick, l.Text)
			}
		}
		if p.BaseBranch != "" {
			fmt.Fprintf(&b, "Base branch: %s. A pull request you open targets it.\n", p.BaseBranch)
		}
		if know, err = workItemKnow(ctx, st, ms, f, ch, r); err != nil {
			return "", err
		}
	}

	if how != "" {
		b.WriteString("\n## How to work\n" + how)
	}
	if len(know) > 0 {
		b.WriteString("\n## What we know\n" + strings.Join(know, "\n"))
	}
	return b.String(), nil
}

// workItemKnow is the parts of What we know for a role on f. Spaces with messages, other
// than ch, are only counted: the role looks them up with tools.
func workItemKnow(
	ctx context.Context, st *store.Store, ms *memory.Store, f store.WorkItem, ch store.Space,
	role store.Role,
) ([]string, error) {
	brief, err := st.Brief(ctx, f.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	spaces, err := st.Spaces(ctx, store.SpaceFilter{WorkItemID: &f.ID})
	if err != nil {
		return nil, err
	}

	subs, err := st.WorkItems(ctx, store.WorkItemFilter{ParentID: &f.ID})
	if err != nil {
		return nil, err
	}
	var counts, doing []string
	open := 0
	for _, w := range subs {
		switch w.Status {
		case "done", "dropped":
			continue
		case "in_progress":
			doing = append(doing, fmt.Sprintf("sub-item %d: %s (%s)", w.ID, firstLine(w.Title), w.Type))
		}
		open++
	}
	if open > 0 {
		counts = append(counts, fmt.Sprintf("%d open %s (list_sub_items).", open, plural(open, "sub-item")))
	}
	earlier := 0
	for _, c := range spaces {
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
		counts = append(counts, fmt.Sprintf("%d earlier %s (search_sessions).", earlier, plural(earlier, "Space")))
	}

	parts := appendPart(nil, "Brief", brief.Body)
	parts = appendPart(parts, "In progress", strings.Join(doing, "\n"))
	if parts, err = itemParts(ctx, st, ms, parts, ch, role); err != nil {
		return nil, err
	}
	return appendPart(parts, "On demand", strings.Join(counts, " ")), nil
}

// itemParts adds a part for each shelf of Scopes: its newest accepted items, and for a part
// with a limit, how many of how many it shows.
func itemParts(
	ctx context.Context, st *store.Store, ms *memory.Store, parts []string, ch store.Space,
	role store.Role,
) ([]string, error) {
	var scopes []memory.Ref
	var err error
	if role.BotID != 0 {
		scopes = BotScopes(ch, role.BotID)
	} else {
		scopes, err = Scopes(ctx, st, ch, role.Name)
	}
	if err != nil {
		return nil, err
	}
	for _, scope := range wakeScopes(ch, scopes) {
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

// changedSince is what changed in the role's Scopes after since: items accepted, items retired
// and a newer brief in full. An item added and retired since is named as retired only, as the
// role may have seen it in its previous turn.
func changedSince(
	ctx context.Context, st *store.Store, ms *memory.Store, ch store.Space, botID int64,
	since time.Time,
) (string, error) {
	scopes := BotScopes(ch, botID)
	added, retired, err := ms.Changed(ctx, wakeScopes(ch, scopes), since)
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
	var brief store.Brief
	if ch.WorkItemID != nil {
		brief, err = st.Brief(ctx, *ch.WorkItemID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return "", err
		}
		if !brief.CreatedAt.After(since) {
			brief.Body = ""
		}
	}
	var parts []string
	parts = appendPart(parts, "Changed since your last turn", strings.Join(lines, "\n"))
	parts = appendPart(parts, "New brief", brief.Body)
	return strings.Join(parts, "\n"), nil
}

// roleIntro is the role's description and skills, when it has any, to put before its context.
func roleIntro(r store.Role) string {
	var b strings.Builder
	if r.Description != "" {
		fmt.Fprintf(&b, "%s\n\n", strings.TrimSpace(r.Description))
	}
	if len(r.Skills) > 0 {
		fmt.Fprintf(&b, "Your skills:\n%s\n\n", strings.Join(r.Skills, ", "))
	}
	return b.String()
}

// _introChangedPrefix marks roleIntro text as a change to a role whose session is already
// running, so the agent knows this replaces what it was told before.
const _introChangedPrefix = "Your description or skills changed:\n\n"

// updatedRoleIntro is roleIntro(r), marked as a change to a role whose session is already
// running, so the agent knows this replaces what it was told before.
func updatedRoleIntro(r store.Role) string {
	return _introChangedPrefix + roleIntro(r)
}

// teamIntro tells a bot who is on its team and how public and direct delivery differ. A
// team of one gets none.
func teamIntro(team store.Team, r store.Role) string {
	if len(team.Roles) < 2 {
		return ""
	}
	names := make([]string, len(team.Roles))
	for i, t := range team.Roles {
		names[i] = "@" + t.Name
	}
	s := fmt.Sprintf("Your team: %s. Use post_to_space and @mentions for work "+
		"the whole team needs: decisions, blockers, questions for the user and outcomes belong in the "+
		"Space. Only a line that starts with @name wakes that bot; an @name inside a sentence "+
		"wakes nobody. Use send_to_role for detailed coordination with exactly one bot; "+
		"their ordinary reply returns directly once, and continuing requires another send_to_role. "+
		"When a direct chain completes with information the team or the user needs, publish "+
		"one concise outcome with post_to_space; raw logs and routine acknowledgements stay "+
		"private. "+
		"When one message mentions several roles, its header ends with \"Asked together:\" naming "+
		"them all: answer your part and do not repeat a bot's. "+
		"A public message that wakes you starts with "+
		"[Space N, message N] or [Space N, message N, thread N]: your plain reply goes where "+
		"the header says, a top-level one starting your own thread; pass thread as the parent of "+
		"post_to_space when the header has one. A turn that posts with post_to_space has "+
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
