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

func (s *Server) capacity(w http.ResponseWriter, r *http.Request, param string, subject func(uuid.UUID, *org.CapacityQuery)) {
	id, ok := s.pathID(w, r, param)
	if !ok {
		return
	}
	v := r.URL.Query()
	q := org.CapacityQuery{By: v.Get("by"), SubUnits: v.Get("subUnits") == "true"}
	subject(id, &q)
	if q.By == "" {
		q.By = org.ByWeek
	}
	fields := map[string]string{}
	if q.By != org.ByDay && q.By != org.ByWeek && q.By != org.ByMonth {
		fields["by"] = "Count by day, week or month."
	}
	q.From = today()
	if raw := v.Get("from"); raw != "" {
		day, err := org.ParseDay(raw)
		if err != nil {
			fields["from"] = sentence(err.Error())
		}
		q.From = day
	}
	q.To = org.DayOf(q.From.Time().AddDate(0, 0, capacityDefaultDays-1))
	if raw := v.Get("to"); raw != "" {
		day, err := org.ParseDay(raw)
		if err != nil {
			fields["to"] = sentence(err.Error())
		}
		q.To = day
	}
	if len(fields) > 0 {
		respondError(w, r, s.Log, ErrValidation(fields))
		return
	}
	periods, err := s.Org.Capacity(r.Context(), q)
	switch {
	case errors.Is(err, org.ErrCapacityRange):
		respondError(w, r, s.Log, ErrValidation(map[string]string{"to": sentence(err.Error())}))
		return
	case errors.Is(err, org.ErrNotFound), errors.Is(err, org.ErrPositionNotFound), errors.Is(err, org.ErrRoleNotFound), errors.Is(err, org.ErrPersonNotFound):
		respondError(w, r, s.Log, ErrNotFound())
		return
	case err != nil:
		respondError(w, r, s.Log, err)
		return
	}
	out := capacityResponse{From: q.From, To: q.To, By: q.By, SubUnits: q.SubUnits && q.UnitID != nil, Periods: make([]capacityPeriodResponse, len(periods))}
	for i, p := range periods {
		out.Periods[i] = capacityPeriodResponse{From: p.From, To: p.To, FTE: p.FTE, Target: p.Target, Seats: p.Seats, Filled: p.Filled, Vacant: p.Vacant, People: p.People}
	}
	respondJSON(w, http.StatusOK, out)
}
