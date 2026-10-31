package org

import (
	"context"
	"errors"

	"github.com/praetorianer777/bearing/backend/internal/db"
)

// Store is where the model is read and written: the cluster, or one transaction many changes share.
type Store interface {
	Read(ctx context.Context, fn func(context.Context, db.DBTX) error) error
	Write(ctx context.Context, fn func(context.Context, db.DBTX) error) (db.LSN, error)
	WriteAdmin(ctx context.Context, fn func(context.Context, db.DBTX) error) (db.LSN, error)
}

// oneTx is a Store inside a transaction already open: every read and write joins it, and what it commits is told
// by the transaction's own end.
type oneTx struct{ q db.DBTX }

func (t oneTx) Read(ctx context.Context, fn func(context.Context, db.DBTX) error) error {
	return fn(ctx, t.q)
}

func (t oneTx) Write(ctx context.Context, fn func(context.Context, db.DBTX) error) (db.LSN, error) {
	return 0, fn(ctx, t.q)
}

// WriteAdmin cannot join: it writes as another database role, which needs a transaction of its own.
func (t oneTx) WriteAdmin(context.Context, func(context.Context, db.DBTX) error) (db.LSN, error) {
	return 0, errAdminInTx
}

var errAdminInTx = errors.New("a write as the administrator role cannot join another transaction")

// Together runs fn with a service whose changes all happen in one transaction: all of them, or none if fn fails.
func (s *Service) Together(ctx context.Context, fn func(ctx context.Context, s *Service) error) (db.LSN, error) {
	return s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		return fn(ctx, &Service{DB: oneTx{q}})
	})
}
