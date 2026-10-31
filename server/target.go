package server

import (
	"context"
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bhanuprakaash/reef/agent"
)

// eventTarget is the repository a tool call works in.
type eventTarget struct {
	ProjectID   int64  `json:"projectId"`
	ProjectName string `json:"projectName"`
	WorkspaceID int64  `json:"workspaceId"`
}

// targetedEvent is an agent event with the repository its tool call works in, when that is one.
type targetedEvent struct {
	agent.Event
	Target *eventTarget `json:"target,omitempty"`
}

type targetDir struct {
	dir string
	eventTarget
}

// spaceTargets lists the workspaces of the work item of Space id, with their Project names. A
// Space with no work item has none.
func (s *server) spaceTargets(ctx context.Context, id int64) ([]targetDir, error) {
	ch, err := s.st.Space(ctx, id)
	if err != nil || ch.WorkItemID == nil {
		return nil, err
	}
	f, err := s.st.WorkItem(ctx, *ch.WorkItemID)
	if err != nil {
		return nil, err
	}
	ws, err := s.itemWorkspaces(ctx, f)
	if err != nil {
		return nil, err
	}
	out := make([]targetDir, 0, len(ws))
	for _, w := range ws {
		p, err := s.st.Project(ctx, w.ProjectID)
		if err != nil {
			return nil, err
		}
		out = append(out, targetDir{w.Dir, eventTarget{p.ID, p.Name, w.ID}})
	}
	return out, nil
}

// _pathKeys are the arguments of a tool call that name a file or folder, or a command that runs
// in one.
var _pathKeys = []string{"file_path", "path", "notebook_path", "cwd", "command"}

// targetOf is the one workspace a tool call's input points into: a path or command inside its
// folder, or naming its folder relative to the agent's working folder. A call that points into
// none, or into more than one, has no target.
func targetOf(dirs []targetDir, input json.RawMessage) *eventTarget {
	var args map[string]any
	if json.Unmarshal(input, &args) != nil {
		return nil
	}
	var found *eventTarget
	for _, d := range dirs {
		for _, k := range _pathKeys {
			v, ok := args[k].(string)
			if !ok || !mentions(v, d.dir) {
				continue
			}
			if found != nil && found.WorkspaceID != d.WorkspaceID {
				return nil
			}
			t := d.eventTarget
			found = &t
		}
	}
	return found
}

// mentions reports whether v holds dir, or its folder name as a path of its own.
func mentions(v, dir string) bool {
	if strings.Contains(v, dir) {
		return true
	}
	name := regexp.QuoteMeta(filepath.Base(dir))
	return regexp.MustCompile(`(^|[\s'"=:;&|(]|\./)` + name + `($|[/\s;&|)'"])`).MatchString(v)
}
