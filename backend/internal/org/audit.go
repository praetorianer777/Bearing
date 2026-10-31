package org

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/praetorianer777/bearing/backend/internal/db"
)

// Event is one change to the organisation model, as the audit log keeps it.
type Event struct {
	ID uuid.UUID
	At time.Time
	// ActorID is whom the change was made for, and ActorName what they are called; both nil when Bearing made it
	// itself.
	ActorID   *uuid.UUID
	ActorName *string
	// Table is what changed: unit, unit_placement, position, reporting_line, person, person_photo, holding, role,
	// position_role, deputy or unit_type.
	Table     string
	Operation string
	// Before and After are the row as it was and as it became; nil before an insert and after a delete.
	Before json.RawMessage
	After  json.RawMessage
}

// AuditFilter narrows the log to one subject's entries, and pages it: newest first, those before an entry, so many.
type AuditFilter struct {
	Subject *uuid.UUID
	Before  *uuid.UUID
	Limit   int
}

// MaxAuditPage bounds one page of the log.
const MaxAuditPage = 200

// Audit reads the log newest first, as an editor. A unit's, position's, person's or role's entries are those of its
// own row and of every relation naming it.
func (s *Service) Audit(ctx context.Context, f AuditFilter) ([]Event, error) {
	if f.Limit < 1 || f.Limit > MaxAuditPage {
		f.Limit = 50
	}
	var out []Event
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		var subjects []uuid.UUID
		if f.Subject != nil {
			subjects = []uuid.UUID{*f.Subject}
		}
		rows, err := q.Query(ctx, `SELECT e.id, e.at, e.actor_id, a.name, e.table_name, e.operation, e.before, e.after
			FROM audit_event e LEFT JOIN account a ON a.id = e.actor_id
			WHERE ($1::uuid[] IS NULL OR e.subjects @> $1) AND ($2::uuid IS NULL OR e.id < $2)
			ORDER BY e.id DESC LIMIT $3`, subjects, f.Before, f.Limit)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Event, error) {
			var e Event
			err := row.Scan(&e.ID, &e.At, &e.ActorID, &e.ActorName, &e.Table, &e.Operation, &e.Before, &e.After)
			return e, err
		})
		return err
	})
	return out, err
}
