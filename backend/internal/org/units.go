package org

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/praetorianer777/bearing/backend/internal/db"
)

// Service keeps the organisation model.
type Service struct {
	DB *db.Cluster
}

// UnitType is a kind of unit, such as a division or a team.
type UnitType struct {
	ID     uuid.UUID
	Name   string
	Icon   string
	Colour string
	Rank   int
	// Units counts the units of this type, archived ones too; a type in use cannot be removed.
	Units int
}

// UnitTypeInput is what a unit type is made or changed with; a nil field is left as it is.
type UnitTypeInput struct {
	Name   *string
	Icon   *string
	Colour *string
	Rank   *int
}

// Unit is a part of the organisation, as it stands on one day.
type Unit struct {
	ID          uuid.UUID
	TypeID      uuid.UUID
	Name        string
	ShortName   *string
	Description string
	// ParentID is the unit it is part of on the day asked about; nil at the root.
	ParentID   *uuid.UUID
	ArchivedOn *Day
	// Placements are where the unit sat over time, oldest first; filled when one unit is asked for.
	Placements []Placement
}

// Placement is where a unit sits from a day, until a day or for good.
type Placement struct {
	ParentID *uuid.UUID
	From     Day
	// To is the first day it no longer holds.
	To *Day
}

// UnitInput is what a unit is made or changed with; a nil field is left as it is. ShortName set to an empty string
// takes the short name away.
type UnitInput struct {
	TypeID      *uuid.UUID
	Name        *string
	ShortName   *string
	Description *string
}

// The refusals the model gives, each a sentence a person can act on.
var (
	ErrNotFound       = errors.New("there is no such unit")
	ErrTypeNotFound   = errors.New("there is no such unit type")
	ErrTypeInUse      = errors.New("units of this type exist; give them another type first")
	ErrTypeNameTaken  = errors.New("another unit type has that name")
	ErrCycle          = errors.New("that would put the unit inside itself")
	ErrRootTaken      = errors.New("another unit is the root on that day; place this one inside it")
	ErrNotThere       = errors.New("the unit does not exist on that day")
	ErrParentNotThere = errors.New("the unit it would be part of does not exist on that day")
	ErrParentArchived = errors.New("the unit it would be part of is archived later, and units inside it would outlive it")
	ErrHasUnits       = errors.New("units are still part of it on that day or after; move or archive them first")
	ErrArchived       = errors.New("the unit is archived; restore it first")
	ErrNotArchived    = errors.New("the unit is not archived")
)

// Invalid is input that breaks a rule of its field.
type Invalid struct {
	Field   string
	Message string
}

func (e Invalid) Error() string { return e.Field + ": " + e.Message }

var (
	iconName = regexp.MustCompile(`^[a-z][a-z-]{0,39}$`)
	colour   = regexp.MustCompile(`^#[0-9a-f]{6}$`)
)

func checkText(field string, value *string, min, max int) error {
	if value == nil {
		return nil
	}
	*value = strings.TrimSpace(*value)
	if n := utf8.RuneCountInString(*value); n < min || n > max {
		if min > 0 && n == 0 {
			return Invalid{field, "Give a " + field + "."}
		}
		return Invalid{field, "Keep it to " + strconv.Itoa(max) + " characters."}
	}
	return nil
}

func (in *UnitTypeInput) check(creating bool) error {
	if creating && in.Name == nil {
		return Invalid{"name", "Give a name."}
	}
	if err := checkText("name", in.Name, 1, 100); err != nil {
		return err
	}
	if in.Icon != nil && !iconName.MatchString(*in.Icon) {
		return Invalid{"icon", "Choose one of the icons."}
	}
	if in.Colour != nil {
		*in.Colour = strings.ToLower(strings.TrimSpace(*in.Colour))
		if !colour.MatchString(*in.Colour) {
			return Invalid{"colour", "Write the colour as #rrggbb."}
		}
	}
	return nil
}

// UnitTypes lists the unit types, larger units first.
func (s *Service) UnitTypes(ctx context.Context) ([]UnitType, error) {
	var out []UnitType
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		rows, err := q.Query(ctx, `SELECT t.id, t.name, t.icon, t.colour, t.rank, (SELECT count(*) FROM unit u WHERE u.type_id = t.id)
			FROM unit_type t ORDER BY t.rank, lower(t.name)`)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, scanUnitType)
		return err
	})
	return out, err
}

func scanUnitType(row pgx.CollectableRow) (UnitType, error) {
	var t UnitType
	return t, row.Scan(&t.ID, &t.Name, &t.Icon, &t.Colour, &t.Rank, &t.Units)
}

const unitTypeColumns = `id, name, icon, colour, rank, (SELECT count(*) FROM unit u WHERE u.type_id = unit_type.id)`

// CreateUnitType adds a unit type, as an administrator.
func (s *Service) CreateUnitType(ctx context.Context, in UnitTypeInput) (UnitType, db.LSN, error) {
	if err := in.check(true); err != nil {
		return UnitType{}, 0, err
	}
	var out UnitType
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		rows, err := q.Query(ctx, `INSERT INTO unit_type (name, icon, colour, rank)
			VALUES ($1, coalesce($2, 'unit'), coalesce($3, '#6f685d'), coalesce($4, 0)) RETURNING `+unitTypeColumns,
			in.Name, in.Icon, in.Colour, in.Rank)
		if err != nil {
			return err
		}
		out, err = pgx.CollectExactlyOneRow(rows, scanUnitType)
		return err
	})
	return out, lsn, typeError(err)
}

// UpdateUnitType changes a unit type, as an administrator.
func (s *Service) UpdateUnitType(ctx context.Context, id uuid.UUID, in UnitTypeInput) (UnitType, db.LSN, error) {
	if err := in.check(false); err != nil {
		return UnitType{}, 0, err
	}
	var out UnitType
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		rows, err := q.Query(ctx, `UPDATE unit_type SET name = coalesce($2, name), icon = coalesce($3, icon),
			colour = coalesce($4, colour), rank = coalesce($5, rank) WHERE id = $1 RETURNING `+unitTypeColumns,
			id, in.Name, in.Icon, in.Colour, in.Rank)
		if err != nil {
			return err
		}
		out, err = pgx.CollectExactlyOneRow(rows, scanUnitType)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrTypeNotFound
		}
		return err
	})
	return out, lsn, typeError(err)
}

// DeleteUnitType removes a unit type no unit has, as an administrator.
func (s *Service) DeleteUnitType(ctx context.Context, id uuid.UUID) (db.LSN, error) {
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		tag, err := q.Exec(ctx, `DELETE FROM unit_type WHERE id = $1`, id)
		if err == nil && tag.RowsAffected() == 0 {
			return ErrTypeNotFound
		}
		return err
	})
	return lsn, typeError(err)
}

func typeError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return ErrTypeNameTaken
		case "23503":
			return ErrTypeInUse
		}
	}
	return err
}

const unitColumns = `u.id, u.type_id, u.name, u.short_name, u.description, u.archived_on`

func scanUnit(row pgx.CollectableRow) (Unit, error) {
	var u Unit
	var archived *time.Time
	err := row.Scan(&u.ID, &u.TypeID, &u.Name, &u.ShortName, &u.Description, &archived, &u.ParentID)
	u.ArchivedOn = dayPtr(archived)
	return u, err
}

// Units lists the units that exist on a day, each with the unit it is part of then.
func (s *Service) Units(ctx context.Context, asOf Day) ([]Unit, error) {
	return s.units(ctx, `SELECT `+unitColumns+`, p.parent_id FROM unit u
		JOIN unit_placement p ON p.unit_id = u.id AND p.valid_from <= $1 AND (p.valid_to IS NULL OR p.valid_to > $1)
		WHERE u.archived_on IS NULL OR u.archived_on > $1
		ORDER BY lower(u.name), u.id`, asOf)
}

// ArchivedUnits lists the units archived on a day or before, each with the unit it was last part of.
func (s *Service) ArchivedUnits(ctx context.Context, asOf Day) ([]Unit, error) {
	return s.units(ctx, `SELECT `+unitColumns+`, p.parent_id FROM unit u
		LEFT JOIN unit_placement p ON p.unit_id = u.id AND p.valid_to = u.archived_on
		WHERE u.archived_on <= $1
		ORDER BY lower(u.name), u.id`, asOf)
}

func (s *Service) units(ctx context.Context, query string, asOf Day) ([]Unit, error) {
	var out []Unit
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		rows, err := q.Query(ctx, query, asOf.Time())
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, scanUnit)
		return err
	})
	return out, err
}

// Unit is one unit as it stands on a day, with every placement it has had and will have. A unit asked about on a
// day it does not exist is still found, with no parent.
func (s *Service) Unit(ctx context.Context, id uuid.UUID, asOf Day) (Unit, error) {
	var out Unit
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		return s.unit(ctx, q, id, asOf, &out)
	})
	return out, err
}

func (s *Service) unit(ctx context.Context, q db.DBTX, id uuid.UUID, asOf Day, out *Unit) error {
	rows, err := q.Query(ctx, `SELECT `+unitColumns+`, p.parent_id FROM unit u
		LEFT JOIN unit_placement p ON p.unit_id = u.id AND p.valid_from <= $2 AND (p.valid_to IS NULL OR p.valid_to > $2)
		WHERE u.id = $1`, id, asOf.Time())
	if err != nil {
		return err
	}
	*out, err = pgx.CollectExactlyOneRow(rows, scanUnit)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	rows, err = q.Query(ctx, `SELECT parent_id, valid_from, valid_to FROM unit_placement WHERE unit_id = $1 ORDER BY valid_from`, id)
	if err != nil {
		return err
	}
	out.Placements, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Placement, error) {
		var p Placement
		var from time.Time
		var to *time.Time
		err := row.Scan(&p.ParentID, &from, &to)
		p.From, p.To = DayOf(from), dayPtr(to)
		return p, err
	})
	return err
}

func (in *UnitInput) check(creating bool) error {
	if creating && in.Name == nil {
		return Invalid{"name", "Give a name."}
	}
	if creating && in.TypeID == nil {
		return Invalid{"typeId", "Choose a type."}
	}
	if err := checkText("name", in.Name, 1, 200); err != nil {
		return err
	}
	if err := checkText("shortName", in.ShortName, 0, 20); err != nil {
		return err
	}
	if in.Description != nil && utf8.RuneCountInString(*in.Description) > 4000 {
		return Invalid{"description", "Keep it to 4000 characters."}
	}
	return nil
}

// CreateUnit adds a unit from a day, part of parent or, with no parent, at the root, as an editor.
func (s *Service) CreateUnit(ctx context.Context, in UnitInput, parent *uuid.UUID, from Day) (Unit, db.LSN, error) {
	if err := in.check(true); err != nil {
		return Unit{}, 0, err
	}
	var out Unit
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		if err := checkParent(ctx, q, parent, from, nil); err != nil {
			return err
		}
		var id uuid.UUID
		err := q.QueryRow(ctx, `INSERT INTO unit (type_id, name, short_name, description) VALUES ($1, $2, nullif($3, ''), coalesce($4, ''))
			RETURNING id`, in.TypeID, in.Name, in.ShortName, in.Description).Scan(&id)
		if err != nil {
			return unitError(err)
		}
		if _, err := q.Exec(ctx, `INSERT INTO unit_placement (unit_id, parent_id, valid_from) VALUES ($1, $2, $3)`,
			id, parent, from.Time()); err != nil {
			return unitError(err)
		}
		return s.unit(ctx, q, id, from, &out)
	})
	return out, lsn, err
}

// checkParent says whether a unit can be part of parent from a day until a day or for good: the parent exists then
// and is not archived before the placement ends.
func checkParent(ctx context.Context, q db.DBTX, parent *uuid.UUID, from Day, to *Day) error {
	if parent == nil {
		return nil
	}
	var there bool
	var archived *time.Time
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM unit_placement p WHERE p.unit_id = u.id AND p.valid_from <= $2
			AND (p.valid_to IS NULL OR p.valid_to > $2)) AND (u.archived_on IS NULL OR u.archived_on > $2), u.archived_on
		FROM unit u WHERE u.id = $1`, *parent, from.Time()).Scan(&there, &archived)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !there {
		return ErrParentNotThere
	}
	if err != nil {
		return err
	}
	if archived != nil && (to == nil || DayOf(*archived).Before(*to)) {
		return ErrParentArchived
	}
	return nil
}

// UpdateUnit renames, retypes or redescribes a unit, as an editor. What a unit is called is not dated; its history
// is in the audit log.
func (s *Service) UpdateUnit(ctx context.Context, id uuid.UUID, in UnitInput, asOf Day) (Unit, db.LSN, error) {
	if err := in.check(false); err != nil {
		return Unit{}, 0, err
	}
	var out Unit
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		tag, err := q.Exec(ctx, `UPDATE unit SET type_id = coalesce($2, type_id), name = coalesce($3, name),
			short_name = CASE WHEN $4::text IS NULL THEN short_name ELSE nullif($4, '') END,
			description = coalesce($5, description) WHERE id = $1`, id, in.TypeID, in.Name, in.ShortName, in.Description)
		if err != nil {
			return unitError(err)
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return s.unit(ctx, q, id, asOf, &out)
	})
	return out, lsn, err
}

// MoveUnit makes a unit part of parent, or the root with no parent, from a day, as an editor. The placement the day
// falls in ends there; placements planned after it stay.
func (s *Service) MoveUnit(ctx context.Context, id uuid.UUID, parent *uuid.UUID, from Day) (Unit, db.LSN, error) {
	if parent != nil && *parent == id {
		return Unit{}, 0, ErrCycle
	}
	var out Unit
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		var archived *time.Time
		err := q.QueryRow(ctx, `SELECT archived_on FROM unit WHERE id = $1 FOR UPDATE`, id).Scan(&archived)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if archived != nil && !from.Before(DayOf(*archived)) {
			return ErrArchived
		}
		var (
			current *uuid.UUID
			start   time.Time
			end     *time.Time
		)
		err = q.QueryRow(ctx, `SELECT parent_id, valid_from, valid_to FROM unit_placement
			WHERE unit_id = $1 AND valid_from <= $2 AND (valid_to IS NULL OR valid_to > $2)`, id, from.Time()).Scan(&current, &start, &end)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotThere
		}
		if err != nil {
			return err
		}
		if err := checkParent(ctx, q, parent, from, dayPtr(end)); err != nil {
			return err
		}
		if sameParent(current, parent) {
			return s.unit(ctx, q, id, from, &out)
		}
		if DayOf(start) == from {
			_, err = q.Exec(ctx, `UPDATE unit_placement SET parent_id = $3 WHERE unit_id = $1 AND valid_from = $2`, id, from.Time(), parent)
		} else {
			if _, err = q.Exec(ctx, `UPDATE unit_placement SET valid_to = $2 WHERE unit_id = $1 AND valid_from = $3`, id, from.Time(), start); err != nil {
				return err
			}
			_, err = q.Exec(ctx, `INSERT INTO unit_placement (unit_id, parent_id, valid_from, valid_to) VALUES ($1, $2, $3, $4)`,
				id, parent, from.Time(), end)
		}
		if err != nil {
			return unitError(err)
		}
		return s.unit(ctx, q, id, from, &out)
	})
	return out, lsn, err
}

func sameParent(a, b *uuid.UUID) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

// ArchiveUnit ends a unit from a day, as an editor. Its placements end there and those planned later go; the days
// before keep it. A unit others are still part of on that day or after cannot be archived.
func (s *Service) ArchiveUnit(ctx context.Context, id uuid.UUID, on Day) (Unit, db.LSN, error) {
	var out Unit
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		var archived *time.Time
		var first time.Time
		err := q.QueryRow(ctx, `SELECT u.archived_on, (SELECT min(valid_from) FROM unit_placement WHERE unit_id = u.id)
			FROM unit u WHERE u.id = $1 FOR UPDATE`, id).Scan(&archived, &first)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if archived != nil {
			return ErrArchived
		}
		if !DayOf(first).Before(on) {
			return ErrNotThere
		}
		var held bool
		if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM unit_placement WHERE parent_id = $1 AND (valid_to IS NULL OR valid_to > $2))`,
			id, on.Time()).Scan(&held); err != nil {
			return err
		}
		if held {
			return ErrHasUnits
		}
		for _, stmt := range []string{
			`DELETE FROM unit_placement WHERE unit_id = $1 AND valid_from >= $2`,
			`UPDATE unit_placement SET valid_to = $2 WHERE unit_id = $1 AND (valid_to IS NULL OR valid_to > $2)`,
			`UPDATE unit SET archived_on = $2 WHERE id = $1`,
		} {
			if _, err := q.Exec(ctx, stmt, id, on.Time()); err != nil {
				return err
			}
		}
		return s.unit(ctx, q, id, on, &out)
	})
	return out, lsn, err
}

// RestoreUnit brings an archived unit back where it last was, for good, as an editor.
func (s *Service) RestoreUnit(ctx context.Context, id uuid.UUID) (Unit, db.LSN, error) {
	var out Unit
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		var archived *time.Time
		err := q.QueryRow(ctx, `SELECT archived_on FROM unit WHERE id = $1 FOR UPDATE`, id).Scan(&archived)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if archived == nil {
			return ErrNotArchived
		}
		var parent *uuid.UUID
		err = q.QueryRow(ctx, `SELECT parent_id FROM unit_placement WHERE unit_id = $1 AND valid_to = $2`, id, *archived).Scan(&parent)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err := checkParent(ctx, q, parent, DayOf(*archived), nil); err != nil {
			return err
		}
		if _, err := q.Exec(ctx, `UPDATE unit_placement SET valid_to = NULL WHERE unit_id = $1 AND valid_to = $2`, id, *archived); err != nil {
			return unitError(err)
		}
		if _, err := q.Exec(ctx, `UPDATE unit SET archived_on = NULL WHERE id = $1`, id); err != nil {
			return err
		}
		return s.unit(ctx, q, id, DayOf(*archived), &out)
	})
	return out, lsn, err
}

func unitError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	switch {
	case pgErr.ConstraintName == "unit_placement_no_cycle":
		return ErrCycle
	case pgErr.ConstraintName == "unit_placement_one_root":
		return ErrRootTaken
	case pgErr.Code == "23503" && pgErr.ConstraintName == "unit_type_id_fkey":
		return ErrTypeNotFound
	case pgErr.Code == "23503":
		return ErrParentNotThere
	}
	return err
}
