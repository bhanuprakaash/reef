package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Consult asks the Bot m.ToBotID, from the direct conversation m.OriginSpaceID, in a
// consultation Space: m.SpaceID continues that one, 0 makes a new one titled title. The
// exchange is created like CreateExchange creates one, and a key already stored returns the
// first row and makes no Space.
func (s *Store) Consult(
	ctx context.Context, m DirectMessage, title, originNote string,
) (DirectMessage, bool, error) {
	if m.OriginSpaceID == nil || m.ToBotID == nil || m.From == "" || m.To == "" || strings.TrimSpace(m.Body) == "" {
		return DirectMessage{}, false, fmt.Errorf("%w: consultation origin, sender, recipient and body are required", ErrInvalid)
	}
	origin, err := s.Space(ctx, *m.OriginSpaceID)
	if err != nil {
		return DirectMessage{}, false, err
	}
	if origin.Kind != KindConversation {
		return DirectMessage{}, false, fmt.Errorf("%w: only a direct conversation consults a Bot", ErrInvalid)
	}
	if err := origin.Writable(); err != nil {
		return DirectMessage{}, false, err
	}
	if m.RequestKey != nil {
		first, err := s.exchangeByKey(ctx, *m.RequestKey)
		if err == nil {
			return first, false, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return DirectMessage{}, false, err
		}
	}
	if m.SpaceID != 0 {
		if err := s.checkConsultation(ctx, m); err != nil {
			return DirectMessage{}, false, err
		}
	} else {
		c, err := s.CreateBotSpace(ctx, Space{Name: title, Role: m.To, Kind: KindConsultation}, *m.ToBotID)
		if err != nil {
			return DirectMessage{}, false, err
		}
		m.SpaceID = c.ID
	}
	return s.CreateExchange(ctx, m, originNote)
}

// checkConsultation refuses to continue a Space that is not a consultation of the Bot asked, for
// the conversation that is asking.
func (s *Store) checkConsultation(ctx context.Context, m DirectMessage) error {
	c, err := s.Space(ctx, m.SpaceID)
	if err != nil {
		return err
	}
	var asked bool
	if err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM direct_message WHERE space_id = ? AND origin_space_id = ?
		AND reply_to_id IS NULL)`, m.SpaceID, *m.OriginSpaceID).Scan(&asked); err != nil {
		return fmt.Errorf("check consultation %d: %w", m.SpaceID, err)
	}
	if c.Kind != KindConsultation || c.BotID == nil || *c.BotID != *m.ToBotID || !asked {
		return fmt.Errorf("%w: Space %d is not a consultation of %s from Space %d",
			ErrInvalid, m.SpaceID, m.To, *m.OriginSpaceID)
	}
	return nil
}
