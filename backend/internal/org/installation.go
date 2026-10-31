package org

import (
	"context"
	"errors"

	"github.com/praetorianer777/bearing/backend/internal/db"
)

var ErrNotAdministrator = errors.New("only an administrator does that")

// DemoPart is whether a part of the demo was loaded, and whether the installation has none of that part of its own.
type DemoPart struct {
	Loaded bool
	Empty  bool
}

// DemoState says how far the demo's parts are in: the organisation and the processes.
type DemoState struct {
	Organisation DemoPart
	Processes    DemoPart
}

// DemoState reads how far the demo is in.
func (s *Service) DemoState(ctx context.Context) (DemoState, error) {
	var d DemoState
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		return q.QueryRow(ctx, `SELECT
			EXISTS (SELECT 1 FROM installation WHERE demo_loaded_at IS NOT NULL), NOT EXISTS (SELECT 1 FROM unit),
			EXISTS (SELECT 1 FROM installation WHERE demo_processes_loaded_at IS NOT NULL), NOT EXISTS (SELECT 1 FROM process)`).
			Scan(&d.Organisation.Loaded, &d.Organisation.Empty, &d.Processes.Loaded, &d.Processes.Empty)
	})
	return d, err
}

// MarkDemoLoaded notes that the demo organisation is loaded, as an administrator.
func (s *Service) MarkDemoLoaded(ctx context.Context) error {
	return s.markDemo(ctx, `UPDATE installation SET demo_loaded_at = now()`)
}

// MarkDemoProcessesLoaded notes that the demo's processes are loaded, as an administrator.
func (s *Service) MarkDemoProcessesLoaded(ctx context.Context) error {
	return s.markDemo(ctx, `UPDATE installation SET demo_processes_loaded_at = now()`)
}

func (s *Service) markDemo(ctx context.Context, update string) error {
	_, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		tag, err := q.Exec(ctx, update)
		if err == nil && tag.RowsAffected() == 0 {
			return ErrNotAdministrator
		}
		return err
	})
	return err
}
