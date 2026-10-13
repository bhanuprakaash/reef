package channel

import (
	"regexp"
	"slices"

	"github.com/bhanuprakaash/reef/store"
)

var _mention = regexp.MustCompile(`\B@([\w-]+)`)

// Route returns the role that message m wakes, or "" for nobody. owner is the author of
// the thread m replies in; roles are the team's role names. In order: a reply wakes its
// thread's owner, the lead when the user owns it; the first @mention of a role wakes it;
// anything else from the user wakes the lead. A role never wakes itself.
func Route(m store.Message, owner string, roles []string, lead string) string {
	if m.ParentID != nil {
		if owner == _userAuthor {
			owner = lead
		}
		if owner != m.Author && slices.Contains(roles, owner) {
			return owner
		}
	}
	for _, g := range _mention.FindAllStringSubmatch(m.Body, -1) {
		if g[1] != m.Author && slices.Contains(roles, g[1]) {
			return g[1]
		}
	}
	if m.Author == _userAuthor {
		return lead
	}
	return ""
}
