//go:build integration

package test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/db"
)

type personJSON struct {
	ID        string
	Name      string
	Email     *string
	Phone     *string
	Kind      string
	AccountID *string
	Subject   *string
	LeftOn    *string
	HasPhoto  bool
}

type personBody struct{ Person personJSON }

// person adds somebody external and removes them after.
func person(t *testing.T, b *browser, fields map[string]any) personJSON {
	t.Helper()
	var made personBody
	if status := b.do(http.MethodPost, "/people", fields, &made); status != http.StatusCreated {
		t.Fatalf("adding %v answered %d", fields, status)
	}
	super := superuser(t, "BEARING_TEST_SUPERUSER_URL")
	t.Cleanup(func() { _, _ = super.Exec(context.Background(), `DELETE FROM person WHERE id = $1`, made.Person.ID) })
	return made.Person
}

// myEntry waits for the entry a sign-in linked: the link is written as the session starts, and a read right after
// may meet a replica that has not caught up.
func myEntry(t *testing.T, b *browser) personJSON {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var me personBody
		if status := b.do(http.MethodGet, "/people/me", nil, &me); status == http.StatusOK {
			return me.Person
		} else if time.Now().After(deadline) {
			t.Fatalf("no entry is linked: %d", status)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// The case the issue names: somebody entered as external, by address, signs in through the realm later and is
// linked to that entry, which then knows them by the realm's sub.
func TestARealmPersonSigningInLaterIsLinkedToTheirExternalEntry(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	editor, _ := withRole(t, admin, "editor")
	super := superuser(t, "BEARING_TEST_SUPERUSER_URL")
	if _, err := super.Exec(context.Background(), `DELETE FROM person WHERE email = 'alice@suite.test' OR account_id = (SELECT id FROM account WHERE email = 'alice@suite.test')`); err != nil {
		t.Fatal(err)
	}
	entry := person(t, editor, map[string]any{"name": "Alice Example", "email": "alice@suite.test", "location": "Berlin"})
	if entry.Kind != "external" {
		t.Fatalf("entered as %q", entry.Kind)
	}

	alice, _ := ssoSignIn(t, "alice", "alice password")
	me := myEntry(t, alice)
	if me.ID != entry.ID || me.Kind != "realm" || me.Subject == nil || me.AccountID == nil {
		t.Errorf("alice's entry is %+v, want the external one, now known by the realm", me)
	}
	// Signing in again changes nothing: the link holds.
	again, _ := ssoSignIn(t, "alice", "alice password")
	if me := myEntry(t, again); me.ID != entry.ID {
		t.Errorf("a second sign-in linked %s", me.ID)
	}
}

func TestSomebodyKeepsTheFieldsOfTheirEntryAdministratorsAllow(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	editor, _ := withRole(t, admin, "editor")
	email := uniqueEmail("pia")
	entry := person(t, editor, map[string]any{"name": "Pia", "email": email})
	admin.do(http.MethodPost, "/accounts", map[string]string{"name": "Pia", "email": email, "role": "member", "password": "pia long password"}, nil)
	pia := signedIn(t, email, "pia long password")
	if me := myEntry(t, pia); me.ID != entry.ID || me.Kind != "local" {
		t.Fatalf("pia's entry is %+v", me)
	}

	var changed personBody
	if status := pia.do(http.MethodPatch, "/people/me", map[string]any{"phone": "+49 30 1234"}, &changed); status != http.StatusOK || changed.Person.Phone == nil {
		t.Errorf("changing her phone answered %d %+v", status, changed.Person)
	}
	if status := pia.do(http.MethodPatch, "/people/me", map[string]any{"name": "Pia the Great"}, nil); status != http.StatusForbidden {
		t.Errorf("changing her name, which editors keep, answered %d", status)
	}
	var before struct{ Fields []string }
	admin.do(http.MethodGet, "/people/self-editable", nil, &before)
	t.Cleanup(func() { admin.do(http.MethodPut, "/people/self-editable", before, nil) })
	var allowed struct{ Fields []string }
	if status := admin.do(http.MethodPut, "/people/self-editable", map[string]any{"fields": []string{"name", "phone"}}, &allowed); status != http.StatusOK || len(allowed.Fields) != 2 {
		t.Fatalf("allowing the name answered %d %+v", status, allowed)
	}
	if status := pia.do(http.MethodPatch, "/people/me", map[string]any{"name": "Pia Example"}, nil); status != http.StatusOK {
		t.Errorf("changing her name once allowed answered %d", status)
	}
	if status := pia.do(http.MethodPut, "/people/me/photo", map[string]any{"data": tinyPNG}, nil); status != http.StatusForbidden {
		t.Errorf("setting her photo, no longer allowed, answered %d", status)
	}
	if status := admin.do(http.MethodPut, "/people/self-editable", map[string]any{"fields": []string{"salary"}}, nil); status != http.StatusUnprocessableEntity {
		t.Errorf("allowing a field that is none answered %d", status)
	}

	// The database holds the line whatever a handler asks of it.
	cluster, err := db.Open(context.Background(), loadConfig(t).DB, quietLog())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cluster.Close)
	var account struct{ Account struct{ ID string } }
	pia.do(http.MethodGet, "/auth/me", nil, &account)
	_, err = cluster.Write(db.WithUser(context.Background(), uuid.MustParse(account.Account.ID)), func(ctx context.Context, q db.DBTX) error {
		_, err := q.Exec(ctx, `UPDATE person SET left_on = '2000-01-01' WHERE id = $1`, entry.ID)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "kept by editors") {
		t.Errorf("pia marked herself as gone: %v", err)
	}
}

// tinyPNG is a one-pixel picture.
var tinyPNG = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89\x00\x00\x00\rIDATx\x9cc\xf8\xff\xff?\x00\x05\xfe\x02\xfe\xa7\x35\x81\x84\x00\x00\x00\x00IEND\xaeB`\x82")

func TestLeaversKeepTheirPast(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	editor, _ := withRole(t, admin, "editor")
	entry := person(t, editor, map[string]any{"name": "Leo Leaving " + uuid.NewString()[:6]})

	if status := editor.do(http.MethodPost, "/people/"+entry.ID+"/leave", map[string]any{"on": day(0)}, nil); status != http.StatusOK {
		t.Fatalf("leaving answered %d", status)
	}
	listed := func(query string) bool {
		var list struct{ People []personJSON }
		editor.do(http.MethodGet, "/people?q="+strings.ReplaceAll(entry.Name, " ", "+")+query, nil, &list)
		return len(list.People) == 1
	}
	if listed("") {
		t.Error("a leaver is listed today")
	}
	if !listed("&asOf=" + day(-1)) {
		t.Error("a leaver is not listed for yesterday")
	}
	if !listed("&left=true") {
		t.Error("a leaver is not listed among those who left")
	}
	var r refusal
	if status := editor.do(http.MethodPost, "/people/"+entry.ID+"/leave", map[string]any{}, &r); status != http.StatusConflict || r.Error.Code != "left" {
		t.Errorf("leaving twice answered %d %+v", status, r)
	}
	if status := editor.do(http.MethodPost, "/people/"+entry.ID+"/return", nil, nil); status != http.StatusOK || !listed("") {
		t.Errorf("returning answered %d", status)
	}
}

func TestEditorsKeepPeopleAndTheirPhotos(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	editor, _ := withRole(t, admin, "editor")
	member, _ := withRole(t, admin, "member")
	email := uniqueEmail("max")
	entry := person(t, editor, map[string]any{"name": "Max", "email": email, "phone": "123"})

	var r refusal
	if status := editor.do(http.MethodPost, "/people", map[string]any{"name": "Max again", "email": email}, &r); status != http.StatusConflict || r.Error.Code != "email_taken" {
		t.Errorf("a second person with the address answered %d %+v", status, r)
	}
	if status := member.do(http.MethodPost, "/people", map[string]any{"name": "Me"}, nil); status != http.StatusForbidden {
		t.Errorf("a member added a person: %d", status)
	}
	var changed personBody
	if status := editor.do(http.MethodPatch, "/people/"+entry.ID, map[string]any{"phone": ""}, &changed); status != http.StatusOK || changed.Person.Phone != nil {
		t.Errorf("clearing the phone answered %d %+v", status, changed.Person)
	}
	if status := editor.do(http.MethodPut, "/people/"+entry.ID+"/photo", map[string]any{"data": []byte("<html></html>")}, nil); status != http.StatusUnprocessableEntity {
		t.Errorf("a page as a photo answered %d", status)
	}
	if status := editor.do(http.MethodPut, "/people/"+entry.ID+"/photo", map[string]any{"data": tinyPNG}, nil); status != http.StatusNoContent {
		t.Fatalf("setting the photo answered %d", status)
	}
	// The editor's write reaches the member's reads once the replica has it.
	var resp *http.Response
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(100 * time.Millisecond) {
		var err error
		if resp, err = member.client.Get(member.base + "/people/" + entry.ID + "/photo"); err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK || time.Now().After(deadline) {
			break
		}
	}
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "image/png" {
		t.Errorf("the photo answered %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	if status := member.do(http.MethodGet, "/people/me", nil, nil); status != http.StatusNotFound {
		t.Errorf("somebody without an entry asked for theirs: %d", status)
	}
	var account struct{ Account struct{ ID string } }
	member.do(http.MethodGet, "/auth/me", nil, &account)
	if status := admin.do(http.MethodPut, "/people/"+entry.ID+"/account", map[string]any{"accountId": account.Account.ID}, nil); status != http.StatusOK {
		t.Errorf("linking an account answered %d", status)
	}
	if me := myEntry(t, member); me.ID != entry.ID {
		t.Errorf("the linked account's entry is %s", me.ID)
	}
	if status := editor.do(http.MethodDelete, "/people/"+entry.ID+"/photo", nil, nil); status != http.StatusNoContent {
		t.Errorf("taking the photo away answered %d", status)
	}
}
