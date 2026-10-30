package org

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/praetorianer777/bearing/backend/internal/db"
)

// Holding is a person holding a position from a day, until a day or for good, at a share of their week.
type Holding struct {
	ID         uuid.UUID
	PersonID   uuid.UUID
	PositionID uuid.UUID
	// Share is the part of the person's week it takes, in percent.
	Share int
	From  Day
	// To is the first day it no longer holds.
	To *Day
}

// HoldingInput is what a holding is given with. A nil share is the whole week; a nil end runs until the position
// is archived or the person leaves, if either is known, or for good.
type HoldingInput struct {
	PersonID   uuid.UUID
	PositionID uuid.UUID
	Share      *int
	From       Day
	To         *Day
}

// HoldingFilter narrows holdings to a person's or a position's, and to those on one day when AsOf is set.
type HoldingFilter struct {
	PersonID   *uuid.UUID
	PositionID *uuid.UUID
	AsOf       *Day
}

var (
	ErrHoldingNotFound   = errors.New("there is no such holding")
	ErrPersonNotThere    = errors.New("the person has left by then")
	ErrHoldingOverlaps   = errors.New("the person holds that position then already")
	ErrSeatsTaken        = errors.New("more people would hold the position then than it has seats")
	ErrHoldingNotStarted = errors.New("the holding starts on that day or later; remove it instead")
	ErrHoldingEnded      = errors.New("the holding has ended by then")
)

func checkShare(share *int) error {
	if share != nil && (*share < 1 || *share > 100) {
		return Invalid{"share", "Give a share from 1 to 100 percent."}
	}
	return nil
}

const holdingColumns = `id, person_id, position_id, share, valid_from, valid_to`

func scanHolding(row pgx.CollectableRow) (Holding, error) {
	var h Holding
	var from time.Time
	var to *time.Time
	err := row.Scan(&h.ID, &h.PersonID, &h.PositionID, &h.Share, &from, &to)
	h.From, h.To = DayOf(from), dayPtr(to)
	return h, err
}

// Holdings lists holdings oldest first, narrowed by the filter.
func (s *Service) Holdings(ctx context.Context, f HoldingFilter) ([]Holding, error) {
	var on *time.Time
	if f.AsOf != nil {
		t := f.AsOf.Time()
		on = &t
	}
	var out []Holding
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		rows, err := q.Query(ctx, `SELECT `+holdingColumns+` FROM holding
			WHERE ($1::uuid IS NULL OR person_id = $1) AND ($2::uuid IS NULL OR position_id = $2)
			  AND ($3::date IS NULL OR valid_from <= $3 AND (valid_to IS NULL OR valid_to > $3))
			ORDER BY valid_from, id`, f.PersonID, f.PositionID, on)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, scanHolding)
		return err
	})
	return out, err
}

// Holding is one holding.
func (s *Service) Holding(ctx context.Context, id uuid.UUID) (Holding, error) {
	var out Holding
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		return holding(ctx, q, id, false, &out)
	})
	return out, err
}

func holding(ctx context.Context, q db.DBTX, id uuid.UUID, lock bool, out *Holding) error {
	sql := `SELECT ` + holdingColumns + ` FROM holding WHERE id = $1`
	if lock {
		sql += ` FOR UPDATE`
	}
	rows, err := q.Query(ctx, sql, id)
	if err != nil {
		return err
	}
	*out, err = pgx.CollectExactlyOneRow(rows, scanHolding)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrHoldingNotFound
	}
	return err
}

// GiveHolding has a person hold a position, as an editor. A holding may start later than today, planned ahead.
func (s *Service) GiveHolding(ctx context.Context, in HoldingInput) (Holding, db.LSN, error) {
	if err := checkShare(in.Share); err != nil {
		return Holding{}, 0, err
	}
	if in.To != nil && !in.From.Before(*in.To) {
		return Holding{}, 0, Invalid{"to", "End it after it starts."}
	}
	var out Holding
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		exists, archived, err := lockPosition(ctx, q, in.PositionID)
		if err != nil {
			return err
		}
		if !livesOn(exists, archived, in.From) || archived != nil && in.To != nil && archived.Before(*in.To) {
			return ErrPositionNotThere
		}
		var left *time.Time
		err = q.QueryRow(ctx, `SELECT left_on FROM person WHERE id = $1 FOR SHARE`, in.PersonID).Scan(&left)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrPersonNotFound
		}
		if err != nil {
			return err
		}
		gone := dayPtr(left)
		if gone != nil && (!in.From.Before(*gone) || in.To != nil && gone.Before(*in.To)) {
			return ErrPersonNotThere
		}
		to := in.To
		for _, end := range []*Day{archived, gone} {
			if to == nil || end != nil && end.Before(*to) {
				to = end
			}
		}
		var until *time.Time
		if to != nil {
			t := to.Time()
			until = &t
		}
		var id uuid.UUID
		if err := q.QueryRow(ctx, `INSERT INTO holding (person_id, position_id, share, valid_from, valid_to)
			VALUES ($1, $2, coalesce($3, 100), $4, $5) RETURNING id`,
			in.PersonID, in.PositionID, in.Share, in.From.Time(), until).Scan(&id); err != nil {
			return holdingError(err)
		}
		return holding(ctx, q, id, false, &out)
	})
	return out, lsn, err
}

// SetShare changes the share of the week a holding takes, for all of it, as an editor. A share that changes from a
// day is a holding ended there and a new one.
func (s *Service) SetShare(ctx context.Context, id uuid.UUID, share int) (Holding, db.LSN, error) {
	if err := checkShare(&share); err != nil {
		return Holding{}, 0, err
	}
	var out Holding
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		tag, err := q.Exec(ctx, `UPDATE holding SET share = $2 WHERE id = $1`, id, share)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrHoldingNotFound
		}
		return holding(ctx, q, id, false, &out)
	})
	return out, lsn, err
}

// EndHolding ends a holding from a day, as an editor; it may already be planned to end later, or not at all.
func (s *Service) EndHolding(ctx context.Context, id uuid.UUID, on Day) (Holding, db.LSN, error) {
	var out Holding
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		if err := holding(ctx, q, id, true, &out); err != nil {
			return err
		}
		switch {
		case !out.From.Before(on):
			return ErrHoldingNotStarted
		case out.To != nil && !on.Before(*out.To):
			return ErrHoldingEnded
		}
		if _, err := q.Exec(ctx, `UPDATE holding SET valid_to = $2 WHERE id = $1`, id, on.Time()); err != nil {
			return err
		}
		return holding(ctx, q, id, false, &out)
	})
	return out, lsn, err
}

// RemoveHolding takes a holding away as if it never was, as an editor: one planned and called off, or entered by
// mistake. One that ends keeps its past; end it instead.
func (s *Service) RemoveHolding(ctx context.Context, id uuid.UUID) (db.LSN, error) {
	return s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		tag, err := q.Exec(ctx, `DELETE FROM holding WHERE id = $1`, id)
		if err == nil && tag.RowsAffected() == 0 {
			return ErrHoldingNotFound
		}
		return err
	})
}

// endHoldings ends the holdings of a person or a position from a day: those that start then or later go, the rest
// end there.
func endHoldings(ctx context.Context, q db.DBTX, column string, id uuid.UUID, on Day) error {
	for _, stmt := range []string{
		`DELETE FROM holding WHERE ` + column + ` = $1 AND valid_from >= $2`,
		`UPDATE holding SET valid_to = $2 WHERE ` + column + ` = $1 AND (valid_to IS NULL OR valid_to > $2)`,
	} {
		if _, err := q.Exec(ctx, stmt, id, on.Time()); err != nil {
			return err
		}
	}
	return nil
}

func holdingError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	switch pgErr.ConstraintName {
	case "holding_once":
		return ErrHoldingOverlaps
	case "holding_within_seats":
		return ErrSeatsTaken
	}
	return err
}
