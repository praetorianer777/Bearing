package org

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/praetorianer777/bearing/backend/internal/db"
)

// PublishedStep is a node of the newest published version of a process with who its responsibilities come down to.
type PublishedStep struct {
	ProcessID   uuid.UUID
	ProcessKey  string
	ProcessName string
	Version     int
	StepPeople
}

// Duty is one responsibility somebody or something has: on which step, of which kind, through which party, and
// who it comes down to on the day.
type Duty struct {
	ProcessID   uuid.UUID
	ProcessKey  string
	ProcessName string
	Version     int
	StepKey     string
	StepName    string
	StepKind    NodeKind
	ResolvedResponsibility
}

// Subject is what duties are asked of: one person, position, role or unit.
type Subject struct {
	PersonID   *uuid.UUID
	PositionID *uuid.UUID
	RoleID     *uuid.UUID
	UnitID     *uuid.UUID
}

// publishedSteps reads the nodes of every process's newest published version, with their responsibilities resolved
// on a day; with match, only nodes whose name or whose process's name holds it, those without responsibilities too.
func publishedSteps(ctx context.Context, q db.DBTX, asOf Day, match string) ([]PublishedStep, error) {
	pattern := ""
	if match != "" {
		pattern = "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(match) + "%"
	}
	rows, err := q.Query(ctx, `WITH newest AS (
			SELECT DISTINCT ON (process_id) id, process_id, number FROM process_version
			WHERE number IS NOT NULL ORDER BY process_id, number DESC
		)
		SELECT p.id, p.key, p.name, nw.number, n.key, n.name, n.kind, r.kind, r.role_id, r.position_id, r.unit_id
		FROM newest nw
		JOIN process p ON p.id = nw.process_id
		JOIN process_node n ON n.version_id = nw.id AND n.kind NOT IN ('start', 'end')
		LEFT JOIN process_responsibility r ON r.version_id = n.version_id AND r.node_key = n.key
		WHERE CASE WHEN $1 = '' THEN r.kind IS NOT NULL ELSE n.name ILIKE $1 OR p.name ILIKE $1 END
		ORDER BY lower(p.name), p.id, n.y, n.x, n.key, r.kind, r.role_id, r.position_id, r.unit_id`, pattern)
	if err != nil {
		return nil, err
	}
	var out []PublishedStep
	var all []Responsibility
	var (
		step PublishedStep
		kind *RACI
		r    Responsibility
	)
	_, err = pgx.ForEachRow(rows, []any{&step.ProcessID, &step.ProcessKey, &step.ProcessName, &step.Version, &step.Key, &step.Name, &step.Kind,
		&kind, &r.RoleID, &r.PositionID, &r.UnitID}, func() error {
		if n := len(out); n == 0 || out[n-1].ProcessID != step.ProcessID || out[n-1].Key != step.Key {
			out = append(out, PublishedStep{ProcessID: step.ProcessID, ProcessKey: step.ProcessKey, ProcessName: step.ProcessName,
				Version: step.Version, StepPeople: StepPeople{Key: step.Key, Name: step.Name, Kind: step.Kind}})
		}
		if kind != nil {
			r.Kind = *kind
			last := &out[len(out)-1]
			last.Responsibilities = append(last.Responsibilities, ResolvedResponsibility{Responsibility: r})
			all = append(all, r)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	w, err := standingFor(ctx, q, all, asOf)
	if err != nil {
		return nil, err
	}
	for i := range out {
		for j := range out[i].Responsibilities {
			out[i].Responsibilities[j].People = w.resolve(out[i].Responsibilities[j].Responsibility)
		}
	}
	return out, nil
}

// WhoDoes is who the steps of the newest published versions whose name, or whose process's name, holds a term come
// down to on a day.
func (s *Service) WhoDoes(ctx context.Context, term string, asOf Day) ([]PublishedStep, error) {
	var out []PublishedStep
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		var err error
		out, err = publishedSteps(ctx, q, asOf, strings.TrimSpace(term))
		return err
	})
	return out, err
}

// Duties is every responsibility a subject has on a day across the newest published versions: a person's are those
// that come down to them, by whatever position, role, headship or deputyship; a position's those naming it, or a role
// it carries, or a unit it heads, or a vacant position it stands in for; a role's those naming it; a unit's those
// naming it or a position in it.
func (s *Service) Duties(ctx context.Context, subject Subject, asOf Day) ([]Duty, error) {
	var out []Duty
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		steps, err := publishedSteps(ctx, q, asOf, "")
		if err != nil {
			return err
		}
		var inUnit map[uuid.UUID]bool
		if subject.UnitID != nil {
			rows, err := q.Query(ctx, `SELECT id FROM position WHERE unit_id = $1
				AND exists_from <= $2 AND (archived_on IS NULL OR archived_on > $2)`, subject.UnitID, asOf.Time())
			if err != nil {
				return err
			}
			ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
			if err != nil {
				return err
			}
			inUnit = make(map[uuid.UUID]bool, len(ids))
			for _, id := range ids {
				inUnit[id] = true
			}
		}
		out = dutiesOf(steps, subject, inUnit)
		return nil
	})
	return out, err
}

// dutiesOf picks a subject's duties from resolved steps; inUnit are the positions of the unit asked about.
func dutiesOf(steps []PublishedStep, subject Subject, inUnit map[uuid.UUID]bool) []Duty {
	same := func(a, b *uuid.UUID) bool { return a != nil && b != nil && *a == *b }
	out := []Duty{}
	for _, step := range steps {
		for _, r := range step.Responsibilities {
			ours := false
			switch {
			case subject.PersonID != nil:
				for _, p := range r.People {
					ours = ours || p.PersonID == *subject.PersonID
				}
			case subject.PositionID != nil:
				ours = same(r.PositionID, subject.PositionID)
				for _, p := range r.People {
					ours = ours || same(p.PositionID, subject.PositionID)
				}
			case subject.RoleID != nil:
				ours = same(r.RoleID, subject.RoleID)
			case subject.UnitID != nil:
				ours = same(r.UnitID, subject.UnitID) || (r.PositionID != nil && inUnit[*r.PositionID])
			}
			if ours {
				out = append(out, Duty{ProcessID: step.ProcessID, ProcessKey: step.ProcessKey, ProcessName: step.ProcessName,
					Version: step.Version, StepKey: step.Key, StepName: step.Name, StepKind: step.Kind, ResolvedResponsibility: r})
			}
		}
	}
	return out
}
