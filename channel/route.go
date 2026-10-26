package channel

import (
	"regexp"
	"slices"

	"github.com/bhanuprakaash/reef/store"
)

var (
	_handoff = regexp.MustCompile(`(?m)^[ \t]*((?:@[\w-]+[ \t,]+)*@[\w-]+)`)
	_mention = regexp.MustCompile(`@([\w-]+)`)
)

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
	for _, run := range _handoff.FindAllString(m.Body, -1) {
		for _, g := range _mention.FindAllStringSubmatch(run, -1) {
			if g[1] != m.Author && slices.Contains(roles, g[1]) && !slices.Contains(woken, g[1]) {
				woken = append(woken, g[1])
			}
		}
	}
	if len(woken) == 0 && m.Author == _userAuthor {
		return []string{lead}
	}
	return woken
}
