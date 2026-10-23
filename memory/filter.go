package memory

import "strings"

// Filter limits which items a search may return. Only accepted items match,
// plus retired ones when IncludeRetired is set; candidates never match. An
// item must be scoped to exactly one of Scopes. No Scopes matches nothing.
type Filter struct {
	Scopes         []Ref
	IncludeRetired bool
}

// Hit is one search result.
type Hit struct {
	ID     int64
	Score  float64
	Status string
}

// where returns an SQL condition on the memory table for f and its arguments.
// f must have at least one scope.
func (f Filter) where() (string, []any) {
	args := []any{StatusAccepted}
	cond := "status IN (?"
	if f.IncludeRetired {
		cond += ", ?"
		args = append(args, StatusRetired)
	}
	cond += ") AND ("
	scopes := make([]string, len(f.Scopes))
	for i, r := range f.Scopes {
		scopes[i] = "(scope_kind = ? AND scope_id = ?)"
		args = append(args, r.Kind, r.ID)
	}
	return cond + strings.Join(scopes, " OR ") + ")", args
}
