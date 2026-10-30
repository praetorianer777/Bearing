package httpapi

import (
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/org"
)

type holdingRequest struct {
	PersonID   uuid.UUID `json:"personId"`
	PositionID uuid.UUID `json:"positionId"`
	// Share is the part of the person's week it takes, in percent; the whole week when left out.
	Share *int `json:"share,omitempty"`
	// From is the day it starts, today when left out.
	From *org.Day `json:"from,omitempty"`
	// To is the first day it no longer holds; left out, it runs until the position or the person is gone.
	To *org.Day `json:"to,omitempty"`
}

type shareRequest struct {
	// Share is the part of the person's week it takes, in percent.
	Share int `json:"share"`
}

type holdingResponse struct {
	ID         uuid.UUID `json:"id"`
	PersonID   uuid.UUID `json:"personId"`
	PositionID uuid.UUID `json:"positionId"`
	// Share is the part of the person's week it takes, in percent.
	Share int     `json:"share"`
	From  org.Day `json:"from"`
	// To is the first day it no longer holds; null is for good.
	To *org.Day `json:"to"`
}

func toHolding(h org.Holding) holdingResponse {
	return holdingResponse{ID: h.ID, PersonID: h.PersonID, PositionID: h.PositionID, Share: h.Share, From: h.From, To: h.To}
}

// holdingError says a refusal about holdings in the API's terms.
func holdingError(err error) error {
	switch {
	case errors.Is(err, org.ErrHoldingNotFound):
		return ErrNotFound()
	case errors.Is(err, org.ErrPersonNotFound):
		return ErrValidation(map[string]string{"personId": sentence(err.Error())})
	case errors.Is(err, org.ErrPositionNotFound):
		return ErrValidation(map[string]string{"positionId": sentence(err.Error())})
	}
	for code, refusal := range map[string]error{
		"position_not_there": org.ErrPositionNotThere, "person_not_there": org.ErrPersonNotThere,
		"holding_overlaps": org.ErrHoldingOverlaps, "seats_taken": org.ErrSeatsTaken,
		"not_started": org.ErrHoldingNotStarted, "ended": org.ErrHoldingEnded,
	} {
		if errors.Is(err, refusal) {
			return ErrConflict(code, sentence(err.Error()))
		}
	}
	return orgError(err)
}

func (s *Server) respondHolding(w http.ResponseWriter, r *http.Request, status int, h org.Holding, err error) {
	if err != nil {
		respondError(w, r, s.Log, holdingError(err))
		return
	}
	respondJSON(w, status, env{"holding": toHolding(h)})
}

func (s *Server) handleHoldings(w http.ResponseWriter, r *http.Request) {
	var f org.HoldingFilter
	fields := map[string]string{}
	for param, into := range map[string]**uuid.UUID{"personId": &f.PersonID, "positionId": &f.PositionID} {
		raw := r.URL.Query().Get(param)
		if raw == "" {
			continue
		}
		id, err := uuid.Parse(raw)
		if err != nil {
			fields[param] = "Name it by its id."
			continue
		}
		*into = &id
	}
	if len(fields) == 0 && f.PersonID == nil && f.PositionID == nil {
		fields["personId"] = "Name a person or a position."
	}
	if raw := r.URL.Query().Get("asOf"); raw != "" {
		day, err := org.ParseDay(raw)
		if err != nil {
			fields["asOf"] = sentence(err.Error())
		}
		f.AsOf = &day
	}
	if len(fields) > 0 {
		respondError(w, r, s.Log, ErrValidation(fields))
		return
	}
	holdings, err := s.Org.Holdings(r.Context(), f)
	if err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	out := make([]holdingResponse, len(holdings))
	for i, h := range holdings {
		out[i] = toHolding(h)
	}
	respondJSON(w, http.StatusOK, env{"holdings": out})
}

func (s *Server) handleHolding(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "holdingID")
	if !ok {
		return
	}
	h, err := s.Org.Holding(r.Context(), id)
	s.respondHolding(w, r, http.StatusOK, h, err)
}

func (s *Server) handleGiveHolding(w http.ResponseWriter, r *http.Request) {
	var req holdingRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	in := org.HoldingInput{PersonID: req.PersonID, PositionID: req.PositionID, Share: req.Share, From: dayOr(req.From), To: req.To}
	h, lsn, err := s.Org.GiveHolding(r.Context(), in)
	noteWrite(r.Context(), lsn)
	s.respondHolding(w, r, http.StatusCreated, h, err)
}

func (s *Server) handleUpdateHolding(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "holdingID")
	if !ok {
		return
	}
	var req shareRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	h, lsn, err := s.Org.SetShare(r.Context(), id, req.Share)
	noteWrite(r.Context(), lsn)
	s.respondHolding(w, r, http.StatusOK, h, err)
}

func (s *Server) handleEndHolding(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "holdingID")
	if !ok {
		return
	}
	var req endRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	h, lsn, err := s.Org.EndHolding(r.Context(), id, dayOr(req.On))
	noteWrite(r.Context(), lsn)
	s.respondHolding(w, r, http.StatusOK, h, err)
}

func (s *Server) handleRemoveHolding(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "holdingID")
	if !ok {
		return
	}
	lsn, err := s.Org.RemoveHolding(r.Context(), id)
	noteWrite(r.Context(), lsn)
	if err != nil {
		respondError(w, r, s.Log, holdingError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
