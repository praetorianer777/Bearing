package orgcsv

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/praetorianer777/bearing/backend/internal/db"
	"github.com/praetorianer777/bearing/backend/internal/org"
)

// Job is the kind of job that runs an import; the worker handles it.
const Job = "org.import"

// Enqueuer adds a job for the worker; jobs.Queue is the real one.
type Enqueuer interface {
	Enqueue(ctx context.Context, kind string, payload any) (string, error)
}

// Import is one import: previewed, queued, running, done or failed.
type Import struct {
	ID         uuid.UUID
	Status     string
	Effective  org.Day
	Preview    Plan
	Result     *Result
	CreatedAt  time.Time
	FinishedAt *time.Time
}

// Result is what running an import did: the plan it carried out, or the row a refusal met.
type Result struct {
	Plan  Plan      `json:"plan"`
	Error *RowError `json:"error,omitempty"`
}

var (
	ErrImportNotFound = errors.New("there is no such import")
	ErrImportRefused  = errors.New("the preview has rows that cannot be imported; correct the files and preview again")
	ErrImportStarted  = errors.New("the import has run or is running already")
)

// Imports keeps imports and hands them to the worker.
type Imports struct {
	DB    org.Store
	Org   *org.Service
	Queue Enqueuer
}

type jobPayload struct {
	ID    uuid.UUID `json:"id"`
	Actor uuid.UUID `json:"actor"`
}

const importColumns = `id, status, effective, preview, result, created_at, finished_at`

func scanImport(row pgx.Row, out *Import) error {
	var effective time.Time
	var preview []byte
	var result []byte
	if err := row.Scan(&out.ID, &out.Status, &effective, &preview, &result, &out.CreatedAt, &out.FinishedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrImportNotFound
		}
		return err
	}
	out.Effective = org.DayOf(effective)
	if err := json.Unmarshal(preview, &out.Preview); err != nil {
		return err
	}
	if result != nil {
		out.Result = &Result{}
		return json.Unmarshal(result, out.Result)
	}
	return nil
}

// Preview plans the files against the organisation as it stands on effective and keeps the import, as an editor.
func (im *Imports) Preview(ctx context.Context, files Files, effective org.Day) (Import, db.LSN, error) {
	plan, err := Prepare(ctx, im.Org, files, effective)
	if err != nil {
		return Import{}, 0, err
	}
	actor, _ := db.UserFrom(ctx)
	var out Import
	lsn, err := im.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		return scanImport(q.QueryRow(ctx, `INSERT INTO org_import (created_by, effective, files, preview) VALUES ($1, $2, $3, $4)
			RETURNING `+importColumns, actor, effective.Time(), files, plan), &out)
	})
	return out, lsn, err
}

// Get is one import, as an editor.
func (im *Imports) Get(ctx context.Context, id uuid.UUID) (Import, error) {
	var out Import
	err := im.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		return scanImport(q.QueryRow(ctx, `SELECT `+importColumns+` FROM org_import WHERE id = $1`, id), &out)
	})
	return out, err
}

// Run queues a previewed import without errors for the worker, as an editor.
func (im *Imports) Run(ctx context.Context, id uuid.UUID) (Import, db.LSN, error) {
	actor, _ := db.UserFrom(ctx)
	var out Import
	lsn, err := im.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		if err := scanImport(q.QueryRow(ctx, `SELECT `+importColumns+` FROM org_import WHERE id = $1 FOR UPDATE`, id), &out); err != nil {
			return err
		}
		switch {
		case out.Status != "previewed":
			return ErrImportStarted
		case len(out.Preview.Errors) > 0:
			return ErrImportRefused
		}
		return scanImport(q.QueryRow(ctx, `UPDATE org_import SET status = 'queued' WHERE id = $1 RETURNING `+importColumns, id), &out)
	})
	if err != nil {
		return out, lsn, err
	}
	// Queued once the status is written, so the worker never meets an import that is not queued yet.
	if im.Queue == nil {
		err = errors.New("imports run in the worker, and this server has no queue")
	} else {
		_, err = im.Queue.Enqueue(ctx, Job, jobPayload{ID: id, Actor: actor})
	}
	if err != nil {
		_, _ = im.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
			_, err := q.Exec(ctx, `UPDATE org_import SET status = 'previewed' WHERE id = $1`, id)
			return err
		})
		return out, lsn, err
	}
	return out, lsn, nil
}

// Handler runs a queued import as the editor who queued it: it plans the files again against the organisation as it
// is now and applies them in one transaction, and records what that did. A refusal is recorded, not retried.
func (im *Imports) Handler() func(ctx context.Context, payload json.RawMessage) error {
	return func(ctx context.Context, payload json.RawMessage) error {
		var job jobPayload
		if err := json.Unmarshal(payload, &job); err != nil {
			return err
		}
		ctx = db.WithUser(ctx, job.Actor)
		var files Files
		var effective time.Time
		claimed := false
		_, err := im.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
			var raw []byte
			err := q.QueryRow(ctx, `UPDATE org_import SET status = 'running' WHERE id = $1 AND status = 'queued'
				RETURNING files, effective`, job.ID).Scan(&raw, &effective)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			if err != nil {
				return err
			}
			claimed = true
			return json.Unmarshal(raw, &files)
		})
		if err != nil || !claimed {
			return err
		}
		plan, err := Apply(ctx, im.Org, files, org.DayOf(effective))
		result := Result{Plan: plan}
		var refused *ApplyError
		switch {
		case errors.As(err, &refused):
			result.Error = &refused.RowError
		case err != nil:
			// Nothing was applied; queued again, the next try claims it.
			_, _ = im.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
				_, err := q.Exec(ctx, `UPDATE org_import SET status = 'queued' WHERE id = $1`, job.ID)
				return err
			})
			return err
		}
		outcome := "done"
		if result.Error != nil || len(plan.Errors) > 0 {
			outcome = "failed"
		}
		_, err = im.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
			_, err := q.Exec(ctx, `UPDATE org_import SET status = $2, result = $3, finished_at = now() WHERE id = $1`, job.ID, outcome, result)
			return err
		})
		return err
	}
}
