package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/bhanuprakaash/reef/store"
)

// transferOwner is the user handing the top-level item to another Bot. The answer is 200 once the
// Bot owns it, or 202 while a turn of the item's Space runs: the transfer then applies when that
// turn ends, or at the next start if the server restarts first. Stop the turn to apply it at once.
func (s *server) transferOwner(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in struct {
		BotID int64 `json:"botId"`
	}
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	if in.BotID == 0 {
		writeError(w, fmt.Errorf("%w: botId is required", store.ErrInvalid))
		return
	}
	ctx := r.Context()
	if _, err := s.st.RequestOwnerTransfer(ctx, id, in.BotID); err != nil {
		writeError(w, err)
		return
	}
	sp, err := s.st.WorkItemActiveSpace(ctx, id)
	switch {
	case errors.Is(err, store.ErrNotFound):
		_, _, err = s.st.ApplyOwnerTransfer(ctx, id)
	case err == nil:
		s.settleLeaving(ctx, sp.ID)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	item, err := s.st.WorkItem(ctx, id)
	if err != nil {
		writeError(w, err)
		return
	}
	out := struct {
		WorkItem store.WorkItem `json:"workItem"`
		Pending  bool           `json:"pending"`
		Note     string         `json:"note,omitempty"`
	}{WorkItem: item, Pending: item.PendingOwnerBotID != nil}
	status := http.StatusOK
	if out.Pending {
		status, out.Note = http.StatusAccepted, "applies after the current turn"
	}
	writeJSON(w, status, out)
}

// applyTransfers makes the Bots of the transfers pending in Space id the owners and delivers each
// handover to its new owner. A failure is logged and returned; the transfer stays pending.
func (s *server) applyTransfers(ctx context.Context, id int64) error {
	ctx = context.WithoutCancel(ctx)
	items, err := s.st.PendingOwnerTransfers(ctx, id)
	if err != nil {
		return err
	}
	var errs []error
	for _, item := range items {
		ex, made, err := s.st.ApplyOwnerTransfer(ctx, item)
		if err != nil {
			log.Printf("transfer work item %d in Space %d: %v", item, id, err)
			errs = append(errs, err)
			continue
		}
		if !made {
			continue
		}
		s.notify(id, "note")
		if err := s.deliverExchange(ctx, id, ex.ID); err != nil {
			log.Printf("deliver the handover of work item %d: %v", item, err)
		}
	}
	return errors.Join(errs...)
}
