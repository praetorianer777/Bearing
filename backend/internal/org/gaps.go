package org

import (
	"cmp"
	"context"
	"math"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/db"
)

// The kinds of gap Bearing finds in its own data.
const (
	// GapVacant is a position with seats nobody fills; Size is the seats.
	GapVacant = "vacant"
	// GapUnderTarget and GapOverTarget are a subject with less or more than its target; Size is the difference in
	// full-time equivalents.
	GapUnderTarget = "under_target"
	GapOverTarget  = "over_target"
	// GapUnheld is a role nobody holds; Size is 0.
	GapUnheld = "unheld"
	// GapSingleHolder is a role one person holds, with nobody beside them; Size is 1.
	GapSingleHolder = "single_holder"
)

var gapOrder = []string{GapVacant, GapUnderTarget, GapOverTarget, GapUnheld, GapSingleHolder}

// GapQuery is where and when to look: the whole organisation, or a unit and with SubUnits the units placed under it
// on some day of the range.
type GapQuery struct {
	UnitID   *uuid.UUID
	SubUnits bool
	From, To Day
}

// Gap is one gap over a run of days with the same size: from From to To, both counted. A position's UnitID is its
// unit's; a unit's is itself.
type Gap struct {
	Kind        string
	SubjectKind string
	SubjectID   uuid.UUID
	Name        string
	UnitID      *uuid.UUID
	From, To    Day
	Size        float64
}

// The positions in reach on each day the counts change, of the units in reach on some day of the range; a position
// that does not exist on a day has no seats then.
const positionSeatsSQL = `WITH RECURSIVE ` + capacityDays + `,
scope AS (
    SELECT id AS unit_id FROM unit WHERE $3::uuid IS NULL OR id = $3
    UNION
    SELECT pl.unit_id FROM scope s
    JOIN unit_placement pl ON $3::uuid IS NOT NULL AND $4::boolean AND pl.parent_id = s.unit_id
         AND pl.valid_from <= $2::date AND (pl.valid_to IS NULL OR pl.valid_to > $1::date)
),
positions AS (
    SELECT p.id, p.title, p.unit_id, p.seats, p.exists_from, p.archived_on FROM position p
    WHERE p.unit_id IN (SELECT unit_id FROM scope) AND p.exists_from <= $2::date AND (p.archived_on IS NULL OR p.archived_on > $1::date)
)
SELECT p.id, p.title, p.unit_id, d.day,
       CASE WHEN p.exists_from <= d.day AND (p.archived_on IS NULL OR p.archived_on > d.day) THEN p.seats ELSE 0 END::float8,
       (SELECT COALESCE(sum(h.share), 0) FROM holding h JOIN person pe ON pe.id = h.person_id
        WHERE h.position_id = p.id AND h.valid_from <= d.day AND (h.valid_to IS NULL OR h.valid_to > d.day)
          AND (pe.left_on IS NULL OR pe.left_on > d.day))::float8 / 100.0
FROM positions p CROSS JOIN days d
ORDER BY p.id, d.day`

// Gaps finds where people are missing, or too many, over a range, earliest first.
func (s *Service) Gaps(ctx context.Context, q GapQuery) ([]Gap, error) {
	if q.From.IsZero() || q.To.IsZero() || q.To.Before(q.From) || q.To.Time().Sub(q.From.Time()) >= MaxCapacityDays*24*time.Hour {
		return nil, ErrCapacityRange
	}
	type seated struct {
		title  string
		unitID uuid.UUID
		days   []capacityDay
	}
	positions := map[uuid.UUID]*seated{}
	var order []uuid.UUID
	type subject struct {
		kind, column string
		id           uuid.UUID
		name         string
		unitID       *uuid.UUID
	}
	var targeted, roles []subject
	err := s.DB.Read(ctx, func(ctx context.Context, tx db.DBTX) error {
		if q.UnitID != nil {
			var found bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM unit WHERE id = $1)`, *q.UnitID).Scan(&found); err != nil {
				return err
			}
			if !found {
				return ErrNotFound
			}
		}
		rows, err := tx.Query(ctx, positionSeatsSQL, q.From.Time(), q.To.Time(), q.UnitID, q.SubUnits)
		if err != nil {
			return err
		}
		for rows.Next() {
			var (
				id, unitID uuid.UUID
				title      string
				d          capacityDay
			)
			if err := rows.Scan(&id, &title, &unitID, &d.day, &d.seats, &d.fte); err != nil {
				rows.Close()
				return err
			}
			d.filled = math.Min(d.fte, d.seats)
			p, ok := positions[id]
			if !ok {
				p = &seated{title: title, unitID: unitID}
				positions[id] = p
				order = append(order, id)
			}
			p.days = append(p.days, d)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		// What has a target over the range, and the roles the positions in reach carry then, or every role.
		ids := order
		rows, err = tx.Query(ctx, `
			WITH RECURSIVE scope AS (
			    SELECT id AS unit_id FROM unit WHERE $3::uuid IS NULL OR id = $3
			    UNION
			    SELECT pl.unit_id FROM scope s
			    JOIN unit_placement pl ON $3::uuid IS NOT NULL AND $4::boolean AND pl.parent_id = s.unit_id
			         AND pl.valid_from <= $2::date AND (pl.valid_to IS NULL OR pl.valid_to > $1::date)
			), reached AS (
			    SELECT r.id FROM role r WHERE $3::uuid IS NULL
			    UNION
			    SELECT pr.role_id FROM position_role pr
			    WHERE pr.position_id = ANY($5) AND pr.valid_from <= $2::date AND (pr.valid_to IS NULL OR pr.valid_to > $1::date)
			)
			SELECT 'unit', u.id, u.name, u.id, EXISTS (SELECT 1 FROM target t WHERE t.unit_id = u.id
			       AND t.valid_from <= $2::date AND (t.valid_to IS NULL OR t.valid_to > $1::date))
			FROM unit u WHERE u.id IN (SELECT unit_id FROM scope)
			UNION ALL
			SELECT 'position', p.id, p.title, p.unit_id, EXISTS (SELECT 1 FROM target t WHERE t.position_id = p.id
			       AND t.valid_from <= $2::date AND (t.valid_to IS NULL OR t.valid_to > $1::date))
			FROM position p WHERE p.id = ANY($5)
			UNION ALL
			SELECT 'role', r.id, r.name, NULL::uuid, EXISTS (SELECT 1 FROM target t WHERE t.role_id = r.id
			       AND t.valid_from <= $2::date AND (t.valid_to IS NULL OR t.valid_to > $1::date))
			FROM role r WHERE r.id IN (SELECT id FROM reached)`, q.From.Time(), q.To.Time(), q.UnitID, q.SubUnits, ids)
		if err != nil {
			return err
		}
		for rows.Next() {
			var sub subject
			var hasTarget bool
			if err := rows.Scan(&sub.kind, &sub.id, &sub.name, &sub.unitID, &hasTarget); err != nil {
				rows.Close()
				return err
			}
			if hasTarget {
				targeted = append(targeted, sub)
			}
			if sub.kind == "role" {
				roles = append(roles, sub)
			}
		}
		rows.Close()
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}

	out := []Gap{}
	for _, id := range order {
		p := positions[id]
		unitID := p.unitID
		for _, run := range runs(everyDay(p.days, q.From, q.To), func(d capacityDay) float64 { return d.seats - d.filled }) {
			if run.size > 0 {
				out = append(out, Gap{Kind: GapVacant, SubjectKind: "position", SubjectID: id, Name: p.title, UnitID: &unitID, From: run.from, To: run.to, Size: run.size})
			}
		}
	}
	for _, sub := range targeted {
		cq := CapacityQuery{From: q.From, To: q.To, By: ByDay, SubUnits: true}
		switch sub.kind {
		case "unit":
			cq.UnitID = &sub.id
		case "position":
			cq.PositionID = &sub.id
		default:
			cq.RoleID = &sub.id
		}
		periods, err := s.Capacity(ctx, cq)
		if err != nil {
			return nil, err
		}
		for _, run := range periodRuns(periods, func(p CapacityPeriod) float64 {
			if p.Target == nil {
				return 0
			}
			return *p.Target - p.FTE
		}) {
			kind, size := GapUnderTarget, run.size
			if size < 0 {
				kind, size = GapOverTarget, -size
			}
			if size > 0 {
				out = append(out, Gap{Kind: kind, SubjectKind: sub.kind, SubjectID: sub.id, Name: sub.name, UnitID: sub.unitID, From: run.from, To: run.to, Size: size})
			}
		}
	}
	for _, sub := range roles {
		periods, err := s.Capacity(ctx, CapacityQuery{RoleID: &sub.id, From: q.From, To: q.To, By: ByDay})
		if err != nil {
			return nil, err
		}
		// 1 is nobody, 2 one person alone, 0 enough people; a run is days of one of them.
		for _, run := range periodRuns(periods, func(p CapacityPeriod) float64 {
			switch p.People {
			case 0:
				return 1
			case 1:
				return 2
			}
			return 0
		}) {
			switch run.size {
			case 1:
				out = append(out, Gap{Kind: GapUnheld, SubjectKind: "role", SubjectID: sub.id, Name: sub.name, From: run.from, To: run.to})
			case 2:
				out = append(out, Gap{Kind: GapSingleHolder, SubjectKind: "role", SubjectID: sub.id, Name: sub.name, From: run.from, To: run.to, Size: 1})
			}
		}
	}
	slices.SortStableFunc(out, func(a, b Gap) int {
		return cmp.Or(a.From.Time().Compare(b.From.Time()), cmp.Compare(slices.Index(gapOrder, a.Kind), slices.Index(gapOrder, b.Kind)),
			cmp.Compare(a.Name, b.Name), cmp.Compare(a.SubjectID.String(), b.SubjectID.String()))
	})
	return out, nil
}

type run struct {
	from, to Day
	size     float64
}

// runs are the stretches of days over which a measure keeps one value, each with that value rounded to two decimals.
func runs(days []capacityDay, measure func(capacityDay) float64) []run {
	var out []run
	for _, d := range days {
		day, size := DayOf(d.day), round2(measure(d))
		if n := len(out); n > 0 && out[n-1].size == size {
			out[n-1].to = day
			continue
		}
		out = append(out, run{from: day, to: day, size: size})
	}
	return out
}

// periodRuns are runs over periods counted by day.
func periodRuns(periods []CapacityPeriod, measure func(CapacityPeriod) float64) []run {
	var out []run
	for _, p := range periods {
		size := round2(measure(p))
		if n := len(out); n > 0 && out[n-1].size == size {
			out[n-1].to = p.To
			continue
		}
		out = append(out, run{from: p.From, to: p.To, size: size})
	}
	return out
}
