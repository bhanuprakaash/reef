package server

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/bhanuprakaash/reef/memory"
	"github.com/bhanuprakaash/reef/space"
	"github.com/bhanuprakaash/reef/store"
)

const _snapshotMessages = 5

type (
	readSpaceIn struct {
		SpaceID int64  `json:"spaceId" jsonschema:"Space id, from a #[name](space:id) reference"`
		Query   string `json:"query,omitempty" jsonschema:"only messages and memory items that contain this text"`
		After   int64  `json:"after,omitempty" jsonschema:"only messages with a larger id"`
	}
	turnSnapshot struct {
		ID        int64      `json:"id"`
		Role      string     `json:"role"`
		Status    string     `json:"status"`
		StartedAt time.Time  `json:"startedAt"`
		EndedAt   *time.Time `json:"endedAt"`
	}
	spaceSnapshot struct {
		AsOf         time.Time       `json:"asOf"`
		Running      bool            `json:"running"`
		LastMessages []store.Message `json:"lastMessages"`
		LastTurn     *turnSnapshot   `json:"lastTurn"`
	}
	readSpaceOut struct {
		SpaceID     int64           `json:"spaceId"`
		Unavailable string          `json:"unavailable,omitempty"`
		Name        string          `json:"name,omitempty"`
		Kind        string          `json:"kind,omitempty"`
		Closed      bool            `json:"closed,omitempty"`
		Messages    []store.Message `json:"messages"`
		Memory      []hitOut        `json:"memory"`
		Snapshot    *spaceSnapshot  `json:"snapshot,omitempty"`
	}
)

// readSpace reads the recorded history of a Space the caller's Space refers to, or one of the
// caller's own work item. It stores nothing and starts nothing there.
func (m *mcpServer) readSpace(
	ctx context.Context, _ *mcp.CallToolRequest, in readSpaceIn,
) (*mcp.CallToolResult, readSpaceOut, error) {
	if _, err := m.currentName(ctx); err != nil {
		return nil, readSpaceOut{}, err
	}
	referenced, err := m.st.SpaceReferenced(ctx, m.tok.SpaceID, in.SpaceID)
	if err != nil {
		return nil, readSpaceOut{}, err
	}
	c, err := m.st.Space(ctx, in.SpaceID)
	if err != nil {
		if !referenced || !errors.Is(err, store.ErrNotFound) {
			return nil, readSpaceOut{}, err
		}
		return nil, readSpaceOut{
			SpaceID:     in.SpaceID,
			Unavailable: "this Space was deleted; its messages and memory are gone",
		}, nil
	}
	if c.Kind == store.KindConversation {
		return nil, readSpaceOut{}, fmt.Errorf("space %d: %w", in.SpaceID, store.ErrNotFound)
	}
	if !referenced && c.ID != m.tok.SpaceID {
		own, err := m.st.Space(ctx, m.tok.SpaceID)
		if err != nil {
			return nil, readSpaceOut{}, err
		}
		if c.WorkItemID == nil || own.WorkItemID == nil || *c.WorkItemID != *own.WorkItemID {
			return nil, readSpaceOut{}, fmt.Errorf("space %d: %w", in.SpaceID, store.ErrNotFound)
		}
	}
	all, err := m.st.Messages(ctx, c.ID, 0)
	if err != nil {
		return nil, readSpaceOut{}, err
	}
	out := readSpaceOut{
		SpaceID: c.ID, Name: c.Name, Kind: c.Kind, Closed: c.ClosedAt != nil,
		Snapshot: &spaceSnapshot{AsOf: time.Now().UTC(), LastMessages: all[max(0, len(all)-_snapshotMessages):]},
	}
	needle := strings.ToLower(in.Query)
	for _, msg := range all {
		if msg.ID > in.After && strings.Contains(strings.ToLower(msg.Body), needle) {
			out.Messages = append(out.Messages, msg)
		}
	}
	if in.Query != "" {
		scope := space.BotScopes(c, *m.tok.BotID)[0]
		hits, err := m.ms.Recall(ctx, memory.Query{Scopes: []memory.Ref{scope}, Text: in.Query})
		if err != nil {
			return nil, readSpaceOut{}, err
		}
		for _, h := range hits {
			out.Memory = append(out.Memory, hitOut{
				ID: h.ID, Kind: h.Kind, Body: h.Body, Scope: refString(h.Scope), Status: h.Status, Via: h.Via,
			})
		}
	}
	turns, err := m.st.Turns(ctx, store.TurnFilter{SpaceID: &c.ID})
	if err != nil {
		return nil, readSpaceOut{}, err
	}
	out.Snapshot.Running = c.ActiveTurn != ""
	if len(turns) > 0 {
		t := turns[len(turns)-1]
		out.Snapshot.Running = out.Snapshot.Running || t.Status == "running"
		out.Snapshot.LastTurn = &turnSnapshot{
			ID: t.ID, Role: t.Role, Status: t.Status, StartedAt: t.StartedAt, EndedAt: t.EndedAt,
		}
	}
	return nil, out, nil
}
