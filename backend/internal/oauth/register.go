package oauth

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/auth"
	"github.com/praetorianer777/bearing/backend/internal/db"
)

// RegistrationRequest is RFC 7591's client metadata, as much of it as Bearing reads.
type RegistrationRequest struct {
	ClientName              string   `json:"client_name"`
	ClientURI               string   `json:"client_uri,omitempty"`
	RedirectURIs            []string `json:"redirect_uris"`
	Scope                   string   `json:"scope,omitempty"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method,omitempty"`
	GrantTypes              []string `json:"grant_types,omitempty"`
	ResponseTypes           []string `json:"response_types,omitempty"`
}

// RegistrationResponse is RFC 7591's answer; the secret is shown this once.
type RegistrationResponse struct {
	ClientID                string   `json:"client_id"`
	ClientSecret            string   `json:"client_secret,omitempty"`
	ClientSecretExpiresAt   int64    `json:"client_secret_expires_at"`
	ClientIDIssuedAt        int64    `json:"client_id_issued_at"`
	ClientName              string   `json:"client_name"`
	ClientURI               string   `json:"client_uri,omitempty"`
	RedirectURIs            []string `json:"redirect_uris"`
	Scope                   string   `json:"scope"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
}

func invalidMetadata(description string) *Error {
	return refuse(http.StatusBadRequest, "invalid_client_metadata", description)
}

// Register keeps an app that registered itself, pending until an administrator allows it. Apps nobody allowed in
// time are removed as the next one registers.
func (s *Service) Register(ctx context.Context, in RegistrationRequest) (*RegistrationResponse, error) {
	name := strings.TrimSpace(in.ClientName)
	if name == "" || utf8.RuneCountInString(name) > MaxNameLength {
		return nil, invalidMetadata(fmt.Sprintf("Name the app in client_name, in 1 to %d characters, as people read it on the consent page.", MaxNameLength))
	}
	clientURI := strings.TrimSpace(in.ClientURI)
	if u, err := url.Parse(clientURI); clientURI != "" && (err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "") {
		return nil, invalidMetadata("Write client_uri as an http or https address, or leave it out.")
	}
	if len(in.RedirectURIs) == 0 || len(in.RedirectURIs) > MaxRedirectURIs {
		return nil, refuse(http.StatusBadRequest, "invalid_redirect_uri", fmt.Sprintf("Register 1 to %d addresses Bearing sends the browser back to.", MaxRedirectURIs))
	}
	redirects := []string{}
	for _, raw := range in.RedirectURIs {
		if !validRedirect(raw) {
			return nil, refuse(http.StatusBadRequest, "invalid_redirect_uri", "The return address "+raw+
				" is neither https nor http on a loopback address, or it has a fragment. Register https addresses, or http://127.0.0.1 or http://localhost at any port.")
		}
		if !slices.Contains(redirects, raw) {
			redirects = append(redirects, raw)
		}
	}
	scopes := Scopes
	if strings.TrimSpace(in.Scope) != "" {
		var err error
		if scopes, err = normalScopes(in.Scope); err != nil {
			return nil, invalidMetadata(strings.ToUpper(err.Error()[:1]) + err.Error()[1:] + ".")
		}
	}
	method := in.TokenEndpointAuthMethod
	if method == "" {
		method = AuthBasic
	}
	if !slices.Contains(AuthMethods, method) {
		return nil, invalidMetadata("Choose none, client_secret_basic or client_secret_post as token_endpoint_auth_method.")
	}
	grants := []string{GrantAuthorizationCode, GrantRefreshToken}
	if len(in.GrantTypes) > 0 {
		for _, g := range in.GrantTypes {
			if !slices.Contains(GrantTypes, g) {
				return nil, invalidMetadata(fmt.Sprintf("Bearing does not offer the %s grant. Choose from %s.", g, strings.Join(GrantTypes, ", ")))
			}
		}
		if !slices.Contains(in.GrantTypes, GrantAuthorizationCode) {
			return nil, invalidMetadata("An app is connected by authorization_code; name it among grant_types.")
		}
		grants = slices.Compact(slices.Sorted(slices.Values(in.GrantTypes)))
	}
	for _, r := range in.ResponseTypes {
		if r != ResponseCode {
			return nil, invalidMetadata("Bearing answers the code response type only.")
		}
	}
	var (
		secret string
		digest []byte
	)
	if method != AuthNone {
		var err error
		if secret, digest, err = auth.NewPrefixedToken(auth.ClientSecretPrefix); err != nil {
			return nil, err
		}
	}
	out := &RegistrationResponse{
		ClientSecret: secret, ClientName: name, ClientURI: clientURI, RedirectURIs: redirects, Scope: joinScope(scopes),
		TokenEndpointAuthMethod: method, GrantTypes: grants, ResponseTypes: []string{ResponseCode},
	}
	_, err := s.DB.WriteAdmin(ctx, func(ctx context.Context, tx db.DBTX) error {
		if _, err := tx.Exec(ctx, `DELETE FROM oauth_client WHERE status = 'pending' AND created_at < $1`, s.now().Add(-PendingClientTTL)); err != nil {
			return err
		}
		var (
			id      uuid.UUID
			created time.Time
		)
		if err := tx.QueryRow(ctx, `
			INSERT INTO oauth_client (name, scopes, client_uri, redirect_uris, auth_method, secret_hash)
			VALUES ($1, $2, $3, $4, $5, $6) RETURNING id, created_at`, name, scopes, clientURI, redirects, method, digest).Scan(&id, &created); err != nil {
			return err
		}
		out.ClientID, out.ClientIDIssuedAt = id.String(), created.Unix()
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.log().Info("an app registered itself and waits for an administrator", "client", out.ClientID, "name", name)
	return out, nil
}
