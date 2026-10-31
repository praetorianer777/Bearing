//go:build integration

package test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/praetorianer777/bearing/backend/internal/app"
	"github.com/praetorianer777/bearing/backend/internal/auth"
	"github.com/praetorianer777/bearing/backend/internal/db"
	"github.com/praetorianer777/bearing/backend/migrations"
)

// The first administrator the stack's seed sets up.
const (
	adminEmail    = "admin@bearing.test"
	adminPassword = "bearing admin password"
)

// browser is a client with a cookie jar, as a person's browser is.
type browser struct {
	t      *testing.T
	client *http.Client
	base   string
}

// newBrowser talks to an api built in this process from this checkout's code, against the stack's database, Valkey
// and Mailpit, so what it tests is the code beside it and what it reaches counts towards coverage.
func newBrowser(t *testing.T) *browser {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	return &browser{t: t, client: &http.Client{Jar: jar}, base: inProcessAPI(t) + "/api/v1"}
}

var (
	apiOnce sync.Once
	apiURL  string
)

func inProcessAPI(t *testing.T) string {
	t.Helper()
	apiOnce.Do(func() {
		cfg := loadConfig(t)
		cluster, err := db.Open(context.Background(), cfg.DB, quietLog())
		if err != nil {
			t.Fatal(err)
		}
		// The server learns its own address only once it listens, and the provider sends browsers back to it.
		var handler http.Handler
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { handler.ServeHTTP(w, r) }))
		cfg.Auth.AppURL = srv.URL
		api, err := app.NewServer(cfg, cluster, valkeyFor(t, cfg), quietLog())
		if err != nil {
			t.Fatal(err)
		}
		handler = api.Routes()
		apiURL = srv.URL
	})
	return apiURL
}

func (b *browser) do(method, path string, body any, into any) int {
	b.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, b.base+path, reader)
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(req)
	if err != nil {
		b.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if into != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, into); err != nil {
			b.t.Fatalf("%s %s answered %d %s: %v", method, path, resp.StatusCode, raw, err)
		}
	}
	return resp.StatusCode
}

type accountBody struct {
	Account struct {
		ID       string
		Email    string
		Name     string
		Role     string
		Disabled bool
	}
}

func signedIn(t *testing.T, email, password string) *browser {
	t.Helper()
	b := newBrowser(t)
	if status := b.do(http.MethodPost, "/auth/sign-in", map[string]string{"email": email, "password": password}, nil); status != http.StatusOK {
		t.Fatalf("signing in as %s answered %d", email, status)
	}
	return b
}

func uniqueEmail(name string) string {
	return fmt.Sprintf("%s-%d@bearing.test", name, time.Now().UnixNano())
}

// mailTo waits for the newest mail to an address in Mailpit and returns its text.
func mailTo(t *testing.T, address string) string {
	t.Helper()
	base := env(t, "BEARING_TEST_MAILPIT_URL")
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(base + "/api/v1/search?query=" + url.QueryEscape("to:"+address))
		if err != nil {
			t.Fatal(err)
		}
		var found struct{ Messages []struct{ ID string } }
		_ = json.NewDecoder(resp.Body).Decode(&found)
		resp.Body.Close()
		if len(found.Messages) > 0 {
			resp, err := http.Get(base + "/api/v1/message/" + found.Messages[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			var msg struct{ Text string }
			_ = json.NewDecoder(resp.Body).Decode(&msg)
			resp.Body.Close()
			return msg.Text
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("no mail reached %s", address)
	return ""
}

var linkToken = regexp.MustCompile(`/(?:invitations|reset-password)/([A-Za-z0-9_-]+)`)

func tokenIn(t *testing.T, text string) string {
	t.Helper()
	m := linkToken.FindStringSubmatch(text)
	if m == nil {
		t.Fatalf("no link in %q", text)
	}
	return m[1]
}

func TestTheSeedSetUpTheInstallation(t *testing.T) {
	var in struct {
		Name  string
		SetUp bool
		Mail  bool
	}
	if status := newBrowser(t).do(http.MethodGet, "/installation", nil, &in); status != http.StatusOK || !in.SetUp || in.Name == "" || !in.Mail {
		t.Errorf("%d %+v", status, in)
	}
	if status := newBrowser(t).do(http.MethodPost, "/installation/setup", map[string]string{
		"name": "Another", "adminName": "Eve", "email": uniqueEmail("eve"), "password": "a long enough password",
	}, nil); status != http.StatusConflict {
		t.Errorf("setting up again answered %d", status)
	}
}

func TestSigningInAndOut(t *testing.T) {
	b := signedIn(t, adminEmail, adminPassword)
	var me accountBody
	if status := b.do(http.MethodGet, "/auth/me", nil, &me); status != http.StatusOK || me.Account.Role != "administrator" {
		t.Fatalf("%d %+v", status, me)
	}
	if status := b.do(http.MethodPost, "/auth/sign-out", nil, nil); status != http.StatusNoContent {
		t.Fatalf("signing out answered %d", status)
	}
	if status := b.do(http.MethodGet, "/auth/me", nil, nil); status != http.StatusUnauthorized {
		t.Errorf("after signing out me answered %d", status)
	}
	var refused struct{ Error struct{ Code string } }
	if status := newBrowser(t).do(http.MethodPost, "/auth/sign-in", map[string]string{"email": adminEmail, "password": "not the password"}, &refused); status != http.StatusUnauthorized || refused.Error.Code != "bad_credentials" {
		t.Errorf("a wrong password answered %d %+v", status, refused)
	}
	if status := newBrowser(t).do(http.MethodPost, "/auth/sign-in", map[string]string{"email": uniqueEmail("nobody"), "password": "whatever it is"}, &refused); status != http.StatusUnauthorized || refused.Error.Code != "bad_credentials" {
		t.Errorf("an unknown address answered %d %+v", status, refused)
	}
}

func TestAnInvitationBringsSomebodyIn(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	email := uniqueEmail("ines")
	var invited struct {
		Invitation struct{ ID, Role string }
		Link       string
	}
	if status := admin.do(http.MethodPost, "/invitations", map[string]string{"email": email, "role": "editor"}, &invited); status != http.StatusCreated {
		t.Fatalf("inviting answered %d", status)
	}
	mailed := mailTo(t, email)
	if !strings.Contains(mailed, invited.Link) {
		t.Errorf("the mail does not carry the link %s:\n%s", invited.Link, mailed)
	}
	token := tokenIn(t, mailed)

	ines := newBrowser(t)
	var link struct{ Email, Role string }
	if status := ines.do(http.MethodGet, "/invitation-links/"+token, nil, &link); status != http.StatusOK || link.Email != email || link.Role != "editor" {
		t.Fatalf("%d %+v", status, link)
	}
	var me accountBody
	if status := ines.do(http.MethodPost, "/invitation-links/"+token, map[string]string{"name": "Ines", "password": "ines long password"}, &me); status != http.StatusCreated || me.Account.Role != "editor" {
		t.Fatalf("accepting answered %d %+v", status, me)
	}
	if status := ines.do(http.MethodGet, "/auth/me", nil, &me); status != http.StatusOK || me.Account.Email != email {
		t.Errorf("the new account is not signed in: %d", status)
	}
	if status := newBrowser(t).do(http.MethodPost, "/invitation-links/"+token, map[string]string{"name": "Again", "password": "ines long password"}, nil); status != http.StatusGone {
		t.Errorf("the link worked twice: %d", status)
	}
	if status := ines.do(http.MethodGet, "/accounts", nil, nil); status != http.StatusForbidden {
		t.Errorf("an editor listed the accounts: %d", status)
	}
	signedIn(t, email, "ines long password")
}

func TestARevokedInvitationNoLongerWorks(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	var invited struct {
		Invitation struct{ ID string }
		Link       string
	}
	admin.do(http.MethodPost, "/invitations", map[string]string{"email": uniqueEmail("rex"), "role": "reader"}, &invited)
	if status := admin.do(http.MethodDelete, "/invitations/"+invited.Invitation.ID, nil, nil); status != http.StatusNoContent {
		t.Fatalf("revoking answered %d", status)
	}
	token := invited.Link[strings.LastIndex(invited.Link, "/")+1:]
	if status := newBrowser(t).do(http.MethodGet, "/invitation-links/"+token, nil, nil); status != http.StatusGone {
		t.Errorf("a revoked link answered %d", status)
	}
}

func TestAnAdministratorManagesAccounts(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	email := uniqueEmail("max")
	var made accountBody
	if status := admin.do(http.MethodPost, "/accounts", map[string]string{"name": "Max", "email": email, "role": "member", "password": "max first password"}, &made); status != http.StatusCreated {
		t.Fatalf("making an account answered %d", status)
	}
	if status := admin.do(http.MethodPost, "/accounts", map[string]string{"name": "Max", "email": email, "role": "member", "password": "max first password"}, nil); status != http.StatusConflict {
		t.Errorf("a second account with the address answered %d", status)
	}
	max := signedIn(t, email, "max first password")

	if status := admin.do(http.MethodPost, "/accounts/"+made.Account.ID+"/disable", nil, nil); status != http.StatusNoContent {
		t.Fatalf("switching off answered %d", status)
	}
	if status := max.do(http.MethodGet, "/auth/me", nil, nil); status != http.StatusUnauthorized {
		t.Errorf("a switched off account is still signed in: %d", status)
	}
	if status := newBrowser(t).do(http.MethodPost, "/auth/sign-in", map[string]string{"email": email, "password": "max first password"}, nil); status != http.StatusForbidden {
		t.Errorf("a switched off account signed in: %d", status)
	}
	admin.do(http.MethodPost, "/accounts/"+made.Account.ID+"/enable", nil, nil)
	if status := admin.do(http.MethodPost, "/accounts/"+made.Account.ID+"/password", map[string]string{"password": "max second password"}, nil); status != http.StatusNoContent {
		t.Fatalf("setting a password answered %d", status)
	}
	signedIn(t, email, "max second password")

	var list struct{ Accounts []struct{ Email string } }
	admin.do(http.MethodGet, "/accounts", nil, &list)
	found := false
	for _, a := range list.Accounts {
		found = found || a.Email == email
	}
	if !found {
		t.Errorf("the list does not hold %s", email)
	}

	var me accountBody
	admin.do(http.MethodGet, "/auth/me", nil, &me)
	if status := admin.do(http.MethodPost, "/accounts/"+me.Account.ID+"/disable", nil, nil); status != http.StatusConflict {
		t.Errorf("an administrator switched themselves off: %d", status)
	}
}

func TestAForgottenPasswordIsResetByMail(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	email := uniqueEmail("rosa")
	admin.do(http.MethodPost, "/accounts", map[string]string{"name": "Rosa", "email": email, "role": "member", "password": "rosa old password"}, nil)
	rosa := signedIn(t, email, "rosa old password")

	if status := newBrowser(t).do(http.MethodPost, "/auth/password-reset", map[string]string{"email": email}, nil); status != http.StatusAccepted {
		t.Fatalf("asking answered %d", status)
	}
	if status := newBrowser(t).do(http.MethodPost, "/auth/password-reset", map[string]string{"email": uniqueEmail("nobody")}, nil); status != http.StatusAccepted {
		t.Errorf("an unknown address answered %d, which says it has no account", status)
	}
	token := tokenIn(t, mailTo(t, email))
	if status := newBrowser(t).do(http.MethodPost, "/auth/password-reset/"+token, map[string]string{"password": "rosa new password"}, nil); status != http.StatusNoContent {
		t.Fatalf("resetting answered %d", status)
	}
	if status := rosa.do(http.MethodGet, "/auth/me", nil, nil); status != http.StatusUnauthorized {
		t.Errorf("the old session outlived the new password: %d", status)
	}
	signedIn(t, email, "rosa new password")
	if status := newBrowser(t).do(http.MethodPost, "/auth/password-reset/"+token, map[string]string{"password": "rosa third password"}, nil); status != http.StatusGone {
		t.Errorf("the reset link worked twice: %d", status)
	}
}

// The database holds the rules itself: a member's write to an account is refused, and nobody reads a hash.
func TestTheDatabaseKeepsAccountsToTheirRules(t *testing.T) {
	ctx := context.Background()
	cluster, err := db.Open(ctx, loadConfig(t).DB, quietLog())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cluster.Close)
	admin := signedIn(t, adminEmail, adminPassword)
	var member accountBody
	admin.do(http.MethodPost, "/accounts", map[string]string{"name": "Mia", "email": uniqueEmail("mia"), "role": "member", "password": "mia long password"}, &member)

	id := uuid.MustParse(member.Account.ID)
	asMember := db.WithUser(ctx, id)
	_, err = cluster.Write(asMember, func(ctx context.Context, q db.DBTX) error {
		tag, err := q.Exec(ctx, `UPDATE account SET role = 'administrator' WHERE id = $1`, id)
		if err == nil && tag.RowsAffected() != 0 {
			return fmt.Errorf("a member made themselves an administrator")
		}
		return err
	})
	if pgErr := (*pgconn.PgError)(nil); err != nil && (!errors.As(err, &pgErr) || pgErr.Code != "42501") {
		t.Error(err)
	}
	err = cluster.Read(asMember, func(ctx context.Context, q db.DBTX) error {
		var hash string
		return q.QueryRow(ctx, `SELECT password_hash FROM account LIMIT 1`).Scan(&hash)
	})
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("the app role read a password hash: %v", err)
	}
	var n int
	_ = cluster.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		return q.QueryRow(ctx, `SELECT count(*) FROM account`).Scan(&n)
	})
	if n != 0 {
		t.Errorf("a transaction acting for nobody saw %d accounts", n)
	}
}

func TestAWriteFromAnotherSiteIsRefused(t *testing.T) {
	b := signedIn(t, adminEmail, adminPassword)
	req, _ := http.NewRequest(http.MethodPost, b.base+"/invitations", strings.NewReader(`{"email":"x@y.test","role":"reader"}`))
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	resp, err := b.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("a cross-site write answered %d", resp.StatusCode)
	}
}

// freshCluster is a database of its own, made and migrated for the test and dropped after, for what needs an
// installation the stack's is not: one not set up yet, or one without an organisation.
func freshCluster(t *testing.T, prefix string) *db.Cluster {
	t.Helper()
	ctx := context.Background()
	super := superuser(t, "BEARING_TEST_SUPERUSER_URL")
	name := fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	if _, err := super.Exec(ctx, `CREATE DATABASE `+name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = super.Exec(context.Background(), `DROP DATABASE `+name+` WITH (FORCE)`) })

	owner, err := pgxpool.New(ctx, swapDatabase(t, env(t, "BEARING_TEST_SUPERUSER_URL"), name))
	if err != nil {
		t.Fatal(err)
	}
	sqlDB := stdlib.OpenDBFromPool(owner)
	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpContext(ctx, sqlDB, "."); err != nil {
		t.Fatal(err)
	}
	sqlDB.Close()
	owner.Close()

	cfg := loadConfig(t).DB
	cfg.PrimaryURL = swapDatabase(t, cfg.PrimaryURL, name)
	cfg.AdminURL = swapDatabase(t, cfg.AdminURL, name)
	cfg.ReplicaURLs = nil
	cluster, err := db.Open(ctx, cfg, quietLog())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cluster.Close)
	return cluster
}

// Setting up runs on a database of its own, since the stack's is set up already.
func TestSettingUpHappensOnce(t *testing.T) {
	ctx := context.Background()
	cluster := freshCluster(t, "bearing_setup")
	accounts := &auth.Service{DB: cluster, Params: auth.DefaultPasswordParams()}

	if in, err := accounts.Installation(ctx); err != nil || in.SetUp {
		t.Fatalf("a fresh database is set up: %+v %v", in, err)
	}
	if _, err := accounts.SetUp(ctx, "Org", "Ada", "ada@bearing.test", "short"); err == nil {
		t.Error("a short password set the installation up")
	}
	var wg sync.WaitGroup
	results := make([]error, 4)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, results[i] = accounts.SetUp(ctx, "Org", "Ada", fmt.Sprintf("ada%d@bearing.test", i), "ada long password")
		}()
	}
	wg.Wait()
	won := 0
	for _, err := range results {
		switch {
		case err == nil:
			won++
		case !errors.Is(err, auth.ErrAlreadySetUp):
			t.Errorf("a concurrent setup failed otherwise: %v", err)
		}
	}
	if won != 1 {
		t.Errorf("%d setups succeeded at once, want 1", won)
	}
	if in, _ := accounts.Installation(ctx); !in.SetUp || in.Name != "Org" {
		t.Errorf("after setting up: %+v", in)
	}
}

func swapDatabase(t *testing.T, raw, name string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	return u.String()
}
