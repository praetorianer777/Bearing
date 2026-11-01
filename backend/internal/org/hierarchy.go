package org

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/praetorianer777/bearing/backend/internal/db"
)

// Link is a position reached over reporting lines on a day: upward a manager's, downward a report's.
type Link struct {
	PositionID uuid.UUID
	Title      string
	UnitID     uuid.UUID
	// From is the position it was reached from: the report going up, the manager going down.
	From uuid.UUID
	Kind LineKind
	// Depth is how many lines away it is, one for the nearest.
	Depth int
	// Holders are who holds it on that day; none means it is vacant then.
	Holders []Holder
}

// Holder is somebody holding a position on a day.
type Holder struct {
	PersonID uuid.UUID
	Name     string
	Share    int
	// Issuer and Subject are how the realm knows them; nil for somebody it does not.
	Issuer, Subject *string
}

// Member is somebody holding a position in a unit on a day.
type Member struct {
	PersonID   uuid.UUID
	Name       string
	Issuer     *string
	Subject    *string
	PositionID uuid.UUID
	Title      string
	UnitID     uuid.UUID
	HeadOfUnit bool
	HoldingID  uuid.UUID
	Share      int
}

// Walk says how far to follow lines: dotted ones as well as solid, and for reports, beyond the direct ones.
type Walk struct {
	Dotted bool
	All    bool
}

// direction is which way a walk follows reporting lines: from the column it stands on to the one it goes to.
type direction struct{ near, far string }

var (
	up   = direction{near: "position_id", far: "reports_to_id"}
	down = direction{near: "reports_to_id", far: "position_id"}
)

// walkQuery follows lines from the positions in $1 on the day in $2, beyond the nearest when $3. Solid lines form a
// tree, so a walk over them alone reaches each position once; dotted lines may run in circles, so a walk over them
// keeps its path and does not go round. Holders come as a row each, which decodes far faster than arrays per
// position, and the order is left to the caller, where sorting thousands of titles costs less than under the
// database's collation.
func walkQuery(d direction, dotted bool) string {
	kind, path, step, guard := `AND l.kind = 'solid'`, ``, ``, ``
	if dotted {
		kind, path, step, guard = ``, `, path`, `, ARRAY[l.near, l.far]`, ` AND l.far <> ALL (r.path)`
	}
	q := `WITH RECURSIVE reached (position_id, from_id, kind, depth` + path + `) AS (
			SELECT l.far, l.near, l.kind, 1` + step + ` FROM reporting_line l
			WHERE l.near = ANY ($1) AND l.valid_from <= $2 AND (l.valid_to IS NULL OR l.valid_to > $2) ` + kind + `
			UNION ALL
			SELECT l.far, l.near, l.kind, r.depth + 1` + strings.Replace(step, "ARRAY[l.near, l.far]", "r.path || l.far", 1) + `
			FROM reporting_line l JOIN reached r ON l.near = r.position_id
			WHERE $3 AND l.valid_from <= $2 AND (l.valid_to IS NULL OR l.valid_to > $2) ` + kind + guard + `
		)
		SELECT r.position_id, p.title, p.unit_id, p.head_of_unit, r.from_id, r.kind, r.depth, h.id, h.person_id, pe.name, h.share, pe.issuer, pe.subject
		FROM reached r JOIN position p ON p.id = r.position_id
		LEFT JOIN (holding h JOIN person pe ON pe.id = h.person_id)
			ON h.position_id = r.position_id AND h.valid_from <= $2 AND (h.valid_to IS NULL OR h.valid_to > $2)
		ORDER BY h.valid_from, h.id`
	return strings.NewReplacer("l.near", "l."+d.near, "l.far", "l."+d.far).Replace(q)
}

// ChainOfPosition is the positions a position reports to on a day, nearest first, all the way up.
func (s *Service) ChainOfPosition(ctx context.Context, id uuid.UUID, asOf Day, w Walk) ([]Link, error) {
	w.All = true
	return s.walkFromPosition(ctx, id, asOf, up, w)
}

// ReportsOfPosition are the positions reporting to a position on a day, nearest first.
func (s *Service) ReportsOfPosition(ctx context.Context, id uuid.UUID, asOf Day, w Walk) ([]Link, error) {
	return s.walkFromPosition(ctx, id, asOf, down, w)
}

// ChainOfPerson is the positions above those a person holds on a day, nearest first, all the way up.
func (s *Service) ChainOfPerson(ctx context.Context, id uuid.UUID, asOf Day, w Walk) ([]Link, error) {
	w.All = true
	return s.walkFromPerson(ctx, id, asOf, up, w)
}

// ReportsOfPerson are the positions below those a person holds on a day, nearest first.
func (s *Service) ReportsOfPerson(ctx context.Context, id uuid.UUID, asOf Day, w Walk) ([]Link, error) {
	return s.walkFromPerson(ctx, id, asOf, down, w)
}

func (s *Service) walkFromPosition(ctx context.Context, id uuid.UUID, asOf Day, d direction, w Walk) ([]Link, error) {
	var out []Link
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		var found bool
		if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM position WHERE id = $1)`, id).Scan(&found); err != nil {
			return err
		}
		if !found {
			return ErrPositionNotFound
		}
		var err error
		out, err = walk(ctx, q, []uuid.UUID{id}, asOf, d, w)
		return err
	})
	return out, err
}

func (s *Service) walkFromPerson(ctx context.Context, id uuid.UUID, asOf Day, d direction, w Walk) ([]Link, error) {
	var out []Link
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		var held []uuid.UUID
		err := q.QueryRow(ctx, `SELECT coalesce(array_agg(h.position_id) FILTER (WHERE h.position_id IS NOT NULL
				AND h.valid_from <= $2 AND (h.valid_to IS NULL OR h.valid_to > $2)), '{}')
			FROM person p LEFT JOIN holding h ON h.person_id = p.id WHERE p.id = $1 GROUP BY p.id`, id, asOf.Time()).Scan(&held)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrPersonNotFound
		}
		if err != nil {
			return err
		}
		out, err = walk(ctx, q, held, asOf, d, w)
		return err
	})
	return out, err
}

// walk lists each position reached once, where it is nearest and by a solid line before a dotted one, leaving out
// those it started from; nearest first, then heads, then by title.
func walk(ctx context.Context, q db.DBTX, from []uuid.UUID, asOf Day, d direction, w Walk) ([]Link, error) {
	if len(from) == 0 {
		return nil, nil
	}
	rows, err := q.Query(ctx, walkQuery(d, w.Dotted), from, asOf.Time(), w.All)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type reached struct {
		Link
		head  bool
		lower string
	}
	nearer := func(a, b reached) bool {
		return a.Depth < b.Depth || a.Depth == b.Depth && a.Kind == Solid && b.Kind != Solid
	}
	at := map[uuid.UUID]int{}
	held := map[uuid.UUID][]Holder{}
	seen := map[uuid.UUID]bool{}
	var kept []reached
	for rows.Next() {
		var r reached
		var holding, person *uuid.UUID
		var name, issuer, subject *string
		var share *int
		if err := rows.Scan(&r.PositionID, &r.Title, &r.UnitID, &r.head, &r.From, &r.Kind, &r.Depth, &holding, &person, &name, &share, &issuer, &subject); err != nil {
			return nil, err
		}
		if holding != nil && !seen[*holding] {
			seen[*holding] = true
			held[r.PositionID] = append(held[r.PositionID], Holder{PersonID: *person, Name: *name, Share: *share, Issuer: issuer, Subject: subject})
		}
		if slices.Contains(from, r.PositionID) {
			continue
		}
		if i, ok := at[r.PositionID]; ok {
			if nearer(r, kept[i]) {
				kept[i] = r
			}
			continue
		}
		at[r.PositionID] = len(kept)
		kept = append(kept, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range kept {
		kept[i].lower, kept[i].Holders = strings.ToLower(kept[i].Title), held[kept[i].PositionID]
	}
	slices.SortFunc(kept, func(a, b reached) int {
		return cmp.Or(cmp.Compare(a.Depth, b.Depth), compareBool(b.head, a.head), strings.Compare(a.lower, b.lower),
			slices.Compare(a.PositionID[:], b.PositionID[:]))
	})
	out := make([]Link, len(kept))
	for i, r := range kept {
		out[i] = r.Link
	}
	return out, nil
}

func compareBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	}
	return -1
}

// Members are the people holding positions in a unit on a day, and with subUnits, in the units inside it then;
// heads first.
func (s *Service) Members(ctx context.Context, unitID uuid.UUID, asOf Day, subUnits bool) ([]Member, error) {
	var out []Member
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		var found bool
		if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM unit WHERE id = $1)`, unitID).Scan(&found); err != nil {
			return err
		}
		if !found {
			return ErrNotFound
		}
		rows, err := q.Query(ctx, `WITH RECURSIVE tree (id) AS (
				SELECT $1::uuid
				UNION ALL
				SELECT pl.unit_id FROM unit_placement pl JOIN tree t ON pl.parent_id = t.id
				WHERE $3 AND pl.valid_from <= $2 AND (pl.valid_to IS NULL OR pl.valid_to > $2)
			)
			SELECT h.person_id, pe.name, pe.issuer, pe.subject, p.id, p.title, p.unit_id, p.head_of_unit, h.id, h.share
			FROM tree t JOIN position p ON p.unit_id = t.id
			JOIN holding h ON h.position_id = p.id AND h.valid_from <= $2 AND (h.valid_to IS NULL OR h.valid_to > $2)
			JOIN person pe ON pe.id = h.person_id`, unitID, asOf.Time(), subUnits)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Member, error) {
			var m Member
			err := row.Scan(&m.PersonID, &m.Name, &m.Issuer, &m.Subject, &m.PositionID, &m.Title, &m.UnitID, &m.HeadOfUnit, &m.HoldingID, &m.Share)
			return m, err
		})
		if err != nil {
			return err
		}
		// The unit's own people first, its head leading, then by name; sorted here, as walks are, for speed.
		names := make(map[uuid.UUID]string, len(out))
		for _, m := range out {
			names[m.HoldingID] = strings.ToLower(m.Name)
		}
		slices.SortFunc(out, func(a, b Member) int {
			return cmp.Or(compareBool(b.UnitID == unitID, a.UnitID == unitID), compareBool(b.HeadOfUnit, a.HeadOfUnit),
				strings.Compare(names[a.HoldingID], names[b.HoldingID]), slices.Compare(a.HoldingID[:], b.HoldingID[:]))
		})
		return nil
	})
	return out, err
}
