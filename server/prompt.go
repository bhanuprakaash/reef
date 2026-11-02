package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/store"
)

const (
	_defaultPromptWait = 10 * time.Minute

	_noAnswerReason = "no answer from the user"
	_deniedReason   = "denied by the user"
)

// promptAnswer is the answer a wake carries to the Bot whose turn waits on promptID.
type promptAnswer struct {
	promptID int64
	a        agent.Answer
}

func (s *server) promptRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /spaces/{id}/prompts", s.listPrompts)
	mux.HandleFunc("/spaces/{id}/prompts", methodNotAllowed)
	mux.HandleFunc("POST /prompts/{id}/answer", s.answerPrompt)
	mux.HandleFunc("/prompts/{id}/answer", methodNotAllowed)
}

// listPrompts lists the prompts of the Space that wait for an answer or expired without one,
// oldest first.
func (s *server) listPrompts(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	prompts, err := s.st.SpacePermissionPrompts(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	if prompts == nil {
		prompts = []store.PermissionPrompt{}
	}
	writeJSON(w, http.StatusOK, prompts)
}

// answerPrompt answers a waiting prompt: its turn goes on with the answer. An expired prompt, whose
// turn ended with a denial, can still be allowed: the Bot is woken to retry, and its next prompt
// for the same rule is allowed once without asking. An answer of "always" also adds the rule to the
// Bot's tools. A prompt already answered is 409.
func (s *server) answerPrompt(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in struct {
		Answer string `json:"answer"`
	}
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	if in.Answer != store.AnswerOnce && in.Answer != store.AnswerAlways && in.Answer != store.AnswerDeny {
		writeError(w, fmt.Errorf("%w: answer must be once, always or deny", store.ErrInvalid))
		return
	}
	ctx := r.Context()
	p, err := s.st.PermissionPrompt(ctx, id)
	if err != nil {
		writeError(w, err)
		return
	}
	if in.Answer == store.AnswerAlways && p.Rule == "" {
		writeError(w, fmt.Errorf("%w: prompt %d has no rule to allow always", store.ErrInvalid, id))
		return
	}
	from, allow := p.Status, in.Answer != store.AnswerDeny
	if from != store.PromptWaiting && from != store.PromptExpired {
		writeError(w, fmt.Errorf("prompt %d is %s: %w", id, p.Status, store.ErrConflict))
		return
	}
	if allow && in.Answer == store.AnswerAlways {
		if err := s.st.AddBotTool(ctx, p.SpaceID, p.BotID, p.Rule); err != nil {
			writeError(w, err)
			return
		}
	}
	to, reason := store.PromptAllowed, ""
	if !allow {
		to, reason = store.PromptDenied, _deniedReason
	}
	late := from == store.PromptExpired
	if late && !allow {
		reason = p.Reason
	}
	p, err = s.st.ResolvePermissionPrompt(ctx, id, from, to, in.Answer, reason, late && allow)
	if err != nil {
		writeError(w, err)
		return
	}
	s.disarmPrompt(id)
	s.publishPrompt(p)
	switch {
	case late && allow:
		err = s.wakeToRetry(ctx, p)
	case late:
	default:
		ans := agent.Answer{Allow: allow, Reason: reason}
		if in.Answer == store.AnswerAlways {
			ans.Rule = p.Rule
		}
		err = s.wakeAnswer(ctx, p, ans)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// promptChanged is the hook of the Session of a Space: it publishes the prompt, and has a waiting
// one expire when nobody answers in time.
func (s *server) promptChanged(p store.PermissionPrompt) {
	s.publishPrompt(p)
	if p.Status == store.PromptWaiting {
		s.armPrompt(p)
		return
	}
	s.disarmPrompt(p.ID)
}

func (s *server) publishPrompt(p store.PermissionPrompt) {
	l := s.lookup(p.SpaceID)
	if l == nil {
		return
	}
	l.mu.Lock()
	l.publishLocked("prompt", p)
	l.mu.Unlock()
}

func (s *server) armPrompt(p store.PermissionPrompt) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.promptTimers[p.ID] = time.AfterFunc(s.promptWait, func() { s.expirePrompt(p) })
}

func (s *server) disarmPrompt(id int64) {
	s.mu.Lock()
	t := s.promptTimers[id]
	delete(s.promptTimers, id)
	s.mu.Unlock()
	if t != nil {
		t.Stop()
	}
}

func (s *server) stopPromptTimers() {
	s.mu.Lock()
	timers := s.promptTimers
	s.promptTimers = map[int64]*time.Timer{}
	s.mu.Unlock()
	for _, t := range timers {
		t.Stop()
	}
}

// dropAnswers removes the answers to permission prompts from ws, the wakes a chain keeps for later:
// an answer belongs to a turn that waits, not to the Space's pause or restart.
func dropAnswers(ws []wake) []wake {
	return slices.DeleteFunc(ws, func(w wake) bool { return w.answer != nil })
}

// expirePrompt denies prompt p, which nobody answered, with the reason "no answer from the user",
// and has its turn go on with that denial. The prompt stays: the user may still allow it.
func (s *server) expirePrompt(p store.PermissionPrompt) {
	if !s.begin() {
		return
	}
	defer s.wg.Done()
	s.mu.Lock()
	delete(s.promptTimers, p.ID)
	s.mu.Unlock()
	row, err := s.st.ResolvePermissionPrompt(
		s.ctx, p.ID, store.PromptWaiting, store.PromptExpired, store.AnswerDeny, _noAnswerReason, false)
	if err != nil {
		if !errors.Is(err, store.ErrConflict) {
			log.Printf("expire prompt %d: %v", p.ID, err)
		}
		return
	}
	s.publishPrompt(row)
	if err := s.wakeAnswer(s.ctx, row, agent.Answer{Reason: _noAnswerReason}); err != nil {
		log.Printf("deny prompt %d: %v", p.ID, err)
	}
}

// wakeAnswer gives a to the turn of p's Bot that waits on p: queued ahead of the wakes behind the
// turn a running chain is on, or as a new chain when the Space is idle.
func (s *server) wakeAnswer(ctx context.Context, p store.PermissionPrompt, a agent.Answer) error {
	w := wake{
		from: _userAuthor, botID: &p.BotID, stored: time.Now(),
		answer: &promptAnswer{promptID: p.ID, a: a},
	}
	return s.runAnswer(ctx, p.SpaceID, w)
}

func (s *server) runAnswer(ctx context.Context, spaceID int64, w wake) error {
	for range 3 {
		if queued, err := s.queueAnswer(ctx, spaceID, w); queued || err != nil {
			return err
		}
		c, _, err := s.claim(ctx, spaceID, _message)
		if errors.Is(err, store.ErrConflict) {
			continue
		}
		if err != nil {
			return err
		}
		return c.resumeAnswer(ctx, w)
	}
	return fmt.Errorf("space %d has a turn running that cannot take the answer: %w", spaceID, errBusy)
}

// queueAnswer puts w ahead of the wakes behind the turn a running chain is on. It reports false,
// having changed nothing, when the Space runs no chain.
func (s *server) queueAnswer(ctx context.Context, id int64, w wake) (bool, error) {
	l := s.lookup(id)
	if l == nil {
		return false, nil
	}
	if err := l.awaitStart(ctx); err != nil {
		return false, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.state != _running {
		return false, nil
	}
	l.queued = append([]wake{w}, l.queued...)
	return true, nil
}

// resumeAnswer runs w as a new chain. The turn it goes on with was started by a chain that has
// passed the checks of a turn.
func (c *chain) resumeAnswer(ctx context.Context, w wake) error {
	sess, err := c.session()
	if err != nil {
		return c.release(err)
	}
	after, err := c.s.st.LastMessageID(ctx, c.id)
	if err != nil {
		return c.release(err)
	}
	return c.start(ctx, sess, _message, []wake{w}, after)
}

// wakeToRetry tells the Bot of the expired prompt p, which the user has now allowed, to try the call
// again.
func (s *server) wakeToRetry(ctx context.Context, p store.PermissionPrompt) error {
	bot, err := s.st.Bot(ctx, p.BotID)
	if err != nil {
		return err
	}
	body := fmt.Sprintf("I allowed your %s call that got no answer: %s. Try it again.", p.Tool, promptCall(p))
	key := "prompt:" + strconv.FormatInt(p.ID, 10)
	dm, created, err := s.st.CreateExchange(ctx, store.DirectMessage{
		SpaceID: p.SpaceID, OriginSpaceID: &p.SpaceID, From: _userAuthor, To: bot.Name,
		ToBotID: &p.BotID, Body: body, Kind: store.ExchangeGuidance, RequestKey: &key,
	}, "To "+bot.Name+": "+body)
	if err != nil || !created {
		return err
	}
	s.notify(p.SpaceID, "note")
	return s.deliverExchange(ctx, p.SpaceID, dm.ID)
}

// promptCall is what the call of p runs or reads: its command, or else the rule it asks for.
func promptCall(p store.PermissionPrompt) string {
	var in struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(p.Input, &in); err == nil && in.Command != "" {
		return in.Command
	}
	if p.Rule != "" {
		return p.Rule
	}
	return p.Tool
}
