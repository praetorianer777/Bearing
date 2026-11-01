//go:build integration

package test

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type registered struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

type consentJSON struct {
	Consent *struct {
		Client    struct{ Name string }
		Scopes    []string
		Pending   bool
		AllowsApp bool
		MayAllow  bool
		Refusal   string
	}
	Redirect string
}

type tokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error"`
}

const callback = "http://127.0.0.1:9/callback"

// register has an app register itself as an MCP client does.
func register(t *testing.T, name, method string) registered {
	t.Helper()
	var out registered
	b := newBrowser(t)
	if status := b.do(http.MethodPost, "/oauth/register", map[string]any{
		"client_name": name, "redirect_uris": []string{callback}, "token_endpoint_auth_method": method,
	}, &out); status != http.StatusCreated {
		t.Fatalf("registering %s answered %d", name, status)
	}
	return out
}

// pkce is a verifier and the S256 challenge made from it.
func pkce() (verifier, challenge string) {
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	verifier = base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:])
}

func authorizeQuery(client, challenge, scope string) string {
	return url.Values{"response_type": {"code"}, "client_id": {client}, "redirect_uri": {callback}, "state": {"s-1"},
		"code_challenge": {challenge}, "code_challenge_method": {"S256"}, "scope": {scope}}.Encode()
}

func consentFor(t *testing.T, b *browser, query string) consentJSON {
	t.Helper()
	var out consentJSON
	if status := b.do(http.MethodGet, "/oauth/consent?"+query, nil, &out); status != http.StatusOK {
		t.Fatalf("the consent page answered %d", status)
	}
	return out
}

// connect has b allow the app on the consent page and returns the code the browser takes back to it.
func connect(t *testing.T, b *browser, query, scope string) string {
	t.Helper()
	var out struct{ Redirect string }
	if status := b.do(http.MethodPost, "/oauth/consent", map[string]any{"query": query, "allow": true, "scope": scope}, &out); status != http.StatusOK {
		t.Fatalf("connecting answered %d", status)
	}
	back, err := url.Parse(out.Redirect)
	if err != nil || !strings.HasPrefix(out.Redirect, callback) || back.Query().Get("state") != "s-1" || back.Query().Get("iss") == "" {
		t.Fatalf("the browser goes back to %q", out.Redirect)
	}
	return back.Query().Get("code")
}

// token posts a form to the token endpoint as an app does.
func token(t *testing.T, form url.Values, basic *registered) (int, tokens) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, inProcessAPI(t)+"/api/v1/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if basic != nil {
		req.SetBasicAuth(url.QueryEscape(basic.ClientID), url.QueryEscape(basic.ClientSecret))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out tokens
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// connected waits for the grants b holds for an app, by its client id, which the token endpoint wrote and b reads once
// the replica has them, and returns their ids.
func connected(t *testing.T, b *browser, client string) []string {
	t.Helper()
	replicated(t)
	deadline := time.Now().Add(5 * time.Second)
	for {
		var apps struct {
			Grants []struct{ ID, ClientID string }
		}
		b.do(http.MethodGet, "/connected-apps", nil, &apps)
		var ids []string
		for _, g := range apps.Grants {
			if g.ClientID == client {
				ids = append(ids, g.ID)
			}
		}
		if len(ids) > 0 || time.Now().After(deadline) {
			return ids
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// bearer calls the API with an access token and says what it answered.
func bearer(t *testing.T, method, path, access string) int {
	t.Helper()
	req, _ := http.NewRequest(method, inProcessAPI(t)+"/api/v1"+path, nil)
	req.Header.Set("Authorization", "Bearer "+access)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode
}

func TestTheAuthorizationServerDescribesItself(t *testing.T) {
	var meta struct {
		Issuer               string
		TokenEndpoint        string   `json:"token_endpoint"`
		CodeChallengeMethods []string `json:"code_challenge_methods_supported"`
	}
	resp, err := http.Get(inProcessAPI(t) + "/.well-known/oauth-authorization-server")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	_ = json.NewDecoder(resp.Body).Decode(&meta)
	if meta.Issuer != inProcessAPI(t) || meta.TokenEndpoint != meta.Issuer+"/api/v1/oauth/token" || len(meta.CodeChallengeMethods) != 1 {
		t.Errorf("%+v", meta)
	}
	var resource struct {
		Resource             string
		AuthorizationServers []string `json:"authorization_servers"`
	}
	resp, err = http.Get(inProcessAPI(t) + "/.well-known/oauth-protected-resource/api/v1/mcp")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	_ = json.NewDecoder(resp.Body).Decode(&resource)
	if resource.Resource != meta.Issuer+"/api/v1/mcp" || len(resource.AuthorizationServers) != 1 {
		t.Errorf("%+v", resource)
	}
	var capabilities struct{ Capabilities []string }
	newBrowser(t).do(http.MethodGet, "/meta", nil, &capabilities)
	if !strings.Contains(strings.Join(capabilities.Capabilities, " "), "oauth.partner-tokens@1") {
		t.Errorf("meta lists %v", capabilities.Capabilities)
	}
}

func TestAnAdministratorAllowsAnAppThenAnybodyConnectsIt(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	member, _ := withRole(t, admin, "member")
	app := register(t, "Assistant", "none")
	verifier, challenge := pkce()
	query := authorizeQuery(app.ClientID, challenge, "read write")

	first := consentFor(t, member, query)
	if first.Consent == nil || !first.Consent.Pending || first.Consent.MayAllow || first.Consent.Refusal == "" {
		t.Fatalf("a member was offered a pending app: %+v", first.Consent)
	}
	if status := member.do(http.MethodPost, "/oauth/consent", map[string]any{"query": query, "allow": true}, nil); status != http.StatusForbidden {
		t.Errorf("a member allowing a pending app answered %d", status)
	}
	if status, out := token(t, url.Values{"grant_type": {"authorization_code"}, "client_id": {app.ClientID}, "code": {"x"},
		"redirect_uri": {callback}, "code_verifier": {verifier}}, nil); status != http.StatusUnauthorized || out.Error != "invalid_client" {
		t.Errorf("a pending app at the token endpoint: %d %+v", status, out)
	}

	asAdmin := consentFor(t, admin, query)
	if asAdmin.Consent == nil || !asAdmin.Consent.AllowsApp || !asAdmin.Consent.MayAllow || asAdmin.Consent.Client.Name != "Assistant" {
		t.Fatalf("an administrator was not offered to allow the app: %+v", asAdmin.Consent)
	}
	connect(t, admin, query, "")

	again := consentFor(t, member, query)
	if again.Consent == nil || again.Consent.Pending || !again.Consent.MayAllow {
		t.Fatalf("the member still cannot connect the app: %+v", again.Consent)
	}
	code := connect(t, member, query, "read")
	if status, out := token(t, url.Values{"grant_type": {"authorization_code"}, "client_id": {app.ClientID}, "code": {code},
		"redirect_uri": {callback}, "code_verifier": {strings.Repeat("x", 43)}}, nil); status != http.StatusBadRequest || out.Error != "invalid_grant" {
		t.Errorf("a wrong verifier: %d %+v", status, out)
	}
	code = connect(t, member, query, "read")
	redeem := url.Values{"grant_type": {"authorization_code"}, "client_id": {app.ClientID}, "code": {code}, "redirect_uri": {callback}, "code_verifier": {verifier}}
	status, granted := token(t, redeem, nil)
	if status != http.StatusOK || granted.Scope != "read" || granted.ExpiresIn != 3600 ||
		!strings.HasPrefix(granted.AccessToken, "bearing_oat_") || !strings.HasPrefix(granted.RefreshToken, "bearing_ort_") {
		t.Fatalf("redeeming the code: %d %+v", status, granted)
	}

	if got := bearer(t, http.MethodGet, "/auth/me", granted.AccessToken); got != http.StatusForbidden {
		t.Errorf("a grant made on the consent page reached the API beyond MCP: %d", got)
	}
	if apps := connected(t, member, app.ClientID); len(apps) != 1 {
		t.Fatalf("the member's connected apps: %+v", apps)
	}

	if status, out := token(t, redeem, nil); status != http.StatusBadRequest || out.Error != "invalid_grant" {
		t.Errorf("a code used twice: %d %+v", status, out)
	}
	if got := bearer(t, http.MethodGet, "/auth/me", granted.AccessToken); got != http.StatusUnauthorized {
		t.Errorf("the grant of a code used twice lives on: %d", got)
	}
}

func TestRefreshTokensRotateAndOneTradedTwiceEndsTheGrant(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	app := register(t, "Rotor", "client_secret_basic")
	verifier, challenge := pkce()
	query := authorizeQuery(app.ClientID, challenge, "")
	code := connect(t, admin, query, "")
	redeem := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {callback}, "code_verifier": {verifier}}

	if status, out := token(t, redeem, &registered{ClientID: app.ClientID, ClientSecret: "bearing_ocs_wrong"}); status != http.StatusUnauthorized || out.Error != "invalid_client" {
		t.Errorf("a wrong secret: %d %+v", status, out)
	}
	status, first := token(t, redeem, &app)
	if status != http.StatusOK || first.Scope != "read write" {
		t.Fatalf("redeeming: %d %+v", status, first)
	}
	status, second := token(t, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {first.RefreshToken}, "scope": {"read"}}, &app)
	if status != http.StatusOK || second.Scope != "read" || second.RefreshToken == first.RefreshToken {
		t.Fatalf("refreshing: %d %+v", status, second)
	}
	if status, out := token(t, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {second.RefreshToken}, "scope": {"read write"}}, &app); status != http.StatusBadRequest || out.Error != "invalid_scope" {
		t.Errorf("widening the scope on refresh: %d %+v", status, out)
	}
	if got := bearer(t, http.MethodGet, "/auth/me", first.AccessToken); got != http.StatusUnauthorized {
		t.Errorf("the access token before the refresh still works: %d", got)
	}
	if status, out := token(t, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {first.RefreshToken}}, &app); status != http.StatusBadRequest || out.Error != "invalid_grant" {
		t.Errorf("a refresh token traded twice: %d %+v", status, out)
	}
	if status, out := token(t, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {second.RefreshToken}}, &app); status != http.StatusBadRequest || out.Error != "invalid_grant" {
		t.Errorf("the grant lives on after its refresh token came back: %d %+v", status, out)
	}
}

func TestAnAppRevokesItsGrantAndPeopleAndAdministratorsDisconnectIt(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	app := register(t, "Notebook", "client_secret_post")
	verifier, challenge := pkce()
	query := authorizeQuery(app.ClientID, challenge, "")
	grant := func() tokens {
		code := connect(t, admin, query, "")
		status, out := token(t, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {callback},
			"code_verifier": {verifier}, "client_id": {app.ClientID}, "client_secret": {app.ClientSecret}}, nil)
		if status != http.StatusOK {
			t.Fatalf("redeeming: %d %+v", status, out)
		}
		return out
	}

	first := grant()
	form := url.Values{"token": {first.RefreshToken}, "client_id": {app.ClientID}, "client_secret": {app.ClientSecret}}
	req, _ := http.NewRequest(http.MethodPost, inProcessAPI(t)+"/api/v1/oauth/revoke", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || bearer(t, http.MethodGet, "/auth/me", first.AccessToken) != http.StatusUnauthorized {
		t.Errorf("revoking answered %d and the access token lives on", resp.StatusCode)
	}

	second := grant()
	mine := connected(t, admin, app.ClientID)
	if status := admin.do(http.MethodDelete, "/connected-apps/"+mine[0], nil, nil); status != http.StatusNoContent {
		t.Fatalf("disconnecting answered %d", status)
	}
	if got := bearer(t, http.MethodGet, "/auth/me", second.AccessToken); got != http.StatusUnauthorized {
		t.Errorf("a disconnected app's token works: %d", got)
	}

	third := grant()
	var clients struct {
		Clients []struct {
			ID, Name, Status string
			People           int
		}
	}
	var listed bool
	for deadline := time.Now().Add(5 * time.Second); !listed && time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		admin.do(http.MethodGet, "/oauth/clients", nil, &clients)
		for _, c := range clients.Clients {
			listed = listed || (c.ID == app.ClientID && c.Status == "allowed" && c.People == 1)
		}
	}
	if !listed {
		t.Fatalf("the administrators' list: %+v", clients)
	}
	member, _ := withRole(t, admin, "member")
	if status := member.do(http.MethodDelete, "/oauth/clients/"+app.ClientID, nil, nil); status != http.StatusForbidden {
		t.Errorf("a member removing an app answered %d", status)
	}
	if status := admin.do(http.MethodDelete, "/oauth/clients/"+app.ClientID, nil, nil); status != http.StatusNoContent {
		t.Fatalf("removing the app answered %d", status)
	}
	if got := bearer(t, http.MethodGet, "/auth/me", third.AccessToken); got != http.StatusUnauthorized {
		t.Errorf("a removed app's token works: %d", got)
	}
}

func TestTheConsentPageNeverSendsTheBrowserToAnAddressNobodyRegistered(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	app := register(t, "Stranger", "none")
	_, challenge := pkce()
	bad := strings.Replace(authorizeQuery(app.ClientID, challenge, ""), url.QueryEscape(callback), url.QueryEscape("https://evil.example/cb"), 1)
	var refusal struct{ Error struct{ Code string } }
	if status := admin.do(http.MethodGet, "/oauth/consent?"+bad, nil, &refusal); status != http.StatusBadRequest || refusal.Error.Code != "unknown_app" {
		t.Errorf("an address nobody registered: %d %+v", status, refusal)
	}
	connect(t, admin, authorizeQuery(app.ClientID, challenge, ""), "")
	noPKCE := strings.Replace(authorizeQuery(app.ClientID, challenge, ""), "code_challenge_method=S256", "code_challenge_method=plain", 1)
	out := consentFor(t, admin, noPKCE)
	if out.Consent != nil || !strings.Contains(out.Redirect, "error=invalid_request") {
		t.Errorf("a request without S256 is answered at the app: %+v", out)
	}
	var cancelled struct{ Redirect string }
	admin.do(http.MethodPost, "/oauth/consent", map[string]any{"query": authorizeQuery(app.ClientID, challenge, ""), "allow": false}, &cancelled)
	if !strings.Contains(cancelled.Redirect, "error=access_denied") {
		t.Errorf("cancelling sends the browser to %q", cancelled.Redirect)
	}
	if status := newBrowser(t).do(http.MethodGet, "/oauth/consent?"+authorizeQuery(app.ClientID, challenge, ""), nil, nil); status != http.StatusUnauthorized {
		t.Errorf("the consent page for nobody signed in answered %d", status)
	}
}
