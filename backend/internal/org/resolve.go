package org

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/praetorianer777/bearing/backend/internal/db"
)

// Resolved is somebody a responsibility comes down to on a day.
type Resolved struct {
	PersonID uuid.UUID
	Name     string
	Share    int
	// PositionID is the position that brings them in; nil when they hold a role directly.
	PositionID *uuid.UUID
	// DeputyFor is the vacant position they stand in for, through PositionID, its deputy.
	DeputyFor *uuid.UUID
}

// ResolvedResponsibility is a responsibility with the people it comes down to; none when nobody holds it, even by
// deputy.
type ResolvedResponsibility struct {
	Responsibility
	People []Resolved
}

// StepPeople is a node of a process with who its responsibilities come down to.
type StepPeople struct {
	Key              string
	Name             string
	Kind             NodeKind
	Responsibilities []ResolvedResponsibility
}

// ProcessPeople is who a version of a process comes down to on a day, node by node.
type ProcessPeople struct {
	// Version is the published version read; nil for the draft.
	Version *int
	Steps   []StepPeople
}

// standing is what the organisation looks like on a day as far as some responsibilities need it.
type standing struct {
	holders     map[uuid.UUID][]Resolved
	deputies    map[uuid.UUID][]uuid.UUID
	roleHolders map[uuid.UUID][]Resolved
	carriers    map[uuid.UUID][]uuid.UUID
	heads       map[uuid.UUID]uuid.UUID
}

// position is who a position comes down to: its holders, or when it has none, the holders of its first deputy that
// has any, standing in. Deputies stand in once; a deputy's own deputies do not.
func (w standing) position(id uuid.UUID) []Resolved {
	if held := w.holders[id]; len(held) > 0 {
		return held
	}
	for _, d := range w.deputies[id] {
		if held := w.holders[d]; len(held) > 0 {
			out := make([]Resolved, len(held))
			for i, r := range held {
				vacant := id
				r.DeputyFor = &vacant
				out[i] = r
			}
			return out
		}
	}
	return nil
}

// resolve is who a responsibility comes down to. A role is whoever holds it directly and whoever a position carrying
// it comes down to; a unit is spoken for by its head.
func (w standing) resolve(r Responsibility) []Resolved {
	switch {
	case r.PositionID != nil:
		return w.position(*r.PositionID)
	case r.UnitID != nil:
		head, ok := w.heads[*r.UnitID]
		if !ok {
			return nil
		}
		return w.position(head)
	case r.RoleID != nil:
		out := append([]Resolved(nil), w.roleHolders[*r.RoleID]...)
		for _, p := range w.carriers[*r.RoleID] {
			out = append(out, w.position(p)...)
		}
		return out
	}
	return nil
}

// ProcessPeople is who the nodes of a version of a process come down to on a day: the newest published version when
// version is nil, or the draft with draft set, or the draft when nothing is published yet.
func (s *Service) ProcessPeople(ctx context.Context, id uuid.UUID, version *int, draft bool, asOf Day) (ProcessPeople, error) {
	var out ProcessPeople
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		var versionID uuid.UUID
		err := q.QueryRow(ctx, `SELECT v.id, v.number FROM process_version v WHERE v.process_id = $1
			AND CASE WHEN $3 THEN v.number IS NULL WHEN $2::int IS NOT NULL THEN v.number = $2
				ELSE v.number IS NOT DISTINCT FROM (SELECT max(number) FROM process_version WHERE process_id = $1) END`,
			id, version, draft).Scan(&versionID, &out.Version)
		if errors.Is(err, pgx.ErrNoRows) {
			var exists bool
			if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM process WHERE id = $1)`, id).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return ErrProcessNotFound
			}
			return ErrVersionNotFound
		}
		if err != nil {
			return err
		}
		g, err := graph(ctx, q, versionID)
		if err != nil {
			return err
		}
		var all []Responsibility
		for _, n := range g.Nodes {
			all = append(all, n.Responsibilities...)
		}
		w, err := standingFor(ctx, q, all, asOf)
		if err != nil {
			return err
		}
		for _, n := range g.Nodes {
			if len(n.Responsibilities) == 0 {
				continue
			}
			step := StepPeople{Key: n.Key, Name: n.Name, Kind: n.Kind}
			for _, r := range n.Responsibilities {
				step.Responsibilities = append(step.Responsibilities, ResolvedResponsibility{Responsibility: r, People: w.resolve(r)})
			}
			out.Steps = append(out.Steps, step)
		}
		return nil
	})
	return out, err
}

// standingFor reads, in a few queries, as much of the organisation on a day as the responsibilities need.
func standingFor(ctx context.Context, q db.DBTX, all []Responsibility, asOf Day) (standing, error) {
	w := standing{
		holders: map[uuid.UUID][]Resolved{}, deputies: map[uuid.UUID][]uuid.UUID{}, roleHolders: map[uuid.UUID][]Resolved{},
		carriers: map[uuid.UUID][]uuid.UUID{}, heads: map[uuid.UUID]uuid.UUID{},
	}
	var positions, roles, units []uuid.UUID
	for _, r := range all {
		switch {
		case r.PositionID != nil:
			positions = append(positions, *r.PositionID)
		case r.RoleID != nil:
			roles = append(roles, *r.RoleID)
		case r.UnitID != nil:
			units = append(units, *r.UnitID)
		}
	}
	day := asOf.Time()
	const live = `p.exists_from <= $2 AND (p.archived_on IS NULL OR p.archived_on > $2)`
	pairs := func(sql string, args ...any) ([][2]uuid.UUID, error) {
		rows, err := q.Query(ctx, sql, args...)
		if err != nil {
			return nil, err
		}
		return pgx.CollectRows(rows, func(row pgx.CollectableRow) ([2]uuid.UUID, error) {
			var p [2]uuid.UUID
			err := row.Scan(&p[0], &p[1])
			return p, err
		})
	}

	heads, err := pairs(`SELECT p.unit_id, p.id FROM position p WHERE p.unit_id = ANY ($1) AND p.head_of_unit AND `+live, units, day)
	if err != nil {
		return w, err
	}
	for _, h := range heads {
		w.heads[h[0]] = h[1]
		positions = append(positions, h[1])
	}
	carriers, err := pairs(`SELECT r.role_id, r.position_id FROM position_role r JOIN position p ON p.id = r.position_id
		WHERE r.role_id = ANY ($1) AND r.valid_from <= $2 AND (r.valid_to IS NULL OR r.valid_to > $2) AND `+live+`
		ORDER BY r.role_id, p.title, p.id`, roles, day)
	if err != nil {
		return w, err
	}
	for _, c := range carriers {
		w.carriers[c[0]] = append(w.carriers[c[0]], c[1])
		positions = append(positions, c[1])
	}
	deputies, err := pairs(`SELECT d.position_id, d.deputy_id FROM deputy d JOIN position p ON p.id = d.deputy_id
		WHERE d.position_id = ANY ($1) AND `+live+` ORDER BY d.position_id, d.rank`, positions, day)
	if err != nil {
		return w, err
	}
	for _, d := range deputies {
		w.deputies[d[0]] = append(w.deputies[d[0]], d[1])
		positions = append(positions, d[1])
	}

	rows, err := q.Query(ctx, `SELECT h.position_id, h.role_id, h.person_id, n.name, h.share FROM holding h JOIN person n ON n.id = h.person_id
		WHERE (h.position_id = ANY ($1) OR h.role_id = ANY ($3)) AND h.valid_from <= $2 AND (h.valid_to IS NULL OR h.valid_to > $2)
		ORDER BY n.name, h.person_id`, positions, day, roles)
	if err != nil {
		return w, err
	}
	var position, role *uuid.UUID
	var r Resolved
	_, err = pgx.ForEachRow(rows, []any{&position, &role, &r.PersonID, &r.Name, &r.Share}, func() error {
		if position != nil {
			held := *position
			r.PositionID = &held
			w.holders[held] = append(w.holders[held], r)
		} else {
			r.PositionID = nil
			w.roleHolders[*role] = append(w.roleHolders[*role], r)
		}
		return nil
	})
	return w, err
}
