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
	go func() {
		defer s.wg.Done()
		ctx, cancel := context.WithTimeout(s.ctx, _reviewTimeout)
		defer cancel()
		err := s.maintain(ctx, spaceID, botID, func(agent.Event) {})
		if err != nil {
			log.Printf("memory review of Bot %d: %v", botID, err)
		}
		if err := s.st.FinishMemoryReview(context.Background(), botID, s.now(), err == nil); err != nil {
			log.Print(err)
		}
	}()
}
