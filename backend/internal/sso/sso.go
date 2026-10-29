// Package sso signs people in through the installation's OpenID Connect provider: the authorization code flow with
// PKCE, the id token verified, and the claims Bearing needs read out of it.
package sso

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	coreoidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/redis/go-redis/v9"
	"golang.org/x/oauth2"
)

// DefaultGroupsClaim is the claim listing somebody's groups, as the suite's realm names it.
const DefaultGroupsClaim = "groups"

// loginTTL is how long a sign-in may take at the provider before its state is forgotten.
const loginTTL = 10 * time.Minute

const statePrefix = "bearing:sso-login:"

var (
	// ErrNoEmail is a token without an email address: the person could be neither shown nor matched.
	ErrNoEmail = errors.New("the identity provider did not send an email address")
	// ErrEmailUnverified is a provider saying it has not checked the address it sent.
	ErrEmailUnverified = errors.New("the identity provider has not verified that email address")
	// ErrUnknownLogin is a callback whose state no sign-in started, or one that took too long.
	ErrUnknownLogin = errors.New("this sign-in is unknown or took too long; start again")
	// ErrUnreachable is a provider that does not answer, which is its address or the network, not the person.
	ErrUnreachable = errors.New("the identity provider could not be reached")
)

// Config is the provider and how Bearing is registered with it.
type Config struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	// RedirectURL is the callback the provider sends the browser back to.
	RedirectURL string
	GroupsClaim string
	// Backchannel maps the issuer's public origin to the one this process reaches it at.
	Backchannel map[string]string
}

// Identity is what a verified token said about somebody.
type Identity struct {
	// Issuer and Subject together are the provider's stable name for a person, which survives an email change.
	Issuer  string
	Subject string
	Email   string
	Name    string
	Groups  []string
	// AuthTime is when the person last proved who they are at the provider, zero when the token does not say.
	AuthTime time.Time
}

// login is what is kept between sending the browser to the provider and its coming back.
type login struct {
	Verifier string `json:"verifier"`
	Nonce    string `json:"nonce"`
	Next     string `json:"next"`
}

// Service signs people in through one provider. The provider is discovered on first use, so Bearing starts while
// the provider is still coming up.
type Service struct {
	cfg    Config
	http   *http.Client
	states *redis.Client

	mu       sync.Mutex
	provider *coreoidc.Provider
}

// New signs in through cfg's provider, keeping sign-ins in progress in states.
func New(cfg Config, states *redis.Client) *Service {
	if cfg.GroupsClaim == "" {
		cfg.GroupsClaim = DefaultGroupsClaim
	}
	return &Service{cfg: cfg, http: Backchannel(cfg.Backchannel), states: states}
}

func (s *Service) context(ctx context.Context) context.Context {
	ctx = coreoidc.ClientContext(ctx, s.http)
	return context.WithValue(ctx, oauth2.HTTPClient, s.http)
}

func (s *Service) discover(ctx context.Context) (*coreoidc.Provider, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.provider != nil {
		return s.provider, nil
	}
	p, err := coreoidc.NewProvider(s.context(ctx), s.cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	s.provider = p
	return p, nil
}

func (s *Service) oauth(p *coreoidc.Provider) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     s.cfg.ClientID,
		ClientSecret: s.cfg.ClientSecret,
		RedirectURL:  s.cfg.RedirectURL,
		Endpoint:     p.Endpoint(),
		Scopes:       []string{coreoidc.ScopeOpenID, "profile", "email"},
	}
}

func random() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// Start begins a sign-in that comes back to next, and returns the provider's address to send the browser to.
func (s *Service) Start(ctx context.Context, next string) (string, error) {
	p, err := s.discover(ctx)
	if err != nil {
		return "", err
	}
	state := random()
	l := login{Verifier: oauth2.GenerateVerifier(), Nonce: random(), Next: next}
	raw, _ := json.Marshal(l)
	if err := s.states.Set(ctx, statePrefix+state, raw, loginTTL).Err(); err != nil {
		return "", fmt.Errorf("keep the sign-in: %w", err)
	}
	return s.oauth(p).AuthCodeURL(state, coreoidc.Nonce(l.Nonce), oauth2.S256ChallengeOption(l.Verifier)), nil
}

// Finish takes the provider's answer: the code is exchanged, the id token verified against the nonce the sign-in
// started with, and its claims read. It returns who signed in and where they were headed.
func (s *Service) Finish(ctx context.Context, state, code string) (Identity, string, error) {
	raw, err := s.states.GetDel(ctx, statePrefix+state).Bytes()
	if errors.Is(err, redis.Nil) || state == "" {
		return Identity{}, "", ErrUnknownLogin
	}
	if err != nil {
		return Identity{}, "", fmt.Errorf("read the sign-in: %w", err)
	}
	var l login
	if err := json.Unmarshal(raw, &l); err != nil {
		return Identity{}, "", ErrUnknownLogin
	}
	p, err := s.discover(ctx)
	if err != nil {
		return Identity{}, l.Next, err
	}
	token, err := s.oauth(p).Exchange(s.context(ctx), code, oauth2.VerifierOption(l.Verifier))
	if err != nil {
		return Identity{}, l.Next, fmt.Errorf("exchange the code: %w", err)
	}
	rawID, _ := token.Extra("id_token").(string)
	if rawID == "" {
		return Identity{}, l.Next, errors.New("the identity provider sent no id token")
	}
	idToken, err := p.Verifier(&coreoidc.Config{ClientID: s.cfg.ClientID}).Verify(s.context(ctx), rawID)
	if err != nil {
		return Identity{}, l.Next, fmt.Errorf("verify the id token: %w", err)
	}
	if idToken.Nonce != l.Nonce {
		return Identity{}, l.Next, errors.New("the id token answers another sign-in")
	}
	var claims map[string]any
	if err := idToken.Claims(&claims); err != nil {
		return Identity{}, l.Next, fmt.Errorf("read the id token: %w", err)
	}
	id, err := identityFromClaims(idToken.Issuer, idToken.Subject, claims, s.cfg.GroupsClaim)
	if err != nil {
		return Identity{}, l.Next, err
	}
	return *id, l.Next, nil
}

// ParseBackchannel reads "public=reachable" pairs, separated by commas.
func ParseBackchannel(spec string) (map[string]string, error) {
	out := map[string]string{}
	for pair := range strings.SplitSeq(spec, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		public, reach, ok := strings.Cut(pair, "=")
		if !ok || public == "" || reach == "" {
			return nil, fmt.Errorf("%q is not public=reachable", pair)
		}
		out[strings.TrimRight(public, "/")] = strings.TrimRight(reach, "/")
	}
	return out, nil
}
