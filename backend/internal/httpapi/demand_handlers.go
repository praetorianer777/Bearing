package httpapi

import (
	"errors"
	"math"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/armature"
	"github.com/praetorianer777/bearing/backend/internal/auth"
	"github.com/praetorianer777/bearing/backend/internal/org"
)

const (
	// demandDefaultWeeks is how many weeks a demand view shows when to is left out: thirteen, as capacity does.
	demandDefaultWeeks = 13
	// demandMaxWeeks is the most weeks one view asks Armature for, as Armature plans at most 26 at a time.
	demandMaxWeeks = 26
)

type partnerResponse struct {
	// Status is ok, not_configured, not_connected, rejected, unreachable or outdated.
	Status string `json:"status"`
	// Message says what to do when it is not ok; it names the partner to update when a capability is missing.
	Message string `json:"message,omitempty"`
}

type demandWeekResponse struct {
	// Start is the week's Monday.
	Start  org.Day  `json:"start"`
	FTE    float64  `json:"fte"`
	Target *float64 `json:"target"`
	// People are those holding positions in the unit at the week's start.
	People int `json:"people"`
	// AllocatedFTE, LoadHours, NominalHours and DaysAway are what Armature counts for those people, each at their share
	// in the unit; null without Armature.
	AllocatedFTE *float64 `json:"allocatedFte"`
	LoadHours    *float64 `json:"loadHours"`
	NominalHours *float64 `json:"nominalHours"`
	DaysAway     *float64 `json:"daysAway"`
}

type overResponse struct {
	PersonID uuid.UUID `json:"personId"`
	Name     string    `json:"name"`
	Week     org.Day   `json:"week"`
	// Reason is over_allocated, projects giving out more than the whole week, or over_loaded, the work estimated
	// beyond the person's share in the unit.
	Reason            string  `json:"reason"`
	AllocationPercent float64 `json:"allocationPercent"`
	LoadHours         float64 `json:"loadHours"`
	NominalHours      float64 `json:"nominalHours"`
	Share             int     `json:"share"`
}

type demandResponse struct {
	From     org.Day              `json:"from"`
	To       org.Day              `json:"to"`
	SubUnits bool                 `json:"subUnits"`
	Partner  partnerResponse      `json:"partner"`
	Weeks    []demandWeekResponse `json:"weeks"`
	// Over are the people allocated or loaded beyond what they have, for editors; Warnings counts them for everybody.
	Over     []overResponse `json:"over"`
	Warnings int            `json:"warnings"`
	// Unmatched are the people Armature does not know, or who have no realm name to ask it by, for editors.
	Unmatched []memberResponse `json:"unmatched"`
}

const demandDescription = "from and to are days written 2006-01-02; the view starts on the Monday of from, this week's when left out, " +
	"and shows 13 weeks, at most 26. Capacity and targets are Bearing's; allocations, load and days away are Armature's resource-load@1, " +
	"read as the caller, each person counting at their share in the unit. Without Armature, or without the capability, partner says why and " +
	"the demand stays null. People allocated over 100 percent, or loaded beyond their share, are named to editors and counted for everybody."

// monday is the Monday of the week holding day.
func monday(day time.Time) time.Time {
	return day.AddDate(0, 0, -((int(day.Weekday()) + 6) % 7))
}

// partnerOf says a refusal of Armature's in the view's terms; err is nil for ok.
func partnerOf(err error) partnerResponse {
	var outdated *armature.OutdatedError
	status := "unreachable"
	switch {
	case err == nil:
		return partnerResponse{Status: "ok"}
	case errors.As(err, &outdated):
		status = "outdated"
	case errors.Is(err, armature.ErrNotConfigured):
		status = "not_configured"
	case errors.Is(err, armature.ErrNotConnected):
		status = "not_connected"
	case errors.Is(err, armature.ErrRejected):
		status = "rejected"
	}
	return partnerResponse{Status: status, Message: sentence(err.Error())}
}

// weekPeople are the people in reach in one week, by person, with their share in the unit.
type weekPeople map[uuid.UUID]org.Member

// demandOf joins the unit's weeks with Armature's: each person counts at their share in the unit. loads is nil
// without Armature.
func demandOf(weeks []demandWeekResponse, people []weekPeople, loads map[armature.Subject]armature.PersonLoad) ([]demandWeekResponse, []overResponse, []memberResponse) {
	var over []overResponse
	unmatched := []memberResponse{}
	seen := map[uuid.UUID]bool{}
	round := func(v float64) *float64 { r := math.Round(v*100) / 100; return &r }
	for i := range weeks {
		weeks[i].People = len(people[i])
		if loads == nil {
			continue
		}
		var allocated, load, nominal, away float64
		start := weeks[i].Start.String()
		for _, m := range people[i] {
			var found *armature.Week
			if m.Issuer != nil && m.Subject != nil {
				if l, ok := loads[armature.Subject{Issuer: *m.Issuer, Subject: *m.Subject}]; ok && l.Found {
					for j := range l.Weeks {
						if l.Weeks[j].Start == start {
							found = &l.Weeks[j]
						}
					}
					if found == nil {
						continue
					}
				}
			}
			if found == nil {
				if !seen[m.PersonID] {
					seen[m.PersonID] = true
					unmatched = append(unmatched, memberResponse(m))
				}
				continue
			}
			part := float64(m.Share) / 100
			allocated += found.AllocationPercent / 100 * part
			load += found.LoadHours * part
			nominal += found.NominalHours * part
			away += found.DaysAway * part
			// A small tolerance, so hours rounded at Armature do not warn by themselves.
			switch {
			case found.AllocationPercent > 100:
				over = append(over, overResponse{PersonID: m.PersonID, Name: m.Name, Week: weeks[i].Start, Reason: "over_allocated",
					AllocationPercent: found.AllocationPercent, LoadHours: found.LoadHours, NominalHours: found.NominalHours, Share: m.Share})
			case found.LoadHours > found.NominalHours*part+0.01:
				over = append(over, overResponse{PersonID: m.PersonID, Name: m.Name, Week: weeks[i].Start, Reason: "over_loaded",
					AllocationPercent: found.AllocationPercent, LoadHours: found.LoadHours, NominalHours: found.NominalHours, Share: m.Share})
			}
		}
		weeks[i].AllocatedFTE, weeks[i].LoadHours, weeks[i].NominalHours, weeks[i].DaysAway = round(allocated), round(load), round(nominal), round(away)
	}
	return weeks, over, unmatched
}

func (s *Server) handleUnitDemand(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "unitID")
	if !ok {
		return
	}
	fields := map[string]string{}
	from, to := readRange(r, fields)
	if r.URL.Query().Get("to") == "" {
		to = org.DayOf(monday(from.Time()).AddDate(0, 0, 7*demandDefaultWeeks-1))
	}
	start := monday(from.Time())
	if len(fields) == 0 && (to.Before(from) || to.Time().Sub(start) >= demandMaxWeeks*7*24*time.Hour) {
		fields["to"] = "Show at most 26 weeks, ending after they start."
	}
	if len(fields) > 0 {
		respondError(w, r, s.Log, ErrValidation(fields))
		return
	}
	subUnits := r.URL.Query().Get("subUnits") == "true"
	from = org.DayOf(start)
	ctx := r.Context()
	periods, err := s.Org.Capacity(ctx, org.CapacityQuery{UnitID: &id, SubUnits: subUnits, From: from, To: to, By: org.ByWeek})
	if err != nil {
		respondError(w, r, s.Log, capacityError(err))
		return
	}
	weeks := make([]demandWeekResponse, len(periods))
	people := make([]weekPeople, len(periods))
	var asked []armature.Subject
	seen := map[armature.Subject]bool{}
	for i, p := range periods {
		weeks[i] = demandWeekResponse{Start: p.From, FTE: p.FTE, Target: p.Target}
		members, err := s.Org.Members(ctx, id, p.From, subUnits)
		if err != nil {
			respondError(w, r, s.Log, orgError(err))
			return
		}
		people[i] = weekPeople{}
		for _, m := range members {
			if had, ok := people[i][m.PersonID]; ok {
				m.Share += had.Share
			}
			people[i][m.PersonID] = m
			if m.Issuer != nil && m.Subject != nil {
				if sub := (armature.Subject{Issuer: *m.Issuer, Subject: *m.Subject}); !seen[sub] {
					seen[sub] = true
					asked = append(asked, sub)
				}
			}
		}
	}
	out := demandResponse{From: from, To: to, SubUnits: subUnits, Over: []overResponse{}, Unmatched: []memberResponse{}}
	var loads map[armature.Subject]armature.PersonLoad
	if s.Armature == nil {
		out.Partner = partnerOf(armature.ErrNotConfigured)
	} else {
		found, err := s.Armature.ResourceLoad(ctx, armature.LoadQuery{From: from.String(), To: to.String(), People: asked})
		out.Partner = partnerOf(err)
		if err == nil {
			loads = map[armature.Subject]armature.PersonLoad{}
			for _, l := range found {
				loads[l.Subject] = l
			}
		}
	}
	var over []overResponse
	var unmatched []memberResponse
	out.Weeks, over, unmatched = demandOf(weeks, people, loads)
	out.Warnings = len(over)
	if p := PrincipalFrom(ctx); p != nil && p.Account.Role.AtLeast(auth.RoleEditor) {
		if over != nil {
			out.Over = over
		}
		out.Unmatched = unmatched
	}
	respondJSON(w, http.StatusOK, out)
}

type armatureStatusResponse struct {
	// Status is not_configured, not_connected or connected.
	Status string `json:"status"`
}

func (s *Server) handleArmatureStatus(w http.ResponseWriter, r *http.Request) {
	status := "not_configured"
	if s.Armature != nil {
		var err error
		if status, err = s.Armature.Status(r.Context()); err != nil {
			respondError(w, r, s.Log, err)
			return
		}
	}
	respondJSON(w, http.StatusOK, armatureStatusResponse{Status: status})
}

func (s *Server) handleForgetArmature(w http.ResponseWriter, r *http.Request) {
	if s.Armature != nil {
		lsn, err := s.Armature.Forget(r.Context())
		noteWrite(r.Context(), lsn)
		if err != nil {
			respondError(w, r, s.Log, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// armatureError says a refusal about the connection in the API's terms.
func armatureError(err error) error {
	var field *armature.FieldError
	switch {
	case errors.As(err, &field):
		return ErrValidation(map[string]string{field.Field: field.Message})
	case errors.Is(err, armature.ErrNoKey):
		return &APIError{Status: http.StatusServiceUnavailable, Code: "no_secret_key", Message: sentence(err.Error())}
	}
	return err
}

func (s *Server) handleArmatureConnection(w http.ResponseWriter, r *http.Request) {
	var c *armature.Connection
	if s.Armature != nil {
		var err error
		if c, err = s.Armature.Connection(r.Context()); err != nil {
			respondError(w, r, s.Log, err)
			return
		}
	}
	respondJSON(w, http.StatusOK, env{"connection": c})
}

func (s *Server) handleConnectArmature(w http.ResponseWriter, r *http.Request) {
	var in armature.ConnectionInput
	if err := decode(r, &in); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	if s.Armature == nil {
		respondError(w, r, s.Log, armatureError(armature.ErrNoKey))
		return
	}
	c, lsn, err := s.Armature.Connect(r.Context(), in)
	noteWrite(r.Context(), lsn)
	if err != nil {
		respondError(w, r, s.Log, armatureError(err))
		return
	}
	respondJSON(w, http.StatusOK, env{"connection": c})
}

func (s *Server) handleDisconnectArmature(w http.ResponseWriter, r *http.Request) {
	if s.Armature != nil {
		lsn, err := s.Armature.Disconnect(r.Context())
		noteWrite(r.Context(), lsn)
		if err != nil {
			respondError(w, r, s.Log, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
