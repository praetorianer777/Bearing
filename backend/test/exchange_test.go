//go:build integration

package test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/praetorianer777/bearing/backend/internal/sso"
)

const tokenExchange = "urn:ietf:params:oauth:grant-type:token-exchange"
const accessTokenType = "urn:ietf:params:oauth:token-type:access_token"

// realmToken signs a realm user in at the stack's Keycloak with a password, as a partner's own sign-in would leave it
// holding their access token.
func realmToken(t *testing.T, username, password, client, secret string) string {
	t.Helper()
	backchannel, err := sso.ParseBackchannel(env(t, "BEARING_OIDC_BACKCHANNEL"))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := sso.Backchannel(backchannel).PostForm(env(t, "BEARING_OIDC_ISSUER")+"/protocol/openid-connect/token", url.Values{
		"grant_type": {"password"}, "client_id": {client}, "client_secret": {secret}, "username": {username}, "password": {password}, "scope": {"openid"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		AccessToken string `json:"access_token"`
	}
	raw, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(raw, &out); err != nil || out.AccessToken == "" {
		t.Fatalf("the realm answered %d %s", resp.StatusCode, raw)
	}
	return out.AccessToken
}

type partnerApp struct {
	Client struct{ ID, Kind, Status, KeycloakClientID string }
	Secret string
}

// partner registers the stack's test partner afresh, one left by an earlier run removed first.
func partner(t *testing.T, admin *browser) registered {
	t.Helper()
	var clients struct {
		Clients []struct{ ID, KeycloakClientID string }
	}
	admin.do(http.MethodGet, "/oauth/clients", nil, &clients)
	for _, c := range clients.Clients {
		if c.KeycloakClientID == "bearing-partner" {
			admin.do(http.MethodDelete, "/oauth/clients/"+c.ID, nil, nil)
		}
	}
	var made partnerApp
	if status := admin.do(http.MethodPost, "/oauth/clients", map[string]any{"name": "Partner", "write": true, "keycloakClientId": "bearing-partner"}, &made); status != http.StatusCreated {
		t.Fatalf("registering the partner answered %d", status)
	}
	if made.Client.Kind != "partner" || made.Client.Status != "allowed" || !strings.HasPrefix(made.Secret, "bearing_ocs_") {
		t.Fatalf("the partner: %+v", made)
	}
	return registered{ClientID: made.Client.ID, ClientSecret: made.Secret}
}

func exchange(t *testing.T, app registered, subject string, extra url.Values) (int, tokens) {
	t.Helper()
	form := url.Values{"grant_type": {tokenExchange}, "subject_token": {subject}, "subject_token_type": {accessTokenType}}
	for k, v := range extra {
		form[k] = v
	}
	return token(t, form, &app)
}

func TestAPartnerTradesARealmTokenForAGrantOfThePersonsOwn(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	app := partner(t, admin)
	var again partnerApp
	if status := admin.do(http.MethodPost, "/oauth/clients", map[string]any{"name": "Twin", "keycloakClientId": "bearing-partner"}, &again); status != http.StatusUnprocessableEntity {
		t.Errorf("a second partner for one realm client answered %d", status)
	}

	status, granted := exchange(t, app, realmToken(t, "bob", "bob password", "bearing-partner", "bearing-partner-dev-secret"), nil)
	if status != http.StatusOK || granted.Scope != "read write" || !strings.HasPrefix(granted.RefreshToken, "bearing_ort_") {
		t.Fatalf("trading bob's token: %d %+v", status, granted)
	}
	req, _ := http.NewRequest(http.MethodGet, inProcessAPI(t)+"/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+granted.AccessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var me accountBody
	_ = json.NewDecoder(resp.Body).Decode(&me)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || me.Account.Email != "bob@suite.test" {
		t.Errorf("the grant acts as %d %+v", resp.StatusCode, me.Account)
	}

	if status, out := exchange(t, app, realmToken(t, "bob", "bob password", "bearing", "bearing-dev-secret"), nil); status != http.StatusBadRequest || out.Error != "invalid_grant" {
		t.Errorf("a token issued to another client: %d %+v", status, out)
	}
	if status, out := exchange(t, app, "not.a.token", nil); status != http.StatusBadRequest || out.Error != "invalid_grant" {
		t.Errorf("a forged token: %d %+v", status, out)
	}
	if status, out := exchange(t, app, realmToken(t, "carol", "carol password", "bearing-partner", "bearing-partner-dev-secret"), nil); status != http.StatusBadRequest || out.Error != "invalid_grant" {
		t.Errorf("somebody without an account: %d %+v", status, out)
	}
	if status, out := exchange(t, app, realmToken(t, "bob", "bob password", "bearing-partner", "bearing-partner-dev-secret"), url.Values{"subject_token_type": {"urn:ietf:params:oauth:token-type:id_token"}}); status != http.StatusBadRequest || out.Error != "invalid_request" {
		t.Errorf("an id token: %d %+v", status, out)
	}

	consented := register(t, "Consented", "client_secret_basic")
	_, challenge := pkce()
	connect(t, admin, authorizeQuery(consented.ClientID, challenge, ""), "")
	if status, out := exchange(t, consented, realmToken(t, "bob", "bob password", "bearing-partner", "bearing-partner-dev-secret"), nil); status != http.StatusBadRequest || out.Error != "unauthorized_client" {
		t.Errorf("an app that registered itself trading a realm token: %d %+v", status, out)
	}
}

func TestAnAppTradesItsOwnTokenForANarrowerOneForARun(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	app := partner(t, admin)
	_, granted := exchange(t, app, realmToken(t, "bob", "bob password", "bearing-partner", "bearing-partner-dev-secret"), nil)

	status, narrow := exchange(t, app, granted.AccessToken, url.Values{"scope": {"read"}, "bearing_expires_in": {"60"}})
	if status != http.StatusOK || narrow.Scope != "read" || narrow.ExpiresIn > 60 || narrow.RefreshToken != "" || narrow.AccessToken == granted.AccessToken {
		t.Fatalf("narrowing: %d %+v", status, narrow)
	}
	if got := bearer(t, http.MethodGet, "/auth/me", narrow.AccessToken); got != http.StatusOK {
		t.Errorf("the narrower token reading: %d", got)
	}
	if got := bearer(t, http.MethodPatch, "/auth/me", narrow.AccessToken); got != http.StatusForbidden {
		t.Errorf("the narrower token writing: %d", got)
	}
	if status, out := exchange(t, app, narrow.AccessToken, nil); status != http.StatusBadRequest || out.Error != "invalid_grant" {
		t.Errorf("narrowing a narrower token: %d %+v", status, out)
	}
	if status, out := exchange(t, app, granted.AccessToken, url.Values{"bearing_expires_in": {"7200"}}); status != http.StatusBadRequest || out.Error != "invalid_request" {
		t.Errorf("a narrower token outliving its grant: %d %+v", status, out)
	}
	other := register(t, "Other", "client_secret_basic")
	_, challenge := pkce()
	connect(t, admin, authorizeQuery(other.ClientID, challenge, ""), "")
	if status, out := exchange(t, other, granted.AccessToken, nil); status != http.StatusBadRequest || out.Error != "invalid_grant" {
		t.Errorf("another app narrowing the partner's token: %d %+v", status, out)
	}

	form := url.Values{"token": {narrow.AccessToken}}
	req, _ := http.NewRequest(http.MethodPost, inProcessAPI(t)+"/api/v1/oauth/revoke", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(url.QueryEscape(app.ClientID), url.QueryEscape(app.ClientSecret))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got := bearer(t, http.MethodGet, "/auth/me", narrow.AccessToken); got != http.StatusUnauthorized {
		t.Errorf("a revoked narrower token: %d", got)
	}
	if got := bearer(t, http.MethodGet, "/auth/me", granted.AccessToken); got != http.StatusOK {
		t.Errorf("revoking the narrower token ended the grant: %d", got)
	}
}
