//go:build integration

package test

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"
)

type searchHit struct {
	Kind, ID, Name, Detail, StepKey string
	Version                         *int
}

type searchBody struct {
	AsOf, Query                                       string
	People, Positions, Units, Roles, Processes, Steps []searchHit
}

func search(t *testing.T, b *browser, query string, extra ...string) searchBody {
	t.Helper()
	v := url.Values{"q": {query}}
	for i := 0; i+1 < len(extra); i += 2 {
		v.Set(extra[i], extra[i+1])
	}
	var out searchBody
	if status := b.do(http.MethodGet, "/search?"+v.Encode(), nil, &out); status != http.StatusOK {
		t.Fatalf("searching %q answered %d", query, status)
	}
	return out
}

func names(hits []searchHit) []string {
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.Name
	}
	return out
}

func TestSearchFindsEveryKindFromTheDemoOrganisation(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)

	finance := search(t, admin, "finance")
	if len(finance.Units) == 0 || finance.Units[0].Name != "Finance and People" {
		t.Errorf("units for finance: %v", names(finance.Units))
	}
	if len(finance.Positions) == 0 || finance.Positions[0].Name != "Head of Finance and People" || finance.Positions[0].Detail != "Finance and People" {
		t.Errorf("positions for finance: %+v", finance.Positions)
	}

	invoice := search(t, admin, "invoice")
	if len(invoice.Processes) == 0 || invoice.Processes[0].Name != "Supplier invoice" {
		t.Errorf("processes for invoice: %v", names(invoice.Processes))
	}
	if len(invoice.Roles) == 0 || invoice.Roles[0].Name != "Invoice approver" {
		t.Errorf("roles for invoice: %v", names(invoice.Roles))
	}
	var step *searchHit
	for i, h := range invoice.Steps {
		if h.Name == "Approve the invoice" {
			step = &invoice.Steps[i]
		}
	}
	if step == nil || step.ID != invoice.Processes[0].ID || step.StepKey == "" || step.Detail != "Supplier invoice" || step.Version == nil {
		t.Errorf("the step approving the invoice: %+v", invoice.Steps)
	}
	if len(invoice.People) != 0 {
		t.Errorf("people for invoice: %v", names(invoice.People))
	}
}

func TestSearchRanksANameAboveWordsElsewhere(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	stamp := fmt.Sprint(time.Now().UnixNano())
	word := "zorblat" + stamp[len(stamp)-6:]
	var described, named struct{ Role struct{ ID string } }
	if status := admin.do(http.MethodPost, "/roles", map[string]any{"key": "described-" + stamp, "name": "Keeper of keys " + stamp,
		"description": "Looks after the " + word + " cupboard."}, &described); status != http.StatusCreated {
		t.Fatalf("making a role answered %d", status)
	}
	t.Cleanup(func() { admin.do(http.MethodDelete, "/roles/"+described.Role.ID, nil, nil) })
	if status := admin.do(http.MethodPost, "/roles", map[string]any{"key": "named-" + stamp, "name": word + " warden"}, &named); status != http.StatusCreated {
		t.Fatalf("making a role answered %d", status)
	}
	t.Cleanup(func() { admin.do(http.MethodDelete, "/roles/"+named.Role.ID, nil, nil) })

	roles := search(t, admin, word).Roles
	if len(roles) != 2 || roles[0].ID != named.Role.ID || roles[1].ID != described.Role.ID {
		t.Errorf("the name first, then the description: %+v", roles)
	}
	if got := search(t, admin, word[:5]).Roles; len(got) < 2 {
		t.Errorf("the start of the word finds both: %v", names(got))
	}
}

func TestSearchIgnoresAccentsAndCaseAndForgivesASlip(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	stamp := fmt.Sprint(time.Now().UnixNano())
	someone := person(t, admin, map[string]any{"name": "Zoë Wójcik-Brçnstedt " + stamp[len(stamp)-4:]})

	for _, typed := range []string{"zoe wojcik", "ZOË WÓJCIK", "Wojcik-Brcnstedt", "brcnsted"} {
		if hits := search(t, admin, typed).People; len(hits) == 0 || hits[0].ID != someone.ID {
			t.Errorf("%q: %v", typed, names(hits))
		}
	}
	if hits := search(t, admin, "Wojcick-Brcnstedt").People; len(hits) == 0 || hits[0].ID != someone.ID {
		t.Errorf("a slip in the name: %v", names(hits))
	}
}

func TestSearchStemsDescriptions(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	stamp := fmt.Sprint(time.Now().UnixNano())
	var made struct{ Role struct{ ID string } }
	if status := admin.do(http.MethodPost, "/roles", map[string]any{"key": "stemmed-" + stamp, "name": "Steward " + stamp,
		"description": "Reconciles the quarterly ledgers and approves the Rechnungen."}, &made); status != http.StatusCreated {
		t.Fatalf("making a role answered %d", status)
	}
	t.Cleanup(func() { admin.do(http.MethodDelete, "/roles/"+made.Role.ID, nil, nil) })
	for _, typed := range []string{"reconciling ledger", "Rechnung"} {
		found := false
		for _, h := range search(t, admin, typed).Roles {
			found = found || h.ID == made.Role.ID
		}
		if !found {
			t.Errorf("%q does not find the steward", typed)
		}
	}
}

func TestSearchKeepsToTheDay(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	stamp := fmt.Sprint(time.Now().UnixNano())
	gone := person(t, admin, map[string]any{"name": "Quillon Departed " + stamp})
	if status := admin.do(http.MethodPost, "/people/"+gone.ID+"/leave", map[string]any{"on": "2020-01-01"}, nil); status != http.StatusOK {
		t.Fatalf("leaving answered %d", status)
	}
	if hits := search(t, admin, "Quillon Departed "+stamp).People; len(hits) != 0 {
		t.Errorf("somebody who left is found today: %v", names(hits))
	}
	if hits := search(t, admin, "Quillon Departed "+stamp, "asOf", "2019-06-01").People; len(hits) != 1 {
		t.Errorf("somebody who left is not found before: %v", names(hits))
	}
}

func TestQuickSearchGivesTheBestFewOfAnyKind(t *testing.T) {
	reader, _ := withRole(t, signedIn(t, adminEmail, adminPassword), "reader")
	var out struct {
		Query string
		Hits  []searchHit
	}
	if status := reader.do(http.MethodGet, "/search/quick?q=supplier+inv", nil, &out); status != http.StatusOK {
		t.Fatalf("quick search answered %d", status)
	}
	if len(out.Hits) == 0 || len(out.Hits) > 8 || out.Hits[0].Kind != "process" || out.Hits[0].Name != "Supplier invoice" {
		t.Errorf("quick hits: %+v", out.Hits)
	}
	if status := reader.do(http.MethodGet, "/search/quick?q=in&limit=3", nil, &out); status != http.StatusOK || len(out.Hits) != 3 {
		t.Errorf("three of them: %d %d", status, len(out.Hits))
	}
}

func TestSearchRefusesWhatItCannotSearch(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	for _, query := range []string{"q=a", "q=%21%21%21", "q=finance&limit=0", "q=finance&limit=51", "q=finance&asOf=soon"} {
		if status := admin.do(http.MethodGet, "/search?"+query, nil, nil); status != http.StatusBadRequest && status != http.StatusUnprocessableEntity {
			t.Errorf("%s answered %d", query, status)
		}
	}
	if status := newBrowser(t).do(http.MethodGet, "/search?q=finance", nil, nil); status != http.StatusUnauthorized {
		t.Errorf("signed out: %d", status)
	}
}
