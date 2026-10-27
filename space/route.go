package space

import (
	"regexp"
	"slices"

	"github.com/bhanuprakaash/reef/store"
)

var (
	_handoff = regexp.MustCompile(`(?m)^[ \t]*((?:@[\w-]+[ \t,]+)*@[\w-]+)`)
	_mention = regexp.MustCompile(`@([\w-]+)`)
)

// Handoffs returns the names in the runs of @names at the start of body or of a line, in order.
// An @name inside a sentence is not one.
func Handoffs(body string) []string {
	var names []string
	for _, run := range _handoff.FindAllString(body, -1) {
		for _, g := range _mention.FindAllStringSubmatch(run, -1) {
			names = append(names, g[1])
		}
	}
	return names
}

// Route returns the roles that message m wakes, none for nobody. owner is the author of
// the thread m replies in; roles are the team's role names. In order: a user reply wakes
// its thread's owner, the lead when the user owns it; every role in a run of @names at the
// start of the body or of a line wakes; anything else from the user wakes the lead. A role
// never wakes itself.
func Route(m store.Message, owner string, roles []string, lead string) []string {
	if m.ParentID != nil && m.Author == _userAuthor {
		if owner == _userAuthor {
			owner = lead
		}
		if slices.Contains(roles, owner) {
			return []string{owner}
		}
	}
	var woken []string
	for _, name := range Handoffs(m.Body) {
		if name != m.Author && slices.Contains(roles, name) && !slices.Contains(woken, name) {
			woken = append(woken, name)
		}
	}
	if len(woken) == 0 && m.Author == _userAuthor {
		return []string{lead}
	}
	return woken
}
