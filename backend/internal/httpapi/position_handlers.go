package httpapi

import (
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/org"
)

type positionRequest struct {
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
	HeadOfUnit  *bool   `json:"headOfUnit,omitempty"`
}

type createPositionRequest struct {
	UnitID      uuid.UUID `json:"unitId"`
	Title       *string   `json:"title,omitempty"`
	Description *string   `json:"description,omitempty"`
	HeadOfUnit  *bool     `json:"headOfUnit,omitempty"`
	// From is the day it starts to exist, today when left out.
	From *org.Day `json:"from,omitempty"`
}

type reportsToRequest struct {
	// ReportsToID is the position it reports to by a solid line; left out, it reports to none from then.
	ReportsToID *uuid.UUID `json:"reportsToId,omitempty"`
	// From is the day the line takes effect, today when left out.
	From *org.Day `json:"from,omitempty"`
}

type dottedLineRequest struct {
	ReportsToID uuid.UUID `json:"reportsToId"`
	// From is the day the line takes effect, today when left out.
	From *org.Day `json:"from,omitempty"`
}

type lineResponse struct {
	ReportsToID uuid.UUID    `json:"reportsToId"`
	Kind        org.LineKind `json:"kind"`
	From        org.Day      `json:"from"`
	// To is the first day it no longer holds; null is for good.
	To *org.Day `json:"to"`
}

type positionResponse struct {
	ID          uuid.UUID `json:"id"`
	UnitID      uuid.UUID `json:"unitId"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	HeadOfUnit  bool      `json:"headOfUnit"`
	ExistsFrom  org.Day   `json:"existsFrom"`
	ArchivedOn  *org.Day  `json:"archivedOn"`
	// ReportsToID is the position it reports to by a solid line on the day asked about.
	ReportsToID *uuid.UUID `json:"reportsToId"`
	// DottedTo are the positions it reports to by dotted lines on that day.
	DottedTo []uuid.UUID `json:"dottedTo"`
	// Lines are every line it has had and will have, oldest first; only when one position is asked for.
	Lines []lineResponse `json:"lines,omitempty"`
}

func toPosition(p org.Position) positionResponse {
	out := positionResponse{ID: p.ID, UnitID: p.UnitID, Title: p.Title, Description: p.Description, HeadOfUnit: p.HeadOfUnit,
		ExistsFrom: p.ExistsFrom, ArchivedOn: p.ArchivedOn, ReportsToID: p.ReportsTo, DottedTo: p.DottedTo}
	if out.DottedTo == nil {
		out.DottedTo = []uuid.UUID{}
	}
	for _, l := range p.Lines {
		out.Lines = append(out.Lines, lineResponse{ReportsToID: l.ReportsTo, Kind: l.Kind, From: l.From, To: l.To})
	}
	return out
}

// positionError says a refusal about positions in the API's terms.
func positionError(err error) error {
	switch {
	case errors.Is(err, org.ErrPositionNotFound):
		return ErrNotFound()
	case errors.Is(err, org.ErrUnitNotThere):
		return &APIError{Status: http.StatusUnprocessableEntity, Code: "validation_failed", Message: "Some fields need attention.",
			Fields: map[string]string{"unitId": sentence(err.Error())}}
	}
	for code, refusal := range map[string]error{
		"unit_gone": org.ErrUnitGone, "not_there": org.ErrPositionNotThere, "archived": org.ErrPositionArchived,
		"not_archived": org.ErrPositionNotArchived, "target_not_there": org.ErrTargetNotThere, "cycle": org.ErrReportsToItself,
		"line_exists": org.ErrLineExists, "line_not_found": org.ErrLineNotFound, "has_reports": org.ErrHasReports,
		"head_taken": org.ErrHeadTaken,
	} {
		if errors.Is(err, refusal) {
			return ErrConflict(code, sentence(err.Error()))
		}
	}
	return orgError(err)
}

func (s *Server) respondPosition(w http.ResponseWriter, r *http.Request, status int, p org.Position, err error) {
	if err != nil {
		respondError(w, r, s.Log, positionError(err))
		return
	}
	respondJSON(w, status, env{"position": toPosition(p)})
}

func (s *Server) handlePositions(w http.ResponseWriter, r *http.Request) {
	day, ok := s.asOf(w, r)
	if !ok {
		return
	}
	var unit *uuid.UUID
	if raw := r.URL.Query().Get("unitId"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			respondError(w, r, s.Log, ErrValidation(map[string]string{"unitId": "Name a unit by its id."}))
			return
		}
		unit = &id
	}
	positions, err := s.Org.Positions(r.Context(), day, unit)
	if err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	out := make([]positionResponse, len(positions))
	for i, p := range positions {
		out[i] = toPosition(p)
	}
	respondJSON(w, http.StatusOK, env{"asOf": day, "positions": out})
}

func (s *Server) handlePosition(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "positionID")
	if !ok {
		return
	}
	day, ok := s.asOf(w, r)
	if !ok {
		return
	}
	p, err := s.Org.Position(r.Context(), id, day)
	if err != nil {
		respondError(w, r, s.Log, positionError(err))
		return
	}
	respondJSON(w, http.StatusOK, env{"asOf": day, "position": toPosition(p)})
}

func (s *Server) handleCreatePosition(w http.ResponseWriter, r *http.Request) {
	var req createPositionRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	in := org.PositionInput{Title: req.Title, Description: req.Description, HeadOfUnit: req.HeadOfUnit}
	p, lsn, err := s.Org.CreatePosition(r.Context(), req.UnitID, in, dayOr(req.From))
	noteWrite(r.Context(), lsn)
	s.respondPosition(w, r, http.StatusCreated, p, err)
}

func (s *Server) handleUpdatePosition(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "positionID")
	if !ok {
		return
	}
	var req positionRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	p, lsn, err := s.Org.UpdatePosition(r.Context(), id, org.PositionInput(req), today())
	noteWrite(r.Context(), lsn)
	s.respondPosition(w, r, http.StatusOK, p, err)
}

func (s *Server) handleSetReportsTo(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "positionID")
	if !ok {
		return
	}
	var req reportsToRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	p, lsn, err := s.Org.SetReportsTo(r.Context(), id, req.ReportsToID, dayOr(req.From))
	noteWrite(r.Context(), lsn)
	s.respondPosition(w, r, http.StatusOK, p, err)
}

func (s *Server) handleAddDottedLine(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "positionID")
	if !ok {
		return
	}
	var req dottedLineRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	p, lsn, err := s.Org.AddDottedLine(r.Context(), id, req.ReportsToID, dayOr(req.From))
	noteWrite(r.Context(), lsn)
	s.respondPosition(w, r, http.StatusCreated, p, err)
}

func (s *Server) handleEndDottedLine(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "positionID")
	if !ok {
		return
	}
	target, ok := s.pathID(w, r, "reportsToID")
	if !ok {
		return
	}
	var req endRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	p, lsn, err := s.Org.EndDottedLine(r.Context(), id, target, dayOr(req.On))
	noteWrite(r.Context(), lsn)
	s.respondPosition(w, r, http.StatusOK, p, err)
}

func (s *Server) handleArchivePosition(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "positionID")
	if !ok {
		return
	}
	var req endRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	p, lsn, err := s.Org.ArchivePosition(r.Context(), id, dayOr(req.On))
	noteWrite(r.Context(), lsn)
	s.respondPosition(w, r, http.StatusOK, p, err)
}

func (s *Server) handleRestorePosition(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "positionID")
	if !ok {
		return
	}
	p, lsn, err := s.Org.RestorePosition(r.Context(), id)
	noteWrite(r.Context(), lsn)
	s.respondPosition(w, r, http.StatusOK, p, err)
}
