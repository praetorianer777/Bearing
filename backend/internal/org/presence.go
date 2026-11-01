package org

import (
	"context"

	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/db"
)

// Cover is who could be there for a position, a role or a unit on a day: its holders first, then the holders of the
// deputies standing in for them.
type Cover struct {
	Holders  []Resolved
	Deputies []Resolved
}

// Covers is who could be there on a day for each of the subjects, in their order. A role is held directly and
// through the positions carrying it, whose deputies stand in; a unit is its head's position.
func (s *Service) Covers(ctx context.Context, day Day, subjects []Responsibility) ([]Cover, error) {
	out := make([]Cover, len(subjects))
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		w, err := standingFor(ctx, q, subjects, day)
		if err != nil {
			return err
		}
		position := func(c *Cover, p uuid.UUID) {
			c.Holders = append(c.Holders, w.holders[p]...)
			for _, d := range w.deputies[p] {
				c.Deputies = append(c.Deputies, w.holders[d]...)
			}
		}
		for i, r := range subjects {
			switch {
			case r.PositionID != nil:
				position(&out[i], *r.PositionID)
			case r.UnitID != nil:
				if head, ok := w.heads[*r.UnitID]; ok {
					position(&out[i], head)
				}
			case r.RoleID != nil:
				out[i].Holders = append(out[i].Holders, w.roleHolders[*r.RoleID]...)
				for _, p := range w.carriers[*r.RoleID] {
					position(&out[i], p)
				}
			}
		}
		return nil
	})
	return out, err
}
