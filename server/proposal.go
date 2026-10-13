package server

import (
	"fmt"
	"net/http"

	"github.com/bhanuprakaash/reef/channel"
	"github.com/bhanuprakaash/reef/store"
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

// wrap sends the text /wrap, ignoring any body.
func (s *server) wrap(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.startTurn(r.Context(), id, channel.WrapCommand, nil); err != nil {
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
	ch, err := s.st.Channel(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	if ch.FeatureID != nil {
		writeError(w, fmt.Errorf("%w: channel %d already has feature %d", store.ErrInvalid, id, *ch.FeatureID))
		return
	}
	err = s.startTurn(r.Context(), id, channel.PromoteCommand, func() error {
		var err error
		ch, err = s.st.PromoteChannel(r.Context(), id, f)
		return err
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
	p, added, err := s.st.ApproveProposal(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		store.Proposal
		DecisionsAdded int `json:"decisionsAdded"`
	}{p, added})
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
