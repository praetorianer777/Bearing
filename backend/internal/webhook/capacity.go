package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/praetorianer777/bearing/backend/internal/db"
)

const (
	// GapWindowDays is how far ahead gaps are watched, from today: thirteen weeks, as capacity@1 looks by default.
	GapWindowDays = 91
	// maxSweepDays is the most days one sweep catches up on, after a worker was down that long.
	maxSweepDays = 31
)

// Gap is a run of days a unit, position or role has a gap of one size, as org.Gaps finds it.
type Gap struct {
	Kind        string
	SubjectKind string
	SubjectID   uuid.UUID
	Name        string
	UnitID      *uuid.UUID
	// From and To are days, written 2006-01-02, both counted.
	From, To string
	Size     float64
}

// GapReader reads the organisation's gaps from one day to another, as the person ctx acts for.
type GapReader func(ctx context.Context, from, to string) ([]Gap, error)

// queue adds a delivery of an event for every endpoint on and subscribed to its topic.
func queue(ctx context.Context, tx db.DBTX, topic string, payload any, occurred time.Time) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO webhook_delivery (endpoint_id, event_id, topic, payload, occurred_at, next_attempt_at, version)
		SELECT id, $1, $2, $3, $4, now(), version FROM webhook_endpoint
		WHERE enabled AND ($2 = ANY (topics) OR $5 = ANY (topics))`, uuid.Must(uuid.NewV7()), topic, body, occurred, TopicAny)
	return err
}

// Sweep announces the holdings that started or ended on the days since the last sweep, up to today, where the write
// that set the day came before it; a write on the day itself announced it already. The first sweep only marks
// today, so an installation turning webhooks on is not told its whole past.
func (s *Service) Sweep(ctx context.Context) (int, error) {
	today := s.now().Format(time.DateOnly)
	sent := 0
	_, err := s.db.WriteAdmin(asSystem(ctx), func(ctx context.Context, tx db.DBTX) error {
		var last *time.Time
		if err := tx.QueryRow(ctx, `SELECT max(day) FROM webhook_sweep`).Scan(&last); err != nil {
			return err
		}
		if last == nil {
			_, err := tx.Exec(ctx, `INSERT INTO webhook_sweep (day) VALUES ($1) ON CONFLICT DO NOTHING`, today)
			return err
		}
		look := &lookup{ctx: ctx, tx: tx}
		for day := last.AddDate(0, 0, 1); day.Format(time.DateOnly) <= today; day = day.AddDate(0, 0, 1) {
			if day.Before(s.now().AddDate(0, 0, -maxSweepDays)) {
				continue
			}
			rows, err := tx.Query(ctx, `SELECT CASE WHEN h.valid_from = $1 THEN 'started' ELSE 'ended' END, to_jsonb(h) - 'created_at' - 'updated_at'
				FROM holding h WHERE (h.valid_from = $1 AND h.created_at::date < $1) OR (h.valid_to = $1 AND h.updated_at::date < $1)
				ORDER BY h.id`, day)
			if err != nil {
				return err
			}
			type turned struct {
				how string
				row map[string]any
			}
			found, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (turned, error) {
				var t turned
				err := row.Scan(&t.how, &t.row)
				return t, err
			})
			if err != nil {
				return err
			}
			for _, t := range found {
				holding := map[string]any{"id": t.row["id"], "share": t.row["share"], "from": t.row["valid_from"], "to": t.row["valid_to"]}
				payload := HoldingPayload(look, t.row, holding)
				payload["actor"] = nil
				topic := TopicHoldingStarted
				if t.how == "ended" {
					topic = TopicHoldingEnded
				}
				if look.err != nil {
					return look.err
				}
				if err := queue(ctx, tx, topic, payload, day); err != nil {
					return err
				}
				sent++
			}
			if _, err := tx.Exec(ctx, `INSERT INTO webhook_sweep (day) VALUES ($1) ON CONFLICT DO NOTHING`, day); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `DELETE FROM webhook_sweep WHERE day < $1`, today)
		return err
	})
	return sent, err
}

// window is the days gaps were read over, both counted.
type window struct{ From, To string }

func gapKey(g Gap, w window) string {
	from := g.From
	if from <= w.From {
		from = ""
	}
	return g.Kind + "|" + g.SubjectKind + "|" + g.SubjectID.String() + "|" + from
}

// gapEnd is the last day of a gap, or "" for one that runs to the window's end and maybe beyond.
func gapEnd(g Gap, w window) string {
	if g.To >= w.To {
		return ""
	}
	return g.To
}

func gapPayload(g Gap, w window) map[string]any {
	var to any
	if end := gapEnd(g, w); end != "" {
		to = end
	}
	return map[string]any{"kind": g.Kind, "subjectKind": g.SubjectKind, "subjectId": g.SubjectID, "name": g.Name, "unitId": g.UnitID,
		"from": g.From, "to": to, "size": g.Size}
}

// DiffGaps is what changed between the gaps read over one window and those read over the next: a gap opened,
// closed, or changed size or end. A gap is known by its kind, its subject and its first day; one that began before
// the window counts as going on, so the window moving a day changes nothing by itself.
func DiffGaps(before []Gap, was window, after []Gap, now window) []Event {
	old := map[string]Gap{}
	for _, g := range before {
		old[gapKey(g, now)] = g
	}
	var out []Event
	seen := map[string]bool{}
	for _, g := range after {
		key := gapKey(g, now)
		seen[key] = true
		prior, ok := old[key]
		switch {
		case !ok:
			out = append(out, Event{TopicGapChanged, map[string]any{"change": "opened", "gap": gapPayload(g, now), "actor": nil}})
		case prior.Size != g.Size || gapEnd(prior, was) != gapEnd(g, now):
			out = append(out, Event{TopicGapChanged, map[string]any{"change": "changed", "gap": gapPayload(g, now), "before": gapPayload(prior, was), "actor": nil}})
		}
	}
	for _, g := range before {
		if key := gapKey(g, now); !seen[key] {
			seen[key] = true
			out = append(out, Event{TopicGapChanged, map[string]any{"change": "closed", "gap": gapPayload(g, was), "actor": nil}})
		}
	}
	return out
}

// CheckGaps reads the gaps over the window from today when anything changed or the day turned, compares them with
// those read last and announces the difference. It reads as the administrator owning an endpoint that takes the
// topic, and does nothing while none does; the first read only remembers.
func (s *Service) CheckGaps(ctx context.Context, changed bool) (int, error) {
	if s.Gaps == nil {
		return 0, nil
	}
	now := window{From: s.now().Format(time.DateOnly), To: s.now().AddDate(0, 0, GapWindowDays-1).Format(time.DateOnly)}
	var owner *uuid.UUID
	var was *window
	err := s.db.ReadAdmin(asSystem(ctx), func(ctx context.Context, tx db.DBTX) error {
		err := tx.QueryRow(ctx, `SELECT e.owner_id FROM webhook_endpoint e JOIN account a ON a.id = e.owner_id
			WHERE e.enabled AND ($1 = ANY (e.topics) OR $2 = ANY (e.topics)) AND a.role = 'administrator' AND a.disabled_at IS NULL
			ORDER BY e.id LIMIT 1`, TopicGapChanged, TopicAny).Scan(&owner)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var from, to time.Time
		err = tx.QueryRow(ctx, `SELECT from_day, to_day FROM webhook_gap_window`).Scan(&from, &to)
		if err == nil {
			was = &window{From: from.Format(time.DateOnly), To: to.Format(time.DateOnly)}
			return nil
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil || (was != nil && was.From == now.From && !changed && owner != nil) {
		return 0, err
	}
	// Nobody listening: what was seen last is forgotten, so the next endpoint starts from what is, not from then.
	if owner == nil {
		if was != nil {
			_, err = s.db.WriteAdmin(asSystem(ctx), func(ctx context.Context, tx db.DBTX) error {
				if _, err := tx.Exec(ctx, `DELETE FROM webhook_gap`); err != nil {
					return err
				}
				_, err := tx.Exec(ctx, `DELETE FROM webhook_gap_window`)
				return err
			})
		}
		return 0, err
	}
	gaps, err := s.Gaps(db.PinPrimary(db.WithUser(ctx, *owner)), now.From, now.To)
	if err != nil {
		return 0, err
	}
	announced := 0
	_, err = s.db.WriteAdmin(asSystem(ctx), func(ctx context.Context, tx db.DBTX) error {
		var from, to time.Time
		err := tx.QueryRow(ctx, `SELECT from_day, to_day FROM webhook_gap_window FOR UPDATE`).Scan(&from, &to)
		first := errors.Is(err, pgx.ErrNoRows)
		if err != nil && !first {
			return err
		}
		if !first {
			prior := window{From: from.Format(time.DateOnly), To: to.Format(time.DateOnly)}
			rows, err := tx.Query(ctx, `SELECT kind, subject_kind, subject_id, name, unit_id, from_day::text, to_day::text, size FROM webhook_gap`)
			if err != nil {
				return err
			}
			before, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Gap, error) {
				var g Gap
				err := row.Scan(&g.Kind, &g.SubjectKind, &g.SubjectID, &g.Name, &g.UnitID, &g.From, &g.To, &g.Size)
				return g, err
			})
			if err != nil {
				return err
			}
			for _, e := range DiffGaps(before, prior, gaps, now) {
				if err := queue(ctx, tx, e.Topic, e.Payload, s.now()); err != nil {
					return err
				}
				announced++
			}
		}
		if _, err := tx.Exec(ctx, `DELETE FROM webhook_gap`); err != nil {
			return err
		}
		for _, g := range gaps {
			if _, err := tx.Exec(ctx, `INSERT INTO webhook_gap (kind, subject_kind, subject_id, from_day, to_day, size, name, unit_id)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8) ON CONFLICT DO NOTHING`, g.Kind, g.SubjectKind, g.SubjectID, g.From, g.To, g.Size, g.Name, g.UnitID); err != nil {
				return err
			}
		}
		_, err = tx.Exec(ctx, `INSERT INTO webhook_gap_window (from_day, to_day) VALUES ($1, $2)
			ON CONFLICT (one) DO UPDATE SET from_day = excluded.from_day, to_day = excluded.to_day`, now.From, now.To)
		return err
	})
	return announced, err
}
