package oauth

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/praetorianer777/bearing/backend/internal/auth"
	"github.com/praetorianer777/bearing/backend/internal/db"
)

// TokenResponse is the token endpoint's answer (RFC 6749 section 5.1).
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

// Credentials are how an app says who it is at the token and revocation endpoints.
type Credentials struct {
	ID, Secret string
	// Basic is HTTP Basic, which a refusal answers with WWW-Authenticate.
	Basic      bool
	sentSecret bool
}

// ReadCredentials reads an app's credentials from the Authorization header or the form, whichever it used; both is
// refused.
func ReadCredentials(header string, form url.Values) (Credentials, error) {
	badBasic := &Error{Status: http.StatusUnauthorized, Code: "invalid_client", Description: "Send the client id and secret as HTTP Basic, form-encoded first.", Basic: true}
	if encoded, ok := strings.CutPrefix(header, "Basic "); ok {
		raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
		id, secret, found := strings.Cut(string(raw), ":")
		if err != nil || !found {
			return Credentials{}, badBasic
		}
		if id, err = url.QueryUnescape(id); err == nil {
			secret, err = url.QueryUnescape(secret)
		}
		if err != nil {
			return Credentials{}, badBasic
		}
		if form.Has("client_secret") || (form.Has("client_id") && form.Get("client_id") != id) {
			return Credentials{}, refuse(http.StatusBadRequest, "invalid_request", "Authenticate one way: HTTP Basic or the form, not both.")
		}
		return Credentials{ID: id, Secret: secret, Basic: true, sentSecret: true}, nil
	}
	if form.Get("client_id") == "" {
		return Credentials{}, refuse(http.StatusUnauthorized, "invalid_client", "Say which app is asking, with its client_id, and its secret when it has one.")
	}
	return Credentials{ID: form.Get("client_id"), Secret: form.Get("client_secret"), sentSecret: form.Has("client_secret")}, nil
}

// clientRow is an app as the authorization server reads it.
type clientRow struct {
	ID           uuid.UUID
	Name         string
	Scopes       []string
	Status       string
	ClientURI    string
	RedirectURIs []string
	digest       []byte
}

// loadClient reads an app, a pending one nobody allowed in time read as gone.
func (s *Service) loadClient(ctx context.Context, tx db.DBTX, id uuid.UUID) (*clientRow, error) {
	var c clientRow
	err := tx.QueryRow(ctx, `
		SELECT id, name, scopes, status, client_uri, redirect_uris, secret_hash
		FROM oauth_client WHERE id = $1 AND (status = 'allowed' OR created_at >= $2)`, id, s.now().Add(-PendingClientTTL)).
		Scan(&c.ID, &c.Name, &c.Scopes, &c.Status, &c.ClientURI, &c.RedirectURIs, &c.digest)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoSuchClient
	}
	return &c, err
}

// authenticate is the allowed app the credentials prove, by its secret, or by nothing for a public one.
func (s *Service) authenticate(ctx context.Context, tx db.DBTX, cred Credentials) (*clientRow, error) {
	refused := &Error{Status: http.StatusUnauthorized, Code: "invalid_client", Basic: cred.Basic,
		Description: "Bearing does not know this app, its secret is wrong, or no Bearing administrator has allowed it yet."}
	id, err := uuid.Parse(cred.ID)
	if err != nil {
		return nil, refused
	}
	c, err := s.loadClient(ctx, tx, id)
	if errors.Is(err, ErrNoSuchClient) {
		return nil, refused
	}
	if err != nil {
		return nil, err
	}
	if c.Status != StatusAllowed {
		return nil, refused
	}
	if c.digest == nil {
		if cred.sentSecret {
			return nil, refused
		}
		return c, nil
	}
	if cred.Secret == "" || subtle.ConstantTimeCompare(auth.HashToken(cred.Secret), c.digest) != 1 {
		return nil, refused
	}
	return c, nil
}

// singleParams may each be sent once at most.
var singleParams = []string{"grant_type", "code", "redirect_uri", "code_verifier", "refresh_token", "scope", "client_id", "client_secret", "resource"}

// Token serves the token endpoint: a code redeemed for a grant, or a grant's refresh token traded for new tokens.
func (s *Service) Token(ctx context.Context, cred Credentials, form url.Values) (*TokenResponse, error) {
	for _, name := range singleParams {
		if len(form[name]) > 1 {
			return nil, refuse(http.StatusBadRequest, "invalid_request", fmt.Sprintf("Send %s once.", name))
		}
	}
	grant := form.Get("grant_type")
	if grant == "" {
		return nil, refuse(http.StatusBadRequest, "invalid_request", "Say which grant, with grant_type.")
	}
	if !slices.Contains(GrantTypes, grant) {
		return nil, refuse(http.StatusBadRequest, "unsupported_grant_type", "Bearing offers "+strings.Join(GrantTypes, ", ")+".")
	}
	var client *clientRow
	if err := s.DB.ReadAdmin(ctx, func(ctx context.Context, tx db.DBTX) error {
		var err error
		client, err = s.authenticate(ctx, tx, cred)
		return err
	}); err != nil {
		return nil, err
	}
	if resource := form.Get("resource"); resource != "" && !s.resourceMatches(resource) {
		return nil, refuse(http.StatusBadRequest, "invalid_target", "The one resource is Bearing's MCP endpoint, "+s.Resource()+".")
	}
	var (
		out     *TokenResponse
		refusal error
	)
	_, err := s.DB.WriteAdmin(ctx, func(ctx context.Context, tx db.DBTX) error {
		var err error
		if grant == GrantAuthorizationCode {
			out, refusal, err = s.redeem(ctx, tx, client, form)
		} else {
			out, refusal, err = s.refresh(ctx, tx, client, form)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	if refusal != nil {
		return nil, refusal
	}
	return out, nil
}

func required(form url.Values, names ...string) error {
	for _, name := range names {
		if form.Get(name) == "" {
			return refuse(http.StatusBadRequest, "invalid_request", fmt.Sprintf("Send %s.", name))
		}
	}
	return nil
}

// active says whether an account may still lend itself to an app.
func active(ctx context.Context, tx db.DBTX, account uuid.UUID) (bool, error) {
	var ok bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM account WHERE id = $1 AND disabled_at IS NULL)`, account).Scan(&ok)
	return ok, err
}

// redeem trades a code for the person's one grant for the app, made afresh: the consent just given says what it may
// do. A code used twice ends the grant it made, as RFC 6749 section 10.5 asks, though the answer refuses.
func (s *Service) redeem(ctx context.Context, tx db.DBTX, client *clientRow, form url.Values) (*TokenResponse, error, error) {
	if err := required(form, "code", "redirect_uri", "code_verifier"); err != nil {
		return nil, err, nil
	}
	var (
		account                       uuid.UUID
		owner                         uuid.UUID
		scopes                        []string
		redirect, challenge, resource string
		expires                       time.Time
		used                          *time.Time
		grantID                       *uuid.UUID
	)
	digest := auth.HashToken(form.Get("code"))
	err := tx.QueryRow(ctx, `
		SELECT client_id, account_id, scopes, redirect_uri, challenge, resource, expires_at, used_at, grant_id
		FROM oauth_code WHERE code_hash = $1 FOR UPDATE`, digest).
		Scan(&owner, &account, &scopes, &redirect, &challenge, &resource, &expires, &used, &grantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ended(), nil
	}
	if err != nil {
		return nil, nil, err
	}
	if owner != client.ID {
		return nil, ended(), nil
	}
	if used != nil {
		if grantID != nil {
			if _, err := tx.Exec(ctx, `DELETE FROM api_token WHERE id = $1`, *grantID); err != nil {
				return nil, nil, err
			}
			s.log().Warn("an authorization code came back a second time; its grant is ended", "client", client.ID, "account", account)
		}
		return nil, ended(), nil
	}
	if !s.now().Before(expires) || redirect != form.Get("redirect_uri") || !VerifyPKCE(challenge, form.Get("code_verifier")) {
		return nil, ended(), nil
	}
	if ok, err := active(ctx, tx, account); err != nil || !ok {
		return nil, ended(), err
	}
	out, id, err := s.issue(ctx, tx, client, account, scopes)
	if err != nil {
		return nil, nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE oauth_code SET used_at = now(), grant_id = $2 WHERE code_hash = $1`, digest, id); err != nil {
		return nil, nil, err
	}
	return out, nil, nil
}

// issue makes the grant, replacing one the person held for the app.
func (s *Service) issue(ctx context.Context, tx db.DBTX, client *clientRow, account uuid.UUID, scopes []string) (*TokenResponse, uuid.UUID, error) {
	access, accessDigest, err := auth.NewPrefixedToken(auth.AccessTokenPrefix)
	if err != nil {
		return nil, uuid.Nil, err
	}
	refresh, refreshDigest, err := auth.NewPrefixedToken(auth.RefreshTokenPrefix)
	if err != nil {
		return nil, uuid.Nil, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM api_token WHERE account_id = $1 AND client_id = $2`, account, client.ID); err != nil {
		return nil, uuid.Nil, err
	}
	now := s.now()
	var id uuid.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO api_token (account_id, name, token_hash, scopes, expires_at, client_id, refresh_hash, access_expires_at, mcp_only)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, true) RETURNING id`,
		account, client.Name, accessDigest, scopes, now.Add(RefreshTokenTTL), client.ID, refreshDigest, now.Add(AccessTokenTTL)).Scan(&id); err != nil {
		return nil, uuid.Nil, fmt.Errorf("store the grant: %w", err)
	}
	return &TokenResponse{AccessToken: access, TokenType: "Bearer", ExpiresIn: int(AccessTokenTTL / time.Second),
		RefreshToken: refresh, Scope: joinScope(scopes)}, id, nil
}

// refresh trades a grant's refresh token for new tokens, both rotated; one traded already ends the grant. It may
// narrow the scope, never widen it.
func (s *Service) refresh(ctx context.Context, tx db.DBTX, client *clientRow, form url.Values) (*TokenResponse, error, error) {
	if err := required(form, "refresh_token"); err != nil {
		return nil, err, nil
	}
	digest := auth.HashToken(form.Get("refresh_token"))
	var (
		id, account uuid.UUID
		held        []string
		expires     time.Time
	)
	err := tx.QueryRow(ctx, `SELECT id, account_id, scopes, expires_at FROM api_token WHERE refresh_hash = $1 AND client_id = $2 FOR UPDATE`,
		digest, client.ID).Scan(&id, &account, &held, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		tag, err := tx.Exec(ctx, `
			DELETE FROM api_token t USING oauth_rotated_refresh r
			WHERE r.refresh_hash = $1 AND t.id = r.grant_id AND t.client_id = $2`, digest, client.ID)
		if err == nil && tag.RowsAffected() > 0 {
			s.log().Warn("a refresh token came back after it was traded; its grant is ended", "client", client.ID)
		}
		return nil, ended(), err
	}
	if err != nil {
		return nil, nil, err
	}
	if !s.now().Before(expires) {
		return nil, ended(), nil
	}
	if ok, err := active(ctx, tx, account); err != nil || !ok {
		return nil, ended(), err
	}
	scopes, refusal := asked(form.Get("scope"), held)
	if refusal != nil {
		return nil, refusal, nil
	}
	access, accessDigest, err := auth.NewPrefixedToken(auth.AccessTokenPrefix)
	if err != nil {
		return nil, nil, err
	}
	refresh, refreshDigest, err := auth.NewPrefixedToken(auth.RefreshTokenPrefix)
	if err != nil {
		return nil, nil, err
	}
	now := s.now()
	if _, err := tx.Exec(ctx, `INSERT INTO oauth_rotated_refresh (refresh_hash, grant_id) VALUES ($1, $2)`, digest, id); err != nil {
		return nil, nil, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM oauth_rotated_refresh WHERE rotated_at < $1`, now.Add(-RefreshTokenTTL)); err != nil {
		return nil, nil, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE api_token SET token_hash = $2, refresh_hash = $3, scopes = $4, expires_at = $5, access_expires_at = $6 WHERE id = $1`,
		id, accessDigest, refreshDigest, scopes, now.Add(RefreshTokenTTL), now.Add(AccessTokenTTL)); err != nil {
		return nil, nil, err
	}
	return &TokenResponse{AccessToken: access, TokenType: "Bearer", ExpiresIn: int(AccessTokenTTL / time.Second),
		RefreshToken: refresh, Scope: joinScope(scopes)}, nil, nil
}

// Revoke ends the grant an access or refresh token of the app's belongs to (RFC 7009). A token Bearing does not
// know, or another app's, is no error, so the answer says nothing of it.
func (s *Service) Revoke(ctx context.Context, cred Credentials, token string) error {
	_, err := s.DB.WriteAdmin(ctx, func(ctx context.Context, tx db.DBTX) error {
		client, err := s.authenticate(ctx, tx, cred)
		if err != nil || token == "" {
			return err
		}
		digest := auth.HashToken(token)
		_, err = tx.Exec(ctx, `DELETE FROM api_token WHERE client_id = $2 AND (token_hash = $1 OR refresh_hash = $1)`, digest, client.ID)
		return err
	})
	return err
}
