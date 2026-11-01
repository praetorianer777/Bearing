package oauth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/praetorianer777/bearing/backend/internal/auth"
	"github.com/praetorianer777/bearing/backend/internal/db"
)

// ErrUnknownApp is a client or a return address Bearing does not know, which is never redirected to.
var ErrUnknownApp = errors.New("the app or its return address is not registered")

// Authorization is an authorize request whose app and return address are known. Anything else wrong with it is
// answered at that address.
type Authorization struct {
	client      *clientRow
	redirectURI string
	state       string
	scopes      []string
	challenge   string
	resource    string
	// problem is answered at the return address instead of the consent page.
	problem *Error
}

// Person is who is asked to connect an app.
type Person struct {
	ID   uuid.UUID
	Name string
	// Administers is whether the person may allow an app nobody allowed yet.
	Administers bool
}

// Consent is what the consent page shows.
type Consent struct {
	Client       ConsentClient `json:"client"`
	RedirectHost string        `json:"redirectHost"`
	Person       string        `json:"person"`
	// Scopes are what the app asks for: read, and write when it may change the organisation.
	Scopes []string `json:"scopes"`
	// Pending is an app no administrator allowed yet; AllowsApp says this person's consent is what allows it.
	Pending   bool `json:"pending"`
	AllowsApp bool `json:"allowsApp"`
	// MayAllow is false when the person may only cancel, and Refusal says why.
	MayAllow bool   `json:"mayAllow"`
	Refusal  string `json:"refusal,omitempty"`
}

// ConsentClient is the app as the consent page names it.
type ConsentClient struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	ClientURI string    `json:"clientUri,omitempty"`
}

// Decision is the person's answer on the consent page.
type Decision struct {
	// Allow is false for cancelling.
	Allow bool
	// Scope narrows what the app asked for, to read; empty keeps it.
	Scope string
}

// authorizeParams may each be sent once at most.
var authorizeParams = []string{"response_type", "client_id", "state", "scope", "code_challenge", "code_challenge_method", "resource"}

// Authorize reads an authorize query. An unknown app and a return address it did not register read the same, and
// are never redirected to.
func (s *Service) Authorize(ctx context.Context, rawQuery string) (*Authorization, error) {
	q, err := url.ParseQuery(rawQuery)
	if err != nil {
		return nil, ErrUnknownApp
	}
	id, err := uuid.Parse(q.Get("client_id"))
	if err != nil {
		return nil, ErrUnknownApp
	}
	var client *clientRow
	err = s.DB.ReadAdmin(ctx, func(ctx context.Context, tx db.DBTX) error {
		client, err = s.loadClient(ctx, tx, id)
		return err
	})
	if errors.Is(err, ErrNoSuchClient) {
		return nil, ErrUnknownApp
	}
	if err != nil {
		return nil, err
	}
	a := &Authorization{client: client, redirectURI: q.Get("redirect_uri"), state: q.Get("state"),
		challenge: q.Get("code_challenge"), resource: q.Get("resource")}
	if a.redirectURI == "" && len(client.RedirectURIs) == 1 {
		a.redirectURI = client.RedirectURIs[0]
	}
	if client.Kind != KindRegistered || len(q["redirect_uri"]) > 1 || !RedirectMatches(client.RedirectURIs, a.redirectURI) {
		return nil, ErrUnknownApp
	}
	for _, name := range authorizeParams {
		if len(q[name]) > 1 {
			a.problem = refuse(http.StatusBadRequest, "invalid_request", fmt.Sprintf("Send %s once.", name))
			return a, nil
		}
	}
	a.scopes, err = asked(q.Get("scope"), client.Scopes)
	switch {
	case q.Get("response_type") != ResponseCode:
		a.problem = refuse(http.StatusBadRequest, "unsupported_response_type", "Bearing answers response_type=code only.")
	case a.state == "":
		a.problem = refuse(http.StatusBadRequest, "invalid_request", "Send a state of your own, so the answer can be tied to the request.")
	case q.Get("code_challenge_method") != ChallengeS256 || !challengeShape.MatchString(a.challenge):
		a.problem = refuse(http.StatusBadRequest, "invalid_request", "Bearing requires PKCE: send a code_challenge with code_challenge_method S256.")
	case err != nil:
		a.problem = refuse(http.StatusBadRequest, "invalid_scope", fmt.Sprintf("Ask for read, and at most %s.", joinScope(client.Scopes)))
	case a.resource != "" && !s.resourceMatches(a.resource):
		a.problem = refuse(http.StatusBadRequest, "invalid_target", "The one resource is Bearing's MCP endpoint, "+s.Resource()+".")
	}
	return a, nil
}

// pendingRefusal is what somebody who is no administrator is told of an app nobody allowed.
func pendingRefusal(name string) string {
	return fmt.Sprintf("A Bearing administrator must allow %s first. Ask one to connect it once; then you can connect it too.", name)
}

// Consent is what the page shows the person, or where the browser goes at once: an error answered at the app.
func (s *Service) Consent(a *Authorization, person Person) (*Consent, string) {
	c := &Consent{
		Client:       ConsentClient{ID: a.client.ID, Name: a.client.Name, ClientURI: a.client.ClientURI},
		RedirectHost: redirectHost(a.redirectURI),
		Person:       person.Name,
		Scopes:       a.scopes,
		Pending:      a.client.Status == StatusPending,
	}
	if c.Scopes == nil {
		c.Scopes = []string{}
	}
	switch {
	// An app nobody allowed is not sent anybody back, so its address cannot borrow Bearing's for a redirect.
	case a.problem != nil && c.Pending:
		c.Refusal = a.problem.Description
	case a.problem != nil:
		return nil, a.errorRedirect(s.issuer(), a.problem)
	case c.Pending && !person.Administers:
		c.Refusal = pendingRefusal(a.client.Name)
	default:
		c.MayAllow, c.AllowsApp = true, c.Pending
	}
	return c, ""
}

// Decide carries out the person's answer and says where the browser goes. Allowing a pending app, which only an
// administrator may, allows it for everybody.
func (s *Service) Decide(ctx context.Context, a *Authorization, person Person, d Decision) (string, error) {
	c, to := s.Consent(a, person)
	switch {
	case to != "":
		return to, nil
	case !d.Allow && c.Pending && !c.MayAllow:
		return "", &RefusedError{Sentence: c.Refusal}
	case !d.Allow:
		return a.errorRedirect(s.issuer(), refuse(http.StatusBadRequest, "access_denied", person.Name+" did not connect "+a.client.Name+".")), nil
	case !c.MayAllow:
		return "", &RefusedError{Sentence: c.Refusal}
	}
	scopes := a.scopes
	if d.Scope != "" {
		narrowed, err := asked(d.Scope, a.scopes)
		if err != nil {
			return "", &RefusedError{Sentence: fmt.Sprintf("Allow read, or %s, as %s asked.", joinScope(a.scopes), a.client.Name)}
		}
		scopes = narrowed
	}
	code, digest, err := auth.NewPrefixedToken(auth.CodePrefix)
	if err != nil {
		return "", err
	}
	_, err = s.DB.WriteAdmin(ctx, func(ctx context.Context, tx db.DBTX) error {
		if c.AllowsApp {
			if _, err := tx.Exec(ctx, `UPDATE oauth_client SET status = 'allowed' WHERE id = $1 AND status = 'pending'`, a.client.ID); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `DELETE FROM oauth_code WHERE expires_at < $1`, s.now().Add(-CodeKeep)); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO oauth_code (code_hash, client_id, account_id, scopes, redirect_uri, challenge, resource, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			digest, a.client.ID, person.ID, scopes, a.redirectURI, a.challenge, a.resource, s.now().Add(CodeTTL))
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.ConstraintName == "oauth_client_allowed" {
			return &RefusedError{Sentence: pendingRefusal(a.client.Name)}
		}
		return err
	})
	if err != nil {
		return "", err
	}
	if c.AllowsApp {
		s.log().Info("an administrator allowed an app", "client", a.client.ID, "name", a.client.Name, "by", person.ID)
	}
	return a.redirect(url.Values{"code": {code}, "iss": {s.issuer()}}), nil
}

// errorRedirect answers a problem at the app's return address, RFC 9207's iss included as on success.
func (a *Authorization) errorRedirect(issuer string, e *Error) string {
	return a.redirect(url.Values{"error": {e.Code}, "error_description": {e.Description}, "iss": {issuer}})
}

// redirect is the return address with the answer and the state added to the query it already has.
func (a *Authorization) redirect(add url.Values) string {
	u, err := url.Parse(a.redirectURI)
	if err != nil {
		return a.redirectURI
	}
	q := u.Query()
	for k, v := range add {
		q[k] = v
	}
	if a.state != "" {
		q.Set("state", a.state)
	}
	u.RawQuery = q.Encode()
	return u.String()
}
