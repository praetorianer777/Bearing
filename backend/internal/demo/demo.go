// Package demo loads a demo organisation into an installation that has none yet, to look at and to test against.
package demo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/db"
	"github.com/praetorianer777/bearing/backend/internal/org"
)

var (
	ErrLoaded   = errors.New("the demo is loaded already")
	ErrNotEmpty = errors.New("the installation has an organisation or processes of its own; the demo goes only where there are none")
)

// Load adds what of the demo may go in, as an administrator, all of it in one transaction or nothing: the
// organisation into an installation without units, and its processes into one without processes whose organisation
// is the demo's. Each part goes in once, and not again even after it is changed or removed.
func Load(ctx context.Context, s *org.Service) (db.LSN, error) {
	return s.Together(ctx, func(ctx context.Context, s *org.Service) error {
		return load(ctx, s)
	})
}

func load(ctx context.Context, s *org.Service) error {
	state, err := s.DemoState(ctx)
	if err != nil {
		return err
	}
	organisation := !state.Organisation.Loaded && state.Organisation.Empty
	flows := !state.Processes.Loaded && state.Processes.Empty && (state.Organisation.Loaded || organisation)
	switch {
	case !organisation && !flows && state.Organisation.Loaded && state.Processes.Loaded:
		return ErrLoaded
	case !organisation && !flows:
		return ErrNotEmpty
	}
	if organisation {
		if err := loadOrganisation(ctx, s); err != nil {
			return err
		}
	}
	if flows {
		return loadProcesses(ctx, s)
	}
	return nil
}

func loadOrganisation(ctx context.Context, s *org.Service) error {
	start := mustDay(since)
	kindIDs, err := unitTypes(ctx, s)
	if err != nil {
		return err
	}

	unitIDs := map[string]uuid.UUID{}
	for _, u := range units {
		var parent *uuid.UUID
		if u.parent != "" {
			id := unitIDs[u.parent]
			parent = &id
		}
		kind := kindIDs[u.kind]
		made, _, err := s.CreateUnit(ctx, org.UnitInput{TypeID: &kind, Name: ptr(u.name), ShortName: ptr(u.short)}, parent, start)
		if err != nil {
			return fmt.Errorf("unit %s: %w", u.key, err)
		}
		unitIDs[u.key] = made.ID
	}

	positionIDs := map[string]uuid.UUID{}
	heads := map[string]string{}
	for _, p := range positions {
		in := org.PositionInput{Title: ptr(p.title), HeadOfUnit: ptr(p.head)}
		if p.seats > 0 {
			in.Seats = ptr(p.seats)
		}
		made, _, err := s.CreatePosition(ctx, unitIDs[p.unit], in, start)
		if err != nil {
			return fmt.Errorf("position %s: %w", p.key, err)
		}
		positionIDs[p.key] = made.ID
		if p.head {
			heads[p.unit] = p.key
		}
	}
	parents := map[string]string{}
	for _, u := range units {
		parents[u.key] = u.parent
	}
	for _, p := range positions {
		to := p.reportsTo
		switch {
		case to != "":
		case p.head && parents[p.unit] != "":
			to = heads[parents[p.unit]]
		case !p.head:
			to = heads[p.unit]
		}
		if to != "" {
			target := positionIDs[to]
			if _, _, err := s.SetReportsTo(ctx, positionIDs[p.key], &target, start); err != nil {
				return fmt.Errorf("line from %s to %s: %w", p.key, to, err)
			}
		}
		for _, d := range p.dottedTo {
			if _, _, err := s.AddDottedLine(ctx, positionIDs[p.key], positionIDs[d], start); err != nil {
				return fmt.Errorf("dotted line from %s to %s: %w", p.key, d, err)
			}
		}
	}

	personIDs := map[string]uuid.UUID{}
	for _, p := range people {
		made, _, err := s.CreatePerson(ctx, org.PersonInput{Name: ptr(p.name), Email: ptr(p.email), Location: ptr(p.location)})
		if err != nil {
			return fmt.Errorf("person %s: %w", p.key, err)
		}
		personIDs[p.key] = made.ID
	}

	roleIDs := map[string]uuid.UUID{}
	for _, r := range roles {
		made, _, err := s.CreateRole(ctx, org.RoleInput{Key: ptr(r.key), Name: ptr(r.name), Description: ptr(r.description)})
		if err != nil {
			return fmt.Errorf("role %s: %w", r.key, err)
		}
		roleIDs[r.key] = made.ID
		for _, p := range r.carriedBy {
			if _, _, err := s.CarryRole(ctx, positionIDs[p], made.ID, start); err != nil {
				return fmt.Errorf("role %s on %s: %w", r.key, p, err)
			}
		}
	}

	for _, h := range holdings {
		in := org.HoldingInput{PersonID: personIDs[h.person], From: start}
		if h.position != "" {
			id := positionIDs[h.position]
			in.PositionID = &id
		} else {
			id := roleIDs[h.role]
			in.RoleID = &id
		}
		if h.share > 0 {
			in.Share = ptr(h.share)
		}
		if h.from != "" {
			in.From = mustDay(h.from)
		}
		if h.to != "" {
			to := mustDay(h.to)
			in.To = &to
		}
		if _, _, err := s.GiveHolding(ctx, in); err != nil {
			return fmt.Errorf("holding of %s%s by %s: %w", h.position, h.role, h.person, err)
		}
	}
	for person, on := range leavers {
		if _, _, err := s.Leave(ctx, personIDs[person], mustDay(on)); err != nil {
			return fmt.Errorf("%s leaving: %w", person, err)
		}
	}
	for position, stand := range deputies {
		ids := make([]uuid.UUID, len(stand))
		for i, d := range stand {
			ids[i] = positionIDs[d]
		}
		if _, _, err := s.SetDeputies(ctx, positionIDs[position], ids); err != nil {
			return fmt.Errorf("deputies of %s: %w", position, err)
		}
	}
	return s.MarkDemoLoaded(ctx)
}

// unitTypes finds the demo's kinds of unit by name, adding those the installation lacks.
func unitTypes(ctx context.Context, s *org.Service) (map[string]uuid.UUID, error) {
	have, err := s.UnitTypes(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]uuid.UUID{}
	for _, k := range kinds {
		for _, t := range have {
			if strings.EqualFold(t.Name, k.name) {
				out[k.name] = t.ID
			}
		}
		if _, ok := out[k.name]; ok {
			continue
		}
		made, _, err := s.CreateUnitType(ctx, org.UnitTypeInput{Name: ptr(k.name), Icon: ptr(k.icon), Colour: ptr(k.colour), Rank: ptr(k.rank)})
		if err != nil {
			return nil, fmt.Errorf("unit type %s: %w", k.name, err)
		}
		out[k.name] = made.ID
	}
	return out, nil
}

// loadProcesses adds the demo's processes, publishing their versions in turn, each owned by the position of its
// owner's title today.
func loadProcesses(ctx context.Context, s *org.Service) error {
	positions, err := s.Positions(ctx, org.DayOf(time.Now()), nil, false)
	if err != nil {
		return err
	}
	owners := map[string]uuid.UUID{}
	for _, p := range positions {
		owners[p.Title] = p.ID
	}
	ids := map[string]uuid.UUID{}
	for _, p := range processes {
		in := org.ProcessInput{Key: ptr(p.key), Name: ptr(p.name), Area: ptr(p.area), Description: ptr(p.description)}
		if id, ok := owners[p.owner]; ok {
			in.OwnerID = &id
		}
		made, _, err := s.CreateProcess(ctx, in)
		if err != nil {
			return fmt.Errorf("process %s: %w", p.key, err)
		}
		ids[p.key] = made.ID
	}
	for _, p := range processes {
		id := ids[p.key]
		save := func(f flow) error {
			current, err := s.Process(ctx, id)
			if err != nil {
				return err
			}
			_, _, err = s.SaveDraft(ctx, id, current.Draft.Revision, f.graph(ids))
			return err
		}
		for i, f := range p.versions {
			if err := save(f); err != nil {
				return fmt.Errorf("version %d of %s: %w", i+1, p.key, err)
			}
			if _, _, err := s.Publish(ctx, id, f.note); err != nil {
				return fmt.Errorf("publishing version %d of %s: %w", i+1, p.key, err)
			}
		}
		if p.draft != nil {
			if err := save(*p.draft); err != nil {
				return fmt.Errorf("draft of %s: %w", p.key, err)
			}
		}
	}
	return s.MarkDemoProcessesLoaded(ctx)
}

// graph lays a flow out top to bottom: each node a row below the nearest node leading to it, the nodes of a row side
// by side in the order they are named.
func (f flow) graph(ids map[string]uuid.UUID) org.Graph {
	next := map[string][]string{}
	for _, a := range f.arrows {
		next[a.from] = append(next[a.from], a.to)
	}
	row := map[string]int{}
	var queue []string
	for _, n := range f.nodes {
		if n.kind == org.Start {
			row[n.key] = 0
			queue = append(queue, n.key)
		}
	}
	for len(queue) > 0 {
		k := queue[0]
		queue = queue[1:]
		for _, to := range next[k] {
			if _, seen := row[to]; !seen {
				row[to] = row[k] + 1
				queue = append(queue, to)
			}
		}
	}
	across := map[int]int{}
	for _, n := range f.nodes {
		across[row[n.key]]++
	}
	placed := map[int]int{}
	var g org.Graph
	for _, n := range f.nodes {
		r := row[n.key]
		node := org.Node{Key: n.key, Kind: n.kind, Name: n.name,
			X: float64(placed[r]*2-across[r]+1) * 130, Y: float64(r) * 110}
		placed[r]++
		if n.sub != "" {
			id := ids[n.sub]
			node.SubprocessID = &id
		}
		g.Nodes = append(g.Nodes, node)
	}
	for _, a := range f.arrows {
		g.Arrows = append(g.Arrows, org.Arrow{Key: a.from + "-" + a.to, From: a.from, To: a.to, Label: a.label})
	}
	return g
}

func mustDay(s string) org.Day {
	d, err := org.ParseDay(s)
	if err != nil {
		panic(err)
	}
	return d
}

func ptr[T any](v T) *T { return &v }
