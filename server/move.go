package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/bhanuprakaash/reef/space"
	"github.com/bhanuprakaash/reef/store"
)

// movePrompt asks the lead of a conversation for the summary a new Teammate starts from. text
// is what the user wrote when they added the Teammate.
func movePrompt(to, text string) string {
	return fmt.Sprintf("The user is adding @%s to this conversation. It becomes a Space with you "+
		"and @%s, who has not seen it. Write the summary @%s starts from, in your own words and "+
		"quoting no message: what the user wants, what you decided or found, what is still open. "+
		"Reply with the summary only, and use no tools.\n\nThe user wrote: %s", to, to, to, text)
}

// moveTarget returns the Teammate a user message adds to the direct conversation id: the first
// @name in a run of them at the start of a line that is a Teammate other than the
// conversation's own. It is false for a message in any other Space, or that adds nobody.
func (s *server) moveTarget(ctx context.Context, id int64, text string) (store.Teammate, bool, error) {
	names := space.Handoffs(text)
	if len(names) == 0 {
		return store.Teammate{}, false, nil
	}
	ch, err := s.st.Space(ctx, id)
	if err != nil || ch.Kind != store.KindConversation {
		return store.Teammate{}, false, err
	}
	all, err := s.st.Teammates(ctx)
	if err != nil {
		return store.Teammate{}, false, err
	}
	for _, name := range names {
		for _, t := range all {
			if t.Name == name && name != ch.Role {
				return t, true, nil
			}
		}
	}
	return store.Teammate{}, false, nil
}

// finishMove ends the turn of a move: the lead's newest message since the user's is the
// summary. The conversation becomes a Space with the Teammate, whose first message is the
// summary from the lead, and the Teammate is woken by it. The conversation's own messages
// never reach the new Space.
func (s *server) finishMove(ctx context.Context, id int64, w wake) error {
	msgs, err := s.st.Messages(ctx, id, w.messageID)
	if err != nil {
		return err
	}
	var summary string
	for _, m := range msgs {
		if m.Author == w.role {
			summary = m.Body
		}
	}
	if summary == "" {
		return fmt.Errorf("%s wrote no summary", w.role)
	}
	to, err := s.st.Teammate(ctx, w.moveTo)
	if err != nil {
		return err
	}
	moved, err := s.st.MoveConversation(ctx, id, to.ID)
	if err != nil {
		return err
	}
	msg, err := s.st.CreateMessage(ctx, store.Message{
		SpaceID: moved.ID, Author: w.role, Body: "@" + to.Name + " " + summary,
	})
	if err != nil {
		return err
	}
	queue, err := s.route(ctx, moved.ID, []store.Message{msg}, w.role)
	if err != nil {
		return err
	}
	c, ch, err := s.claim(ctx, moved.ID, _message)
	if err != nil {
		return err
	}
	if _, err := s.checkSpace(ctx, c.l, _message, ch); err != nil {
		return c.release(err)
	}
	sess, err := c.session()
	if err != nil {
		return c.release(err)
	}
	return c.start(ctx, sess, _message, queue, msg.ID)
}

// moveConversation starts the move of the conversation ch to a Space with the Teammate
// teammateID, as if the user had written "@name".
func (s *server) moveConversation(ctx context.Context, ch store.Space, teammateID int64) error {
	to, err := s.st.Teammate(ctx, teammateID)
	if errors.Is(err, store.ErrNotFound) {
		err = fmt.Errorf("%w: unknown teammate %d", store.ErrInvalid, teammateID)
	}
	if err != nil {
		return err
	}
	if to.Name == ch.Role {
		return fmt.Errorf("%w: %s already leads conversation %d", store.ErrInvalid, to.Name, ch.ID)
	}
	return s.startUserTurn(ctx, ch.ID, userTurn{kind: _move, text: "@" + to.Name, to: to})
}
