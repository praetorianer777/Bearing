package org

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/praetorianer777/bearing/backend/internal/db"
)

// MaxTargetFTE bounds a target, as the database does.
const MaxTargetFTE = 100000

// Target is what a unit, a position or a role should have, in full-time equivalents, from a day until a day or for
// good. A unit's target is for the unit with everything inside it.
type Target struct {
	ID                         uuid.UUID
	UnitID, PositionID, RoleID *uuid.UUID
	FTE                        float64
	From                       Day
	// To is the first day it no longer holds.
	To *Day
}

// TargetSubject names whose targets: exactly one of a unit, a position and a role.
type TargetSubject struct {
	UnitID, PositionID, RoleID *uuid.UUID
}

// TargetInput is a target set for a span; a nil end is for good.
type TargetInput struct {
	TargetSubject
	FTE  float64
	From Day
	To   *Day
}

var (
	ErrTargetNotFound = errors.New("there is no such target")
	ErrTargetWhat     = errors.New("a target is of a unit, a position or a role, one of them")
)

const targetColumns = `id, unit_id, position_id, role_id, fte::float8, valid_from, valid_to`

func scanTarget(row pgx.CollectableRow) (Target, error) {
	var t Target
	var from time.Time
	var to *time.Time
	err := row.Scan(&t.ID, &t.UnitID, &t.PositionID, &t.RoleID, &t.FTE, &from, &to)
	t.From, t.To = DayOf(from), dayPtr(to)
	return t, err
}

// column is the subject's column and id, or ErrTargetWhat when it does not name exactly one.
func (s TargetSubject) column() (string, uuid.UUID, error) {
	named := 0
	var column string
	var id uuid.UUID
	for _, c := range []struct {
		name string
		id   *uuid.UUID
	}{{"unit_id", s.UnitID}, {"position_id", s.PositionID}, {"role_id", s.RoleID}} {
		if c.id != nil {
			named++
			column, id = c.name, *c.id
		}
	}
	if named != 1 {
		return "", uuid.Nil, ErrTargetWhat
	}
	return column, id, nil
}

// Targets lists a subject's targets oldest first, or with asOf the one on that day, if any.
func (s *Service) Targets(ctx context.Context, subject TargetSubject, asOf *Day) ([]Target, error) {
	column, id, err := subject.column()
	if err != nil {
		return nil, err
	}
	var on *time.Time
	if asOf != nil {
		t := asOf.Time()
		on = &t
	}
	out := []Target{}
	err = s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		rows, err := q.Query(ctx, `SELECT `+targetColumns+` FROM target WHERE `+column+` = $1
			AND ($2::date IS NULL OR valid_from <= $2 AND (valid_to IS NULL OR valid_to > $2))
			ORDER BY valid_from, id`, id, on)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, scanTarget)
		return err
	})
	return out, err
}

func target(ctx context.Context, q db.DBTX, id uuid.UUID, out *Target) error {
	rows, err := q.Query(ctx, `SELECT `+targetColumns+` FROM target WHERE id = $1`, id)
	if err != nil {
		return err
	}
	*out, err = pgx.CollectExactlyOneRow(rows, scanTarget)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrTargetNotFound
	}
	return err
}

// SetTarget sets a subject's target for a span, as an editor. What was set for those days gives way: a target inside
// the span goes, and one reaching into it ends where the span starts or starts where it ends.
func (s *Service) SetTarget(ctx context.Context, in TargetInput) (Target, db.LSN, error) {
	column, id, err := in.column()
	if err != nil {
		return Target{}, 0, err
	}
	if math.IsNaN(in.FTE) || in.FTE < 0 || in.FTE > MaxTargetFTE {
		return Target{}, 0, Invalid{"fte", "Give full-time equivalents from 0 to 100000."}
	}
	in.FTE = round2(in.FTE)
	if in.To != nil && !in.From.Before(*in.To) {
		return Target{}, 0, Invalid{"to", "End it after it starts."}
	}
	var until *time.Time
	if in.To != nil {
		t := in.To.Time()
		until = &t
	}
	var out Target
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		table := map[string]string{"unit_id": "unit", "position_id": "position", "role_id": "role"}[column]
		missing := map[string]error{"unit_id": ErrNotFound, "position_id": ErrPositionNotFound, "role_id": ErrRoleNotFound}[column]
		// The subject's row is locked, so two targets set at once for it take turns rather than overlap.
		err := q.QueryRow(ctx, `SELECT id FROM `+table+` WHERE id = $1 FOR UPDATE`, id).Scan(new(uuid.UUID))
		if errors.Is(err, pgx.ErrNoRows) {
			return missing
		}
		if err != nil {
			return err
		}
		// One reaching over both ends of the span is cut at its start, and its part after the span comes back as its own.
		if until != nil {
			var tailFTE float64
			var tailTo *time.Time
			var over uuid.UUID
			err := q.QueryRow(ctx, `SELECT id, fte::float8, valid_to FROM target WHERE `+column+` = $1 AND valid_from < $2
				AND (valid_to IS NULL OR valid_to > $3)`, id, in.From.Time(), until).Scan(&over, &tailFTE, &tailTo)
			switch {
			case err == nil:
				if _, err := q.Exec(ctx, `UPDATE target SET valid_to = $2 WHERE id = $1`, over, in.From.Time()); err != nil {
					return err
				}
				if _, err := q.Exec(ctx, `INSERT INTO target (`+column+`, fte, valid_from, valid_to) VALUES ($1, $2, $3, $4)`,
					id, tailFTE, until, tailTo); err != nil {
					return err
				}
			case !errors.Is(err, pgx.ErrNoRows):
				return err
			}
		}
		for _, stmt := range []struct {
			sql  string
			args []any
		}{
			{`DELETE FROM target WHERE ` + column + ` = $1 AND daterange($2::date, $3::date) @> daterange(valid_from, valid_to)`,
				[]any{id, in.From.Time(), until}},
			{`UPDATE target SET valid_to = $2 WHERE ` + column + ` = $1 AND valid_from < $2::date AND (valid_to IS NULL OR valid_to > $2::date)`,
				[]any{id, in.From.Time()}},
			{`UPDATE target SET valid_from = $3::date WHERE ` + column + ` = $1 AND $3::date IS NOT NULL AND valid_from >= $2::date AND valid_from < $3::date`,
				[]any{id, in.From.Time(), until}},
		} {
			if _, err := q.Exec(ctx, stmt.sql, stmt.args...); err != nil {
				return err
			}
		}
		var made uuid.UUID
		if err := q.QueryRow(ctx, `INSERT INTO target (`+column+`, fte, valid_from, valid_to) VALUES ($1, $2, $3, $4) RETURNING id`,
			id, in.FTE, in.From.Time(), until).Scan(&made); err != nil {
			return err
		}
		return target(ctx, q, made, &out)
	})
	return out, lsn, err
}

// RemoveTarget takes a target away for all its days, as an editor.
func (s *Service) RemoveTarget(ctx context.Context, id uuid.UUID) (db.LSN, error) {
	return s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		tag, err := q.Exec(ctx, `DELETE FROM target WHERE id = $1`, id)
		if err == nil && tag.RowsAffected() == 0 {
			return ErrTargetNotFound
		}
		return err
	})
}
