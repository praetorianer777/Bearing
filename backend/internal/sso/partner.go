package sso

import (
	"context"
	"errors"
	"fmt"

	coreoidc "github.com/coreos/go-oidc/v3/oidc"
)

// ErrNotForBearing is a realm token that is no access token a partner may trade here: not signed by the provider,
// expired, not meant for Bearing, or issued to another client than the partner's.
var ErrNotForBearing = errors.New("the token is not one a partner may trade at Bearing")

// VerifyAccessToken checks a realm access token a partner trades: signed by the provider and live, with Bearing
// among its audiences and issued to the partner's own client, azp. It says who the token names.
func (s *Service) VerifyAccessToken(ctx context.Context, raw, azp string) (Identity, error) {
	p, err := s.discover(ctx)
	if err != nil {
		return Identity{}, err
	}
	token, err := p.Verifier(&coreoidc.Config{ClientID: s.cfg.ClientID}).Verify(s.context(ctx), raw)
	if err != nil {
		return Identity{}, fmt.Errorf("%w: %w", ErrNotForBearing, err)
	}
	var claims map[string]any
	if err := token.Claims(&claims); err != nil {
		return Identity{}, fmt.Errorf("read the token: %w", err)
	}
	if got := asString(claims["azp"]); got != azp {
		return Identity{}, fmt.Errorf("%w: it was issued to %q", ErrNotForBearing, got)
	}
	id, err := identityFromClaims(token.Issuer, token.Subject, claims, s.cfg.GroupsClaim)
	if err != nil {
		return Identity{}, err
	}
	return *id, nil
}
