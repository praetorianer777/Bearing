package httpapi

import (
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/org"
)

type targetRequest struct {
	// UnitID, PositionID or RoleID names whose target it is, one of them.
	UnitID     *uuid.UUID `json:"unitId,omitempty"`
	PositionID *uuid.UUID `json:"positionId,omitempty"`
	RoleID     *uuid.UUID `json:"roleId,omitempty"`
	// FTE is what the subject should have, in full-time equivalents, from 0 to 100000.
	FTE float64 `json:"fte"`
	// From is the day it starts, today when left out.
	From *org.Day `json:"from,omitempty"`
	// To is the first day it no longer holds; left out, it is for good.
	To *org.Day `json:"to,omitempty"`
}

type targetResponse struct {
	ID uuid.UUID `json:"id"`
	// UnitID, PositionID or RoleID is whose target it is; the others are null.
	UnitID     *uuid.UUID `json:"unitId"`
	PositionID *uuid.UUID `json:"positionId"`
	RoleID     *uuid.UUID `json:"roleId"`
	FTE        float64    `json:"fte"`
	From       org.Day    `json:"from"`
	// To is the first day it no longer holds; null is for good.
	To *org.Day `json:"to"`
}

func toTarget(t org.Target) targetResponse {
	return targetResponse{ID: t.ID, UnitID: t.UnitID, PositionID: t.PositionID, RoleID: t.RoleID, FTE: t.FTE, From: t.From, To: t.To}
}

// targetError says a refusal about targets in the API's terms, where an unknown subject is a field to fix.
func targetError(err error) error {
	switch {
	case errors.Is(err, org.ErrTargetNotFound):
		return ErrNotFound()
	case errors.Is(err, org.ErrTargetWhat):
		return ErrValidation(map[string]string{"unitId": sentence(err.Error())})
	case errors.Is(err, org.ErrNotFound):
		return ErrValidation(map[string]string{"unitId": "There is no such unit."})
	case errors.Is(err, org.ErrPositionNotFound):
		return ErrValidation(map[string]string{"positionId": sentence(err.Error())})
	case errors.Is(err, org.ErrRoleNotFound):
		return ErrValidation(map[string]string{"roleId": sentence(err.Error())})
	}
	return orgError(err)
}

func (s *Server) handleTargets(w http.ResponseWriter, r *http.Request) {
	var subject org.TargetSubject
	for param, into := range map[string]**uuid.UUID{"unitId": &subject.UnitID, "positionId": &subject.PositionID, "roleId": &subject.RoleID} {
		raw := r.URL.Query().Get(param)
		if raw == "" {
			continue
		}
		id, err := uuid.Parse(raw)
		if err != nil {
			respondError(w, r, s.Log, ErrValidation(map[string]string{param: "Give an id, such as the list of units answers."}))
			return
		}
		*into = &id
	}
	var asOf *org.Day
	if r.URL.Query().Get("asOf") != "" {
		day, ok := s.asOf(w, r)
		if !ok {
			return
		}
		asOf = &day
	}
	targets, err := s.Org.Targets(r.Context(), subject, asOf)
	if err != nil {
		respondError(w, r, s.Log, targetError(err))
		return
	}
	out := make([]targetResponse, len(targets))
	for i, t := range targets {
		out[i] = toTarget(t)
	}
	respondJSON(w, http.StatusOK, env{"targets": out})
}

func (s *Server) handleSetTarget(w http.ResponseWriter, r *http.Request) {
	var req targetRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	in := org.TargetInput{TargetSubject: org.TargetSubject{UnitID: req.UnitID, PositionID: req.PositionID, RoleID: req.RoleID},
		FTE: req.FTE, From: dayOr(req.From), To: req.To}
	t, lsn, err := s.Org.SetTarget(r.Context(), in)
	noteWrite(r.Context(), lsn)
	if err != nil {
		respondError(w, r, s.Log, targetError(err))
		return
	}
	respondJSON(w, http.StatusCreated, env{"target": toTarget(t)})
}

func (s *Server) handleRemoveTarget(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "targetID")
	if !ok {
		return
	}
	lsn, err := s.Org.RemoveTarget(r.Context(), id)
	noteWrite(r.Context(), lsn)
	if err != nil {
		respondError(w, r, s.Log, targetError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
