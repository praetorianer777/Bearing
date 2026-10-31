package org

import (
	"context"
	"errors"

	"github.com/praetorianer777/bearing/backend/internal/db"
)

var ErrNotAdministrator = errors.New("only an administrator does that")

// DemoState says whether the demo organisation was loaded, and whether the installation has no units at all.
func (s *Service) DemoState(ctx context.Context) (loaded, empty bool, err error) {
	err = s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		return q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM installation WHERE demo_loaded_at IS NOT NULL),
			NOT EXISTS (SELECT 1 FROM unit)`).Scan(&loaded, &empty)
	})
	return loaded, empty, err
}

// MarkDemoLoaded notes that the demo organisation is loaded, as an administrator.
func (s *Service) MarkDemoLoaded(ctx context.Context) error {
	_, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		tag, err := q.Exec(ctx, `UPDATE installation SET demo_loaded_at = now()`)
		if err == nil && tag.RowsAffected() == 0 {
			return ErrNotAdministrator
		}
		return err
	})
	return err
}
