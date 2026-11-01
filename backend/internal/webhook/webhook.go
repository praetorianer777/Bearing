// Package webhook posts changes to the organisation to addresses its administrators named (webhooks@1), signed
// with a secret each endpoint was shown once, with a log of every attempt, retries, redelivery and a test ping.
// Adapted from Stator's internal/webhook, which is adapted from Armature's: one name, one promise.
package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/praetorianer777/bearing/backend/internal/config"
	"github.com/praetorianer777/bearing/backend/internal/db"
	"github.com/praetorianer777/bearing/backend/internal/netguard"
	"github.com/praetorianer777/bearing/backend/internal/secret"
)

const (
	// MaxAttempts is how many times one event is tried before it is given up.
	MaxAttempts = 6
	// TopicAny subscribes an endpoint to every topic.
	TopicAny = "*"
	// TopicPing is what a test delivery carries.
	TopicPing = "ping"
	// SecretPrefix marks a webhook secret, so one is never taken for a token.
	SecretPrefix = "bearing_whs_"
	// DefaultDeliveries and MaxDeliveries bound one read of the log.
	DefaultDeliveries = 50
	MaxDeliveries     = 200
	// MaxName and MaxURL are the database's bounds on an endpoint.
	MaxName = 100
	MaxURL  = 2000
	// DisableAfter is how long an endpoint fails every attempt before the worker turns it off, and
	// DisableAfterFailures the fewest failures that do, so a receiver down for an afternoon is kept.
	DisableAfter         = 24 * time.Hour
	DisableAfterFailures = MaxAttempts
	// The headers a delivery carries, named as the siblings name theirs. The timestamped signature also covers when
	// the attempt was sent, so a receiver can refuse one replayed later; the version is the body's, so a receiver
	// can refuse one it does not know before parsing it.
	SignatureHeader            = "X-Bearing-Signature-256"
	TimestampHeader            = "X-Bearing-Timestamp"
	TimestampedSignatureHeader = "X-Bearing-Signature-Timestamped-256"
	EventHeader                = "X-Bearing-Event"
	DeliveryHeader             = "X-Bearing-Delivery"
	VersionHeader              = "X-Bearing-Webhook-Version"
	userAgent                  = "Bearing-Webhook"
	// CurrentVersion is the envelope a new endpoint is registered for. It rises only for a change that breaks a
	// receiver.
	CurrentVersion = 1
	// Source is what every envelope names as its sender.
	Source = "bearing"
	// ReasonFailing is an endpoint the worker turned off, not a person.
	ReasonFailing = "failing"
)

// Backoff is the wait before each retry, by attempt number; a sixth failure is the last.
var Backoff = []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 12 * time.Hour}

// Subscribable is every value an endpoint's topics may hold.
var Subscribable = append([]string{TopicAny}, Topics...)

// DisabledReasons and States list the values for the document.
var (
	DisabledReasons = []string{ReasonFailing}
	States          = []DeliveryState{StatePending, StateDelivered, StateFailed, StateWithheld, StateCancelled}
)

var (
	// ErrNotFound is an endpoint or delivery that is not here, or not the caller's to see.
	ErrNotFound = errors.New("there is no such webhook")
	// ErrNoKey refuses an endpoint whose secret could not be sealed.
	ErrNoKey = errors.New("webhooks need BEARING_SECRET_KEY to keep their secrets")
)

// FieldError is an input a person has to change.
type FieldError struct {
	Field   string
	Message string
}

func (e *FieldError) Error() string { return e.Message }

// Owner is the administrator who saved an endpoint last.
type Owner struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// Webhook is one address the organisation posts to.
type Webhook struct {
	ID      uuid.UUID `json:"id"`
	Name    string    `json:"name"`
	URL     string    `json:"url"`
	Topics  []string  `json:"topics"`
	Enabled bool      `json:"enabled"`
	// Version is the envelope version the endpoint was registered for.
	Version int `json:"version"`
	// DisabledReason says the worker turned it off, and why; null when a person did or it is on.
	DisabledReason *string `json:"disabledReason"`
	// Failures counts the attempts that failed since the last delivered.
	Failures int `json:"failures"`
	// Owner is null once the administrator who saved it is gone; nothing but a ping is sent until somebody saves it
	// again.
	Owner     *Owner    `json:"owner"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	// Secret is set only on the answer that made or rotated it.
	Secret string `json:"secret,omitempty"`
}

// DeliveryState is where a delivery stands.
type DeliveryState string

const (
	StatePending   DeliveryState = "pending"
	StateDelivered DeliveryState = "delivered"
	StateFailed    DeliveryState = "failed"
	// StateWithheld is an event nothing was sent for, as the endpoint has no administrator owning it.
	StateWithheld DeliveryState = "withheld"
	// StateCancelled is an attempt that came due while the endpoint was off.
	StateCancelled DeliveryState = "cancelled"
)

// Delivery is one attempt at one event.
type Delivery struct {
	ID        uuid.UUID `json:"id"`
	WebhookID uuid.UUID `json:"webhookId"`
	// EventID is the envelope's id, the same on every attempt and redelivery, so a receiver can tell a repeat.
	EventID uuid.UUID `json:"eventId"`
	Topic   string    `json:"topic"`
	// Attempt numbers the automatic tries 1 to MaxAttempts, and the sends by hand apart from them.
	Attempt int `json:"attempt"`
	// Manual is a test or a redelivery somebody asked for.
	Manual bool          `json:"manual"`
	State  DeliveryState `json:"state"`
	// Status is what the receiver answered, null when it was not reached.
	Status        *int       `json:"status"`
	Error         string     `json:"error"`
	NextAttemptAt *time.Time `json:"nextAttemptAt"`
	AttemptedAt   *time.Time `json:"attemptedAt"`
	DeliveredAt   *time.Time `json:"deliveredAt"`
	CreatedAt     time.Time  `json:"createdAt"`
}

// Input makes or changes an endpoint; enabled left out keeps it as it is, and a new endpoint starts on. Version
// left out is the current one for a new endpoint, and keeps the one a changed endpoint has.
type Input struct {
	Name    string   `json:"name"`
	URL     string   `json:"url"`
	Topics  []string `json:"topics"`
	Enabled *bool    `json:"enabled,omitempty"`
	Version *int     `json:"version,omitempty"`
}

// Sign is the header value for a body: sha256= and the hex HMAC, the siblings' shape, so a receiver written for one
// serves all.
func Sign(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// SignTimestamped signs "{timestamp}.{body}", the time in Unix seconds as the timestamp header carries it.
func SignTimestamped(body []byte, secret string, at time.Time) string {
	return Sign(append([]byte(strconv.FormatInt(at.Unix(), 10)+"."), body...), secret)
}

// Versions are the envelope versions this release sends.
func Versions() []int { return []int{CurrentVersion} }

// Service keeps endpoints and deliveries and does the posting.
type Service struct {
	db     *db.Cluster
	box    *secret.Box
	client *http.Client
	allow  netguard.Allow
	log    *slog.Logger
	now    func() time.Time
}

// New returns the service; a nil box refuses to make an endpoint and fails every delivery, since no secret opens.
// allow is what BEARING_OUTBOUND_ALLOW names.
func New(cluster *db.Cluster, box *secret.Box, allow netguard.Allow, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{db: cluster, box: box, client: netguard.Client(sendTimeout, allow), allow: allow, log: log, now: time.Now}
}

// sealContext binds a sealed secret to its endpoint, so a copy onto another row does not open.
func sealContext(id uuid.UUID) []byte { return []byte("webhook:" + id.String()) }

func newSecret() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("make a webhook secret: %w", err)
	}
	return SecretPrefix + base64.RawURLEncoding.EncodeToString(raw), nil
}

const selectWebhooks = `
SELECT e.id, e.name, e.url, e.topics, e.enabled, e.version, e.disabled_reason, e.failures, e.owner_id, a.name, e.created_at, e.updated_at
FROM webhook_endpoint e LEFT JOIN account a ON a.id = e.owner_id`

func scanWebhook(row pgx.Row) (*Webhook, error) {
	var (
		w         Webhook
		ownerID   *uuid.UUID
		ownerName *string
	)
	if err := row.Scan(&w.ID, &w.Name, &w.URL, &w.Topics, &w.Enabled, &w.Version, &w.DisabledReason, &w.Failures, &ownerID, &ownerName,
		&w.CreatedAt, &w.UpdatedAt); err != nil {
		return nil, err
	}
	if ownerID != nil {
		w.Owner = &Owner{ID: *ownerID}
		if ownerName != nil {
			w.Owner.Name = *ownerName
		}
	}
	return &w, nil
}

func one(ctx context.Context, tx db.DBTX, id uuid.UUID) (*Webhook, error) {
	w, err := scanWebhook(tx.QueryRow(ctx, selectWebhooks+` WHERE e.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return w, err
}

// List is the organisation's endpoints, by name.
func (s *Service) List(ctx context.Context) ([]Webhook, error) {
	out := []Webhook{}
	err := s.db.Read(ctx, func(ctx context.Context, tx db.DBTX) error {
		rows, err := tx.Query(ctx, selectWebhooks+` ORDER BY lower(e.name), e.id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			w, err := scanWebhook(rows)
			if err != nil {
				return err
			}
			out = append(out, *w)
		}
		return rows.Err()
	})
	return out, err
}

// Get is one endpoint.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Webhook, error) {
	var out *Webhook
	err := s.db.Read(ctx, func(ctx context.Context, tx db.DBTX) error {
		var err error
		out, err = one(ctx, tx, id)
		return err
	})
	return out, err
}

// clean checks an input and returns it trimmed, its topics deduplicated in the order Subscribable gives them.
func (s *Service) clean(in Input) (Input, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len([]rune(in.Name)) > MaxName {
		return in, &FieldError{Field: "name", Message: fmt.Sprintf("Give the webhook a name of at most %d characters, such as the system it feeds.", MaxName)}
	}
	in.URL = strings.TrimSpace(in.URL)
	if err := s.checkURL(in.URL); err != nil {
		return in, err
	}
	if len(in.Topics) == 0 {
		return in, &FieldError{Field: "topics", Message: "Choose at least one event to send."}
	}
	for _, t := range in.Topics {
		if !slices.Contains(Subscribable, t) {
			return in, &FieldError{Field: "topics", Message: fmt.Sprintf("A webhook cannot take the event %q. Choose from %s.", t, strings.Join(Subscribable, ", "))}
		}
	}
	topics := []string{}
	for _, t := range Subscribable {
		if slices.Contains(in.Topics, t) {
			topics = append(topics, t)
		}
	}
	in.Topics = topics
	if in.Version != nil && !slices.Contains(Versions(), *in.Version) {
		return in, &FieldError{Field: "version", Message: fmt.Sprintf("Bearing sends webhook envelope version %d. Register the webhook for that version.", CurrentVersion)}
	}
	return in, nil
}

// checkURL refuses what is no address to post to, and an address that names the server's own network outright. A
// name is resolved only when it is posted to, where netguard decides again on every attempt.
func (s *Service) checkURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Fragment != "" || len(raw) > MaxURL {
		return &FieldError{Field: "url", Message: "Enter the address to post to, starting with https://, without a user name or a # part."}
	}
	host := strings.ToLower(u.Hostname())
	if s.allow.Names(host) {
		return nil
	}
	addr, notAddr := netip.ParseAddr(host)
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || (notAddr == nil && netguard.Reserved(addr)) {
		return &FieldError{Field: "url", Message: "Bearing may not post to an address inside the server's own network. Use the receiver's public address, or ask the operator to add the host to BEARING_OUTBOUND_ALLOW."}
	}
	return nil
}

func isUniqueName(err error) bool {
	return err != nil && strings.Contains(err.Error(), "webhook_endpoint_name_idx")
}

var errNameTaken = &FieldError{Field: "name", Message: "A webhook with that name is already here. Choose another name."}

// Create makes an endpoint and returns it with its secret, this once.
func (s *Service) Create(ctx context.Context, in Input) (*Webhook, db.LSN, error) {
	in, err := s.clean(in)
	if err != nil {
		return nil, 0, err
	}
	if s.box == nil {
		return nil, 0, ErrNoKey
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, 0, err
	}
	plain, err := newSecret()
	if err != nil {
		return nil, 0, err
	}
	sealed, err := s.box.Seal([]byte(plain), sealContext(id))
	if err != nil {
		return nil, 0, err
	}
	enabled := in.Enabled == nil || *in.Enabled
	version := CurrentVersion
	if in.Version != nil {
		version = *in.Version
	}
	var out *Webhook
	lsn, err := s.db.Write(ctx, func(ctx context.Context, tx db.DBTX) error {
		_, err := tx.Exec(ctx, `INSERT INTO webhook_endpoint (id, name, url, secret_sealed, topics, enabled, version)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`, id, in.Name, in.URL, sealed, in.Topics, enabled, version)
		if isUniqueName(err) {
			return errNameTaken
		}
		if err != nil {
			return err
		}
		out, err = one(ctx, tx, id)
		return err
	})
	if err != nil {
		return nil, lsn, err
	}
	out.Secret = plain
	return out, lsn, nil
}

// Update changes the name, address, topics or whether it is on, and makes the caller its owner.
func (s *Service) Update(ctx context.Context, id uuid.UUID, in Input) (*Webhook, db.LSN, error) {
	in, err := s.clean(in)
	if err != nil {
		return nil, 0, err
	}
	var out *Webhook
	lsn, err := s.db.Write(ctx, func(ctx context.Context, tx db.DBTX) error {
		tag, err := tx.Exec(ctx, `UPDATE webhook_endpoint SET name = $2, url = $3, topics = $4, enabled = COALESCE($5, enabled),
			version = COALESCE($6, version) WHERE id = $1`, id, in.Name, in.URL, in.Topics, in.Enabled, in.Version)
		if isUniqueName(err) {
			return errNameTaken
		}
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		out, err = one(ctx, tx, id)
		return err
	})
	return out, lsn, err
}

// RotateSecret issues a new secret; the old one stops working at once, as in the siblings.
func (s *Service) RotateSecret(ctx context.Context, id uuid.UUID) (*Webhook, db.LSN, error) {
	if s.box == nil {
		return nil, 0, ErrNoKey
	}
	plain, err := newSecret()
	if err != nil {
		return nil, 0, err
	}
	sealed, err := s.box.Seal([]byte(plain), sealContext(id))
	if err != nil {
		return nil, 0, err
	}
	var out *Webhook
	lsn, err := s.db.Write(ctx, func(ctx context.Context, tx db.DBTX) error {
		tag, err := tx.Exec(ctx, `UPDATE webhook_endpoint SET secret_sealed = $2 WHERE id = $1`, id, sealed)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		out, err = one(ctx, tx, id)
		return err
	})
	if err != nil {
		return nil, lsn, err
	}
	out.Secret = plain
	return out, lsn, nil
}

// Delete removes the endpoint and its log.
func (s *Service) Delete(ctx context.Context, id uuid.UUID) (db.LSN, error) {
	return s.db.Write(ctx, func(ctx context.Context, tx db.DBTX) error {
		tag, err := tx.Exec(ctx, `DELETE FROM webhook_endpoint WHERE id = $1`, id)
		if err == nil && tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return err
	})
}

const selectDeliveries = `
SELECT id, endpoint_id, event_id, topic, attempt, manual, state, status, error, next_attempt_at, attempted_at, delivered_at, created_at
FROM webhook_delivery`

func scanDelivery(row pgx.Row) (*Delivery, error) {
	var d Delivery
	if err := row.Scan(&d.ID, &d.WebhookID, &d.EventID, &d.Topic, &d.Attempt, &d.Manual, &d.State, &d.Status, &d.Error,
		&d.NextAttemptAt, &d.AttemptedAt, &d.DeliveredAt, &d.CreatedAt); err != nil {
		return nil, err
	}
	return &d, nil
}

// Deliveries is an endpoint's log, newest first.
func (s *Service) Deliveries(ctx context.Context, id uuid.UUID, limit int) ([]Delivery, error) {
	limit = min(max(limit, 1), MaxDeliveries)
	out := []Delivery{}
	err := s.db.Read(ctx, func(ctx context.Context, tx db.DBTX) error {
		if _, err := one(ctx, tx, id); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, selectDeliveries+` WHERE endpoint_id = $1 ORDER BY created_at DESC, id DESC LIMIT $2`, id, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			d, err := scanDelivery(rows)
			if err != nil {
				return err
			}
			out = append(out, *d)
		}
		return rows.Err()
	})
	return out, err
}

// FromConfig is the service as the process's configuration sets it up: without BEARING_SECRET_KEY it keeps no
// secrets, so no endpoint can be made.
func FromConfig(cfg config.Outbound, cluster *db.Cluster, log *slog.Logger) (*Service, error) {
	var box *secret.Box
	if cfg.SecretKey != nil {
		var err error
		if box, err = secret.New(cfg.SecretKey); err != nil {
			return nil, fmt.Errorf("BEARING_SECRET_KEY: %w", err)
		}
	}
	return New(cluster, box, netguard.ParseAllow(cfg.Allow), log), nil
}
