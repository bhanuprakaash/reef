package server

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strconv"

	"github.com/bhanuprakaash/reef/memory"
	"github.com/bhanuprakaash/reef/space"
	"github.com/bhanuprakaash/reef/store"
)

func (s *server) proposalRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /spaces/{id}/wrap", s.wrap)
	mux.HandleFunc("/spaces/{id}/wrap", methodNotAllowed)
	mux.HandleFunc("GET /spaces/{id}/proposal", s.pendingProposal)
	mux.HandleFunc("/spaces/{id}/proposal", methodNotAllowed)
	mux.HandleFunc("POST /proposals/{id}/approve", s.approveProposal)
	mux.HandleFunc("/proposals/{id}/approve", methodNotAllowed)
	mux.HandleFunc("POST /proposals/{id}/reject", s.rejectProposal)
	mux.HandleFunc("/proposals/{id}/reject", methodNotAllowed)
}

func (s *server) wrap(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.startUserTurn(r.Context(), id, userTurn{kind: _wrap, text: space.WrapCommand}); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *server) pendingProposal(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	p, err := s.st.PendingProposal(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *server) approveProposal(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	a, err := s.approve(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		store.Proposal
		ItemsAdded int      `json:"itemsAdded"`
		Warnings   []string `json:"warnings,omitempty"`
	}{a.Proposal, a.Added, a.Warnings})
}

// approve approves the pending proposal id and, for a wrap, stops the Space's agent.
func (s *server) approve(ctx context.Context, id int64) (Approval, error) {
	s.approveMu.Lock()
	a, err := ApproveProposal(ctx, s.st, s.ms, id)
	s.approveMu.Unlock()
	if err != nil {
		return Approval{}, err
	}
	if a.Proposal.Kind == store.ProposalWrap {
		if l := s.lookup(a.Proposal.SpaceID); l != nil {
			s.stopLive(l)
		}
	}
	return a, nil
}

// Approval is what approving a proposal did: how many memory items it saved, and a line for
// each saved item that looks like a stored one or whose vector was not stored.
type Approval struct {
	Proposal store.Proposal
	Added    int
	Warnings []string
}

// ApproveProposal approves a pending proposal. A wrap first saves each item with its role as
// author, skipping one whose exact body is already accepted or candidate in its scope, then
// retires its retire ids, skipping any already retired; only then does the store save the
// brief, close the Space and mark the proposal approved, in one transaction. It is not one
// transaction as a whole: a failure leaves the proposal pending and the Space open, and
// approving again saves each item once. Two approves of one proposal at once can both save its
// items; the REST handler serializes them.
func ApproveProposal(
	ctx context.Context, st *store.Store, ms *memory.Store, id int64,
) (Approval, error) {
	p, err := st.Proposal(ctx, id)
	if err != nil {
		return Approval{}, err
	}
	if p.Status != store.ProposalPending {
		return Approval{}, fmt.Errorf("%w: proposal %d is already %s",
			store.ErrInvalid, id, p.Status)
	}
	var a Approval
	if p.Kind == store.ProposalWrap {
		c, err := st.Space(ctx, p.SpaceID)
		if err != nil {
			return Approval{}, err
		}
		if p.BotID == nil {
			return Approval{}, fmt.Errorf("%w: proposal has no Bot ID", store.ErrInvalid)
		}
		scopes := space.BotScopes(c, *p.BotID)
		if a.Added, a.Warnings, err = saveItems(ctx, ms, scopes, p); err != nil {
			return Approval{}, err
		}
		for _, rid := range p.Retire {
			it, err := ms.Read(ctx, rid)
			if err != nil {
				return Approval{}, fmt.Errorf("retire memory %d: %w", rid, err)
			}
			if it.Status == memory.StatusRetired {
				continue
			}
			if err := ms.Retire(ctx, rid); err != nil {
				return Approval{}, err
			}
		}
	}
	a.Proposal, err = st.ApproveProposal(ctx, id)
	return a, err
}

func saveItems(
	ctx context.Context, ms *memory.Store, scopes []memory.Ref, p store.Proposal,
) (int, []string, error) {
	added := 0
	var warnings []string
	for _, it := range p.Items {
		i := slices.IndexFunc(scopes, func(r memory.Ref) bool { return r.Kind == it.Scope })
		if i < 0 {
			return added, warnings, fmt.Errorf("%w: scope %q is not one of the Space's", store.ErrInvalid, it.Scope)
		}
		replaces := make([]int64, 0, len(it.Replaces))
		for _, rid := range it.Replaces {
			old, err := ms.Read(ctx, rid)
			if err != nil {
				return added, warnings, fmt.Errorf("replace memory %d: %w", rid, err)
			}
			if old.Status == memory.StatusAccepted {
				replaces = append(replaces, rid)
			}
		}
		saved, err := ms.Saved(ctx, scopes[i], it.Body, p.Retire)
		if err != nil {
			return added, warnings, err
		}
		if saved {
			for _, rid := range replaces {
				warnings = append(warnings,
					fmt.Sprintf("memory %d not replaced: item %q already saved", rid, it.Body))
			}
			continue
		}
		res, err := ms.Remember(ctx,
			memory.Writer{
				Author: p.Role, Role: p.Role, BotID: p.BotID, Approved: true, Scope: scopes[i],
				Links: []memory.Link{{
					Rel: memory.RelSource, ToKind: "proposal", ToID: strconv.FormatInt(p.ID, 10),
				}},
			},
			memory.Note{Kind: it.Kind, Body: it.Body, Why: it.Why, Replaces: replaces})
		if err != nil {
			return added, warnings, fmt.Errorf("save item %q: %w", it.Body, err)
		}
		added++
		for _, s := range res.Similar {
			warnings = append(warnings, fmt.Sprintf("memory %d looks like memory %d", res.Item.ID, s.ID))
		}
		if res.EmbedErr != nil {
			warnings = append(warnings,
				fmt.Sprintf("memory %d: vector not stored: %v", res.Item.ID, res.EmbedErr))
		}
	}
	return added, warnings, nil
}

func (s *server) rejectProposal(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	p, err := s.st.RejectProposal(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}
