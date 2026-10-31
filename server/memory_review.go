package server

import (
	"context"
	"log"
	"time"

	"github.com/bhanuprakaash/reef/agent"
)

const (
	_reviewEvery   = 7 * 24 * time.Hour
	_reviewStale   = time.Hour
	_reviewTimeout = 10 * time.Minute
)

// reviewMemory starts the review of Bot botID's own memory in the background when it is due,
// after one of its turns in Space spaceID has ended and its reply is stored. One review runs per
// Bot at a time, however many Spaces it works in. The review shares nothing with the Space's
// session. The marker moves only when the review ends well.
func (s *server) reviewMemory(spaceID, botID int64) {
	if s.maintain == nil {
		return
	}
	ok, err := s.st.ClaimMemoryReview(s.ctx, botID, s.now(), _reviewEvery, _reviewStale)
	if err != nil {
		log.Printf("claim memory review of Bot %d: %v", botID, err)
		return
	}
	if !ok || !s.begin() {
		return
	}
	ctx, cancel := context.WithTimeout(s.ctx, _reviewTimeout)
	s.mu.Lock()
	if s.reviews == nil {
		s.reviews = map[int64]context.CancelFunc{}
	}
	s.reviews[botID] = cancel
	s.mu.Unlock()
	go func() {
		defer s.wg.Done()
		defer func() {
			s.mu.Lock()
			delete(s.reviews, botID)
			s.mu.Unlock()
			cancel()
		}()
		err := s.maintain(ctx, spaceID, botID, func(agent.Event) {})
		if err != nil {
			log.Printf("memory review of Bot %d: %v", botID, err)
		}
		if err := s.st.FinishMemoryReview(context.Background(), botID, s.now(), err == nil); err != nil {
			log.Print(err)
		}
	}()
}

// stopReview ends the running review of Bot botID, whose reef MCP server loaded its token once.
func (s *server) stopReview(botID int64) {
	s.mu.Lock()
	cancel := s.reviews[botID]
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}
