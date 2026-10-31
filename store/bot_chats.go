package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Kinds of entry in a Bot's chats list.
const (
	ChatPersonal     = "chat"
	ChatSpace        = "space"
	ChatConsultation = "consultation"
)

// ChatOrigin is the chat a consultation was asked from: its Space and a label naming the
// requesting Bot and the chat.
type ChatOrigin struct {
	SpaceID int64  `json:"spaceId"`
	Label   string `json:"label"`
}

// BotChat is one entry of a Bot's chats: a personal chat, a Space it is in, or a consultation
// it asked or was asked. BotID is the Bot whose activity the entry opens: the Bot itself, or
// for a consultation it asked, the Bot it asked. Active is true while a turn runs there.
type BotChat struct {
	Kind         string      `json:"kind"`
	SpaceID      int64       `json:"spaceId"`
	BotID        int64       `json:"botId"`
	Label        string      `json:"label"`
	Origin       *ChatOrigin `json:"origin,omitempty"`
	Active       bool        `json:"active"`
	Closed       bool        `json:"closed"`
	LastActivity time.Time   `json:"lastActivity"`
}

// _consultRequest is the request that began a consultation: the first with an origin chat.
const _consultRequest = "(SELECT MIN(id) FROM direct_message WHERE space_id = space.id " +
	"AND reply_to_id IS NULL AND origin_space_id IS NOT NULL AND space.kind = 'consultation')"

// BotChats lists the Bot's personal chats, the Spaces it is a member of, and the consultations
// it asked or was asked, the newest activity first, narrowed and paged by p like
// BotConversationsPage. It reads only stored rows.
func (s *Store) BotChats(ctx context.Context, botID int64, p ConversationPage) ([]BotChat, error) {
	bot, err := s.Bot(ctx, botID)
	if err != nil {
		return nil, err
	}
	member := "EXISTS (SELECT 1 FROM space_member WHERE space_id = space.id AND bot_id = ?)"
	where := " AND ((space.kind != 'consultation' AND " + member + ") OR " +
		"(space.kind = 'consultation' AND (" + member + " OR req.from_bot_id = ?)))"
	args := []any{botID, botID, botID}
	if p.ProjectID != nil {
		where += " AND EXISTS (SELECT 1 FROM space_project WHERE project_id = ? " +
			"AND space_id IN (space.id, req.origin_space_id))"
		args = append(args, *p.ProjectID)
	}
	if p.Before != nil {
		where += " AND (" + _activity + ", space.id) < " + _cursorActivity
		args = append(args, *p.Before)
	}
	order := _activity + " DESC, space.id DESC"
	if p.Limit > 0 {
		order += fmt.Sprintf(" LIMIT %d", p.Limit)
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT space.id, space.kind, space.name, space.closed_at IS NOT NULL, "+_activity+", "+
			"EXISTS (SELECT 1 FROM turn WHERE space_id = space.id AND status = 'running' "+
			"AND (space.kind = 'consultation' OR bot_id = ?)), "+
			"COALESCE((SELECT bot_id FROM space_member WHERE space_id = space.id AND lead = 1), 0), "+
			"COALESCE((SELECT bot.name FROM space_member JOIN bot ON bot.id = bot_id "+
			"WHERE space_id = space.id AND lead = 1), ''), "+
			"COALESCE(req.from_bot_id, 0), COALESCE((SELECT name FROM bot WHERE id = req.from_bot_id), ''), "+
			"COALESCE(req.origin_space_id, 0), COALESCE((SELECT name FROM space WHERE id = req.origin_space_id), '') "+
			"FROM space LEFT JOIN message m ON m.id = (SELECT MAX(id) FROM message WHERE space_id = space.id) "+
			"LEFT JOIN direct_message req ON req.id = "+_consultRequest+" WHERE 1 = 1"+where+" ORDER BY "+order,
		append([]any{botID}, args...)...)
	if err != nil {
		return nil, fmt.Errorf("list Bot chats: %w", err)
	}
	return scanAll(rows, func(r interface{ Scan(...any) error }) (BotChat, error) {
		var c BotChat
		var name, leadName, askerName, originName, last string
		var leadID, askerID, originID int64
		if err := r.Scan(&c.SpaceID, &c.Kind, &name, &c.Closed, &last, &c.Active,
			&leadID, &leadName, &askerID, &askerName, &originID, &originName); err != nil {
			return BotChat{}, err
		}
		var perr error
		if c.LastActivity, perr = time.Parse("2006-01-02 15:04:05.000", last); perr != nil {
			return BotChat{}, fmt.Errorf("Space %d: bad activity time: %w", c.SpaceID, perr)
		}
		c.BotID = botID
		switch c.Kind {
		case KindConversation:
			c.Kind, c.Label = ChatPersonal, name
		case KindConsultation:
			c.Kind = ChatConsultation
			c.Origin = &ChatOrigin{SpaceID: originID, Label: originLabel(askerName, originName)}
			if askerID == botID {
				c.BotID, c.Label = leadID, "Consultation with "+leadName
			} else {
				c.Label = consultationLabel(askerName)
			}
		default:
			c.Kind, c.Label = ChatSpace, spaceLabel(bot.Name, name)
		}
		return c, nil
	})
}

func spaceLabel(bot, space string) string { return bot + " · " + space }

func consultationLabel(asker string) string { return "Consultation from " + asker }

func originLabel(asker, chat string) string {
	if asker == "" {
		return chat
	}
	return spaceLabel(asker, chat)
}

// ActivityTurn is a turn of the Bot with the tool calls the backend exposed for it.
type ActivityTurn struct {
	Turn
	ToolCalls []ToolCall `json:"toolCalls"`
}

// BotActivity is what the records hold of one Bot in one Space: its messages and the messages
// that woke it, its exchanges, and its turns with their tool calls. Label names the Bot and
// the Space; Origin is the requesting chat of a consultation.
type BotActivity struct {
	Space     Space           `json:"space"`
	BotID     int64           `json:"botId"`
	Label     string          `json:"label"`
	Origin    *ChatOrigin     `json:"origin,omitempty"`
	Messages  []Message       `json:"messages"`
	Exchanges []DirectMessage `json:"exchanges"`
	Turns     []ActivityTurn  `json:"turns"`
}

// BotActivity reads the activity of a member Bot in a Space. It writes nothing and needs no
// workspace, so a closed Space reads like an open one. A Bot that is not a member is ErrNotFound.
func (s *Store) BotActivity(ctx context.Context, spaceID, botID int64) (BotActivity, error) {
	c, err := s.Space(ctx, spaceID)
	if err != nil {
		return BotActivity{}, err
	}
	var botName string
	err = s.db.QueryRowContext(ctx,
		"SELECT bot.name FROM space_member JOIN bot ON bot.id = bot_id WHERE space_id = ? AND bot_id = ?",
		spaceID, botID).Scan(&botName)
	if errors.Is(err, sql.ErrNoRows) {
		return BotActivity{}, fmt.Errorf("%w: bot %d is not a member of Space %d", ErrNotFound, botID, spaceID)
	}
	if err != nil {
		return BotActivity{}, fmt.Errorf("read member: %w", err)
	}
	a := BotActivity{Space: c, BotID: botID, Label: spaceLabel(botName, c.Name), Turns: []ActivityTurn{}}
	switch c.Kind {
	case KindConversation:
		a.Label = c.Name
	case KindConsultation:
		var askerName, originName string
		var originID int64
		err := s.db.QueryRowContext(ctx,
			"SELECT req.origin_space_id, COALESCE((SELECT name FROM bot WHERE id = req.from_bot_id), ''), "+
				"(SELECT name FROM space WHERE id = req.origin_space_id) FROM direct_message req "+
				"WHERE req.id = (SELECT MIN(id) FROM direct_message WHERE space_id = ? "+
				"AND reply_to_id IS NULL AND origin_space_id IS NOT NULL)", spaceID).
			Scan(&originID, &askerName, &originName)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return BotActivity{}, fmt.Errorf("read consultation origin: %w", err)
		}
		if err == nil {
			a.Origin = &ChatOrigin{SpaceID: originID, Label: originLabel(askerName, originName)}
			a.Label = consultationLabel(askerName)
		}
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, space_id, parent_id, author, bot_id, body, created_at, exchange_id FROM message "+
			"WHERE space_id = ? AND (? IN ('conversation', 'consultation') OR bot_id = ? "+
			"OR id IN (SELECT wake_message_id FROM turn WHERE space_id = ? AND bot_id = ?) "+
			"OR exchange_id IN (SELECT id FROM direct_message WHERE space_id = ? AND (from_bot_id = ? OR to_bot_id = ?))) "+
			"ORDER BY id", spaceID, c.Kind, botID, spaceID, botID, spaceID, botID, botID)
	if err != nil {
		return BotActivity{}, fmt.Errorf("list activity messages: %w", err)
	}
	if a.Messages, err = scanAll(rows, scanMessage); err != nil {
		return BotActivity{}, err
	}
	if a.Exchanges, err = s.DirectMessagesForBot(ctx, spaceID, botID, 0); err != nil {
		return BotActivity{}, err
	}
	a.Messages, a.Exchanges = nonNil(a.Messages), nonNil(a.Exchanges)
	turns, err := s.Turns(ctx, TurnFilter{SpaceID: &spaceID, BotID: &botID})
	if err != nil {
		return BotActivity{}, err
	}
	for _, t := range turns {
		calls, err := s.ToolCalls(ctx, t.ID)
		if err != nil {
			return BotActivity{}, err
		}
		a.Turns = append(a.Turns, ActivityTurn{Turn: t, ToolCalls: nonNil(calls)})
	}
	return a, nil
}
