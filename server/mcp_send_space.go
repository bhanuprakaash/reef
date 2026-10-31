package server

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/bhanuprakaash/reef/store"
)

const (
	_sendQuestion    = "question"
	_sendInstruction = "instruction"
)

type (
	sendToSpaceIn struct {
		SpaceID int64  `json:"spaceId" jsonschema:"Space id, from a #[name](space:id) reference in this conversation"`
		Bot     string `json:"bot" jsonschema:"the Bot in that Space to send to: its id or its name"`
		Body    string `json:"body" jsonschema:"the question, or the instruction the user gave, with the context it needs"`
		Kind    string `json:"kind" jsonschema:"question when you need an answer back; instruction to pass on what the user told you"`
	}
	sendToSpaceOut struct {
		Exchange int64  `json:"exchange"`
		State    string `json:"state"`
	}
)

// registerSendToSpace adds send_to_space to the tools of a direct conversation.
func (m *mcpServer) registerSendToSpace(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "send_to_space",
		Description: "Send a question or an instruction to a Bot that already works in a Space " +
			"this conversation refers to, in its existing session there. A question's answer " +
			"wakes you here: after calling send_to_space, finish your turn. An instruction is " +
			"what the user told you to pass on, and gets no answer wake. A paused Space keeps " +
			"it waiting until the user continues the Space; a closed Space or a Bot that is " +
			"not a member fails it, visibly in this conversation. It gives the Bot no access " +
			"to this conversation and adds nobody to it.",
	}, m.sendToSpace)
}

func (m *mcpServer) sendToSpace(
	ctx context.Context, _ *mcp.CallToolRequest, in sendToSpaceIn,
) (*mcp.CallToolResult, sendToSpaceOut, error) {
	name, err := m.currentName(ctx)
	if err != nil {
		return nil, sendToSpaceOut{}, err
	}
	kind, body := store.ExchangeQuestion, in.Body
	switch in.Kind {
	case _sendQuestion:
	case _sendInstruction:
		kind = store.ExchangeGuidance
		body = fmt.Sprintf("Instruction from the user, passed on by %s from Space %d: %s", name, m.tok.SpaceID, in.Body)
	default:
		return nil, sendToSpaceOut{}, fmt.Errorf("%w: kind is %q or %q", store.ErrInvalid, _sendQuestion, _sendInstruction)
	}
	referenced, err := m.st.SpaceReferenced(ctx, m.tok.SpaceID, in.SpaceID)
	if err != nil {
		return nil, sendToSpaceOut{}, err
	}
	dest, err := m.st.Space(ctx, in.SpaceID)
	if err != nil {
		return nil, sendToSpaceOut{}, err
	}
	if !referenced || dest.Kind == store.KindConversation {
		return nil, sendToSpaceOut{}, fmt.Errorf("space %d: %w", in.SpaceID, store.ErrNotFound)
	}
	to, err := m.askedBot(ctx, in.Bot)
	if err != nil {
		return nil, sendToSpaceOut{}, err
	}
	turn, err := m.runningTurn(ctx, name)
	if err != nil {
		return nil, sendToSpaceOut{}, err
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%d\x00%s\x00%s", dest.ID, to.ID, kind, in.Body)))
	key := fmt.Sprintf("send:%d:%x", turn, sum[:8])
	dm, _, err := m.st.CreateExchange(ctx, store.DirectMessage{
		SpaceID: dest.ID, OriginSpaceID: &m.tok.SpaceID, From: name, To: to.Name,
		FromBotID: m.tok.BotID, ToBotID: &to.ID, Body: body, Kind: kind, RequestKey: &key,
	}, fmt.Sprintf("Sent to %s in %s: %s", to.Name, dest.Name, in.Body))
	if err != nil {
		return nil, sendToSpaceOut{}, err
	}
	return nil, sendToSpaceOut{Exchange: dm.ID, State: dm.State}, nil
}
