// Package demo loads a demo organisation into an installation that has none yet, to look at and to test against.
package demo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/db"
	"github.com/praetorianer777/bearing/backend/internal/org"
)

var (
	ErrLoaded   = errors.New("the demo organisation is loaded already")
	ErrNotEmpty = errors.New("the installation has an organisation of its own; the demo goes only into an empty one")
)

// Load adds the demo organisation as an administrator, all of it in one transaction or nothing. Loaded once, it is
// not loaded again, even after its units are changed or removed.
func Load(ctx context.Context, s *org.Service) (db.LSN, error) {
	return s.Together(ctx, func(ctx context.Context, s *org.Service) error {
		return load(ctx, s)
	})
}

func load(ctx context.Context, s *org.Service) error {
	loaded, empty, err := s.DemoState(ctx)
	switch {
	case err != nil:
		return err
	case loaded:
		return ErrLoaded
	case !empty:
		return ErrNotEmpty
	}
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

func mustDay(s string) org.Day {
	d, err := org.ParseDay(s)
	if err != nil {
		panic(err)
	}
	return d
}

func ptr[T any](v T) *T { return &v }
