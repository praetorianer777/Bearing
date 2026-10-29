package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/auth"
	"github.com/praetorianer777/bearing/backend/internal/db"
)

type fakeSessions map[string]auth.Account

func (f fakeSessions) Authenticate(_ context.Context, token string) (auth.Account, string, error) {
	if token == "broken" {
		return auth.Account{}, "", errors.New("valkey is down")
	}
	a, ok := f[token]
	if !ok {
		return auth.Account{}, "", auth.ErrNoSession
	}
	return a, "key-" + token, nil
}

var (
	ada  = auth.Account{ID: uuid.New(), Email: "ada@bearing.test", Name: "Ada", Role: auth.RoleAdministrator}
	bert = auth.Account{ID: uuid.New(), Email: "bert@bearing.test", Name: "Bert", Role: auth.RoleMember}
)

func signedInServer() *Server {
	s := newServer(fakeDB{})
	s.Accounts = fakeSessions{"ada": ada, "bert": bert}
	return s
}

func call(s *Server, method, path, cookie string, body string, headers map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if cookie != "" {
		r.AddCookie(&http.Cookie{Name: s.cookieName(), Value: cookie})
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, r)
	return rec
}

func code(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body errorEnvelope
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return body.Error.Code
}

func TestOperationsNeedSomebodySignedInUnlessPublic(t *testing.T) {
	s := signedInServer()
	if rec := call(s, http.MethodGet, APIPrefix+"/accounts", "", "", nil); rec.Code != http.StatusUnauthorized || code(t, rec) != "unauthorized" {
		t.Errorf("nobody: %d %s", rec.Code, rec.Body)
	}
	if rec := call(s, http.MethodGet, APIPrefix+"/accounts", "bert", "", nil); rec.Code != http.StatusForbidden || code(t, rec) != "forbidden" {
		t.Errorf("a member: %d %s", rec.Code, rec.Body)
	}
	if rec := call(s, http.MethodGet, APIPrefix+"/meta", "", "", nil); rec.Code != http.StatusOK {
		t.Errorf("public: %d", rec.Code)
	}
}

func TestMeSaysWhoIsSignedIn(t *testing.T) {
	rec := call(signedInServer(), http.MethodGet, APIPrefix+"/auth/me", "bert", "", nil)
	var body struct{ Account accountResponse }
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusOK || body.Account.Email != bert.Email || body.Account.Role != auth.RoleMember {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
}

func TestADeadSessionCookieIsDropped(t *testing.T) {
	rec := call(signedInServer(), http.MethodGet, APIPrefix+"/auth/me", "gone", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("%d", rec.Code)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].MaxAge >= 0 {
		t.Errorf("the dead cookie was kept: %v", cookies)
	}
}

func TestASessionThatCannotBeCheckedIsUnavailable(t *testing.T) {
	rec := call(signedInServer(), http.MethodGet, APIPrefix+"/auth/me", "broken", "", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
}

func TestTheSessionActsForItsAccountAndKeepsItsWrites(t *testing.T) {
	s := signedInServer()
	var acting uuid.UUID
	var key string
	h := s.identify(s.readYourWrites(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		acting, _ = db.UserFrom(r.Context())
		key = s.freshnessKey(w, r)
	})))
	s.Fresh = memFresh{}
	h = s.identify(s.readYourWrites(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		acting, _ = db.UserFrom(r.Context())
		key = s.freshnessKey(w, r)
	})))
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.AddCookie(&http.Cookie{Name: s.cookieName(), Value: "ada"})
	h.ServeHTTP(httptest.NewRecorder(), r)
	if acting != ada.ID || key != "s:key-ada" {
		t.Errorf("acting %v, freshness key %q", acting, key)
	}
}

func TestAWriteFromAnotherSiteIsRefused(t *testing.T) {
	s := signedInServer()
	for _, c := range []struct {
		name    string
		headers map[string]string
		refused bool
	}{
		{"cross-site", map[string]string{"Sec-Fetch-Site": "cross-site"}, true},
		{"same-site but another origin", map[string]string{"Sec-Fetch-Site": "same-site"}, true},
		{"same origin", map[string]string{"Sec-Fetch-Site": "same-origin"}, false},
		{"typed by hand", map[string]string{"Sec-Fetch-Site": "none"}, false},
		{"an older browser elsewhere", map[string]string{"Origin": "https://evil.example"}, true},
		{"an older browser here", map[string]string{"Origin": "http://example.com"}, false},
		{"a script", nil, false},
	} {
		rec := call(s, http.MethodPost, APIPrefix+"/auth/sign-out", "ada", "", c.headers)
		if refused := code(t, rec) == "cross_site"; refused != c.refused {
			t.Errorf("%s: %d %s", c.name, rec.Code, rec.Body)
		}
	}
	if rec := call(s, http.MethodGet, APIPrefix+"/auth/me", "ada", "", map[string]string{"Sec-Fetch-Site": "cross-site"}); rec.Code != http.StatusOK {
		t.Errorf("a read from elsewhere was refused: %d", rec.Code)
	}
}

func TestRequestsAreCheckedBeforeAnythingIsDone(t *testing.T) {
	s := signedInServer()
	for _, c := range []struct {
		path, body string
		fields     []string
	}{
		{"/installation/setup", `{"name":" ","adminName":"","email":"nope","password":"short"}`, []string{"name", "adminName", "email", "password"}},
		{"/accounts", `{"name":"","email":"x@y","role":"boss","password":"short"}`, []string{"name", "role", "password"}},
		{"/invitations", `{"email":"","role":"reader"}`, []string{"email"}},
		{"/invitation-links/abc", `{"name":"","password":""}`, []string{"name", "password"}},
	} {
		rec := call(s, http.MethodPost, APIPrefix+c.path, "ada", c.body, nil)
		var body errorEnvelope
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: %d %s", c.path, rec.Code, rec.Body)
			continue
		}
		for _, f := range c.fields {
			if body.Error.Fields[f] == "" {
				t.Errorf("%s: no word on %s in %v", c.path, f, body.Error.Fields)
			}
		}
	}
	if rec := call(s, http.MethodPost, APIPrefix+"/auth/sign-in", "", `{"email":"a","pasword":"x"}`, nil); rec.Code != http.StatusBadRequest {
		t.Errorf("a misspelt field was taken: %d", rec.Code)
	}
}

func TestAuthRefusalsSayWhatToDo(t *testing.T) {
	for err, want := range map[error]int{
		auth.ErrAlreadySetUp:     http.StatusConflict,
		auth.ErrBadCredentials:   http.StatusUnauthorized,
		auth.ErrDisabled:         http.StatusForbidden,
		auth.ErrEmailTaken:       http.StatusConflict,
		auth.ErrLinkInvalid:      http.StatusGone,
		auth.ErrNotFound:         http.StatusNotFound,
		auth.ErrSelf:             http.StatusConflict,
		auth.ErrNoPasswordToMail: http.StatusConflict,
		auth.ErrPasswordTooShort: http.StatusUnprocessableEntity,
	} {
		var apiErr *APIError
		if !errors.As(authError(err), &apiErr) || apiErr.Status != want || apiErr.Message == "" {
			t.Errorf("%v became %v, want status %d", err, authError(err), want)
		}
	}
	other := errors.New("boom")
	if authError(other) != other {
		t.Error("an unknown error was dressed up")
	}
}
