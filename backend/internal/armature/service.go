package armature

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"

	"github.com/praetorianer777/bearing/backend/internal/config"
	"github.com/praetorianer777/bearing/backend/internal/db"
	"github.com/praetorianer777/bearing/backend/internal/netguard"
	"github.com/praetorianer777/bearing/backend/internal/secret"
)

const (
	// CapabilitiesTTL is how long what Armature lists at /meta is believed.
	CapabilitiesTTL = 5 * time.Minute
	// RefreshAhead is how long before its access token runs out a grant is renewed.
	RefreshAhead = 2 * time.Minute
	// LoadCacheTTL is how long an answer of resource-load@1 is kept for the same reader and question; never as the
	// truth, only so a page opened twice asks once.
	LoadCacheTTL    = time.Minute
	loadCachePrefix = "bearing:armature:load:"
)

// The ways a read of Armature does not happen, each said to the reader in a sentence.
var (
	ErrNotConfigured = errors.New("bearing is not connected to Armature yet; ask an administrator to connect it")
	ErrNotConnected  = errors.New("you are not connected to Armature; sign in through the realm once, so Bearing can read Armature as you")
	ErrUnreachable   = errors.New("armature did not answer; try again in a moment, and tell an administrator if it keeps happening")
	ErrRejected      = errors.New("armature no longer accepts your connection; sign in through the realm again to renew it")
	ErrNoKey         = errors.New("connecting Armature needs BEARING_SECRET_KEY to keep its secret")
)

// OutdatedError is an Armature that does not offer a capability Bearing needs, which names the partner to update.
type OutdatedError struct{ Capability string }

func (e *OutdatedError) Error() string {
	return "the connected Armature does not offer " + e.Capability + ", which Bearing needs for this; ask an administrator to update Armature to a release that offers it"
}

// Connection is the Armature Bearing reads, as an administrator named it.
type Connection struct {
	BaseURL   string    `json:"baseUrl"`
	ClientID  string    `json:"clientId"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ConnectionInput names the Armature and the partner app Bearing is registered there as.
type ConnectionInput struct {
	BaseURL      string `json:"baseUrl"`
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
}

// FieldError is an input a person has to change.
type FieldError struct{ Field, Message string }

func (e *FieldError) Error() string { return e.Message }

// Service keeps the connection and the grants, and reads Armature.
type Service struct {
	db    *db.Cluster
	box   *secret.Box
	allow netguard.Allow
	call  caller
	cache *redis.Client
	log   *slog.Logger
	now   func() time.Time

	mu      sync.Mutex
	offered map[string]offer
}

type offer struct {
	names []string
	at    time.Time
}

// New returns the service; without a box nothing can be connected, and without a cache nothing is kept.
func New(cluster *db.Cluster, box *secret.Box, allow netguard.Allow, cache *redis.Client, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{db: cluster, box: box, allow: allow, call: caller{http: netguard.Client(CallTimeout, allow)}, cache: cache, log: log,
		now: time.Now, offered: map[string]offer{}}
}

func connectionContext() []byte        { return []byte("armature:connection") }
func grantContext(id uuid.UUID) []byte { return []byte("armature:grant:" + id.String()) }

// Connection is the Armature named, or nil when none is.
func (s *Service) Connection(ctx context.Context) (*Connection, error) {
	var out *Connection
	err := s.db.Read(ctx, func(ctx context.Context, tx db.DBTX) error {
		var c Connection
		err := tx.QueryRow(ctx, `SELECT base_url, client_id, updated_at FROM armature_connection`).Scan(&c.BaseURL, &c.ClientID, &c.UpdatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		out = &c
		return err
	})
	return out, err
}

// Connect names the Armature and the app Bearing is there; the grants made with an earlier one end.
func (s *Service) Connect(ctx context.Context, in ConnectionInput) (*Connection, db.LSN, error) {
	in.BaseURL = strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
	in.ClientID = strings.TrimSpace(in.ClientID)
	if u, err := url.Parse(in.BaseURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return nil, 0, &FieldError{"baseUrl", "Enter Armature's address, starting with https://."}
	}
	if in.ClientID == "" {
		return nil, 0, &FieldError{"clientId", "Enter the client id Armature registered Bearing as."}
	}
	if in.ClientSecret == "" {
		return nil, 0, &FieldError{"clientSecret", "Enter the secret Armature gave Bearing's client."}
	}
	if s.box == nil {
		return nil, 0, ErrNoKey
	}
	sealed, err := s.box.Seal([]byte(in.ClientSecret), connectionContext())
	if err != nil {
		return nil, 0, err
	}
	var out Connection
	lsn, err := s.db.Write(ctx, func(ctx context.Context, tx db.DBTX) error {
		// A replacement rather than an upsert: the app role may not read the sealed secret an upsert compares.
		if _, err := tx.Exec(ctx, `DELETE FROM armature_connection`); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `INSERT INTO armature_connection (base_url, client_id, secret_sealed) VALUES ($1, $2, $3)
			RETURNING base_url, client_id, updated_at`, in.BaseURL, in.ClientID, sealed).Scan(&out.BaseURL, &out.ClientID, &out.UpdatedAt)
	})
	if err != nil {
		return nil, lsn, err
	}
	// Grants are Armature's, so another Armature has none of them.
	more, err := s.db.WriteAdmin(db.WithUser(ctx, uuid.Nil), func(ctx context.Context, tx db.DBTX) error {
		_, err := tx.Exec(ctx, `DELETE FROM armature_grant WHERE created_at < (SELECT updated_at FROM armature_connection)`)
		return err
	})
	return &out, max(lsn, more), err
}

// Disconnect forgets the Armature and every grant made there.
func (s *Service) Disconnect(ctx context.Context) (db.LSN, error) {
	lsn, err := s.db.Write(ctx, func(ctx context.Context, tx db.DBTX) error {
		_, err := tx.Exec(ctx, `DELETE FROM armature_connection`)
		return err
	})
	if err != nil {
		return lsn, err
	}
	more, err := s.db.WriteAdmin(db.WithUser(ctx, uuid.Nil), func(ctx context.Context, tx db.DBTX) error {
		_, err := tx.Exec(ctx, `DELETE FROM armature_grant`)
		return err
	})
	return max(lsn, more), err
}

// app is the connection with its secret opened, read as the admin role.
type app struct{ base, clientID, secret string }

func (s *Service) app(ctx context.Context) (*app, error) {
	var out app
	var sealed []byte
	err := s.db.ReadAdmin(db.WithUser(ctx, uuid.Nil), func(ctx context.Context, tx db.DBTX) error {
		return tx.QueryRow(ctx, `SELECT base_url, client_id, secret_sealed FROM armature_connection`).Scan(&out.base, &out.clientID, &sealed)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotConfigured
	}
	if err != nil {
		return nil, err
	}
	plain, err := s.box.Open(sealed, connectionContext())
	if err != nil {
		return nil, fmt.Errorf("open the Armature client secret: %w", err)
	}
	out.secret = string(plain)
	return &out, nil
}

// Offers says what Armature lists at /meta, kept for CapabilitiesTTL.
func (s *Service) Offers(ctx context.Context, base string) ([]string, error) {
	s.mu.Lock()
	o, ok := s.offered[base]
	s.mu.Unlock()
	if ok && s.now().Sub(o.at) < CapabilitiesTTL {
		return o.names, nil
	}
	names, err := s.call.meta(ctx, base)
	if err != nil {
		s.log.Info("Armature's /meta could not be read", "base", base, "error", err)
		return nil, ErrUnreachable
	}
	s.mu.Lock()
	s.offered[base] = offer{names: names, at: s.now()}
	s.mu.Unlock()
	return names, nil
}

// need refuses an Armature without the capability.
func (s *Service) need(ctx context.Context, base, capability string) error {
	names, err := s.Offers(ctx, base)
	if err != nil {
		return err
	}
	if !slices.Contains(names, capability) {
		return &OutdatedError{Capability: capability}
	}
	return nil
}

// ConnectAtSignIn trades the realm access token of somebody signing in for a grant of theirs at Armature. It never
// fails the sign-in: what goes wrong is logged, and the person reads Armature once a later sign-in works.
func (s *Service) ConnectAtSignIn(ctx context.Context, account uuid.UUID, realmToken string) {
	if s == nil || s.box == nil || realmToken == "" {
		return
	}
	if err := s.connectAtSignIn(ctx, account, realmToken); err != nil && !errors.Is(err, ErrNotConfigured) {
		s.log.Info("the sign-in was not traded for an Armature grant", "account", account, "error", err)
	}
}

func (s *Service) connectAtSignIn(ctx context.Context, account uuid.UUID, realmToken string) error {
	a, err := s.app(ctx)
	if err != nil {
		return err
	}
	if err := s.need(ctx, a.base, CapabilityTokenExchange); err != nil {
		return err
	}
	t, err := s.call.exchange(ctx, a.base, a.clientID, a.secret, realmToken)
	if err != nil {
		return err
	}
	_, err = s.db.WriteAdmin(db.WithUser(ctx, uuid.Nil), func(ctx context.Context, tx db.DBTX) error {
		return s.store(ctx, tx, account, t)
	})
	return err
}

func (s *Service) store(ctx context.Context, tx db.DBTX, account uuid.UUID, t tokens) error {
	access, err := s.box.Seal([]byte(t.AccessToken), grantContext(account))
	if err != nil {
		return err
	}
	refresh, err := s.box.Seal([]byte(t.RefreshToken), grantContext(account))
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO armature_grant (account_id, access_sealed, refresh_sealed, access_expires_at) VALUES ($1, $2, $3, $4)
		ON CONFLICT (account_id) DO UPDATE SET access_sealed = excluded.access_sealed, refresh_sealed = excluded.refresh_sealed,
			access_expires_at = excluded.access_expires_at`, account, access, refresh, s.now().Add(time.Duration(t.ExpiresIn)*time.Second))
	return err
}

// accessFor is a live access token of the person's grant, renewed first when it is about to run out. The renewal
// holds the row, as Armature ends a grant whose rotated refresh token is presented a second time.
func (s *Service) accessFor(ctx context.Context, a *app, account uuid.UUID) (string, error) {
	var token string
	ended := false
	_, err := s.db.WriteAdmin(db.WithUser(ctx, uuid.Nil), func(ctx context.Context, tx db.DBTX) error {
		var access, refresh []byte
		var expires time.Time
		err := tx.QueryRow(ctx, `SELECT access_sealed, refresh_sealed, access_expires_at FROM armature_grant WHERE account_id = $1 FOR UPDATE`, account).
			Scan(&access, &refresh, &expires)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotConnected
		}
		if err != nil {
			return err
		}
		if s.now().Add(RefreshAhead).Before(expires) {
			plain, err := s.box.Open(access, grantContext(account))
			token = string(plain)
			return err
		}
		plain, err := s.box.Open(refresh, grantContext(account))
		if err != nil {
			return err
		}
		t, err := s.call.refresh(ctx, a.base, a.clientID, a.secret, string(plain))
		var status *errStatus
		if errors.As(err, &status) && status.Status < 500 {
			ended = true
			_, err := tx.Exec(ctx, `DELETE FROM armature_grant WHERE account_id = $1`, account)
			return err
		}
		if err != nil {
			return ErrUnreachable
		}
		token = t.AccessToken
		return s.store(ctx, tx, account, t)
	})
	if err == nil && ended {
		return "", ErrRejected
	}
	return token, err
}

// Status is where the reader stands with Armature: not_configured, not_connected or connected.
func (s *Service) Status(ctx context.Context) (string, error) {
	c, err := s.Connection(ctx)
	if err != nil || c == nil {
		return "not_configured", err
	}
	var connected bool
	err = s.db.Read(ctx, func(ctx context.Context, tx db.DBTX) error {
		return tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM armature_grant WHERE account_id = current_actor_id())`).Scan(&connected)
	})
	if connected {
		return "connected", err
	}
	return "not_connected", err
}

// Forget ends the reader's own grant.
func (s *Service) Forget(ctx context.Context) (db.LSN, error) {
	return s.db.Write(ctx, func(ctx context.Context, tx db.DBTX) error {
		_, err := tx.Exec(ctx, `DELETE FROM armature_grant WHERE account_id = current_actor_id()`)
		return err
	})
}

// ResourceLoad is the people's weeks from resource-load@1, read as the reader, kept for LoadCacheTTL.
func (s *Service) ResourceLoad(ctx context.Context, q LoadQuery) ([]PersonLoad, error) {
	reader, ok := db.UserFrom(ctx)
	if !ok {
		return nil, ErrNotConnected
	}
	if s.box == nil {
		return nil, ErrNotConfigured
	}
	a, err := s.app(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.need(ctx, a.base, CapabilityResourceLoad); err != nil {
		return nil, err
	}
	question, _ := json.Marshal(q)
	sum := sha256.Sum256(append([]byte(reader.String()+a.base), question...))
	key := loadCachePrefix + hex.EncodeToString(sum[:])
	if s.cache != nil {
		if kept, err := s.cache.Get(ctx, key).Bytes(); err == nil {
			var out []PersonLoad
			if json.Unmarshal(kept, &out) == nil {
				return out, nil
			}
		}
	}
	access, err := s.accessFor(ctx, a, reader)
	if err != nil {
		return nil, err
	}
	out, err := s.call.resourceLoad(ctx, a.base, access, q)
	var status *errStatus
	switch {
	case errors.As(err, &status) && (status.Status == 401 || status.Status == 403):
		return nil, ErrRejected
	case errors.As(err, &status) && status.Status == 404:
		return nil, &OutdatedError{Capability: CapabilityResourceLoad}
	case err != nil:
		s.log.Info("Armature's resource load could not be read", "error", err)
		return nil, ErrUnreachable
	}
	if s.cache != nil {
		if kept, err := json.Marshal(out); err == nil {
			s.cache.Set(ctx, key, kept, LoadCacheTTL)
		}
	}
	return out, nil
}

// FromConfig is the service as the process's configuration sets it up: without BEARING_SECRET_KEY nothing can be
// connected.
func FromConfig(cfg config.Outbound, cluster *db.Cluster, cache *redis.Client, log *slog.Logger) (*Service, error) {
	var box *secret.Box
	if cfg.SecretKey != nil {
		var err error
		if box, err = secret.New(cfg.SecretKey); err != nil {
			return nil, fmt.Errorf("BEARING_SECRET_KEY: %w", err)
		}
	}
	return New(cluster, box, netguard.ParseAllow(cfg.Allow), cache, log), nil
}
