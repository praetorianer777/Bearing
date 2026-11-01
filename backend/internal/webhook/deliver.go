package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/praetorianer777/bearing/backend/internal/db"
	"github.com/praetorianer777/bearing/backend/internal/netguard"
)

const (
	// sendTimeout bounds one request; a slow receiver is a failed attempt.
	sendTimeout = 10 * time.Second
	// maxResponseBytes is the most of an answer that is read and dropped, so the connection can be used again.
	maxResponseBytes = 4 << 10
	// ClaimLease is how long a claimed delivery is kept from other senders; one that dies holding it delays the
	// attempt by at most this much.
	ClaimLease = 2 * time.Minute
	// batch is how many changes or due deliveries one pass claims; passes repeat until none is left, so a backlog is
	// drained rather than nibbled.
	batch = 100
	// SendInterval is how often the worker looks for changes and due deliveries, and so the most a first attempt
	// waits.
	SendInterval = 2 * time.Second
	// DeliveryRetention is how long the log keeps an attempt, as the siblings'.
	DeliveryRetention = 30 * 24 * time.Hour
	// pruneEvery is how often attempts past DeliveryRetention are deleted.
	pruneEvery = time.Hour
)

// The sentences a delivery's log shows for what stopped it. The errors themselves name hosts and ports this server
// can reach, which the worker's log may say and a delivery row may not.
const (
	whyBlocked   = "This address is inside the server's own network, so Bearing may not post to it. Use the receiver's public address, or ask the operator to add the host to BEARING_OUTBOUND_ALLOW."
	whyTimeout   = "The receiver did not answer in time. Check that it is running, then send it again."
	whyUnreached = "The receiver could not be reached. Check the address, then send it again."
	whyNoSecret  = "The webhook's secret does not open with this server's key. Rotate the secret and give the receiver the new one."
	whyOff       = "The webhook was turned off before this was sent."
	whyNoOwner   = "Nothing was sent: the administrator who saved this webhook is no longer one. Save it again to send as yourself."
	whyVersion   = "Nothing was sent: this delivery is in envelope version %d, which this Bearing does not send. Register the webhook for version %d."
)

// Envelope is what every delivery's body is, in version 1.
type Envelope struct {
	Version    int             `json:"version"`
	ID         uuid.UUID       `json:"id"`
	Source     string          `json:"source"`
	Topic      string          `json:"topic"`
	OccurredAt time.Time       `json:"occurredAt"`
	Payload    json.RawMessage `json:"payload"`
}

// asSystem is ctx acting for nobody: the log and the counts are the worker's, and a request's caller must not
// become the endpoint's owner by sending a test.
func asSystem(ctx context.Context) context.Context {
	return db.PinPrimary(db.WithUser(ctx, uuid.Nil))
}

// FanOut turns the changes the model tables noted into events and queues a delivery of each for every endpoint on
// and subscribed to its topic, deleting the changes in the same transaction, so each is announced once. It says how
// many changes it handled.
func (s *Service) FanOut(ctx context.Context) (int, error) {
	handled := 0
	for {
		n := 0
		_, err := s.db.WriteAdmin(asSystem(ctx), func(ctx context.Context, tx db.DBTX) error {
			rows, err := tx.Query(ctx, `DELETE FROM webhook_change WHERE id IN (
					SELECT id FROM webhook_change ORDER BY id LIMIT $1 FOR UPDATE SKIP LOCKED)
				RETURNING id, at, actor_id, table_name, operation::text, before, after`, batch)
			if err != nil {
				return err
			}
			changes, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Change, error) {
				var c Change
				err := row.Scan(&c.ID, &c.At, &c.ActorID, &c.Table, &c.Operation, &c.Before, &c.After)
				return c, err
			})
			if err != nil {
				return err
			}
			n = len(changes)
			look := &lookup{ctx: ctx, tx: tx}
			today := s.now().Format(time.DateOnly)
			for _, c := range changes {
				for _, e := range Announce(c, look, today) {
					if look.err != nil {
						return look.err
					}
					if err := queue(ctx, tx, e.Topic, e.Payload, c.At); err != nil {
						return err
					}
				}
			}
			return look.err
		})
		if err != nil {
			return handled, err
		}
		handled += n
		if n < batch || ctx.Err() != nil {
			return handled, nil
		}
	}
}

// SendDue posts every delivery whose time has come, and says how many were tried. Each is leased first, so senders
// in several workers never post one twice.
func (s *Service) SendDue(ctx context.Context) (int, error) {
	tried := 0
	for {
		var found []uuid.UUID
		_, err := s.db.WriteAdmin(asSystem(ctx), func(ctx context.Context, tx db.DBTX) error {
			rows, err := tx.Query(ctx, `UPDATE webhook_delivery d SET next_attempt_at = now() + make_interval(secs => $2)
				FROM (SELECT id FROM webhook_delivery WHERE state = 'pending' AND next_attempt_at <= now()
					ORDER BY next_attempt_at LIMIT $1 FOR UPDATE SKIP LOCKED) claimed
				WHERE d.id = claimed.id RETURNING d.id`, batch, ClaimLease.Seconds())
			if err != nil {
				return err
			}
			found, err = pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
			return err
		})
		if err != nil {
			return tried, err
		}
		for _, id := range found {
			if ctx.Err() != nil {
				return tried, nil
			}
			if _, _, err := s.attempt(ctx, id); err != nil {
				s.log.Warn("a webhook attempt could not be recorded", "delivery", id, "error", err)
			}
		}
		tried += len(found)
		if len(found) < batch || ctx.Err() != nil {
			return tried, nil
		}
	}
}

// Prune deletes the attempts past DeliveryRetention.
func (s *Service) Prune(ctx context.Context) (int64, error) {
	var gone int64
	_, err := s.db.WriteAdmin(asSystem(ctx), func(ctx context.Context, tx db.DBTX) error {
		tag, err := tx.Exec(ctx, `DELETE FROM webhook_delivery WHERE created_at < now() - make_interval(secs => $1) AND state <> 'pending'`,
			DeliveryRetention.Seconds())
		gone = tag.RowsAffected()
		return err
	})
	return gone, err
}

// Test posts a ping to the endpoint now and returns the attempt as logged. It is sent even while the endpoint is off,
// to check it before turning it on.
func (s *Service) Test(ctx context.Context, id uuid.UUID) (*Delivery, db.LSN, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return nil, 0, err
	}
	var made uuid.UUID
	system := asSystem(ctx)
	lsn, err := s.db.WriteAdmin(system, func(ctx context.Context, tx db.DBTX) error {
		return tx.QueryRow(ctx, `INSERT INTO webhook_delivery (endpoint_id, event_id, topic, payload, occurred_at, manual, next_attempt_at, version)
			SELECT id, $2, $3, $4, now(), true, now() + make_interval(secs => $5), version FROM webhook_endpoint WHERE id = $1
			RETURNING id`, id, uuid.New(), TopicPing, map[string]any{"message": "Bearing can reach this address."}, ClaimLease.Seconds()).Scan(&made)
	})
	if err != nil {
		return nil, 0, err
	}
	sent, recorded, err := s.attempt(system, made)
	return sent, max(lsn, recorded), err
}

// Redeliver tries a logged delivery's event again, now, numbered among the event's sends by hand.
func (s *Service) Redeliver(ctx context.Context, webhookID, deliveryID uuid.UUID) (*Delivery, db.LSN, error) {
	err := s.db.Read(ctx, func(ctx context.Context, tx db.DBTX) error {
		var found bool
		err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM webhook_delivery WHERE id = $1 AND endpoint_id = $2)`, deliveryID, webhookID).Scan(&found)
		if err == nil && !found {
			return ErrNotFound
		}
		return err
	})
	if err != nil {
		return nil, 0, err
	}
	var made uuid.UUID
	system := asSystem(ctx)
	lsn, err := s.db.WriteAdmin(system, func(ctx context.Context, tx db.DBTX) error {
		// The automatic chain's next number is its pending retry's to take when that fails, so a send by hand is
		// numbered among the sends by hand.
		return tx.QueryRow(ctx, `INSERT INTO webhook_delivery (endpoint_id, event_id, topic, payload, occurred_at, attempt, manual, next_attempt_at, version)
			SELECT d.endpoint_id, d.event_id, d.topic, d.payload, d.occurred_at,
			       COALESCE((SELECT max(attempt) FROM webhook_delivery x WHERE x.endpoint_id = d.endpoint_id AND x.event_id = d.event_id AND x.manual), 0) + 1,
			       true, now() + make_interval(secs => $2), d.version
			FROM webhook_delivery d WHERE d.id = $1
			RETURNING id`, deliveryID, ClaimLease.Seconds()).Scan(&made)
	})
	if err != nil {
		return nil, 0, err
	}
	sent, recorded, err := s.attempt(system, made)
	return sent, max(lsn, recorded), err
}

// claimed is a delivery about to be attempted, with its endpoint.
type claimed struct {
	endpointID, eventID uuid.UUID
	url, topic          string
	sealed              []byte
	enabled, manual     bool
	// owned says the endpoint's owner is still an administrator.
	owned    bool
	payload  json.RawMessage
	occurred time.Time
	attempt  int
	state    DeliveryState
	// version is the envelope the delivery was queued in, which its retries and redeliveries keep.
	version int
}

// outcome is how one attempt went.
type outcome struct {
	state  DeliveryState
	status *int
	why    string
	// counts says whether it moves the endpoint's count of failures.
	counts bool
}

// attempt posts one logged delivery and records how it went: delivered, withheld, cancelled, or failed with the next
// attempt due later.
func (s *Service) attempt(ctx context.Context, id uuid.UUID) (*Delivery, db.LSN, error) {
	ctx = asSystem(ctx)
	var c claimed
	err := s.db.ReadAdmin(ctx, func(ctx context.Context, tx db.DBTX) error {
		return tx.QueryRow(ctx, `SELECT d.endpoint_id, d.event_id, e.url, d.topic, e.secret_sealed, e.enabled, d.manual,
				coalesce((SELECT a.role = 'administrator' AND a.disabled_at IS NULL FROM account a WHERE a.id = e.owner_id), false),
				d.payload, d.occurred_at, d.attempt, d.state, d.version
			FROM webhook_delivery d JOIN webhook_endpoint e ON e.id = d.endpoint_id WHERE d.id = $1`, id).
			Scan(&c.endpointID, &c.eventID, &c.url, &c.topic, &c.sealed, &c.enabled, &c.manual, &c.owned,
				&c.payload, &c.occurred, &c.attempt, &c.state, &c.version)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, ErrNotFound
	}
	if err != nil {
		return nil, 0, err
	}
	if c.state != StatePending {
		done, err := s.read(ctx, id)
		return done, 0, err
	}
	return s.record(ctx, id, c, s.send(ctx, id, c))
}

// send builds the body, signs it and posts it.
func (s *Service) send(ctx context.Context, id uuid.UUID, c claimed) outcome {
	if !c.enabled && !c.manual {
		return outcome{state: StateCancelled, why: whyOff}
	}
	if !c.owned && c.topic != TopicPing {
		return outcome{state: StateWithheld, why: whyNoOwner}
	}
	// Only a later release, taken back, leaves a delivery in a version this one cannot build. Every attempt would
	// fail alike, so it is neither retried nor counted against the endpoint.
	if c.version != CurrentVersion {
		s.log.Warn("a webhook delivery is in an envelope version this release does not send", "delivery", id, "version", c.version)
		return outcome{state: StateFailed, why: fmt.Sprintf(whyVersion, c.version, CurrentVersion)}
	}
	body, err := json.Marshal(Envelope{Version: c.version, ID: c.eventID, Source: Source, Topic: c.topic, OccurredAt: c.occurred.UTC(), Payload: c.payload})
	if err != nil {
		return outcome{state: StateFailed, why: whyUnreached, counts: true}
	}
	plain, err := s.box.Open(c.sealed, sealContext(c.endpointID))
	if err != nil {
		s.log.Warn("a webhook secret could not be opened", "webhook", c.endpointID, "error", err)
		return outcome{state: StateFailed, why: whyNoSecret, counts: true}
	}
	status, err := s.post(ctx, c.url, string(plain), c.topic, c.version, id, body)
	if err != nil {
		s.log.Info("a webhook delivery did not reach its receiver", "delivery", id, "error", err)
		return outcome{state: StateFailed, why: why(err), counts: true}
	}
	if status < 200 || status > 299 {
		return outcome{state: StateFailed, status: &status, why: fmt.Sprintf("The receiver answered %d. Check its log, then send it again.", status), counts: true}
	}
	return outcome{state: StateDelivered, status: &status}
}

// why says what stopped a request in a sentence.
func why(err error) string {
	switch {
	case errors.Is(err, netguard.ErrBlocked):
		return whyBlocked
	case errors.Is(err, context.DeadlineExceeded), strings.Contains(err.Error(), "timeout"):
		return whyTimeout
	default:
		return whyUnreached
	}
}

// record writes how an attempt went, schedules the next after a failure, and turns the endpoint off once it has
// failed for DisableAfter.
func (s *Service) record(ctx context.Context, id uuid.UUID, c claimed, o outcome) (*Delivery, db.LSN, error) {
	var out *Delivery
	lsn, err := s.db.WriteAdmin(ctx, func(ctx context.Context, tx db.DBTX) error {
		if _, err := tx.Exec(ctx, `UPDATE webhook_delivery SET state = $2, status = $3, error = $4, next_attempt_at = NULL, attempted_at = now(),
				delivered_at = CASE WHEN $2 = 'delivered' THEN now() END WHERE id = $1`, id, o.state, o.status, o.why); err != nil {
			return err
		}
		switch {
		case o.state == StateDelivered:
			if _, err := tx.Exec(ctx, `UPDATE webhook_endpoint SET failures = 0, failing_since = NULL WHERE id = $1`, c.endpointID); err != nil {
				return err
			}
		case o.counts && !c.manual:
			disabled, err := s.countFailure(ctx, tx, c)
			if err != nil {
				return err
			}
			if !disabled && c.attempt < MaxAttempts {
				if _, err := tx.Exec(ctx, `INSERT INTO webhook_delivery (endpoint_id, event_id, topic, payload, occurred_at, attempt, next_attempt_at, version)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8) ON CONFLICT (endpoint_id, event_id, attempt) WHERE NOT manual DO NOTHING`,
					c.endpointID, c.eventID, c.topic, c.payload, c.occurred, c.attempt+1, s.now().Add(NextWait(c.attempt)), c.version); err != nil {
					return err
				}
			}
		}
		var err error
		out, err = scanDelivery(tx.QueryRow(ctx, selectDeliveries+` WHERE id = $1`, id))
		return err
	})
	return out, lsn, err
}

// NextWait is how long after a failed attempt the next one is due.
func NextWait(attempt int) time.Duration {
	return Backoff[min(max(attempt, 1)-1, len(Backoff)-1)]
}

// ShouldDisable says whether an endpoint that has failed failures times in a row, since since, has failed long
// enough to be turned off at now.
func ShouldDisable(failures int, since, now time.Time) bool {
	return failures >= DisableAfterFailures && !since.After(now.Add(-DisableAfter))
}

// countFailure adds a failure to the endpoint and turns it off when it has failed long enough, cancelling what was
// still due.
func (s *Service) countFailure(ctx context.Context, tx db.DBTX, c claimed) (bool, error) {
	var (
		failures int
		since    time.Time
		enabled  bool
	)
	err := tx.QueryRow(ctx, `UPDATE webhook_endpoint SET failures = failures + 1, failing_since = COALESCE(failing_since, now())
		WHERE id = $1 RETURNING failures, failing_since, enabled`, c.endpointID).Scan(&failures, &since, &enabled)
	if err != nil {
		return false, err
	}
	if !enabled {
		return true, nil
	}
	if !ShouldDisable(failures, since, s.now()) {
		return false, nil
	}
	if _, err := tx.Exec(ctx, `UPDATE webhook_endpoint SET enabled = false, disabled_reason = $2 WHERE id = $1`, c.endpointID, ReasonFailing); err != nil {
		return false, err
	}
	s.log.Warn("a webhook failed for a day and was turned off", "webhook", c.endpointID, "failures", failures)
	_, err = tx.Exec(ctx, `UPDATE webhook_delivery SET state = 'cancelled', error = $2, next_attempt_at = NULL
		WHERE endpoint_id = $1 AND state = 'pending' AND NOT manual`, c.endpointID, whyOff)
	return true, err
}

func (s *Service) read(ctx context.Context, id uuid.UUID) (*Delivery, error) {
	var out *Delivery
	err := s.db.ReadAdmin(ctx, func(ctx context.Context, tx db.DBTX) error {
		var err error
		out, err = scanDelivery(tx.QueryRow(ctx, selectDeliveries+` WHERE id = $1`, id))
		return err
	})
	return out, err
}

// post sends one request and returns the status, or the error that stopped it.
func (s *Service) post(ctx context.Context, address, secret, topic string, version int, deliveryID uuid.UUID, body []byte) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, address, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set(EventHeader, topic)
	req.Header.Set(DeliveryHeader, deliveryID.String())
	req.Header.Set(VersionHeader, strconv.Itoa(version))
	// Signed as sent, so a retry or a redelivery carries its own time.
	sent := s.now()
	req.Header.Set(SignatureHeader, Sign(body, secret))
	req.Header.Set(TimestampHeader, strconv.FormatInt(sent.Unix(), 10))
	req.Header.Set(TimestampedSignatureHeader, SignTimestamped(body, secret, sent))
	resp, err := s.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes))
	return resp.StatusCode, nil
}

// lookup reads what payloads name in the transaction that announces the changes, as the admin role. It keeps the
// first error, which ends the transaction.
type lookup struct {
	ctx context.Context
	tx  db.DBTX
	err error
}

func (l *lookup) object(sql string, args ...any) map[string]any {
	if l.err != nil {
		return nil
	}
	var out map[string]any
	err := l.tx.QueryRow(l.ctx, sql, args...).Scan(&out)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		l.err = err
	}
	return out
}

func (l *lookup) Position(id string) map[string]any {
	if id == "" {
		return nil
	}
	return l.object(`SELECT jsonb_build_object('id', id, 'title', title, 'unitId', unit_id, 'headOfUnit', head_of_unit) FROM position WHERE id = $1`, id)
}

func (l *lookup) Person(id string) map[string]any {
	return l.object(`SELECT jsonb_build_object('id', id, 'name', name, 'issuer', issuer, 'subject', subject) FROM person WHERE id = $1`, id)
}

func (l *lookup) Unit(id string) map[string]any {
	return l.object(`SELECT jsonb_build_object('id', u.id, 'name', u.name, 'archivedOn', u.archived_on, 'parentId',
			(SELECT parent_id FROM unit_placement WHERE unit_id = u.id AND valid_from <= current_date AND (valid_to IS NULL OR valid_to > current_date)))
		FROM unit u WHERE u.id = $1`, id)
}

func (l *lookup) Role(id string) map[string]any {
	return l.object(`SELECT jsonb_build_object('id', id, 'key', key, 'name', name) FROM role WHERE id = $1`, id)
}

func (l *lookup) Process(id string) map[string]any {
	return l.object(`SELECT jsonb_build_object('id', id, 'key', key, 'name', name) FROM process WHERE id = $1`, id)
}

func (l *lookup) Actor(id uuid.UUID) map[string]any {
	return l.object(`SELECT jsonb_build_object('id', id, 'name', name) FROM account WHERE id = $1`, id)
}

func (l *lookup) Vacant(positionID, day string) bool {
	if l.err != nil {
		return false
	}
	var held bool
	if err := l.tx.QueryRow(l.ctx, `SELECT EXISTS (SELECT 1 FROM holding WHERE position_id = $1 AND valid_from <= $2::date
			AND (valid_to IS NULL OR valid_to > $2::date))`, positionID, day).Scan(&held); err != nil {
		l.err = err
	}
	return !held
}
