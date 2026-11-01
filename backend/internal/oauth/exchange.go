package oauth

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/praetorianer777/bearing/backend/internal/auth"
	"github.com/praetorianer777/bearing/backend/internal/db"
	"github.com/praetorianer777/bearing/backend/internal/sso"
)

// ParamExpiresIn asks a narrower token to end sooner than the grant's access token, in seconds.
const ParamExpiresIn = "bearing_expires_in"

// exchange trades a token (RFC 8693): a realm access token of a partner's person for a grant of theirs, or an access
// token of the app's own grant for a narrower one.
func (s *Service) exchange(ctx context.Context, client *clientRow, form url.Values) (*TokenResponse, error) {
	if err := required(form, "subject_token", "subject_token_type"); err != nil {
		return nil, err
	}
	if form.Get("subject_token_type") != TokenTypeAccess {
		return nil, refuse(http.StatusBadRequest, "invalid_request", "Bearing exchanges access tokens only: subject_token_type is "+TokenTypeAccess+".")
	}
	if kind := form.Get("requested_token_type"); kind != "" && kind != TokenTypeAccess {
		return nil, refuse(http.StatusBadRequest, "invalid_request", "Bearing issues access tokens only: requested_token_type is "+TokenTypeAccess+".")
	}
	if strings.HasPrefix(form.Get("subject_token"), auth.AccessTokenPrefix) {
		return s.narrow(ctx, client, form)
	}
	if client.Kind != KindPartner || client.KeycloakClientID == nil {
		return nil, refuse(http.StatusBadRequest, "unauthorized_client",
			client.Name+" may not trade realm tokens. A person connects it on Bearing's consent page instead.")
	}
	if s.Realm == nil {
		return nil, refuse(http.StatusBadRequest, "unauthorized_client", "Single sign-on is not set up in this Bearing, so there is no realm token to trade.")
	}
	scopes, err := asked(form.Get("scope"), client.Scopes)
	if err != nil {
		return nil, err
	}
	identity, err := s.Realm(ctx, form.Get("subject_token"), *client.KeycloakClientID)
	switch {
	case errors.Is(err, sso.ErrUnreachable):
		return nil, err
	case err != nil:
		s.log().Info("a partner's realm token was refused", "client", client.ID, "error", err)
		return nil, refuse(http.StatusBadRequest, "invalid_grant", "The realm token was refused: "+strings.TrimSuffix(err.Error(), ".")+".")
	}
	account, err := s.AccountFor(ctx, identity)
	switch {
	case errors.Is(err, auth.ErrAwaitingAdmission), errors.Is(err, auth.ErrDisabled):
		return nil, refuse(http.StatusBadRequest, "invalid_grant", identity.Name+" has no Bearing account to act as. An administrator lets them in under Accounts.")
	case err != nil:
		return nil, err
	}
	var out *TokenResponse
	_, err = s.DB.WriteAdmin(ctx, func(ctx context.Context, tx db.DBTX) error {
		var err error
		out, _, err = s.issue(ctx, tx, client, account.ID, scopes, false)
		return err
	})
	if err != nil {
		return nil, err
	}
	out.IssuedTokenType = TokenTypeAccess
	return out, nil
}

func errSubject() *Error {
	return refuse(http.StatusBadRequest, "invalid_grant", "The subject_token is not a live access token of this app's own grant. Refresh the grant and exchange the new one.")
}

// narrow trades an access token of the app's own grant for one that holds no more and ends no later, without a
// refresh token: for one run, or for a part of the app that should only read.
func (s *Service) narrow(ctx context.Context, client *clientRow, form url.Values) (*TokenResponse, error) {
	digest := auth.HashToken(form.Get("subject_token"))
	var (
		out     *TokenResponse
		refusal error
	)
	_, err := s.DB.WriteAdmin(ctx, func(ctx context.Context, tx db.DBTX) error {
		var (
			grant uuid.UUID
			held  []string
			left  float64
		)
		err := tx.QueryRow(ctx, `
			SELECT t.id, t.scopes, EXTRACT(EPOCH FROM t.access_expires_at - now())::float8 FROM api_token t JOIN account a ON a.id = t.account_id
			WHERE t.token_hash = $1 AND t.client_id = $2 AND t.access_expires_at > now() AND a.disabled_at IS NULL FOR SHARE OF t`,
			digest, client.ID).Scan(&grant, &held, &left)
		if errors.Is(err, pgx.ErrNoRows) {
			refusal = errSubject()
			return nil
		}
		if err != nil {
			return err
		}
		remaining := time.Duration(math.Floor(left)) * time.Second
		scopes, refused := asked(form.Get("scope"), held)
		if refused != nil {
			refusal = refuse(http.StatusBadRequest, "invalid_scope", "An exchange may only narrow the grant: ask for read, and at most "+joinScope(held)+".")
			return nil
		}
		ttl := remaining
		if raw := form.Get(ParamExpiresIn); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 || time.Duration(n)*time.Second > AccessTokenTTL {
				refusal = refuse(http.StatusBadRequest, "invalid_request", fmt.Sprintf("%s is a whole number of seconds from 1 to %d.", ParamExpiresIn, int(AccessTokenTTL/time.Second)))
				return nil
			}
			ttl = min(ttl, time.Duration(n)*time.Second)
		}
		if ttl < time.Second {
			refusal = errSubject()
			return nil
		}
		token, tokenDigest, err := auth.NewPrefixedToken(auth.AccessTokenPrefix)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM oauth_exchanged_token WHERE expires_at < now()`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO oauth_exchanged_token (token_hash, grant_id, scopes, expires_at)
			VALUES ($1, $2, $3, now() + make_interval(secs => $4))`, tokenDigest, grant, scopes, ttl.Seconds()); err != nil {
			return err
		}
		out = &TokenResponse{AccessToken: token, TokenType: "Bearer", ExpiresIn: int(ttl / time.Second), Scope: joinScope(scopes), IssuedTokenType: TokenTypeAccess}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if refusal != nil {
		return nil, refusal
	}
	return out, nil
}
