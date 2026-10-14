package server

import (
	"net/http"
	"time"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/store"
)

// inspectorRole is one role's turn and token sums across the channel.
type inspectorRole struct {
	Role    string      `json:"role"`
	Turns   int         `json:"turns"`
	Usage   agent.Usage `json:"usage"`
	CostUSD *float64    `json:"costUsd"`
}

// inspectorTurn is one turn with what the inspector shows of it.
type inspectorTurn struct {
	ID        int64               `json:"id"`
	Role      string              `json:"role"`
	StartedAt time.Time           `json:"startedAt"`
	Status    string              `json:"status"`
	Model     string              `json:"model"`
	Usage     agent.Usage         `json:"usage"`
	CostUSD   *float64            `json:"costUsd"`
	Prompt    string              `json:"prompt"`
	ToolCalls []inspectorToolCall `json:"toolCalls"`
}

type inspectorToolCall struct {
	Name   string `json:"name"`
	Input  string `json:"input"`
	Result string `json:"result"`
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
	if _, err := s.st.Channel(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	turns, err := s.st.Turns(r.Context(), store.TurnFilter{ChannelID: &id})
	if err != nil {
		writeError(w, err)
		return
	}

	roles := map[string]*inspectorRole{}
	var order []string
	turnViews := make([]inspectorTurn, 0, len(turns))
	for _, t := range turns {
		role, ok := roles[t.Role]
		if !ok {
			role = &inspectorRole{Role: t.Role}
			roles[t.Role] = role
			order = append(order, t.Role)
		}
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
		tcs := make([]inspectorToolCall, len(calls))
		for i, c := range calls {
			tcs[i] = inspectorToolCall{Name: c.Name, Input: c.Input, Result: c.Result}
		}
		turnViews = append(turnViews, inspectorTurn{
			ID: t.ID, Role: t.Role, StartedAt: t.StartedAt, Status: t.Status,
			Model: t.Model, Usage: t.Usage, CostUSD: t.CostUSD, Prompt: t.Prompt, ToolCalls: tcs,
		})
	}
	roleViews := make([]inspectorRole, len(order))
	for i, role := range order {
		roleViews[i] = *roles[role]
	}

	writeJSON(w, http.StatusOK, inspectorResponse{Roles: roleViews, Turns: turnViews})
}
