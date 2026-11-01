package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/armature"
	"github.com/praetorianer777/bearing/backend/internal/auth"
	"github.com/praetorianer777/bearing/backend/internal/org"
)

type absenceGapResponse struct {
	// SubjectKind is position, role or unit: what has nobody there.
	SubjectKind string    `json:"subjectKind"`
	SubjectID   uuid.UUID `json:"subjectId"`
	Name        string    `json:"name"`
	// StepKey, StepName and Kind name the step of a process the subject answers for, and how; only for a process.
	StepKey  string   `json:"stepKey,omitempty"`
	StepName string   `json:"stepName,omitempty"`
	Kind     org.RACI `json:"kind,omitempty"`
	// From and To are the first and last working day of a run with nobody there; Days counts its working days.
	From org.Day `json:"from"`
	To   org.Day `json:"to"`
	Days int     `json:"days"`
}

type absencesResponse struct {
	From    org.Day         `json:"from"`
	To      org.Day         `json:"to"`
	Partner partnerResponse `json:"partner"`
	// Hidden says the caller may not see them: who is away is for editors, as a person's load is.
	Hidden bool                 `json:"hidden"`
	Gaps   []absenceGapResponse `json:"gaps"`
}

const absenceDescription = "from and to are days written 2006-01-02, this week's Monday and 13 weeks on when left out, at most 26 weeks. " +
	"A working day has nobody there when every holder and every deputy is away, as Armature's resource-load@1 says, read as the caller; " +
	"somebody Armature does not know counts as there, and a subject nobody holds is a vacancy, not this. Absences never carry a reason. " +
	"Only editors see them; for anybody else hidden is true."

// absenceSubject is one thing that should have somebody there.
type absenceSubject struct {
	who      org.Responsibility
	kind     string
	id       uuid.UUID
	name     string
	stepKey  string
	stepName string
	raci     org.RACI
}

// workingDays are the days from from to to, both counted, but Saturdays and Sundays.
func workingDays(from, to time.Time) []time.Time {
	var out []time.Time
	for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
		if day.Weekday() != time.Saturday && day.Weekday() != time.Sunday {
			out = append(out, day)
		}
	}
	return out
}

// nobodyThere are, per subject, the runs of working days when it had holders and every holder and deputy was away.
// covers[d][i] is subject i on day d.
func nobodyThere(days []time.Time, covers [][]org.Cover, away func(org.Resolved, time.Time) bool) [][][2]int {
	if len(covers) == 0 {
		return nil
	}
	out := make([][][2]int, len(covers[0]))
	for i := range out {
		start := -1
		for d := range days {
			c := covers[d][i]
			gap := len(c.Holders) > 0
			for _, p := range append(append([]org.Resolved(nil), c.Holders...), c.Deputies...) {
				if gap && !away(p, days[d]) {
					gap = false
				}
			}
			switch {
			case gap && start < 0:
				start = d
			case !gap && start >= 0:
				out[i] = append(out[i], [2]int{start, d - 1})
				start = -1
			}
		}
		if start >= 0 {
			out[i] = append(out[i], [2]int{start, len(days) - 1})
		}
	}
	return out
}

// respondAbsences answers when each subject has nobody there over the range the query asks for.
func (s *Server) respondAbsences(w http.ResponseWriter, r *http.Request, from, to org.Day, subjects []absenceSubject) {
	ctx := r.Context()
	out := absencesResponse{From: from, To: to, Gaps: []absenceGapResponse{}}
	if p := PrincipalFrom(ctx); p == nil || !p.Account.Role.AtLeast(auth.RoleEditor) {
		out.Hidden, out.Partner = true, partnerResponse{Status: "ok"}
		respondJSON(w, http.StatusOK, out)
		return
	}
	days := workingDays(from.Time(), to.Time())
	asked := make([]org.Responsibility, len(subjects))
	for i, sub := range subjects {
		asked[i] = sub.who
	}
	covers := make([][]org.Cover, len(days))
	var people []armature.Subject
	seen := map[armature.Subject]bool{}
	for d, day := range days {
		c, err := s.Org.Covers(ctx, org.DayOf(day), asked)
		if err != nil {
			respondError(w, r, s.Log, err)
			return
		}
		covers[d] = c
		for _, each := range c {
			for _, p := range append(append([]org.Resolved(nil), each.Holders...), each.Deputies...) {
				if p.Issuer != nil && p.Subject != nil {
					if sub := (armature.Subject{Issuer: *p.Issuer, Subject: *p.Subject}); !seen[sub] {
						seen[sub] = true
						people = append(people, sub)
					}
				}
			}
		}
	}
	if s.Armature == nil {
		out.Partner = partnerOf(armature.ErrNotConfigured)
		respondJSON(w, http.StatusOK, out)
		return
	}
	loads, err := s.Armature.ResourceLoad(ctx, armature.LoadQuery{From: from.String(), To: to.String(), People: people})
	out.Partner = partnerOf(err)
	if err != nil {
		respondJSON(w, http.StatusOK, out)
		return
	}
	awayOn := map[armature.Subject]map[string]bool{}
	for _, l := range loads {
		days := map[string]bool{}
		for _, week := range l.Weeks {
			for _, d := range week.Away {
				days[d] = true
			}
		}
		awayOn[l.Subject] = days
	}
	away := func(p org.Resolved, day time.Time) bool {
		if p.Issuer == nil || p.Subject == nil {
			return false
		}
		return awayOn[armature.Subject{Issuer: *p.Issuer, Subject: *p.Subject}][day.Format(time.DateOnly)]
	}
	for i, runs := range nobodyThere(days, covers, away) {
		sub := subjects[i]
		for _, run := range runs {
			out.Gaps = append(out.Gaps, absenceGapResponse{SubjectKind: sub.kind, SubjectID: sub.id, Name: sub.name, StepKey: sub.stepKey,
				StepName: sub.stepName, Kind: sub.raci, From: org.DayOf(days[run[0]]), To: org.DayOf(days[run[1]]), Days: run[1] - run[0] + 1})
		}
	}
	respondJSON(w, http.StatusOK, out)
}

// absenceRange reads from and to, this week's Monday and 13 weeks on by default, refusing more than 26 weeks.
func (s *Server) absenceRange(w http.ResponseWriter, r *http.Request) (org.Day, org.Day, bool) {
	fields := map[string]string{}
	from, to := readRange(r, fields)
	if r.URL.Query().Get("from") == "" {
		from = org.DayOf(monday(from.Time()))
	}
	if r.URL.Query().Get("to") == "" {
		to = org.DayOf(from.Time().AddDate(0, 0, 7*demandDefaultWeeks-1))
	}
	if len(fields) == 0 && (to.Before(from) || to.Time().Sub(from.Time()) >= demandMaxWeeks*7*24*time.Hour) {
		fields["to"] = "Show at most 26 weeks, ending after they start."
	}
	if len(fields) > 0 {
		respondError(w, r, s.Log, ErrValidation(fields))
		return org.Day{}, org.Day{}, false
	}
	return from, to, true
}

func (s *Server) handleRoleAbsences(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "roleID")
	if !ok {
		return
	}
	from, to, ok := s.absenceRange(w, r)
	if !ok {
		return
	}
	role, err := s.Org.Role(r.Context(), id, from)
	if err != nil {
		respondError(w, r, s.Log, roleError(err))
		return
	}
	s.respondAbsences(w, r, from, to, []absenceSubject{{who: org.Responsibility{RoleID: &id}, kind: "role", id: id, name: role.Name}})
}

// partyName is the name of the role, position or unit a responsibility names.
func (s *Server) partyName(r *http.Request, who org.Responsibility, day org.Day) (string, uuid.UUID, string, error) {
	ctx := r.Context()
	switch {
	case who.RoleID != nil:
		role, err := s.Org.Role(ctx, *who.RoleID, day)
		return "role", *who.RoleID, role.Name, err
	case who.PositionID != nil:
		p, err := s.Org.Position(ctx, *who.PositionID, day)
		return "position", *who.PositionID, p.Title, err
	default:
		u, err := s.Org.Unit(ctx, *who.UnitID, day)
		return "unit", *who.UnitID, u.Name, err
	}
}

func (s *Server) handleProcessAbsences(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "processID")
	if !ok {
		return
	}
	from, to, ok := s.absenceRange(w, r)
	if !ok {
		return
	}
	var version *int
	if raw := r.URL.Query().Get("version"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			respondError(w, r, s.Log, ErrValidation(map[string]string{"version": "Name a published version by its number."}))
			return
		}
		version = &n
	}
	people, err := s.Org.ProcessPeople(r.Context(), id, version, false, from)
	if err != nil {
		respondError(w, r, s.Log, processError(err))
		return
	}
	var subjects []absenceSubject
	for _, step := range people.Steps {
		for _, res := range step.Responsibilities {
			if res.Kind != org.Responsible && res.Kind != org.Accountable {
				continue
			}
			kind, party, name, err := s.partyName(r, res.Responsibility, from)
			if err != nil {
				respondError(w, r, s.Log, orgError(err))
				return
			}
			subjects = append(subjects, absenceSubject{who: res.Responsibility, kind: kind, id: party, name: name, stepKey: step.Key, stepName: step.Name, raci: res.Kind})
		}
	}
	s.respondAbsences(w, r, from, to, subjects)
}

func (s *Server) handleAbsences(w http.ResponseWriter, r *http.Request) {
	from, to, ok := s.absenceRange(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	var roots []uuid.UUID
	subUnits := true
	if raw := r.URL.Query().Get("unitId"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			respondError(w, r, s.Log, ErrValidation(map[string]string{"unitId": "Name a unit by its id."}))
			return
		}
		roots, subUnits = []uuid.UUID{id}, r.URL.Query().Get("subUnits") == "true"
	} else {
		units, err := s.Org.Units(ctx, from)
		if err != nil {
			respondError(w, r, s.Log, err)
			return
		}
		for _, u := range units {
			if u.ParentID == nil {
				roots = append(roots, u.ID)
			}
		}
	}
	var subjects []absenceSubject
	seen := map[uuid.UUID]bool{}
	for _, root := range roots {
		members, err := s.Org.Members(ctx, root, from, subUnits)
		if err != nil {
			respondError(w, r, s.Log, orgError(err))
			return
		}
		for _, m := range members {
			if !seen[m.PositionID] {
				seen[m.PositionID] = true
				position := m.PositionID
				subjects = append(subjects, absenceSubject{who: org.Responsibility{PositionID: &position}, kind: "position", id: position, name: m.Title})
			}
		}
	}
	s.respondAbsences(w, r, from, to, subjects)
}
