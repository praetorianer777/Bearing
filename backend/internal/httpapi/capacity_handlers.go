package httpapi

import (
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/org"
)

// capacityDefaultDays is how far a range reaches when only its start, or nothing, is given: thirteen weeks.
const capacityDefaultDays = 91

type capacityPeriodResponse struct {
	From org.Day `json:"from"`
	To   org.Day `json:"to"`
	// FTE is the full-time equivalents the holdings add up to, each counting its share of the person's week.
	FTE float64 `json:"fte"`
	// Target is the mean of the subject's target on the days of the period that have one; null when none has, and
	// for people.
	Target *float64 `json:"target"`
	// Seats, Filled and Vacant are in seats, for positions and units only.
	Seats  *float64 `json:"seats,omitempty"`
	Filled *float64 `json:"filled,omitempty"`
	Vacant *float64 `json:"vacant,omitempty"`
	// People is the most people holding on one day of the period.
	People int `json:"people"`
}

type capacityResponse struct {
	From     org.Day                  `json:"from"`
	To       org.Day                  `json:"to"`
	By       string                   `json:"by"`
	SubUnits bool                     `json:"subUnits"`
	Periods  []capacityPeriodResponse `json:"periods"`
}

const capacityDescription = "from and to are days written 2006-01-02, both counted; from is today when left out and to thirteen weeks on. " +
	"The range is at most 731 days. by is day, week (from Monday) or month, week when left out; a week or a month is the mean of its " +
	"working days in the range. Planned holdings, leavers and archived positions count on their days."

func (s *Server) handleUnitCapacity(w http.ResponseWriter, r *http.Request) {
	s.capacity(w, r, "unitID", func(id uuid.UUID, q *org.CapacityQuery) { q.UnitID = &id })
}

func (s *Server) handlePositionCapacity(w http.ResponseWriter, r *http.Request) {
	s.capacity(w, r, "positionID", func(id uuid.UUID, q *org.CapacityQuery) { q.PositionID = &id })
}

func (s *Server) handleRoleCapacity(w http.ResponseWriter, r *http.Request) {
	s.capacity(w, r, "roleID", func(id uuid.UUID, q *org.CapacityQuery) { q.RoleID = &id })
}

func (s *Server) handlePersonCapacity(w http.ResponseWriter, r *http.Request) {
	s.capacity(w, r, "personID", func(id uuid.UUID, q *org.CapacityQuery) { q.PersonID = &id })
}

// readRange reads from and to, today and thirteen weeks on when left out, noting in fields what does not read.
func readRange(r *http.Request, fields map[string]string) (org.Day, org.Day) {
	v := r.URL.Query()
	from := today()
	if raw := v.Get("from"); raw != "" {
		day, err := org.ParseDay(raw)
		if err != nil {
			fields["from"] = sentence(err.Error())
		}
		from = day
	}
	to := org.DayOf(from.Time().AddDate(0, 0, capacityDefaultDays-1))
	if raw := v.Get("to"); raw != "" {
		day, err := org.ParseDay(raw)
		if err != nil {
			fields["to"] = sentence(err.Error())
		}
		to = day
	}
	return from, to
}

// handleRealmPersonCapacity is a person's capacity named as partners know them, by the realm's issuer and sub.
func (s *Server) handleRealmPersonCapacity(w http.ResponseWriter, r *http.Request) {
	issuer, subject := r.URL.Query().Get("issuer"), r.URL.Query().Get("subject")
	if issuer == "" || subject == "" {
		respondError(w, r, s.Log, ErrValidation(map[string]string{"subject": "Name the person by issuer and subject, as the realm does."}))
		return
	}
	p, err := s.Org.PersonInRealm(r.Context(), issuer, subject)
	if err != nil {
		respondError(w, r, s.Log, capacityError(err))
		return
	}
	s.capacityOf(w, r, func(q *org.CapacityQuery) { q.PersonID = &p.ID })
}

// capacityError says a refusal about capacity in the API's terms.
func capacityError(err error) error {
	switch {
	case errors.Is(err, org.ErrCapacityRange):
		return ErrValidation(map[string]string{"to": sentence(err.Error())})
	case errors.Is(err, org.ErrCapacityHidden):
		return ErrForbidden(sentence(err.Error()))
	case errors.Is(err, org.ErrNotFound), errors.Is(err, org.ErrPositionNotFound), errors.Is(err, org.ErrRoleNotFound), errors.Is(err, org.ErrPersonNotFound):
		return ErrNotFound()
	}
	return err
}

func (s *Server) capacity(w http.ResponseWriter, r *http.Request, param string, subject func(uuid.UUID, *org.CapacityQuery)) {
	id, ok := s.pathID(w, r, param)
	if !ok {
		return
	}
	s.capacityOf(w, r, func(q *org.CapacityQuery) { subject(id, q) })
}

func (s *Server) capacityOf(w http.ResponseWriter, r *http.Request, subject func(*org.CapacityQuery)) {
	v := r.URL.Query()
	q := org.CapacityQuery{By: v.Get("by"), SubUnits: v.Get("subUnits") == "true"}
	subject(&q)
	if q.By == "" {
		q.By = org.ByWeek
	}
	fields := map[string]string{}
	if q.By != org.ByDay && q.By != org.ByWeek && q.By != org.ByMonth {
		fields["by"] = "Count by day, week or month."
	}
	q.From, q.To = readRange(r, fields)
	if len(fields) > 0 {
		respondError(w, r, s.Log, ErrValidation(fields))
		return
	}
	periods, err := s.Org.Capacity(r.Context(), q)
	if err != nil {
		respondError(w, r, s.Log, capacityError(err))
		return
	}
	out := capacityResponse{From: q.From, To: q.To, By: q.By, SubUnits: q.SubUnits && q.UnitID != nil, Periods: make([]capacityPeriodResponse, len(periods))}
	for i, p := range periods {
		out.Periods[i] = capacityPeriodResponse{From: p.From, To: p.To, FTE: p.FTE, Target: p.Target, Seats: p.Seats, Filled: p.Filled, Vacant: p.Vacant, People: p.People}
	}
	respondJSON(w, http.StatusOK, out)
}

type gapResponse struct {
	// Kind is vacant (Size in seats), under_target or over_target (Size in full-time equivalents), unheld (Size 0)
	// or single_holder (Size 1).
	Kind string `json:"kind"`
	// SubjectKind is position, unit or role.
	SubjectKind string    `json:"subjectKind"`
	SubjectID   uuid.UUID `json:"subjectId"`
	Name        string    `json:"name"`
	// UnitID is a position's unit, or a unit itself; null for a role.
	UnitID *uuid.UUID `json:"unitId"`
	From   org.Day    `json:"from"`
	// To is the gap's last day, counted.
	To   org.Day `json:"to"`
	Size float64 `json:"size"`
}

func (s *Server) handleGaps(w http.ResponseWriter, r *http.Request) {
	fields := map[string]string{}
	q := org.GapQuery{SubUnits: r.URL.Query().Get("subUnits") == "true"}
	if raw := r.URL.Query().Get("unitId"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			fields["unitId"] = "Give a unit's id, such as the list of units answers."
		}
		q.UnitID = &id
	}
	q.From, q.To = readRange(r, fields)
	if len(fields) > 0 {
		respondError(w, r, s.Log, ErrValidation(fields))
		return
	}
	gaps, err := s.Org.Gaps(r.Context(), q)
	switch {
	case errors.Is(err, org.ErrCapacityRange):
		respondError(w, r, s.Log, ErrValidation(map[string]string{"to": sentence(err.Error())}))
		return
	case errors.Is(err, org.ErrNotFound):
		respondError(w, r, s.Log, ErrValidation(map[string]string{"unitId": "There is no such unit."}))
		return
	case err != nil:
		respondError(w, r, s.Log, err)
		return
	}
	out := make([]gapResponse, len(gaps))
	for i, g := range gaps {
		out[i] = gapResponse{Kind: g.Kind, SubjectKind: g.SubjectKind, SubjectID: g.SubjectID, Name: g.Name, UnitID: g.UnitID, From: g.From, To: g.To, Size: g.Size}
	}
	respondJSON(w, http.StatusOK, env{"from": q.From, "to": q.To, "gaps": out})
}
