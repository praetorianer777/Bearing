package org

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/praetorianer777/bearing/backend/internal/db"
)

// Role is a function that does not follow the tree. Positions carry it for a time, so whoever holds them has it,
// and people may hold it directly as well.
type Role struct {
	ID          uuid.UUID
	Key         string
	Name        string
	Description string
	// Positions are those carrying it on the day asked about; filled when one role is asked for.
	Positions []uuid.UUID
	// Holders are who has it on that day, directly or through a position; filled when one role is asked for.
	Holders []RoleHolder
}

// RoleHolder is somebody who has a role on a day, by a holding of the role itself or of a position carrying it.
type RoleHolder struct {
	PersonID  uuid.UUID
	HoldingID uuid.UUID
	Share     int
	// Through is the position they have it through; nil when they hold the role directly.
	Through *uuid.UUID
}

// RoleInput is what a role is made or changed with; a nil field is left as it is.
type RoleInput struct {
	Key         *string
	Name        *string
	Description *string
}

var (
	ErrRoleNotFound   = errors.New("there is no such role")
	ErrRoleKeyTaken   = errors.New("another role has that key")
	ErrRoleInUse      = errors.New("the role is or was held or carried; it stays for the history")
	ErrRoleCarried    = errors.New("the position carries that role then already")
	ErrRoleNotCarried = errors.New("the position does not carry that role on that day")
	ErrDeputyNotThere = errors.New("a deputy must be another position that exists today, named once")
)

var roleKey = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

func (in *RoleInput) check(creating bool) error {
	if creating && in.Key == nil {
		return Invalid{"key", "Give a key."}
	}
	if creating && in.Name == nil {
		return Invalid{"name", "Give a name."}
	}
	if in.Key != nil && !roleKey.MatchString(*in.Key) {
		return Invalid{"key", "Use small letters, digits and dashes, up to 63."}
	}
	if err := checkText("name", in.Name, 1, 200); err != nil {
		return err
	}
	if in.Description != nil && utf8.RuneCountInString(*in.Description) > 4000 {
		return Invalid{"description", "Keep it to 4000 characters."}
	}
	return nil
}

func scanRole(row pgx.CollectableRow) (Role, error) {
	var r Role
	err := row.Scan(&r.ID, &r.Key, &r.Name, &r.Description)
	return r, err
}

// Roles lists every role by name.
func (s *Service) Roles(ctx context.Context) ([]Role, error) {
	var out []Role
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		rows, err := q.Query(ctx, `SELECT id, key, name, description FROM role ORDER BY lower(name), id`)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, scanRole)
		return err
	})
	return out, err
}

// Role is one role with the positions carrying it and who has it on a day.
func (s *Service) Role(ctx context.Context, id uuid.UUID, asOf Day) (Role, error) {
	var out Role
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		return role(ctx, q, id, asOf, &out)
	})
	return out, err
}

func role(ctx context.Context, q db.DBTX, id uuid.UUID, asOf Day, out *Role) error {
	rows, err := q.Query(ctx, `SELECT id, key, name, description FROM role WHERE id = $1`, id)
	if err != nil {
		return err
	}
	*out, err = pgx.CollectExactlyOneRow(rows, scanRole)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrRoleNotFound
	}
	if err != nil {
		return err
	}
	err = q.QueryRow(ctx, `SELECT coalesce(array_agg(position_id ORDER BY position_id), '{}') FROM position_role
		WHERE role_id = $1 AND valid_from <= $2 AND (valid_to IS NULL OR valid_to > $2)`, id, asOf.Time()).Scan(&out.Positions)
	if err != nil {
		return err
	}
	rows, err = q.Query(ctx, `SELECT h.person_id, h.id, h.share, h.position_id FROM holding h
		WHERE h.role_id = $1 AND h.valid_from <= $2 AND (h.valid_to IS NULL OR h.valid_to > $2)
		UNION ALL
		SELECT h.person_id, h.id, h.share, h.position_id FROM holding h
		JOIN position_role r ON r.position_id = h.position_id AND r.role_id = $1
			AND r.valid_from <= $2 AND (r.valid_to IS NULL OR r.valid_to > $2)
		WHERE h.valid_from <= $2 AND (h.valid_to IS NULL OR h.valid_to > $2)
		ORDER BY 4 NULLS FIRST, 1`, id, asOf.Time())
	if err != nil {
		return err
	}
	out.Holders, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (RoleHolder, error) {
		var h RoleHolder
		err := row.Scan(&h.PersonID, &h.HoldingID, &h.Share, &h.Through)
		return h, err
	})
	return err
}

// CreateRole adds a role, as an editor.
func (s *Service) CreateRole(ctx context.Context, in RoleInput) (Role, db.LSN, error) {
	if err := in.check(true); err != nil {
		return Role{}, 0, err
	}
	var out Role
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		var id uuid.UUID
		if err := q.QueryRow(ctx, `INSERT INTO role (key, name, description) VALUES ($1, $2, coalesce($3, '')) RETURNING id`,
			in.Key, in.Name, in.Description).Scan(&id); err != nil {
			return roleError(err)
		}
		return role(ctx, q, id, DayOf(time.Now()), &out)
	})
	return out, lsn, err
}

// UpdateRole changes a role's key, name or description, as an editor.
func (s *Service) UpdateRole(ctx context.Context, id uuid.UUID, in RoleInput, asOf Day) (Role, db.LSN, error) {
	if err := in.check(false); err != nil {
		return Role{}, 0, err
	}
	var out Role
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		tag, err := q.Exec(ctx, `UPDATE role SET key = coalesce($2, key), name = coalesce($3, name),
			description = coalesce($4, description) WHERE id = $1`, id, in.Key, in.Name, in.Description)
		if err != nil {
			return roleError(err)
		}
		if tag.RowsAffected() == 0 {
			return ErrRoleNotFound
		}
		return role(ctx, q, id, asOf, &out)
	})
	return out, lsn, err
}

// DeleteRole removes a role nobody ever held and no position ever carried, as an editor.
func (s *Service) DeleteRole(ctx context.Context, id uuid.UUID) (db.LSN, error) {
	return s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		tag, err := q.Exec(ctx, `DELETE FROM role WHERE id = $1`, id)
		if pgErr := (*pgconn.PgError)(nil); errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return ErrRoleInUse
		}
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrRoleNotFound
		}
		return nil
	})
}

// CarryRole has a position carry a role from a day, as an editor, so whoever holds it has the role.
func (s *Service) CarryRole(ctx context.Context, id, roleID uuid.UUID, from Day) (Position, db.LSN, error) {
	var out Position
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		exists, archived, err := lockPosition(ctx, q, id)
		if err != nil {
			return err
		}
		if !livesOn(exists, archived, from) {
			return ErrPositionNotThere
		}
		var to *time.Time
		if archived != nil {
			t := archived.Time()
			to = &t
		}
		if _, err := q.Exec(ctx, `INSERT INTO position_role (position_id, role_id, valid_from, valid_to) VALUES ($1, $2, $3, $4)`,
			id, roleID, from.Time(), to); err != nil {
			return roleError(err)
		}
		return s.position(ctx, q, id, from, &out)
	})
	return out, lsn, err
}

// DropRole has a position no longer carry a role from a day, as an editor.
func (s *Service) DropRole(ctx context.Context, id, roleID uuid.UUID, on Day) (Position, db.LSN, error) {
	var out Position
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		if _, _, err := lockPosition(ctx, q, id); err != nil {
			return err
		}
		var start time.Time
		err := q.QueryRow(ctx, `SELECT valid_from FROM position_role WHERE position_id = $1 AND role_id = $2
			AND valid_from <= $3 AND (valid_to IS NULL OR valid_to > $3)`, id, roleID, on.Time()).Scan(&start)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrRoleNotCarried
		}
		if err != nil {
			return err
		}
		if DayOf(start) == on {
			_, err = q.Exec(ctx, `DELETE FROM position_role WHERE position_id = $1 AND role_id = $2 AND valid_from = $3`, id, roleID, start)
		} else {
			_, err = q.Exec(ctx, `UPDATE position_role SET valid_to = $4 WHERE position_id = $1 AND role_id = $2 AND valid_from = $3`,
				id, roleID, start, on.Time())
		}
		if err != nil {
			return err
		}
		return s.position(ctx, q, id, on, &out)
	})
	return out, lsn, err
}

// SetDeputies replaces a position's deputies, as an editor, first to stand in first. Deputies are a standing order,
// not dated; one archived later drops out of the order from then.
func (s *Service) SetDeputies(ctx context.Context, id uuid.UUID, deputies []uuid.UUID) (Position, db.LSN, error) {
	sorted := slices.Clone(deputies)
	slices.SortFunc(sorted, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
	if slices.Contains(deputies, id) || len(slices.Compact(sorted)) != len(deputies) {
		return Position{}, 0, ErrDeputyNotThere
	}
	today := DayOf(time.Now())
	var out Position
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		if _, _, err := lockPosition(ctx, q, id); err != nil {
			return err
		}
		var there int
		if err := q.QueryRow(ctx, `SELECT count(*) FROM position WHERE id = ANY ($1)
			AND exists_from <= $2 AND (archived_on IS NULL OR archived_on > $2)`, deputies, today.Time()).Scan(&there); err != nil {
			return err
		}
		if there != len(deputies) {
			return ErrDeputyNotThere
		}
		if _, err := q.Exec(ctx, `DELETE FROM deputy WHERE position_id = $1`, id); err != nil {
			return err
		}
		if _, err := q.Exec(ctx, `INSERT INTO deputy (position_id, deputy_id, rank)
			SELECT $1, d, n FROM unnest($2::uuid[]) WITH ORDINALITY AS t (d, n)`, id, deputies); err != nil {
			return err
		}
		return s.position(ctx, q, id, today, &out)
	})
	return out, lsn, err
}

func roleError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	switch pgErr.ConstraintName {
	case "role_key_key":
		return ErrRoleKeyTaken
	case "position_role_once":
		return ErrRoleCarried
	case "position_role_role_id_fkey", "holding_role_id_fkey":
		return ErrRoleNotFound
	}
	return err
}
