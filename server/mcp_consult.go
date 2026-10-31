package server

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/bhanuprakaash/reef/store"
)

type (
	consultIn struct {
		Bot          string `json:"bot" jsonschema:"the Bot to ask: its id or its name"`
		Body         string `json:"body" jsonschema:"the focused question with the context it needs"`
		Consultation int64  `json:"consultation,omitempty" jsonschema:"id of an earlier consultation with this Bot, for a related follow-up; omit for an unrelated question"`
	}
	consultOut struct {
		Consultation int64  `json:"consultation"`
		Exchange     int64  `json:"exchange"`
		State        string `json:"state"`
	}
)

// registerConsult adds consult to the tools of a direct conversation: any other Space, and so a
// consultation, has none.
func (m *mcpServer) registerConsult(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "consult",
		Description: "Ask another Bot a focused question without leaving this conversation. It " +
			"works in its own visible consultation, and its reply wakes you here: after calling " +
			"consult, finish your turn. For a related follow-up pass the same consultation id; " +
			"an unrelated question starts a new consultation. It gives the Bot no access to this " +
			"conversation and adds nobody to it.",
	}, m.consult)
}

func (m *mcpServer) consult(
	ctx context.Context, _ *mcp.CallToolRequest, in consultIn,
) (*mcp.CallToolResult, consultOut, error) {
	name, err := m.currentName(ctx)
	if err != nil {
		return nil, consultOut{}, err
	}
	asked, err := m.askedBot(ctx, in.Bot)
	if err != nil {
		return nil, consultOut{}, err
	}
	if asked.ID == *m.tok.BotID {
		return nil, consultOut{}, fmt.Errorf("%w: cannot consult yourself", store.ErrInvalid)
	}
	running, err := m.st.Turns(ctx, store.TurnFilter{SpaceID: &m.tok.SpaceID, Status: "running"})
	if err != nil {
		return nil, consultOut{}, err
	}
	var turn int64
	for _, t := range running {
		if t.BotID != nil && *t.BotID == *m.tok.BotID {
			turn = t.ID
		}
	}
	if turn == 0 {
		return nil, consultOut{}, fmt.Errorf("%w: %s has no running turn to consult from", store.ErrInvalid, name)
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%d\x00%s", asked.ID, in.Consultation, in.Body)))
	key := fmt.Sprintf("consult:%d:%x", turn, sum[:8])
	dm, _, err := m.st.Consult(ctx, store.DirectMessage{
		SpaceID: in.Consultation, OriginSpaceID: &m.tok.SpaceID, From: name, To: asked.Name,
		FromBotID: m.tok.BotID, ToBotID: &asked.ID, Body: in.Body, Kind: store.ExchangeRequest,
		RequestKey: &key,
	}, "Consultation from "+name, "Asked "+asked.Name+": "+in.Body)
	if err != nil {
		return nil, consultOut{}, err
	}
	return nil, consultOut{Consultation: dm.SpaceID, Exchange: dm.ID, State: dm.State}, nil
}

// askedBot finds the Bot with the id or the name ref.
func (m *mcpServer) askedBot(ctx context.Context, ref string) (store.Bot, error) {
	if id, err := strconv.ParseInt(ref, 10, 64); err == nil {
		return m.st.Bot(ctx, id)
	}
	bots, err := m.st.Bots(ctx)
	if err != nil {
		return store.Bot{}, err
	}
	for _, b := range bots {
		if b.Name == ref {
			return b, nil
		}
	}
	return store.Bot{}, fmt.Errorf("%w: unknown bot %q", store.ErrInvalid, ref)
}
