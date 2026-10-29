package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/org"
)

type unitTypeRequest struct {
	Name   *string `json:"name,omitempty"`
	Icon   *string `json:"icon,omitempty"`
	Colour *string `json:"colour,omitempty"`
	Rank   *int    `json:"rank,omitempty"`
}

type unitTypeResponse struct {
	ID     uuid.UUID `json:"id"`
	Name   string    `json:"name"`
	Icon   string    `json:"icon"`
	Colour string    `json:"colour"`
	Rank   int       `json:"rank"`
	// Units counts the units of this type, archived ones too.
	Units int `json:"units"`
}

type unitRequest struct {
	TypeID      *uuid.UUID `json:"typeId,omitempty"`
	Name        *string    `json:"name,omitempty"`
	ShortName   *string    `json:"shortName,omitempty"`
	Description *string    `json:"description,omitempty"`
}

type createUnitRequest struct {
	TypeID      *uuid.UUID `json:"typeId,omitempty"`
	Name        *string    `json:"name,omitempty"`
	ShortName   *string    `json:"shortName,omitempty"`
	Description *string    `json:"description,omitempty"`
	// ParentID is the unit it is part of; left out, the unit is the root.
	ParentID *uuid.UUID `json:"parentId,omitempty"`
	// From is the day it starts to exist, today when left out.
	From *org.Day `json:"from,omitempty"`
}

type moveUnitRequest struct {
	// ParentID is the unit it becomes part of; left out, the unit becomes the root.
	ParentID *uuid.UUID `json:"parentId,omitempty"`
	// From is the day the move takes effect, today when left out.
	From *org.Day `json:"from,omitempty"`
}

type archiveUnitRequest struct {
	// On is the first day the unit no longer exists, today when left out.
	On *org.Day `json:"on,omitempty"`
}

type placementResponse struct {
	ParentID *uuid.UUID `json:"parentId"`
	From     org.Day    `json:"from"`
	// To is the first day it no longer holds; null is for good.
	To *org.Day `json:"to"`
}

type unitResponse struct {
	ID          uuid.UUID `json:"id"`
	TypeID      uuid.UUID `json:"typeId"`
	Name        string    `json:"name"`
	ShortName   *string   `json:"shortName"`
	Description string    `json:"description"`
	// ParentID is the unit it is part of on the day asked about; null at the root.
	ParentID   *uuid.UUID `json:"parentId"`
	ArchivedOn *org.Day   `json:"archivedOn"`
	// Placements are where the unit sat over time, oldest first; only when one unit is asked for.
	Placements []placementResponse `json:"placements,omitempty"`
}

func toUnitType(t org.UnitType) unitTypeResponse {
	return unitTypeResponse{ID: t.ID, Name: t.Name, Icon: t.Icon, Colour: t.Colour, Rank: t.Rank, Units: t.Units}
}

func toUnit(u org.Unit) unitResponse {
	out := unitResponse{ID: u.ID, TypeID: u.TypeID, Name: u.Name, ShortName: u.ShortName, Description: u.Description,
		ParentID: u.ParentID, ArchivedOn: u.ArchivedOn}
	for _, p := range u.Placements {
		out.Placements = append(out.Placements, placementResponse{ParentID: p.ParentID, From: p.From, To: p.To})
	}
	return out
}

// orgError says a refusal of the organisation model in the API's terms.
func orgError(err error) error {
	var invalid org.Invalid
	switch {
	case errors.As(err, &invalid):
		return ErrValidation(map[string]string{invalid.Field: invalid.Message})
	case errors.Is(err, org.ErrNotFound):
		return ErrNotFound()
	case errors.Is(err, org.ErrTypeNotFound):
		return &APIError{Status: http.StatusUnprocessableEntity, Code: "validation_failed", Message: "Some fields need attention.",
			Fields: map[string]string{"typeId": "There is no such unit type."}}
	case errors.Is(err, org.ErrTypeNameTaken):
		return &APIError{Status: http.StatusConflict, Code: "name_taken", Message: sentence(err.Error()),
			Fields: map[string]string{"name": sentence(err.Error())}}
	}
	for code, refusal := range map[string]error{
		"type_in_use": org.ErrTypeInUse, "cycle": org.ErrCycle, "root_taken": org.ErrRootTaken, "not_there": org.ErrNotThere,
		"parent_not_there": org.ErrParentNotThere, "parent_archived": org.ErrParentArchived, "has_units": org.ErrHasUnits,
		"archived": org.ErrArchived, "not_archived": org.ErrNotArchived,
	} {
		if errors.Is(err, refusal) {
			return ErrConflict(code, sentence(err.Error()))
		}
	}
	return err
}

// today is the day a read or a change applies to when the caller names none.
func today() org.Day { return org.DayOf(time.Now()) }

// asOf is the day a read is asked about: the asOf query parameter, today without one.
func (s *Server) asOf(w http.ResponseWriter, r *http.Request) (org.Day, bool) {
	raw := r.URL.Query().Get("asOf")
	if raw == "" {
		return today(), true
	}
	day, err := org.ParseDay(raw)
	if err != nil {
		respondError(w, r, s.Log, ErrValidation(map[string]string{"asOf": sentence(err.Error())}))
		return org.Day{}, false
	}
	return day, true
}

func dayOr(d *org.Day) org.Day {
	if d == nil {
		return today()
	}
	return *d
}

func (s *Server) pathID(w http.ResponseWriter, r *http.Request, param string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, param))
	if err != nil {
		respondError(w, r, s.Log, ErrNotFound())
		return uuid.Nil, false
	}
	return id, true
}

func (s *Server) handleUnitTypes(w http.ResponseWriter, r *http.Request) {
	types, err := s.Org.UnitTypes(r.Context())
	if err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	out := make([]unitTypeResponse, len(types))
	for i, t := range types {
		out[i] = toUnitType(t)
	}
	respondJSON(w, http.StatusOK, env{"unitTypes": out})
}

func (s *Server) handleCreateUnitType(w http.ResponseWriter, r *http.Request) {
	var req unitTypeRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	t, lsn, err := s.Org.CreateUnitType(r.Context(), org.UnitTypeInput(req))
	noteWrite(r.Context(), lsn)
	if err != nil {
		respondError(w, r, s.Log, orgError(err))
		return
	}
	respondJSON(w, http.StatusCreated, env{"unitType": toUnitType(t)})
}

func (s *Server) handleUpdateUnitType(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "unitTypeID")
	if !ok {
		return
	}
	var req unitTypeRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	t, lsn, err := s.Org.UpdateUnitType(r.Context(), id, org.UnitTypeInput(req))
	noteWrite(r.Context(), lsn)
	if errors.Is(err, org.ErrTypeNotFound) {
		err = ErrNotFound()
	}
	if err != nil {
		respondError(w, r, s.Log, orgError(err))
		return
	}
	respondJSON(w, http.StatusOK, env{"unitType": toUnitType(t)})
}

func (s *Server) handleDeleteUnitType(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "unitTypeID")
	if !ok {
		return
	}
	lsn, err := s.Org.DeleteUnitType(r.Context(), id)
	noteWrite(r.Context(), lsn)
	if errors.Is(err, org.ErrTypeNotFound) {
		err = ErrNotFound()
	}
	if err != nil {
		respondError(w, r, s.Log, orgError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUnits(w http.ResponseWriter, r *http.Request) {
	day, ok := s.asOf(w, r)
	if !ok {
		return
	}
	list := s.Org.Units
	if r.URL.Query().Get("archived") == "true" {
		list = s.Org.ArchivedUnits
	}
	units, err := list(r.Context(), day)
	if err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	out := make([]unitResponse, len(units))
	for i, u := range units {
		out[i] = toUnit(u)
	}
	respondJSON(w, http.StatusOK, env{"asOf": day, "units": out})
}

func (s *Server) handleUnit(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "unitID")
	if !ok {
		return
	}
	day, ok := s.asOf(w, r)
	if !ok {
		return
	}
	u, err := s.Org.Unit(r.Context(), id, day)
	if err != nil {
		respondError(w, r, s.Log, orgError(err))
		return
	}
	respondJSON(w, http.StatusOK, env{"asOf": day, "unit": toUnit(u)})
}

func (s *Server) handleCreateUnit(w http.ResponseWriter, r *http.Request) {
	var req createUnitRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	in := org.UnitInput{TypeID: req.TypeID, Name: req.Name, ShortName: req.ShortName, Description: req.Description}
	u, lsn, err := s.Org.CreateUnit(r.Context(), in, req.ParentID, dayOr(req.From))
	noteWrite(r.Context(), lsn)
	if err != nil {
		respondError(w, r, s.Log, orgError(err))
		return
	}
	respondJSON(w, http.StatusCreated, env{"unit": toUnit(u)})
}

func (s *Server) handleUpdateUnit(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "unitID")
	if !ok {
		return
	}
	var req unitRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	u, lsn, err := s.Org.UpdateUnit(r.Context(), id, org.UnitInput(req), today())
	noteWrite(r.Context(), lsn)
	if err != nil {
		respondError(w, r, s.Log, orgError(err))
		return
	}
	respondJSON(w, http.StatusOK, env{"unit": toUnit(u)})
}

func (s *Server) handleMoveUnit(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "unitID")
	if !ok {
		return
	}
	var req moveUnitRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	u, lsn, err := s.Org.MoveUnit(r.Context(), id, req.ParentID, dayOr(req.From))
	noteWrite(r.Context(), lsn)
	if err != nil {
		respondError(w, r, s.Log, orgError(err))
		return
	}
	respondJSON(w, http.StatusOK, env{"unit": toUnit(u)})
}

func (s *Server) handleArchiveUnit(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "unitID")
	if !ok {
		return
	}
	var req archiveUnitRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	u, lsn, err := s.Org.ArchiveUnit(r.Context(), id, dayOr(req.On))
	noteWrite(r.Context(), lsn)
	if err != nil {
		respondError(w, r, s.Log, orgError(err))
		return
	}
	respondJSON(w, http.StatusOK, env{"unit": toUnit(u)})
}

func (s *Server) handleRestoreUnit(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "unitID")
	if !ok {
		return
	}
	u, lsn, err := s.Org.RestoreUnit(r.Context(), id)
	noteWrite(r.Context(), lsn)
	if err != nil {
		respondError(w, r, s.Log, orgError(err))
		return
	}
	respondJSON(w, http.StatusOK, env{"unit": toUnit(u)})
}
