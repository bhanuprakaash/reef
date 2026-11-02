package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/space"
	"github.com/bhanuprakaash/reef/store"
)

// inspectorRole is one role's turn and token sums across the Space.
type inspectorRole struct {
	Role    string      `json:"role"`
	BotID   *int64      `json:"botId"`
	Turns   int         `json:"turns"`
	Usage   agent.Usage `json:"usage"`
	CostUSD *float64    `json:"costUsd"`
}

// inspectorTurn is one turn with what the inspector shows of it.
type inspectorTurn struct {
	ID           int64                `json:"id"`
	Role         string               `json:"role"`
	BotID        *int64               `json:"botId"`
	StartedAt    time.Time            `json:"startedAt"`
	Status       string               `json:"status"`
	Model        string               `json:"model"`
	Usage        agent.Usage          `json:"usage"`
	CostUSD      *float64             `json:"costUsd"`
	Instructions string               `json:"instructions"`
	Prompt       string               `json:"prompt"`
	Reply        string               `json:"reply"`
	ToolCalls    []*inspectorToolCall `json:"toolCalls"`
}

// inspectorToolCall is one tool call; a subagent's calls are the Children of its Agent call.
type inspectorToolCall struct {
	Name     string               `json:"name"`
	Input    string               `json:"input"`
	Result   string               `json:"result"`
	Targets  []eventTarget        `json:"targets,omitempty"`
	Children []*inspectorToolCall `json:"children,omitempty"`
}

type inspectorResponse struct {
	Roles []inspectorRole `json:"roles"`
	Turns []inspectorTurn `json:"turns"`
}

func (s *server) getInspector(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if _, err := s.st.Space(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	turns, err := s.st.Turns(r.Context(), store.TurnFilter{SpaceID: &id})
	if err != nil {
		writeError(w, err)
		return
	}
	dirs, err := s.spaceTargets(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}

	roles := map[string]*inspectorRole{}
	var order []string
	turnViews := make([]inspectorTurn, 0, len(turns))
	instructions := map[int64]string{}
	for _, t := range turns {
		key := "name:" + t.Role
		if t.BotID != nil {
			key = "bot:" + strconv.FormatInt(*t.BotID, 10)
		}
		role, ok := roles[key]
		if !ok {
			role = &inspectorRole{Role: t.Role, BotID: t.BotID}
			roles[key] = role
			order = append(order, key)
		}
		role.Role = t.Role
		role.Turns++
		role.Usage.Add(t.Usage)
		if t.CostUSD != nil {
			if role.CostUSD == nil {
				role.CostUSD = new(float64)
			}
			*role.CostUSD += *t.CostUSD
		}

		calls, err := s.st.ToolCalls(r.Context(), t.ID)
		if err != nil {
			writeError(w, err)
			return
		}
		byID := make(map[int64]*inspectorToolCall, len(calls))
		tcs := make([]*inspectorToolCall, 0, len(calls))
		for _, c := range calls {
			tc := &inspectorToolCall{Name: c.Name, Input: c.Input, Result: c.Result, Targets: targetOf(dirs, json.RawMessage(c.Input))}
			byID[c.ID] = tc
			if p, ok := byID[c.ParentID]; ok {
				p.Children = append(p.Children, tc)
			} else {
				tcs = append(tcs, tc)
			}
		}
		var sys string
		if t.BotSessionID != nil {
			var ok bool
			if sys, ok = instructions[*t.BotSessionID]; !ok {
				bs, err := s.st.BotSession(r.Context(), *t.BotSessionID)
				if err != nil {
					writeError(w, err)
					return
				}
				sys = space.Instructions(bs.Apply(store.Role{}), bs.Skills)
				instructions[*t.BotSessionID] = sys
			}
		}
		turnViews = append(turnViews, inspectorTurn{
			ID: t.ID, Role: t.Role, BotID: t.BotID, StartedAt: t.StartedAt, Status: t.Status,
			Model: t.Model, Usage: t.Usage, CostUSD: t.CostUSD, Instructions: sys, Prompt: t.Prompt,
			Reply: t.Reply, ToolCalls: tcs,
		})
	}
	roleViews := make([]inspectorRole, len(order))
	for i, role := range order {
		roleViews[i] = *roles[role]
	}

	writeJSON(w, http.StatusOK, inspectorResponse{Roles: roleViews, Turns: turnViews})
}
