package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/praetorianer777/bearing/backend/internal/sso"
)

type fakeSSO struct {
	startedWith string
	finishErr   error
}

func (f *fakeSSO) Start(_ context.Context, next string) (string, error) {
	f.startedWith = next
	return "https://id.example/authorize?state=s", nil
}

func (f *fakeSSO) Finish(context.Context, string, string) (sso.Identity, string, error) {
	return sso.Identity{}, "", f.finishErr
}

func TestASignInIsSentToTheProviderAndNeverAway(t *testing.T) {
	s := signedInServer()
	provider := &fakeSSO{}
	s.SSO = provider
	for next, want := range map[string]string{"/people": "/people", "//evil.example": "/", "https://evil.example": "/", "/\\evil": "/", "": "/"} {
		rec := call(s, http.MethodGet, APIPrefix+"/auth/oidc/start?next="+next, "", "", nil)
		if rec.Code != http.StatusFound || rec.Header().Get("Location") != "https://id.example/authorize?state=s" {
			t.Fatalf("%d %s", rec.Code, rec.Header().Get("Location"))
		}
		if provider.startedWith != want {
			t.Errorf("next %q became %q, want %q", next, provider.startedWith, want)
		}
	}
}

func TestWithoutAProviderThereIsNoSingleSignOn(t *testing.T) {
	s := signedInServer()
	s.AppURL = "http://app.test"
	if rec := call(s, http.MethodGet, APIPrefix+"/auth/oidc/start", "", "", nil); rec.Code != http.StatusNotFound {
		t.Errorf("start answered %d", rec.Code)
	}
	if rec := call(s, http.MethodGet, APIPrefix+"/auth/oidc/callback", "", "", nil); rec.Header().Get("Location") != "http://app.test/sign-in?sso=failed" {
		t.Errorf("callback went to %q", rec.Header().Get("Location"))
	}
}

func TestACallbackThatGoesWrongSaysSoOnTheSignInPage(t *testing.T) {
	s := signedInServer()
	s.AppURL = "http://app.test"
	s.SSO = &fakeSSO{finishErr: errors.New("bad code")}
	for query, outcome := range map[string]string{"?error=access_denied": "cancelled", "?state=s&code=c": "failed"} {
		rec := call(s, http.MethodGet, APIPrefix+"/auth/oidc/callback"+query, "", "", nil)
		if rec.Code != http.StatusFound || rec.Header().Get("Location") != "http://app.test/sign-in?sso="+outcome {
			t.Errorf("%s: %d %q", query, rec.Code, rec.Header().Get("Location"))
		}
	}
}

func TestGroupGrantsAreChecked(t *testing.T) {
	rec := call(signedInServer(), http.MethodPut, APIPrefix+"/group-grants", "ada",
		`{"grants":[{"group":"quality","role":"editor"},{"group":"quality","role":"reader"},{"group":" ","role":"boss"}]}`, nil)
	var body errorEnvelope
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	for _, f := range []string{"grants[1].group", "grants[2].group", "grants[2].role"} {
		if body.Error.Fields[f] == "" {
			t.Errorf("no word on %s in %v", f, body.Error.Fields)
		}
	}
	if rec := call(signedInServer(), http.MethodPost, APIPrefix+"/join-requests/nope/admit", "ada", `{"role":"member"}`, nil); rec.Code != http.StatusNotFound {
		t.Errorf("a malformed id answered %d", rec.Code)
	}
}
