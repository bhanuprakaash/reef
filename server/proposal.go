package server

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strconv"

	"github.com/bhanuprakaash/reef/channel"
	"github.com/bhanuprakaash/reef/memory"
	"github.com/bhanuprakaash/reef/store"
	"github.com/bhanuprakaash/reef/worktree"
)

func (s *server) proposalRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /channels/{id}/wrap", s.wrap)
	mux.HandleFunc("/channels/{id}/wrap", methodNotAllowed)
	mux.HandleFunc("POST /channels/{id}/promote", s.promote)
	mux.HandleFunc("/channels/{id}/promote", methodNotAllowed)
	mux.HandleFunc("GET /channels/{id}/proposal", s.pendingProposal)
	mux.HandleFunc("/channels/{id}/proposal", methodNotAllowed)
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
	if err := s.startUserTurn(r.Context(), id, userTurn{kind: _wrap, text: channel.WrapCommand}); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *server) promote(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in struct {
		Slug      string `json:"slug"`
		Name      string `json:"name"`
		FeatureID *int64 `json:"featureId"`
	}
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	newFeature := in.Slug != "" || in.Name != ""
	if newFeature == (in.FeatureID != nil) {
		writeError(w, fmt.Errorf("%w: give slug and name, or featureId", store.ErrInvalid))
		return
	}
	var f store.Feature
	if in.FeatureID != nil {
		f.ID = *in.FeatureID
	} else {
		f.Slug, f.Name = in.Slug, in.Name
	}
	if err := s.st.CheckPromote(r.Context(), id, f); err != nil {
		writeError(w, err)
		return
	}
	var ch store.Channel
	err = s.startUserTurn(r.Context(), id, userTurn{
		kind: _promote, text: channel.PromoteCommand,
		prepare: func() error {
			before, err := s.st.Channel(r.Context(), id)
			if err != nil {
				return err
			}
			p, err := s.st.Project(r.Context(), before.ProjectID)
			if err != nil {
				return err
			}
			if !worktree.HasCommit(r.Context(), p.Path) {
				return fmt.Errorf("%w: project %s is not a git repo with a commit", store.ErrInvalid, p.Path)
			}
			ch, err = s.st.PromoteChannel(r.Context(), id, f)
			return err
		},
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, ch)
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
	s.approveMu.Lock()
	a, err := ApproveProposal(r.Context(), s.st, s.ms, id)
	s.approveMu.Unlock()
	if err != nil {
		writeError(w, err)
		return
	}
	if a.Proposal.Kind == store.ProposalWrap {
		if l := s.lookup(a.Proposal.ChannelID); l != nil {
			s.stopLive(l)
		}
	}
	writeJSON(w, http.StatusOK, struct {
		store.Proposal
		ItemsAdded int      `json:"itemsAdded"`
		Warnings   []string `json:"warnings,omitempty"`
	}{a.Proposal, a.Added, a.Warnings})
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
// brief, close the channel and mark the proposal approved, in one transaction. It is not one
// transaction as a whole: a failure leaves the proposal pending and the channel open, and
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
		c, err := st.Channel(ctx, p.ChannelID)
		if err != nil {
			return Approval{}, err
		}
		if a.Added, a.Warnings, err = saveItems(ctx, ms, channel.Scopes(c, p.Role), p); err != nil {
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
			return added, warnings, fmt.Errorf("%w: scope %q is not one of the channel's", store.ErrInvalid, it.Scope)
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
				Author: p.Role, Role: p.Role, Scope: scopes[i],
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
