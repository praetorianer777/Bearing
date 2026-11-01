package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/db"
	"github.com/praetorianer777/bearing/backend/internal/oauth"
)

// fakeOAuth knows bearer tokens by name and serves the rest of the authorization server from a real one without a
// database, which is enough for what needs none.
type fakeOAuth struct {
	*oauth.Service
	tokens map[string]fakeToken
}

type fakeToken struct {
	account uuid.UUID
	via     oauth.Via
}

func (f fakeOAuth) Bearer(_ context.Context, token string) (uuid.UUID, oauth.Via, error) {
	t, ok := f.tokens[token]
	if !ok {
		return uuid.Nil, oauth.Via{}, oauth.ErrNoToken
	}
	return t.account, t.via, nil
}

func (f fakeOAuth) Grants(context.Context) ([]oauth.Grant, error) { return nil, nil }

func (f fakeOAuth) Disconnect(context.Context, uuid.UUID) (db.LSN, error) {
	return 0, oauth.ErrNoSuchGrant
}

func (f fakeOAuth) Clients(context.Context) ([]oauth.Client, error) { return nil, nil }

func (f fakeOAuth) RemoveClient(context.Context, uuid.UUID) (db.LSN, error) {
	return 0, oauth.ErrNoSuchClient
}

func oauthServer() *Server {
	s := signedInServer()
	f := fakeOAuth{Service: &oauth.Service{Issuer: "https://bearing.example"}, tokens: map[string]fakeToken{}}
	f.tokens["mcp"] = fakeToken{bert.ID, oauth.Via{ID: uuid.New(), MCPOnly: true}}
	f.tokens["reading"] = fakeToken{ada.ID, oauth.Via{ID: uuid.New(), ReadOnly: true}}
	f.tokens["writing"] = fakeToken{ada.ID, oauth.Via{ID: uuid.New()}}
	f.tokens["gone"] = fakeToken{uuid.New(), oauth.Via{ID: uuid.New()}}
	s.OAuth = f
	return s
}

func bearerCall(s *Server, method, path, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, APIPrefix+path, strings.NewReader("{}"))
	r.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, r)
	return rec
}

func TestABearerTokenActsWithinWhatItWasGiven(t *testing.T) {
	s := oauthServer()
	if rec := bearerCall(s, http.MethodGet, "/auth/me", "writing"); rec.Code != http.StatusOK {
		t.Errorf("a token that may write, reading: %d %s", rec.Code, rec.Body)
	}
	if rec := bearerCall(s, http.MethodGet, "/auth/me", "mcp"); rec.Code != http.StatusForbidden {
		t.Errorf("a token for MCP beyond it: %d", rec.Code)
	}
	if rec := bearerCall(s, http.MethodPatch, "/auth/me", "reading"); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "only read") {
		t.Errorf("a reading token writing: %d %s", rec.Code, rec.Body)
	}
	for _, token := range []string{"unknown", "gone"} {
		rec := bearerCall(s, http.MethodGet, "/auth/me", token)
		challenge := rec.Header().Get("WWW-Authenticate")
		if rec.Code != http.StatusUnauthorized || !strings.Contains(challenge, `error="invalid_token"`) ||
			!strings.Contains(challenge, "https://bearing.example/.well-known/oauth-protected-resource/api/v1/mcp") {
			t.Errorf("%s: %d %q", token, rec.Code, challenge)
		}
	}
}

func TestTheOAuthEndpointsAnswerInOAuthsShape(t *testing.T) {
	s := oauthServer()
	r := httptest.NewRequest(http.MethodPost, APIPrefix+"/oauth/token", strings.NewReader(url.Values{"grant_type": {"password"}}.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, r)
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), `"error":"invalid_client"`) {
		t.Errorf("a token request naming no app: %d %s", rec.Code, rec.Body)
	}
	r = httptest.NewRequest(http.MethodPost, APIPrefix+"/oauth/register", strings.NewReader(`{"nonsense":1}`))
	rec = httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, r)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_client_metadata") {
		t.Errorf("registering nonsense: %d %s", rec.Code, rec.Body)
	}
	r = httptest.NewRequest(http.MethodGet, APIPrefix+"/oauth/authorize?client_id=x&state=y", nil)
	rec = httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, r)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != s.AppURL+"/connect?client_id=x&state=y" {
		t.Errorf("authorize: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	r = httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil)
	rec = httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, r)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Cache-Control"), "max-age=300") ||
		!strings.Contains(rec.Body.String(), `"issuer":"https://bearing.example"`) {
		t.Errorf("metadata: %d %s", rec.Code, rec.Body)
	}
	if rec := call(s, http.MethodDelete, APIPrefix+"/connected-apps/"+uuid.NewString(), "bert", "", nil); rec.Code != http.StatusNotFound {
		t.Errorf("disconnecting an app never connected: %d", rec.Code)
	}
	if rec := call(s, http.MethodDelete, APIPrefix+"/oauth/clients/"+uuid.NewString(), "ada", "", nil); rec.Code != http.StatusNotFound {
		t.Errorf("removing an app never registered: %d", rec.Code)
	}
}

func TestATokenMakesNoTokenAndConnectsNoApp(t *testing.T) {
	s := oauthServer()
	for _, path := range []string{"/tokens", "/oauth/consent"} {
		if rec := bearerCall(s, http.MethodPost, path, "writing"); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "signed in") {
			t.Errorf("%s with a token: %d %s", path, rec.Code, rec.Body)
		}
	}
}
