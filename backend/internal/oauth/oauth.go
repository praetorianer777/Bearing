// Package oauth makes Bearing an OAuth authorization server for partner apps, as Stator and Armature are: an app
// registers itself (RFC 7591), an administrator allows it by connecting it once, and anybody signed in connects it
// on a consent page by authorization code with PKCE. What they allow is a grant that acts at the MCP endpoint alone.
package oauth

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/db"
)

// CapabilityPartnerTokens is what /meta lists for the consent flow, under the name the siblings list it by.
const CapabilityPartnerTokens = "oauth.partner-tokens@1"

// The RFCs' names, and the grants the token endpoint serves.
const (
	GrantAuthorizationCode = "authorization_code"
	GrantRefreshToken      = "refresh_token"
)

// GrantTypes are the grants the metadata names.
var GrantTypes = []string{GrantAuthorizationCode, GrantRefreshToken}

// An app that registered itself is pending until an administrator allows it.
const (
	StatusPending = "pending"
	StatusAllowed = "allowed"
)

// Scopes a grant may hold. One holding read alone may only read; one holding write may do whatever its person may,
// and no more.
const (
	ScopeRead  = "read"
	ScopeWrite = "write"
)

// Scopes lists every scope, as the metadata names them.
var Scopes = []string{ScopeRead, ScopeWrite}

const (
	// AccessTokenTTL and RefreshTokenTTL are the siblings': an hour, and 90 days from the last refresh.
	AccessTokenTTL  = time.Hour
	RefreshTokenTTL = 90 * 24 * time.Hour
	// CodeTTL is how long an authorization code waits to be redeemed; CodeKeep how long a used one is kept, so its
	// reuse is seen.
	CodeTTL  = 60 * time.Second
	CodeKeep = 24 * time.Hour
	// PendingClientTTL is how long an app that registered itself waits for an administrator before it is removed.
	PendingClientTTL = 24 * time.Hour
	// MaxRedirectURIs bounds the return addresses an app registers.
	MaxRedirectURIs = 5
	// MaxNameLength bounds an app's name, which the consent page and the list of connected apps show.
	MaxNameLength = 60
)

// Error is a refusal in OAuth's own shape (RFC 6749 section 5.2).
type Error struct {
	Status      int    `json:"-"`
	Code        string `json:"error"`
	Description string `json:"error_description"`
	// Basic asks for WWW-Authenticate: Basic, for a client that used it.
	Basic bool `json:"-"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Description }

func refuse(status int, code, description string) *Error {
	return &Error{Status: status, Code: code, Description: description}
}

// ended is what a grant or code that is no more is answered with.
func ended() *Error {
	return refuse(http.StatusBadRequest, "invalid_grant", "This connection has ended. Connect Bearing again.")
}

// ErrNoSuchClient is an app that is not registered, or not any more.
var ErrNoSuchClient = errors.New("that app is not registered")

// RefusedError is something the person may not do, with the sentence why.
type RefusedError struct{ Sentence string }

func (e *RefusedError) Error() string { return e.Sentence }

// Service is the authorization server.
type Service struct {
	DB *db.Cluster
	// Issuer is BEARING_APP_URL, the base of every address in the metadata.
	Issuer string
	Log    *slog.Logger
	// Now is the time; nil is time.Now. Tests wait out a code with it.
	Now func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now == nil {
		return time.Now()
	}
	return s.Now()
}

func (s *Service) issuer() string { return strings.TrimSuffix(s.Issuer, "/") }

// Resource is the MCP endpoint as RFC 8707 names it, the one resource a grant is for.
func (s *Service) Resource() string { return s.issuer() + APIPrefix + MCPPath }

// resourceMatches takes the resource with or without a trailing slash.
func (s *Service) resourceMatches(asked string) bool {
	return strings.TrimSuffix(asked, "/") == s.Resource()
}

// splitScope reads a scope parameter, each scope once.
func splitScope(raw string) []string {
	return slices.Compact(slices.Sorted(slices.Values(strings.Fields(raw))))
}

func joinScope(scopes []string) string { return strings.Join(scopes, " ") }

// asked is the scopes asked for within those allowed: all of them when none is named, read always among them.
func asked(raw string, allowed []string) ([]string, error) {
	scopes := splitScope(raw)
	if len(scopes) == 0 {
		return slices.Clone(allowed), nil
	}
	if !slices.Contains(scopes, ScopeRead) || slices.ContainsFunc(scopes, func(s string) bool { return !slices.Contains(allowed, s) }) {
		return nil, refuse(http.StatusBadRequest, "invalid_scope", "Ask for read, and at most "+joinScope(allowed)+".")
	}
	return scopes, nil
}

// normalScopes are scopes as an app may register them: known, read among them, sorted and each once.
func normalScopes(raw string) ([]string, error) {
	out := splitScope(raw)
	for _, scope := range out {
		if !slices.Contains(Scopes, scope) {
			return nil, fmt.Errorf("%s is not a scope. Name read, and write for an app that changes the organisation", scope)
		}
	}
	if !slices.Contains(out, ScopeRead) {
		return nil, errors.New("name read among the scopes; every app reads")
	}
	return out, nil
}

// Via is what a request made with a bearer token acts through.
type Via struct {
	// ID is the api_token row, stamped on what the request writes.
	ID uuid.UUID
	// ReadOnly holds the request to reading.
	ReadOnly bool
	// MCPOnly holds the request to the MCP endpoint.
	MCPOnly bool
}

func (s *Service) log() *slog.Logger {
	if s.Log == nil {
		return slog.Default()
	}
	return s.Log
}
