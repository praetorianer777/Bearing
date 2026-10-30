package org

import (
	"context"
	"errors"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/praetorianer777/bearing/backend/internal/db"
)

// LineKind is how a position reports: solid is disciplinary, dotted functional.
type LineKind string

const (
	Solid  LineKind = "solid"
	Dotted LineKind = "dotted"
)

// Position is a slot in a unit that people hold, as it stands on one day.
type Position struct {
	ID          uuid.UUID
	UnitID      uuid.UUID
	Title       string
	Description string
	HeadOfUnit  bool
	// Seats is how many people may hold it at once.
	Seats      int
	ExistsFrom Day
	ArchivedOn *Day
	// ReportsTo is the position it reports to by a solid line on the day asked about.
	ReportsTo *uuid.UUID
	// DottedTo are the positions it reports to by dotted lines on that day.
	DottedTo []uuid.UUID
	// Holders are the holdings of it on that day; none means it is vacant then.
	Holders []Holding
	// Roles are the roles it carries on that day.
	Roles []uuid.UUID
	// Deputies are the positions standing in for it, first first, those that exist on that day.
	Deputies []uuid.UUID
	// Lines are every line it has had and will have, oldest first; filled when one position is asked for.
	Lines []Line
}

// Line is a reporting line from a position, from a day until a day or for good.
type Line struct {
	ReportsTo uuid.UUID
	Kind      LineKind
	From      Day
	// To is the first day it no longer holds.
	To *Day
}

// PositionInput is what a position is made or changed with; a nil field is left as it is.
type PositionInput struct {
	Title       *string
	Description *string
	HeadOfUnit  *bool
	Seats       *int
}

var (
	ErrPositionNotFound    = errors.New("there is no such position")
	ErrUnitNotThere        = errors.New("the unit does not exist on that day")
	ErrUnitGone            = errors.New("the unit is archived later, and the position would outlive it")
	ErrPositionNotThere    = errors.New("the position does not exist on that day")
	ErrPositionArchived    = errors.New("the position is archived; restore it first")
	ErrPositionNotArchived = errors.New("the position is not archived")
	ErrTargetNotThere      = errors.New("the position it would report to does not exist for all of that time")
	ErrReportsToItself     = errors.New("that line would have the position report to itself, through others or directly")
	ErrLineExists          = errors.New("the position already reports to that one by a dotted line then")
	ErrLineNotFound        = errors.New("the position does not report to that one by a dotted line on that day")
	ErrHasReports          = errors.New("positions still report to it on that day or after; give them another line first")
	ErrHeadTaken           = errors.New("the unit has a head already")
	ErrHasPositions        = errors.New("positions in it still exist on that day or after; archive them first")
	ErrTooFewSeats         = errors.New("more people hold the position from today on than it would have seats")
)

func (in *PositionInput) check(creating bool) error {
	if creating && in.Title == nil {
		return Invalid{"title", "Give a title."}
	}
	if err := checkText("title", in.Title, 1, 200); err != nil {
		return err
	}
	if in.Description != nil && utf8.RuneCountInString(*in.Description) > 4000 {
		return Invalid{"description", "Keep it to 4000 characters."}
	}
	if in.Seats != nil && (*in.Seats < 1 || *in.Seats > 1000) {
		return Invalid{"seats", "Give from 1 to 1000 seats."}
	}
	return nil
}

const positionColumns = `p.id, p.unit_id, p.title, p.description, p.head_of_unit, p.seats, p.exists_from, p.archived_on,
	(SELECT l.reports_to_id FROM reporting_line l WHERE l.position_id = p.id AND l.kind = 'solid'
		AND l.valid_from <= $1 AND (l.valid_to IS NULL OR l.valid_to > $1)),
	coalesce((SELECT array_agg(l.reports_to_id ORDER BY l.reports_to_id) FROM reporting_line l WHERE l.position_id = p.id
		AND l.kind = 'dotted' AND l.valid_from <= $1 AND (l.valid_to IS NULL OR l.valid_to > $1)), '{}'),
	coalesce((SELECT array_agg(r.role_id ORDER BY r.role_id) FROM position_role r WHERE r.position_id = p.id
		AND r.valid_from <= $1 AND (r.valid_to IS NULL OR r.valid_to > $1)), '{}'),
	coalesce((SELECT array_agg(d.deputy_id ORDER BY d.rank) FROM deputy d JOIN position x ON x.id = d.deputy_id
		WHERE d.position_id = p.id AND x.exists_from <= $1 AND (x.archived_on IS NULL OR x.archived_on > $1)), '{}')`

func scanPosition(row pgx.CollectableRow) (Position, error) {
	var p Position
	var from time.Time
	var archived *time.Time
	err := row.Scan(&p.ID, &p.UnitID, &p.Title, &p.Description, &p.HeadOfUnit, &p.Seats, &from, &archived, &p.ReportsTo, &p.DottedTo, &p.Roles, &p.Deputies)
	p.ExistsFrom, p.ArchivedOn = DayOf(from), dayPtr(archived)
	return p, err
}

// Positions lists the positions that exist on a day, in one unit or in all, heads first; vacant keeps to those
// nobody holds then.
func (s *Service) Positions(ctx context.Context, asOf Day, unitID *uuid.UUID, vacant bool) ([]Position, error) {
	var out []Position
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		rows, err := q.Query(ctx, `SELECT `+positionColumns+` FROM position p
			WHERE p.exists_from <= $1 AND (p.archived_on IS NULL OR p.archived_on > $1) AND ($2::uuid IS NULL OR p.unit_id = $2)
			  AND (NOT $3 OR NOT EXISTS (SELECT 1 FROM holding h WHERE h.position_id = p.id
			      AND h.valid_from <= $1 AND (h.valid_to IS NULL OR h.valid_to > $1)))
			ORDER BY p.head_of_unit DESC, lower(p.title), p.id`, asOf.Time(), unitID, vacant)
		if err != nil {
			return err
		}
		if out, err = pgx.CollectRows(rows, scanPosition); err != nil {
			return err
		}
		return fillHolders(ctx, q, asOf, out)
	})
	return out, err
}

// fillHolders reads who holds the positions on a day.
func fillHolders(ctx context.Context, q db.DBTX, asOf Day, positions []Position) error {
	ids := make([]uuid.UUID, len(positions))
	at := make(map[uuid.UUID]int, len(positions))
	for i, p := range positions {
		ids[i], at[p.ID] = p.ID, i
	}
	rows, err := q.Query(ctx, `SELECT `+holdingColumns+` FROM holding WHERE position_id = ANY ($1)
		AND valid_from <= $2 AND (valid_to IS NULL OR valid_to > $2) ORDER BY valid_from, id`, ids, asOf.Time())
	if err != nil {
		return err
	}
	holdings, err := pgx.CollectRows(rows, scanHolding)
	for _, h := range holdings {
		p := &positions[at[*h.PositionID]]
		p.Holders = append(p.Holders, h)
	}
	return err
}

// Position is one position as it stands on a day, with every line it has had and will have.
func (s *Service) Position(ctx context.Context, id uuid.UUID, asOf Day) (Position, error) {
	var out Position
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		return s.position(ctx, q, id, asOf, &out)
	})
	return out, err
}

func (s *Service) position(ctx context.Context, q db.DBTX, id uuid.UUID, asOf Day, out *Position) error {
	rows, err := q.Query(ctx, `SELECT `+positionColumns+` FROM position p WHERE p.id = $2`, asOf.Time(), id)
	if err != nil {
		return err
	}
	*out, err = pgx.CollectExactlyOneRow(rows, scanPosition)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrPositionNotFound
	}
	if err != nil {
		return err
	}
	holders := []Position{*out}
	if err := fillHolders(ctx, q, asOf, holders); err != nil {
		return err
	}
	*out = holders[0]
	rows, err = q.Query(ctx, `SELECT reports_to_id, kind, valid_from, valid_to FROM reporting_line
		WHERE position_id = $1 ORDER BY valid_from, kind, reports_to_id`, id)
	if err != nil {
		return err
	}
	out.Lines, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Line, error) {
		var l Line
		var from time.Time
		var to *time.Time
		err := row.Scan(&l.ReportsTo, &l.Kind, &from, &to)
		l.From, l.To = DayOf(from), dayPtr(to)
		return l, err
	})
	return err
}

// unitThere says whether a unit exists from a day and is not archived before to, nil being for good.
func unitThere(ctx context.Context, q db.DBTX, unit uuid.UUID, from Day, to *Day) error {
	err := checkParent(ctx, q, &unit, from, to)
	switch {
	case errors.Is(err, ErrParentNotThere):
		return ErrUnitNotThere
	case errors.Is(err, ErrParentArchived):
		return ErrUnitGone
	}
	return err
}

// CreatePosition adds a position to a unit from a day, as an editor.
func (s *Service) CreatePosition(ctx context.Context, unitID uuid.UUID, in PositionInput, from Day) (Position, db.LSN, error) {
	if err := in.check(true); err != nil {
		return Position{}, 0, err
	}
	var out Position
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		if err := unitThere(ctx, q, unitID, from, nil); err != nil {
			return err
		}
		var id uuid.UUID
		err := q.QueryRow(ctx, `INSERT INTO position (unit_id, title, description, head_of_unit, seats, exists_from)
			VALUES ($1, $2, coalesce($3, ''), coalesce($4, false), coalesce($5, 1), $6) RETURNING id`,
			unitID, in.Title, in.Description, in.HeadOfUnit, in.Seats, from.Time()).Scan(&id)
		if err != nil {
			return positionError(err)
		}
		return s.position(ctx, q, id, from, &out)
	})
	return out, lsn, err
}

// UpdatePosition retitles, redescribes or makes a position its unit's head or not, as an editor.
func (s *Service) UpdatePosition(ctx context.Context, id uuid.UUID, in PositionInput, asOf Day) (Position, db.LSN, error) {
	if err := in.check(false); err != nil {
		return Position{}, 0, err
	}
	var out Position
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		tag, err := q.Exec(ctx, `UPDATE position SET title = coalesce($2, title), description = coalesce($3, description),
			head_of_unit = coalesce($4, head_of_unit), seats = coalesce($5, seats) WHERE id = $1`,
			id, in.Title, in.Description, in.HeadOfUnit, in.Seats)
		if err != nil {
			return positionError(err)
		}
		if tag.RowsAffected() == 0 {
			return ErrPositionNotFound
		}
		return s.position(ctx, q, id, asOf, &out)
	})
	return out, lsn, err
}

// lockPosition reads the days a position exists, holding it against other changes until the transaction ends.
func lockPosition(ctx context.Context, q db.DBTX, id uuid.UUID) (from Day, archived *Day, err error) {
	var start time.Time
	var end *time.Time
	err = q.QueryRow(ctx, `SELECT exists_from, archived_on FROM position WHERE id = $1 FOR UPDATE`, id).Scan(&start, &end)
	if errors.Is(err, pgx.ErrNoRows) {
		return Day{}, nil, ErrPositionNotFound
	}
	return DayOf(start), dayPtr(end), err
}

// livesOn says whether a position exists on a day.
func livesOn(from Day, archived *Day, on Day) bool {
	return !on.Before(from) && (archived == nil || on.Before(*archived))
}

// targetThere says whether a position exists from a day until to, nil being for good.
func targetThere(ctx context.Context, q db.DBTX, target uuid.UUID, from Day, to *Day) error {
	var start time.Time
	var archived *time.Time
	err := q.QueryRow(ctx, `SELECT exists_from, archived_on FROM position WHERE id = $1`, target).Scan(&start, &archived)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrTargetNotThere
	}
	if err != nil {
		return err
	}
	if from.Before(DayOf(start)) || archived != nil && (to == nil || DayOf(*archived).Before(*to)) {
		return ErrTargetNotThere
	}
	return nil
}

// SetReportsTo gives a position a solid line to another from a day, or none with a nil target, as an editor. The
// solid line the day falls in ends there; one planned later stays, and the new line runs until it.
func (s *Service) SetReportsTo(ctx context.Context, id uuid.UUID, target *uuid.UUID, from Day) (Position, db.LSN, error) {
	if target != nil && *target == id {
		return Position{}, 0, ErrReportsToItself
	}
	var out Position
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		exists, archived, err := lockPosition(ctx, q, id)
		if err != nil {
			return err
		}
		if !livesOn(exists, archived, from) {
			return ErrPositionNotThere
		}
		var (
			current *uuid.UUID
			start   time.Time
			end     *time.Time
		)
		err = q.QueryRow(ctx, `SELECT reports_to_id, valid_from, valid_to FROM reporting_line WHERE position_id = $1
			AND kind = 'solid' AND valid_from <= $2 AND (valid_to IS NULL OR valid_to > $2)`, id, from.Time()).Scan(&current, &start, &end)
		covering := err == nil
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if covering && sameParent(current, target) {
			return s.position(ctx, q, id, from, &out)
		}
		if !covering {
			// The new line runs until the next one planned, if any, so they do not overlap.
			err = q.QueryRow(ctx, `SELECT min(valid_from) FROM reporting_line WHERE position_id = $1 AND kind = 'solid' AND valid_from > $2`,
				id, from.Time()).Scan(&end)
			if err != nil {
				return err
			}
		}
		until := dayPtr(end)
		if until == nil && archived != nil {
			until = archived
		}
		if target != nil {
			if err := targetThere(ctx, q, *target, from, until); err != nil {
				return err
			}
		}
		if covering {
			if DayOf(start) == from {
				_, err = q.Exec(ctx, `DELETE FROM reporting_line WHERE position_id = $1 AND kind = 'solid' AND valid_from = $2`, id, from.Time())
			} else {
				_, err = q.Exec(ctx, `UPDATE reporting_line SET valid_to = $2 WHERE position_id = $1 AND kind = 'solid' AND valid_from = $3`,
					id, from.Time(), start)
			}
			if err != nil {
				return err
			}
		}
		if target != nil {
			var to *time.Time
			if until != nil {
				t := until.Time()
				to = &t
			}
			if _, err := q.Exec(ctx, `INSERT INTO reporting_line (position_id, reports_to_id, kind, valid_from, valid_to)
				VALUES ($1, $2, 'solid', $3, $4)`, id, *target, from.Time(), to); err != nil {
				return positionError(err)
			}
		}
		return s.position(ctx, q, id, from, &out)
	})
	return out, lsn, err
}

// AddDottedLine has a position report to another by a dotted line from a day, as an editor.
func (s *Service) AddDottedLine(ctx context.Context, id, target uuid.UUID, from Day) (Position, db.LSN, error) {
	if target == id {
		return Position{}, 0, ErrReportsToItself
	}
	var out Position
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		exists, archived, err := lockPosition(ctx, q, id)
		if err != nil {
			return err
		}
		if !livesOn(exists, archived, from) {
			return ErrPositionNotThere
		}
		if err := targetThere(ctx, q, target, from, archived); err != nil {
			return err
		}
		var to *time.Time
		if archived != nil {
			t := archived.Time()
			to = &t
		}
		if _, err := q.Exec(ctx, `INSERT INTO reporting_line (position_id, reports_to_id, kind, valid_from, valid_to)
			VALUES ($1, $2, 'dotted', $3, $4)`, id, target, from.Time(), to); err != nil {
			return positionError(err)
		}
		return s.position(ctx, q, id, from, &out)
	})
	return out, lsn, err
}

// EndDottedLine ends a dotted line from a day, as an editor.
func (s *Service) EndDottedLine(ctx context.Context, id, target uuid.UUID, on Day) (Position, db.LSN, error) {
	var out Position
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		if _, _, err := lockPosition(ctx, q, id); err != nil {
			return err
		}
		var start time.Time
		err := q.QueryRow(ctx, `SELECT valid_from FROM reporting_line WHERE position_id = $1 AND reports_to_id = $2 AND kind = 'dotted'
			AND valid_from <= $3 AND (valid_to IS NULL OR valid_to > $3)`, id, target, on.Time()).Scan(&start)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrLineNotFound
		}
		if err != nil {
			return err
		}
		if DayOf(start) == on {
			_, err = q.Exec(ctx, `DELETE FROM reporting_line WHERE position_id = $1 AND reports_to_id = $2 AND kind = 'dotted' AND valid_from = $3`,
				id, target, start)
		} else {
			_, err = q.Exec(ctx, `UPDATE reporting_line SET valid_to = $4 WHERE position_id = $1 AND reports_to_id = $2 AND kind = 'dotted'
				AND valid_from = $3`, id, target, start, on.Time())
		}
		if err != nil {
			return err
		}
		return s.position(ctx, q, id, on, &out)
	})
	return out, lsn, err
}

// ArchivePosition ends a position from a day, as an editor. Its own lines, its holdings and its roles end there; positions still
// reporting to it then must be given another line first.
func (s *Service) ArchivePosition(ctx context.Context, id uuid.UUID, on Day) (Position, db.LSN, error) {
	var out Position
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		exists, archived, err := lockPosition(ctx, q, id)
		if err != nil {
			return err
		}
		if archived != nil {
			return ErrPositionArchived
		}
		if !exists.Before(on) {
			return ErrPositionNotThere
		}
		var reported bool
		if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM reporting_line WHERE reports_to_id = $1 AND (valid_to IS NULL OR valid_to > $2))`,
			id, on.Time()).Scan(&reported); err != nil {
			return err
		}
		if reported {
			return ErrHasReports
		}
		if err := endHoldings(ctx, q, "position_id", id, on); err != nil {
			return err
		}
		for _, stmt := range []string{
			`DELETE FROM position_role WHERE position_id = $1 AND valid_from >= $2`,
			`UPDATE position_role SET valid_to = $2 WHERE position_id = $1 AND (valid_to IS NULL OR valid_to > $2)`,
		} {
			if _, err := q.Exec(ctx, stmt, id, on.Time()); err != nil {
				return err
			}
		}
		for _, stmt := range []string{
			`DELETE FROM reporting_line WHERE position_id = $1 AND valid_from >= $2`,
			`UPDATE reporting_line SET valid_to = $2 WHERE position_id = $1 AND (valid_to IS NULL OR valid_to > $2)`,
			`UPDATE position SET archived_on = $2 WHERE id = $1`,
		} {
			if _, err := q.Exec(ctx, stmt, id, on.Time()); err != nil {
				return err
			}
		}
		return s.position(ctx, q, id, on, &out)
	})
	return out, lsn, err
}

// RestorePosition brings an archived position back for good, as an editor, with the lines that ended with it whose
// other end still exists.
func (s *Service) RestorePosition(ctx context.Context, id uuid.UUID) (Position, db.LSN, error) {
	var out Position
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		_, archived, err := lockPosition(ctx, q, id)
		if err != nil {
			return err
		}
		if archived == nil {
			return ErrPositionNotArchived
		}
		var unit uuid.UUID
		if err := q.QueryRow(ctx, `SELECT unit_id FROM position WHERE id = $1`, id).Scan(&unit); err != nil {
			return err
		}
		if err := unitThere(ctx, q, unit, *archived, nil); err != nil {
			return err
		}
		if _, err := q.Exec(ctx, `UPDATE position SET archived_on = NULL WHERE id = $1`, id); err != nil {
			return positionError(err)
		}
		if _, err := q.Exec(ctx, `UPDATE reporting_line l SET valid_to = NULL FROM position t
			WHERE l.position_id = $1 AND l.valid_to = $2 AND t.id = l.reports_to_id AND t.archived_on IS NULL`, id, archived.Time()); err != nil {
			return positionError(err)
		}
		return s.position(ctx, q, id, *archived, &out)
	})
	return out, lsn, err
}

func positionError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	switch pgErr.ConstraintName {
	case "reporting_line_no_cycle":
		return ErrReportsToItself
	case "reporting_line_once":
		return ErrLineExists
	case "position_one_head":
		return ErrHeadTaken
	case "holding_within_seats":
		return ErrTooFewSeats
	case "position_unit_id_fkey":
		return ErrUnitNotThere
	case "reporting_line_reports_to_id_fkey":
		return ErrTargetNotThere
	}
	return err
}
