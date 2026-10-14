package server

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"slices"
	"time"

	"github.com/bhanuprakaash/reef/store"
)

func (s *server) inboxRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /inbox", s.listInbox)
	mux.HandleFunc("/inbox", methodNotAllowed)
	mux.HandleFunc("GET /inbox/events", s.inboxEvents)
	mux.HandleFunc("/inbox/events", methodNotAllowed)
}

// inbox lists the store's inbox without the waiting items of a channel with a chain running:
// between two turns its last message is an agent's, and the role it wakes has not started.
// The running channels are read before and after the store, so a chain that starts or ends
// during the read is not missed.
func (s *server) inbox(ctx context.Context) ([]store.InboxItem, error) {
	running := map[int64]bool{}
	s.markRunning(running)
	items, err := s.st.Inbox(ctx)
	if err != nil {
		return nil, err
	}
	s.markRunning(running)
	items = slices.DeleteFunc(items, func(i store.InboxItem) bool {
		return i.Kind == "waiting" && running[i.ChannelID]
	})
	if items == nil {
		items = []store.InboxItem{}
	}
	return items, nil
}

func (s *server) markRunning(running map[int64]bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, l := range s.lives {
		l.mu.Lock()
		if l.state != _idle {
			running[id] = true
		}
		l.mu.Unlock()
	}
}

// inboxChanged tells the inbox streams that something that can change the inbox happened.
// It never waits, so it may be called with a channel's lock held.
func (s *server) inboxChanged() {
	s.inboxMu.Lock()
	close(s.inboxWake)
	s.inboxWake = make(chan struct{})
	s.inboxMu.Unlock()
}

func (s *server) listInbox(w http.ResponseWriter, r *http.Request) {
	items, err := s.inbox(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// inboxEvents streams the whole inbox as an inbox event, when the stream connects and each
// time the list differs from the one sent last.
func (s *server) inboxEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	rc := http.NewResponseController(w)
	tick := time.NewTicker(_keepAlive)
	defer tick.Stop()
	var last []byte
	for {
		quitting := false
		select {
		case <-s.quit:
			quitting = true
		default:
		}
		s.inboxMu.Lock()
		wake := s.inboxWake
		s.inboxMu.Unlock()
		items, err := s.inbox(r.Context())
		if err != nil {
			if last == nil {
				writeError(w, err)
			}
			return
		}
		if f, ok := newFrame("inbox", items); ok && !bytes.Equal(f.data, last) {
			if err := writeFrames(w, []frame{f}); err != nil || rc.Flush() != nil {
				return
			}
			last = f.data
		}
		if quitting {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-s.quit:
		case <-wake:
		case <-tick.C:
			if _, err := io.WriteString(w, ": keepalive\n\n"); err != nil || rc.Flush() != nil {
				return
			}
		}
	}
}
