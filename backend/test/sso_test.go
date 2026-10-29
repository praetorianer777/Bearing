//go:build integration

package test

import (
	"context"
	"errors"
	"html"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/praetorianer777/bearing/backend/internal/sso"
)

var loginForm = regexp.MustCompile(`<form[^>]+id="kc-form-login"[^>]+action="([^"]+)"`)

// errStop ends a redirect chain at the web client, where a browser would show a page.
var errStop = errors.New("stop")

// ssoSignIn signs a realm user in through Keycloak's own login form, as a browser does: start at the api, post the
// form, and follow the provider back. It returns the browser and where the api finally sent it in the web client.
func ssoSignIn(t *testing.T, username, password string) (*browser, *url.URL) {
	t.Helper()
	b := newBrowser(t)
	backchannel, err := sso.ParseBackchannel(env(t, "BEARING_OIDC_BACKCHANNEL"))
	if err != nil {
		t.Fatal(err)
	}
	app, _ := url.Parse(inProcessAPI(t))
	jar, _ := cookiejar.New(nil)
	b.client = &http.Client{
		Jar:       jar,
		Transport: sso.Backchannel(backchannel).Transport,
		CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			if req.URL.Host == app.Host && !strings.HasPrefix(req.URL.Path, "/api/") {
				return errStop
			}
			return nil
		},
	}
	resp, err := b.client.Get(b.base + "/auth/oidc/start?next=/people")
	if err != nil {
		t.Fatal(err)
	}
	page := readAll(t, resp)
	m := loginForm.FindStringSubmatch(page)
	if m == nil {
		t.Fatalf("no login form at the provider:\n%.500s", page)
	}
	resp, err = b.client.PostForm(html.UnescapeString(m[1]), url.Values{"username": {username}, "password": {password}, "credentialId": {""}})
	if err != nil && !errors.Is(err, errStop) {
		t.Fatal(err)
	}
	if resp == nil || resp.StatusCode != http.StatusFound {
		t.Fatalf("the provider did not send the browser back: %v", resp)
	}
	resp.Body.Close()
	landed, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	return b, landed
}

func readAll(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	var sb strings.Builder
	buf := make([]byte, 32<<10)
	for {
		n, err := resp.Body.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			return sb.String()
		}
	}
}

// forget removes what earlier runs left of a realm user here, so each run meets them for the first time.
func forget(t *testing.T, email string) {
	t.Helper()
	super := superuser(t, "BEARING_TEST_SUPERUSER_URL")
	ctx := context.Background()
	for _, stmt := range []string{`DELETE FROM join_request WHERE email = $1`, `DELETE FROM account WHERE email = $1`} {
		if _, err := super.Exec(ctx, stmt, email); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAnAccountMadeAheadIsLinkedAtTheFirstSignIn(t *testing.T) {
	b, landed := ssoSignIn(t, "alice", "alice password")
	if landed.Path != "/people" {
		t.Fatalf("alice landed on %s, want where she was headed", landed)
	}
	var me struct {
		Account struct {
			Email, Role string
			Groups      []string
		}
	}
	if status := b.do(http.MethodGet, "/auth/me", nil, &me); status != http.StatusOK {
		t.Fatalf("me answered %d", status)
	}
	if me.Account.Email != "alice@suite.test" || me.Account.Role != "administrator" || !slices.Contains(me.Account.Groups, "bearing-administrators") {
		t.Errorf("alice is %+v", me.Account)
	}
	// The second time she is known by her identity, not her address.
	if _, landed := ssoSignIn(t, "alice", "alice password"); landed.Path != "/people" {
		t.Errorf("alice's second sign-in landed on %s", landed)
	}
}

func TestSomebodyNobodyLetInAsksToJoinAndIsLetIn(t *testing.T) {
	forget(t, "carol@suite.test")
	_, landed := ssoSignIn(t, "carol", "carol password")
	if landed.Path != "/sign-in" || landed.Query().Get("sso") != "pending" {
		t.Fatalf("carol landed on %s, want the sign-in page saying she asked", landed)
	}
	admin := signedIn(t, adminEmail, adminPassword)
	// Carol's request is another person's write, which reaches the administrator's replica a moment later.
	var id string
	var groups []string
	for range 50 {
		var requests struct {
			JoinRequests []struct {
				ID, Email string
				Groups    []string
			}
		}
		admin.do(http.MethodGet, "/join-requests", nil, &requests)
		for _, r := range requests.JoinRequests {
			if r.Email == "carol@suite.test" {
				id, groups = r.ID, r.Groups
			}
		}
		if id != "" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if id == "" {
		t.Fatal("carol's request is not listed")
	}
	if !slices.Contains(groups, "quality") {
		t.Errorf("carol's request lists the groups %v", groups)
	}
	if status := admin.do(http.MethodPost, "/join-requests/"+id+"/admit", map[string]string{"role": "editor"}, nil); status != http.StatusCreated {
		t.Fatalf("admitting answered %d", status)
	}
	b, landed := ssoSignIn(t, "carol", "carol password")
	if landed.Path != "/people" {
		t.Fatalf("carol, let in, landed on %s", landed)
	}
	var me accountBody
	if b.do(http.MethodGet, "/auth/me", nil, &me); me.Account.Role != "editor" {
		t.Errorf("carol came in as %q", me.Account.Role)
	}
}

func TestAGroupGrantLetsItsMembersInAtOnce(t *testing.T) {
	forget(t, "dave@suite.test")
	admin := signedIn(t, adminEmail, adminPassword)
	var before groupGrants
	admin.do(http.MethodGet, "/group-grants", nil, &before)
	t.Cleanup(func() { admin.do(http.MethodPut, "/group-grants", before, nil) })
	if status := admin.do(http.MethodPut, "/group-grants", groupGrants{Grants: []grant{{"marketing", "reader"}, {"readers", "member"}}}, nil); status != http.StatusOK {
		t.Fatalf("setting grants answered %d", status)
	}
	b, landed := ssoSignIn(t, "dave", "dave password")
	if landed.Path != "/people" {
		t.Fatalf("dave landed on %s", landed)
	}
	var me accountBody
	if b.do(http.MethodGet, "/auth/me", nil, &me); me.Account.Role != "member" {
		t.Errorf("dave, in marketing and readers, came in as %q, want the higher role", me.Account.Role)
	}
}

func TestASwitchedOffAccountCannotSignInThroughTheProvider(t *testing.T) {
	ssoSignIn(t, "bob", "bob password")
	admin := signedIn(t, adminEmail, adminPassword)
	var list struct{ Accounts []struct{ ID, Email string } }
	admin.do(http.MethodGet, "/accounts", nil, &list)
	var id string
	for _, a := range list.Accounts {
		if a.Email == "bob@suite.test" {
			id = a.ID
		}
	}
	admin.do(http.MethodPost, "/accounts/"+id+"/disable", nil, nil)
	t.Cleanup(func() { admin.do(http.MethodPost, "/accounts/"+id+"/enable", nil, nil) })
	if _, landed := ssoSignIn(t, "bob", "bob password"); landed.Query().Get("sso") != "disabled" {
		t.Errorf("switched off, bob landed on %s", landed)
	}
}

type grant struct {
	Group string `json:"group"`
	Role  string `json:"role"`
}
type groupGrants struct {
	Grants []grant `json:"grants"`
}
