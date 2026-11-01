package oauth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/praetorianer777/bearing/backend/internal/auth"
	"github.com/praetorianer777/bearing/backend/internal/db"
)

// ErrNoToken is a bearer token that names no live token.
var ErrNoToken = errors.New("no such token")

// Bearer is whom a bearer token acts for, and what it acts through. A grant's access token lives an hour; a token
// of an app nobody allows any more, or of a switched off account, acts for nobody.
func (s *Service) Bearer(ctx context.Context, token string) (uuid.UUID, Via, error) {
	var (
		account uuid.UUID
		scopes  []string
		via     Via
	)
	err := s.DB.ReadAdmin(ctx, func(ctx context.Context, tx db.DBTX) error {
		return tx.QueryRow(ctx, `
			SELECT t.id, t.account_id, t.scopes, t.mcp_only FROM api_token t JOIN account a ON a.id = t.account_id
			WHERE t.token_hash = $1 AND a.disabled_at IS NULL
			  AND CASE WHEN t.client_id IS NULL THEN t.expires_at > $2 ELSE t.access_expires_at > $2 END`,
			auth.HashToken(token), s.now()).Scan(&via.ID, &account, &scopes, &via.MCPOnly)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, Via{}, ErrNoToken
	}
	if err != nil {
		return uuid.Nil, Via{}, err
	}
	via.ReadOnly = len(scopes) == 1
	return account, via, nil
}

// Grant is an app somebody connected, as they see it.
type Grant struct {
	ID       uuid.UUID
	ClientID uuid.UUID
	App      string
	Scopes   []string
	// ConnectedAt is when the grant was made; Until when it ends unless the app uses it.
	ConnectedAt time.Time
	Until       time.Time
}

// Grants are the apps the person ctx acts for connected, by name.
func (s *Service) Grants(ctx context.Context) ([]Grant, error) {
	out := []Grant{}
	err := s.DB.Read(ctx, func(ctx context.Context, tx db.DBTX) error {
		rows, err := tx.Query(ctx, `SELECT id, client_id, name, scopes, created_at, expires_at FROM api_token
			WHERE account_id = (SELECT current_actor_id()) AND client_id IS NOT NULL AND expires_at > $1 ORDER BY lower(name), id`, s.now())
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Grant, error) {
			var g Grant
			err := row.Scan(&g.ID, &g.ClientID, &g.App, &g.Scopes, &g.ConnectedAt, &g.Until)
			return g, err
		})
		return err
	})
	return out, err
}

// ErrNoSuchGrant is a grant the person does not hold.
var ErrNoSuchGrant = errors.New("you have not connected that app")

// Disconnect ends a grant of the person ctx acts for.
func (s *Service) Disconnect(ctx context.Context, id uuid.UUID) (db.LSN, error) {
	return s.DB.Write(ctx, func(ctx context.Context, tx db.DBTX) error {
		tag, err := tx.Exec(ctx, `DELETE FROM api_token WHERE id = $1 AND client_id IS NOT NULL`, id)
		if err == nil && tag.RowsAffected() == 0 {
			return ErrNoSuchGrant
		}
		return err
	})
}

// Client is an app as administrators see it.
type Client struct {
	ID           uuid.UUID
	Name         string
	Status       string
	Scopes       []string
	ClientURI    string
	RedirectURIs []string
	// Confidential is false for a public app, which keeps no secret.
	Confidential bool
	// People counts those who hold a live grant for it.
	People    int
	CreatedAt time.Time
}

// Clients are the apps, by name, pending ones nobody allowed in time left out. Only administrators read them.
func (s *Service) Clients(ctx context.Context) ([]Client, error) {
	out := []Client{}
	err := s.DB.Read(ctx, func(ctx context.Context, tx db.DBTX) error {
		rows, err := tx.Query(ctx, `SELECT c.id, c.name, c.status, c.scopes, c.client_uri, c.redirect_uris, c.secret_hash IS NOT NULL,
				oauth_client_people(c.id), c.created_at
			FROM oauth_client c WHERE c.status = 'allowed' OR c.created_at >= $1 ORDER BY lower(c.name), c.id`, s.now().Add(-PendingClientTTL))
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Client, error) {
			var c Client
			err := row.Scan(&c.ID, &c.Name, &c.Status, &c.Scopes, &c.ClientURI, &c.RedirectURIs, &c.Confidential, &c.People, &c.CreatedAt)
			return c, err
		})
		return err
	})
	return out, err
}

// RemoveClient removes an app and every grant of it. Only administrators may.
func (s *Service) RemoveClient(ctx context.Context, id uuid.UUID) (db.LSN, error) {
	return s.DB.Write(ctx, func(ctx context.Context, tx db.DBTX) error {
		tag, err := tx.Exec(ctx, `DELETE FROM oauth_client WHERE id = $1`, id)
		if err == nil && tag.RowsAffected() == 0 {
			return ErrNoSuchClient
		}
		return err
	})
}
