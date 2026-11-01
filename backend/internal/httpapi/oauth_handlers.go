package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/db"
	"github.com/praetorianer777/bearing/backend/internal/oauth"
)

// OAuth is the authorization server, oauth.Service the real one.
type OAuth interface {
	Metadata() oauth.Metadata
	ResourceMetadata() oauth.ResourceMetadata
	ResourceMetadataURL() string
	Register(ctx context.Context, in oauth.RegistrationRequest) (*oauth.RegistrationResponse, error)
	Authorize(ctx context.Context, rawQuery string) (*oauth.Authorization, error)
	Consent(a *oauth.Authorization, person oauth.Person) (*oauth.Consent, string)
	Decide(ctx context.Context, a *oauth.Authorization, person oauth.Person, d oauth.Decision) (string, error)
	Token(ctx context.Context, cred oauth.Credentials, form url.Values) (*oauth.TokenResponse, error)
	Revoke(ctx context.Context, cred oauth.Credentials, token string) error
	Bearer(ctx context.Context, token string) (uuid.UUID, oauth.Via, error)
	Grants(ctx context.Context) ([]oauth.Grant, error)
	Disconnect(ctx context.Context, id uuid.UUID) (db.LSN, error)
	Clients(ctx context.Context) ([]oauth.Client, error)
	RemoveClient(ctx context.Context, id uuid.UUID) (db.LSN, error)
}

// tokenForm is the token endpoint's form, in OAuth's names (RFC 6749 sections 4.1.3 and 6).
type tokenForm struct {
	GrantType    string `json:"grant_type"`
	Code         string `json:"code,omitempty"`
	RedirectURI  string `json:"redirect_uri,omitempty"`
	CodeVerifier string `json:"code_verifier,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
	Resource     string `json:"resource,omitempty"`
	ClientID     string `json:"client_id,omitempty"`
	ClientSecret string `json:"client_secret,omitempty"`
}

// revokeForm is the revocation endpoint's form (RFC 7009).
type revokeForm struct {
	Token         string `json:"token"`
	TokenTypeHint string `json:"token_type_hint,omitempty"`
	ClientID      string `json:"client_id,omitempty"`
	ClientSecret  string `json:"client_secret,omitempty"`
}

type consentRequest struct {
	// Query is the authorize request's query, as the consent page was opened with it.
	Query string `json:"query"`
	Allow bool   `json:"allow"`
	// Scope narrows what the app asked for, to read; empty keeps what it asked.
	Scope string `json:"scope,omitempty"`
}

type consentResponse struct {
	// Consent is what to ask the person; nil when the browser goes back to the app at once.
	Consent *oauth.Consent `json:"consent"`
	// Redirect is where the browser goes instead; empty when the person is asked.
	Redirect string `json:"redirect"`
}

type redirectResponse struct {
	Redirect string `json:"redirect"`
}

type grantResponse struct {
	ID          uuid.UUID `json:"id"`
	ClientID    uuid.UUID `json:"clientId"`
	App         string    `json:"app"`
	Scopes      []string  `json:"scopes"`
	ConnectedAt time.Time `json:"connectedAt"`
	Until       time.Time `json:"until"`
}

type clientResponse struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	Status       string    `json:"status"`
	Scopes       []string  `json:"scopes"`
	ClientURI    string    `json:"clientUri"`
	RedirectURIs []string  `json:"redirectUris"`
	Confidential bool      `json:"confidential"`
	People       int       `json:"people"`
	CreatedAt    time.Time `json:"createdAt"`
}

// respondOAuth answers in OAuth's own shape, which partners' libraries read, rather than the API's envelope.
func (s *Server) respondOAuth(w http.ResponseWriter, r *http.Request, err error) {
	var e *oauth.Error
	if !errors.As(err, &e) {
		respondError(w, r, s.Log, err)
		return
	}
	if e.Basic {
		w.Header().Set("WWW-Authenticate", `Basic realm="bearing"`)
	}
	respondJSON(w, e.Status, e)
}

// respondMetadata answers a metadata document, which partners may keep a while.
func respondMetadata(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age="+strconv.Itoa(int(oauth.MetadataMaxAge/time.Second)))
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) handleOAuthMetadata(w http.ResponseWriter, _ *http.Request) {
	respondMetadata(w, s.OAuth.Metadata())
}

func (s *Server) handleResourceMetadata(w http.ResponseWriter, _ *http.Request) {
	respondMetadata(w, s.OAuth.ResourceMetadata())
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var in oauth.RegistrationRequest
	if err := decode(r, &in); err != nil {
		respondJSON(w, http.StatusBadRequest, &oauth.Error{Code: "invalid_client_metadata", Description: "Send the client metadata as JSON (RFC 7591)."})
		return
	}
	out, err := s.OAuth.Register(r.Context(), in)
	if err != nil {
		s.respondOAuth(w, r, err)
		return
	}
	respondJSON(w, http.StatusCreated, out)
}

// handleAuthorize sends the browser to the consent page with the request as it came; the page asks the API what to
// show, once the person is signed in.
func (s *Server) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, s.AppURL+oauth.ConsentPath+"?"+r.URL.RawQuery, http.StatusFound)
}

func (s *Server) person(r *http.Request) oauth.Person {
	a := PrincipalFrom(r.Context()).Account
	return oauth.Person{ID: a.ID, Name: a.Name, Administers: a.Role == administrator}
}

// authorization reads the authorize request the consent page passes on; an unknown app is a page error, never a
// redirect.
func (s *Server) authorization(w http.ResponseWriter, r *http.Request, query string) (*oauth.Authorization, bool) {
	a, err := s.OAuth.Authorize(r.Context(), query)
	if errors.Is(err, oauth.ErrUnknownApp) {
		respondError(w, r, s.Log, &APIError{Status: http.StatusBadRequest, Code: "unknown_app",
			Message: "Bearing does not know this app, or the address it wants to go back to. Ask the app's makers to register it again."})
		return nil, false
	}
	if err != nil {
		respondError(w, r, s.Log, err)
		return nil, false
	}
	return a, true
}

func (s *Server) handleConsent(w http.ResponseWriter, r *http.Request) {
	a, ok := s.authorization(w, r, r.URL.RawQuery)
	if !ok {
		return
	}
	c, to := s.OAuth.Consent(a, s.person(r))
	respondJSON(w, http.StatusOK, consentResponse{Consent: c, Redirect: to})
}

func (s *Server) handleDecide(w http.ResponseWriter, r *http.Request) {
	var in consentRequest
	if err := decode(r, &in); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	a, ok := s.authorization(w, r, in.Query)
	if !ok {
		return
	}
	to, err := s.OAuth.Decide(r.Context(), a, s.person(r), oauth.Decision{Allow: in.Allow, Scope: in.Scope})
	var refused *oauth.RefusedError
	if errors.As(err, &refused) {
		respondError(w, r, s.Log, ErrForbidden(refused.Sentence))
		return
	}
	if err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	respondJSON(w, http.StatusOK, redirectResponse{Redirect: to})
}

// oauthForm reads a form-encoded body, which OAuth's endpoints take.
func oauthForm(r *http.Request) (url.Values, error) {
	r.Body = http.MaxBytesReader(nil, r.Body, maxBody)
	if err := r.ParseForm(); err != nil {
		return nil, &oauth.Error{Status: http.StatusBadRequest, Code: "invalid_request", Description: "Send the parameters form-encoded."}
	}
	return r.PostForm, nil
}

func (s *Server) handleToken(w http.ResponseWriter, r *http.Request) {
	form, err := oauthForm(r)
	if err != nil {
		s.respondOAuth(w, r, err)
		return
	}
	cred, err := oauth.ReadCredentials(r.Header.Get("Authorization"), form)
	if err != nil {
		s.respondOAuth(w, r, err)
		return
	}
	out, err := s.OAuth.Token(r.Context(), cred, form)
	if err != nil {
		s.respondOAuth(w, r, err)
		return
	}
	w.Header().Set("Pragma", "no-cache")
	respondJSON(w, http.StatusOK, out)
}

func (s *Server) handleRevoke(w http.ResponseWriter, r *http.Request) {
	form, err := oauthForm(r)
	if err != nil {
		s.respondOAuth(w, r, err)
		return
	}
	cred, err := oauth.ReadCredentials(r.Header.Get("Authorization"), form)
	if err == nil {
		err = s.OAuth.Revoke(r.Context(), cred, form.Get("token"))
	}
	if err != nil {
		s.respondOAuth(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleGrants(w http.ResponseWriter, r *http.Request) {
	grants, err := s.OAuth.Grants(r.Context())
	if err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	out := make([]grantResponse, len(grants))
	for i, g := range grants {
		out[i] = grantResponse{ID: g.ID, ClientID: g.ClientID, App: g.App, Scopes: g.Scopes, ConnectedAt: g.ConnectedAt, Until: g.Until}
	}
	respondJSON(w, http.StatusOK, env{"grants": out})
}

func (s *Server) handleDisconnect(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "grantID"))
	if err != nil {
		respondError(w, r, s.Log, ErrNotFound())
		return
	}
	lsn, err := s.OAuth.Disconnect(r.Context(), id)
	noteWrite(r.Context(), lsn)
	if errors.Is(err, oauth.ErrNoSuchGrant) {
		err = ErrNotFound()
	}
	if err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleClients(w http.ResponseWriter, r *http.Request) {
	clients, err := s.OAuth.Clients(r.Context())
	if err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	out := make([]clientResponse, len(clients))
	for i, c := range clients {
		out[i] = clientResponse{ID: c.ID, Name: c.Name, Status: c.Status, Scopes: c.Scopes, ClientURI: c.ClientURI,
			RedirectURIs: c.RedirectURIs, Confidential: c.Confidential, People: c.People, CreatedAt: c.CreatedAt}
	}
	respondJSON(w, http.StatusOK, env{"clients": out})
}

func (s *Server) handleRemoveClient(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "clientID"))
	if err != nil {
		respondError(w, r, s.Log, ErrNotFound())
		return
	}
	lsn, err := s.OAuth.RemoveClient(r.Context(), id)
	noteWrite(r.Context(), lsn)
	if errors.Is(err, oauth.ErrNoSuchClient) {
		err = ErrNotFound()
	}
	if err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
