package oauth

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestPKCEHoldsTheVerifierToItsChallenge(t *testing.T) {
	verifier := strings.Repeat("v", 43)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	if !VerifyPKCE(challenge, verifier) {
		t.Error("the verifier of the challenge was refused")
	}
	for _, c := range []struct{ challenge, verifier string }{
		{challenge, strings.Repeat("w", 43)},
		{challenge, "short"},
		{"not-a-challenge", verifier},
	} {
		if VerifyPKCE(c.challenge, c.verifier) {
			t.Errorf("%q against %q was taken", c.verifier, c.challenge)
		}
	}
}

func TestAReturnAddressIsHttpsOrALoopbackAtAnyPort(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://app.example/cb":       true,
		"http://127.0.0.1:8080/cb":     true,
		"http://localhost/cb":          true,
		"http://[::1]:9/cb":            true,
		"http://app.example/cb":        false,
		"https://app.example/cb#frag":  false,
		"https://user:pw@app.example/": false,
		"/relative":                    false,
	} {
		if got := validRedirect(raw); got != want {
			t.Errorf("%s: %v", raw, got)
		}
	}
	registered := []string{"http://127.0.0.1:9/callback", "https://app.example/cb"}
	for asked, want := range map[string]bool{
		"https://app.example/cb":           true,
		"http://127.0.0.1:54321/callback":  true,
		"http://127.0.0.1:54321/elsewhere": false,
		"https://app.example/cb?x=1":       false,
		"http://localhost:9/callback":      false,
	} {
		if got := RedirectMatches(registered, asked); got != want {
			t.Errorf("%s: %v", asked, got)
		}
	}
	if redirectHost("https://app.example:8443/cb") != "app.example:8443" {
		t.Error("the host the person is told of")
	}
}

func TestScopesAreReadAndAtMostWhatIsAllowed(t *testing.T) {
	if got, err := asked("", []string{"read", "write"}); err != nil || joinScope(got) != "read write" {
		t.Errorf("nothing asked: %v %v", got, err)
	}
	if got, err := asked("write read read", []string{"read", "write"}); err != nil || joinScope(got) != "read write" {
		t.Errorf("both asked: %v %v", got, err)
	}
	for _, raw := range []string{"write", "read admin"} {
		if _, err := asked(raw, []string{"read", "write"}); err == nil {
			t.Errorf("%q was taken", raw)
		}
	}
	if _, err := asked("read write", []string{"read"}); err == nil {
		t.Error("write was taken from an app that may only read")
	}
	if got, err := normalScopes("write read"); err != nil || joinScope(got) != "read write" {
		t.Errorf("registering both: %v %v", got, err)
	}
	for _, raw := range []string{"write", "read delete"} {
		if _, err := normalScopes(raw); err == nil {
			t.Errorf("registering %q was taken", raw)
		}
	}
}

func TestAnAppSaysWhoItIsOneWay(t *testing.T) {
	basic := "Basic " + base64.StdEncoding.EncodeToString([]byte("app%201:se%3Acret"))
	cred, err := ReadCredentials(basic, url.Values{})
	if err != nil || cred.ID != "app 1" || cred.Secret != "se:cret" || !cred.Basic {
		t.Errorf("basic: %+v %v", cred, err)
	}
	if _, err := ReadCredentials(basic, url.Values{"client_secret": {"x"}}); err == nil {
		t.Error("basic and the form were both taken")
	}
	if _, err := ReadCredentials("Basic !!!", url.Values{}); err == nil {
		t.Error("a broken basic header was taken")
	}
	cred, err = ReadCredentials("", url.Values{"client_id": {"app"}})
	if err != nil || cred.ID != "app" || cred.sentSecret {
		t.Errorf("a public app: %+v %v", cred, err)
	}
	var e *Error
	if _, err := ReadCredentials("", url.Values{}); err == nil || !asError(err, &e) || e.Status != http.StatusUnauthorized {
		t.Errorf("nobody named: %v", err)
	}
}

func asError(err error, into **Error) bool {
	e, ok := err.(*Error)
	*into = e
	return ok
}

func TestTheMetadataNamesEveryAddressUnderTheIssuer(t *testing.T) {
	s := &Service{Issuer: "https://bearing.example/"}
	m := s.Metadata()
	if m.Issuer != "https://bearing.example" || m.TokenEndpoint != "https://bearing.example/api/v1/oauth/token" ||
		m.AuthorizationEndpoint != "https://bearing.example/api/v1/oauth/authorize" {
		t.Errorf("%+v", m)
	}
	r := s.ResourceMetadata()
	if r.Resource != "https://bearing.example/api/v1/mcp" || r.AuthorizationServers[0] != m.Issuer {
		t.Errorf("%+v", r)
	}
	if s.ResourceMetadataURL() != "https://bearing.example/.well-known/oauth-protected-resource/api/v1/mcp" {
		t.Error(s.ResourceMetadataURL())
	}
	if !s.resourceMatches("https://bearing.example/api/v1/mcp/") || s.resourceMatches("https://elsewhere.example/api/v1/mcp") {
		t.Error("the resource")
	}
}

func TestAnAuthorizationAnswersAtTheAppWithItsState(t *testing.T) {
	a := &Authorization{redirectURI: "https://app.example/cb?keep=1", state: "s 1"}
	got, _ := url.Parse(a.errorRedirect("https://bearing.example", refuse(http.StatusBadRequest, "access_denied", "No.")))
	q := got.Query()
	if q.Get("keep") != "1" || q.Get("state") != "s 1" || q.Get("error") != "access_denied" || q.Get("iss") != "https://bearing.example" {
		t.Errorf("%s", got)
	}
	if !strings.Contains(pendingRefusal("Rotor"), "Rotor") || ended().Code != "invalid_grant" || (&RefusedError{Sentence: "x"}).Error() != "x" {
		t.Error("the sentences")
	}
}
