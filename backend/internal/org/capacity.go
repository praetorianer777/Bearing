package org

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/praetorianer777/bearing/backend/internal/db"
)

// MaxCapacityDays bounds a capacity range, so one request reads at most two years of days.
const MaxCapacityDays = 731

// Capacity is counted per day, or per week (from Monday) or month, a period being the mean of its working days in
// the range.
const (
	ByDay   = "day"
	ByWeek  = "week"
	ByMonth = "month"
)

var ErrCapacityRange = fmt.Errorf("give a range from a day to a day no earlier, at most %d days long", MaxCapacityDays)

// CapacityQuery is whose capacity, over which days, in which periods. Exactly one subject is named; SubUnits
// counts in the units inside a unit, as the tree stands on each day.
type CapacityQuery struct {
	UnitID, PositionID, RoleID, PersonID *uuid.UUID
	SubUnits                             bool
	From, To                             Day
	By                                   string
}

// CapacityPeriod is a subject's capacity over a period of the range: FTE is the full-time equivalents its holdings
// add up to, a holding counting its share of the person's week; Seats, Filled and Vacant are for positions and
// units, a position's filled seats being at most its seats; People is the most people holding on one day of it.
// Target is the mean of the subject's target on the days of the period that have one, nil when none has.
type CapacityPeriod struct {
	From, To Day
	FTE      float64
	Target   *float64
	Seats    *float64
	Filled   *float64
	Vacant   *float64
	People   int
}

type capacityDay struct {
	day                time.Time
	fte, seats, filled float64
	people             int
	target             *float64
}

// capacityDays are the days capacity is counted on: the range's first and every day in it on which something it
// counts starts or ends. Between two of them nothing changes, so the days between take the one before.
const capacityDays = `days AS (
    SELECT $1::date AS day
    UNION
    SELECT b.day FROM (
        SELECT valid_from AS day FROM holding UNION SELECT valid_to FROM holding
        UNION SELECT valid_from FROM unit_placement UNION SELECT valid_to FROM unit_placement
        UNION SELECT valid_from FROM position_role UNION SELECT valid_to FROM position_role
        UNION SELECT exists_from FROM position UNION SELECT archived_on FROM position
        UNION SELECT archived_on FROM unit UNION SELECT left_on FROM person
    ) b WHERE b.day > $1::date AND b.day <= $2::date
)`

// heldOn are the positions in reach on each day with the shares holding them, and what a day adds up to.
const heldOn = `
holds AS MATERIALIZED (
    SELECT r.day, r.position_id, h.person_id, h.share FROM reach r
    JOIN holding h ON h.position_id = r.position_id AND h.valid_from <= r.day AND (h.valid_to IS NULL OR h.valid_to > r.day)
    JOIN person p ON p.id = h.person_id AND (p.left_on IS NULL OR p.left_on > r.day)
),
held AS (
    SELECT r.day, r.seats, COALESCE(sum(holds.share), 0) / 100.0 AS fte
    FROM reach r LEFT JOIN holds ON holds.day = r.day AND holds.position_id = r.position_id
    GROUP BY r.day, r.position_id, r.seats
),
totals AS (
    SELECT day, sum(fte) AS fte, sum(seats) AS seats, sum(least(fte, seats)) AS filled FROM held GROUP BY day
),
people AS (SELECT day, count(DISTINCT person_id) AS n FROM holds GROUP BY day)
SELECT d.day, COALESCE(t.fte, 0)::float8, COALESCE(t.seats, 0)::float8, COALESCE(t.filled, 0)::float8, COALESCE(pe.n, 0)::int
FROM days d LEFT JOIN totals t ON t.day = d.day LEFT JOIN people pe ON pe.day = d.day
ORDER BY d.day`

const positionExists = `p.exists_from <= d.day AND (p.archived_on IS NULL OR p.archived_on > d.day)`

const positionCapacitySQL = `WITH ` + capacityDays + `,
reach AS (SELECT d.day, p.id AS position_id, p.seats FROM days d JOIN position p ON p.id = $3 AND ` + positionExists + `),` + heldOn

// The unit, and with $4 the units placed under it on each day, while they are not archived.
const unitCapacitySQL = `WITH RECURSIVE ` + capacityDays + `,
tree AS (
    SELECT d.day, u.id AS unit_id FROM days d JOIN unit u ON u.id = $3 AND (u.archived_on IS NULL OR u.archived_on > d.day)
    WHERE EXISTS (SELECT 1 FROM unit_placement pl WHERE pl.unit_id = u.id AND pl.valid_from <= d.day AND (pl.valid_to IS NULL OR pl.valid_to > d.day))
    UNION ALL
    SELECT t.day, pl.unit_id FROM tree t
    JOIN unit_placement pl ON $4::boolean AND pl.parent_id = t.unit_id AND pl.valid_from <= t.day AND (pl.valid_to IS NULL OR pl.valid_to > t.day)
    JOIN unit u ON u.id = pl.unit_id AND (u.archived_on IS NULL OR u.archived_on > t.day)
),
reach AS (SELECT d.day, p.id AS position_id, p.seats FROM tree d JOIN position p ON p.unit_id = d.unit_id AND ` + positionExists + `),` + heldOn

// A role's holdings are those of it directly and those of the positions carrying it on the day; each counts once.
const roleCapacitySQL = `WITH ` + capacityDays + `,
carriers AS (
    SELECT d.day, pr.position_id FROM days d
    JOIN position_role pr ON pr.role_id = $3 AND pr.valid_from <= d.day AND (pr.valid_to IS NULL OR pr.valid_to > d.day)
    JOIN position pos ON pos.id = pr.position_id AND pos.exists_from <= d.day AND (pos.archived_on IS NULL OR pos.archived_on > d.day)
),
held AS (
    SELECT d.day, h.person_id, h.share FROM days d
    JOIN holding h ON h.role_id = $3 AND h.valid_from <= d.day AND (h.valid_to IS NULL OR h.valid_to > d.day)
    UNION ALL
    SELECT c.day, h.person_id, h.share FROM carriers c
    JOIN holding h ON h.position_id = c.position_id AND h.valid_from <= c.day AND (h.valid_to IS NULL OR h.valid_to > c.day)
),
holds AS (SELECT held.* FROM held JOIN person p ON p.id = held.person_id AND (p.left_on IS NULL OR p.left_on > held.day))
SELECT d.day, COALESCE(sum(holds.share), 0)::float8 / 100.0, 0::float8, 0::float8, count(DISTINCT holds.person_id)::int
FROM days d LEFT JOIN holds ON holds.day = d.day
GROUP BY d.day ORDER BY d.day`

// A person's capacity is every holding of theirs, of a position or of a role directly, until they leave.
const personCapacitySQL = `WITH ` + capacityDays + `
SELECT d.day, COALESCE(sum(h.share), 0)::float8 / 100.0, 0::float8, 0::float8, (count(h.id) > 0)::int
FROM days d
LEFT JOIN person p ON p.id = $3 AND (p.left_on IS NULL OR p.left_on > d.day)
LEFT JOIN holding h ON h.person_id = p.id AND h.valid_from <= d.day AND (h.valid_to IS NULL OR h.valid_to > d.day)
GROUP BY d.day ORDER BY d.day`

// Capacity is a subject's capacity over a range, period by period.
func (s *Service) Capacity(ctx context.Context, q CapacityQuery) ([]CapacityPeriod, error) {
	if q.From.IsZero() || q.To.IsZero() || q.To.Before(q.From) || q.To.Time().Sub(q.From.Time()) >= MaxCapacityDays*24*time.Hour {
		return nil, ErrCapacityRange
	}
	var (
		sql, exists, targeted string
		id                    uuid.UUID
		missing               error
		seated                bool
	)
	switch {
	case q.UnitID != nil:
		sql, exists, id, missing, seated = unitCapacitySQL, `SELECT EXISTS (SELECT 1 FROM unit WHERE id = $1)`, *q.UnitID, ErrNotFound, true
		targeted = "unit_id"
	case q.PositionID != nil:
		sql, exists, id, missing, seated = positionCapacitySQL, `SELECT EXISTS (SELECT 1 FROM position WHERE id = $1)`, *q.PositionID, ErrPositionNotFound, true
		targeted = "position_id"
	case q.RoleID != nil:
		sql, exists, id, missing = roleCapacitySQL, `SELECT EXISTS (SELECT 1 FROM role WHERE id = $1)`, *q.RoleID, ErrRoleNotFound
		targeted = "role_id"
	case q.PersonID != nil:
		sql, exists, id, missing = personCapacitySQL, `SELECT EXISTS (SELECT 1 FROM person WHERE id = $1)`, *q.PersonID, ErrPersonNotFound
	default:
		return nil, errors.New("capacity names no subject")
	}
	args := []any{q.From.Time(), q.To.Time(), id}
	if q.UnitID != nil {
		args = append(args, q.SubUnits)
	}
	var (
		days    []capacityDay
		targets []Target
	)
	err := s.DB.Read(ctx, func(ctx context.Context, tx db.DBTX) error {
		var found bool
		if err := tx.QueryRow(ctx, exists, id).Scan(&found); err != nil {
			return err
		}
		if !found {
			return missing
		}
		rows, err := tx.Query(ctx, sql, args...)
		if err != nil {
			return err
		}
		days, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (capacityDay, error) {
			var d capacityDay
			err := row.Scan(&d.day, &d.fte, &d.seats, &d.filled, &d.people)
			return d, err
		})
		if err != nil || targeted == "" {
			return err
		}
		rows, err = tx.Query(ctx, `SELECT `+targetColumns+` FROM target WHERE `+targeted+` = $1
			AND valid_from <= $3 AND (valid_to IS NULL OR valid_to > $2) ORDER BY valid_from`, id, q.From.Time(), q.To.Time())
		if err != nil {
			return err
		}
		targets, err = pgx.CollectRows(rows, scanTarget)
		return err
	})
	if err != nil {
		return nil, err
	}
	every := everyDay(days, q.From, q.To)
	for i := range every {
		on := DayOf(every[i].day)
		for _, t := range targets {
			if !on.Before(t.From) && (t.To == nil || on.Before(*t.To)) {
				fte := t.FTE
				every[i].target = &fte
			}
		}
	}
	return periodsOf(every, q.By, seated), nil
}

// everyDay spreads what was counted on the days something changed over every day of the range, each day taking the
// last count on or before it.
func everyDay(changes []capacityDay, from, to Day) []capacityDay {
	out := make([]capacityDay, 0, int(to.Time().Sub(from.Time()).Hours()/24)+1)
	next := 0
	var last capacityDay
	for day := from.Time(); !day.After(to.Time()); day = day.AddDate(0, 0, 1) {
		for next < len(changes) && !changes[next].day.After(day) {
			last = changes[next]
			next++
		}
		last.day, last.target = day, nil
		out = append(out, last)
	}
	return out
}

// periodStart is the first day of the period a day falls in.
func periodStart(day time.Time, by string) time.Time {
	switch by {
	case ByWeek:
		return day.AddDate(0, 0, -((int(day.Weekday()) + 6) % 7))
	case ByMonth:
		return time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, time.UTC)
	}
	return day
}

func working(day time.Time) bool {
	return day.Weekday() != time.Saturday && day.Weekday() != time.Sunday
}

// periodsOf folds days into periods. A week or a month is the mean of its working days in the range, or of all its
// days in the range when none of them is a working day; a period cut by the range runs from and to the range's ends.
func periodsOf(days []capacityDay, by string, seated bool) []CapacityPeriod {
	out := []CapacityPeriod{}
	for start := 0; start < len(days); {
		key := periodStart(days[start].day, by)
		end := start
		for end < len(days) && periodStart(days[end].day, by).Equal(key) {
			end++
		}
		span := days[start:end]
		counted := []capacityDay{}
		for _, d := range span {
			if by == ByDay || working(d.day) {
				counted = append(counted, d)
			}
		}
		if len(counted) == 0 {
			counted = span
		}
		var fte, seats, filled, target float64
		people, targeted := 0, 0
		for _, d := range counted {
			fte += d.fte
			seats += d.seats
			filled += d.filled
			people = max(people, d.people)
			if d.target != nil {
				target += *d.target
				targeted++
			}
		}
		n := float64(len(counted))
		p := CapacityPeriod{From: DayOf(span[0].day), To: DayOf(span[len(span)-1].day), FTE: round2(fte / n), People: people}
		if targeted > 0 {
			mean := round2(target / float64(targeted))
			p.Target = &mean
		}
		if seated {
			s, f := round2(seats/n), round2(filled/n)
			v := round2(s - f)
			p.Seats, p.Filled, p.Vacant = &s, &f, &v
		}
		out = append(out, p)
		start = end
	}
	return out
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
