package org

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/praetorianer777/bearing/backend/internal/db"
)

// ErrNoLevels is a walk up the management chain that asks for no level.
var ErrNoLevels = errors.New("count the levels up from one")

// Entry is a person as the directory has them on a day: the positions they hold, where, and the roles they have.
type Entry struct {
	Person    Person
	Positions []EntryPosition
	Roles     []EntryRole
}

// EntryPosition is a position somebody holds on a day, with the unit it sits in.
type EntryPosition struct {
	PositionID uuid.UUID
	Title      string
	UnitID     uuid.UUID
	UnitName   string
	HeadOfUnit bool
	Share      int
}

// EntryRole is a role somebody has on a day, held directly or through a position carrying it.
type EntryRole struct {
	RoleID uuid.UUID
	Key    string
	Name   string
	// Through is the position they have it through; nil when they hold it directly.
	Through *uuid.UUID
}

// Entry is what the person holds on a day, heads of units first.
func (s *Service) Entry(ctx context.Context, p Person, asOf Day) (Entry, error) {
	out := Entry{Person: p}
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		rows, err := q.Query(ctx, `SELECT p.id, p.title, p.unit_id, u.name, p.head_of_unit, h.share
			FROM holding h JOIN position p ON p.id = h.position_id JOIN unit u ON u.id = p.unit_id
			WHERE h.person_id = $1 AND h.valid_from <= $2 AND (h.valid_to IS NULL OR h.valid_to > $2)
			ORDER BY p.head_of_unit DESC, lower(p.title), p.id`, p.ID, asOf.Time())
		if err != nil {
			return err
		}
		if out.Positions, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (EntryPosition, error) {
			var e EntryPosition
			err := row.Scan(&e.PositionID, &e.Title, &e.UnitID, &e.UnitName, &e.HeadOfUnit, &e.Share)
			return e, err
		}); err != nil {
			return err
		}
		rows, err = q.Query(ctx, `SELECT r.id, r.key, r.name, NULL::uuid FROM holding h JOIN role r ON r.id = h.role_id
			WHERE h.person_id = $1 AND h.valid_from <= $2 AND (h.valid_to IS NULL OR h.valid_to > $2)
			UNION ALL
			SELECT r.id, r.key, r.name, h.position_id FROM holding h
			JOIN position_role c ON c.position_id = h.position_id AND c.valid_from <= $2 AND (c.valid_to IS NULL OR c.valid_to > $2)
			JOIN role r ON r.id = c.role_id
			WHERE h.person_id = $1 AND h.valid_from <= $2 AND (h.valid_to IS NULL OR h.valid_to > $2)
			ORDER BY 3, 1, 4 NULLS FIRST`, p.ID, asOf.Time())
		if err != nil {
			return err
		}
		out.Roles, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (EntryRole, error) {
			var e EntryRole
			err := row.Scan(&e.RoleID, &e.Key, &e.Name, &e.Through)
			return e, err
		})
		return err
	})
	return out, err
}

// RolePeople are who a role comes down to on a day: whoever holds it directly, and the holders of the positions
// carrying it, or their deputies standing in where one is vacant.
func (s *Service) RolePeople(ctx context.Context, id uuid.UUID, asOf Day) ([]Resolved, error) {
	var out []Resolved
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		var found bool
		if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM role WHERE id = $1)`, id).Scan(&found); err != nil {
			return err
		}
		if !found {
			return ErrRoleNotFound
		}
		r := Responsibility{RoleID: &id}
		w, err := standingFor(ctx, q, []Responsibility{r}, asOf)
		out = w.resolve(r)
		return err
	})
	return out, err
}

// Manager is a position a given number of solid lines above somebody, with who it comes down to on the day.
type Manager struct {
	Link
	// People are its holders, or when it is vacant its first deputy's, standing in; none when nobody is there.
	People []Resolved
}

// Managers are the positions exactly levels solid lines above those a person holds on a day; none when the chain
// ends sooner.
func (s *Service) Managers(ctx context.Context, personID uuid.UUID, asOf Day, levels int) ([]Manager, error) {
	if levels < 1 {
		return nil, ErrNoLevels
	}
	chain, err := s.ChainOfPerson(ctx, personID, asOf, Walk{})
	if err != nil {
		return nil, err
	}
	out := atLevel(chain, levels)
	asked := make([]Responsibility, len(out))
	for i := range out {
		asked[i] = Responsibility{PositionID: &out[i].PositionID}
	}
	if len(out) == 0 {
		return nil, nil
	}
	err = s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		w, err := standingFor(ctx, q, asked, asOf)
		for i := range out {
			out[i].People = w.position(out[i].PositionID)
		}
		return err
	})
	return out, err
}

// atLevel are the links exactly levels lines away, in the walk's order.
func atLevel(chain []Link, levels int) []Manager {
	var out []Manager
	for _, l := range chain {
		if l.Depth == levels {
			out = append(out, Manager{Link: l})
		}
	}
	return out
}

// Subtree is the unit and every unit inside it on a day, each after the one it is part of, with how deep it sits
// below the first: zero for the unit itself.
func (s *Service) Subtree(ctx context.Context, id uuid.UUID, asOf Day) ([]Unit, []int, error) {
	root, err := s.Unit(ctx, id, asOf)
	if err != nil {
		return nil, nil, err
	}
	all, err := s.Units(ctx, asOf)
	if err != nil {
		return nil, nil, err
	}
	root.Placements = nil
	units, depths := subtree(root, all)
	return units, depths, nil
}

// subtree orders root and the units below it, breadth first, from the units of one day.
func subtree(root Unit, all []Unit) ([]Unit, []int) {
	children := map[uuid.UUID][]Unit{}
	for _, u := range all {
		if u.ParentID != nil {
			children[*u.ParentID] = append(children[*u.ParentID], u)
		}
	}
	units, depths := []Unit{root}, []int{0}
	for i := 0; i < len(units); i++ {
		for _, c := range children[units[i].ID] {
			units, depths = append(units, c), append(depths, depths[i]+1)
		}
	}
	return units, depths
}
