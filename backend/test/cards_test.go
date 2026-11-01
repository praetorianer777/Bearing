//go:build integration

package test

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
)

type cardJSON struct {
	Kind, ID, Name, Detail, URL string
	PhotoURL                    *string
	Subject                     *string
}

// A partner shows chips for what Bearing knows: a person, the position they hold, its unit and the role it carries,
// each with a line beneath its name and a link back.
func TestAPartnerShowsCardsAndFindsThemByTheWordsTyped(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	editor, _ := withRole(t, admin, "editor")
	partner, _ := withRole(t, admin, "member")
	word := "Zephyr" + strings.ToLower(uuid.NewString()[:5])
	made := units(t, editor, unitType(t, admin), rootUnit(t, admin), word+" Division", word+" Team")
	division, team := made[0], made[1]
	position := positions(t, editor, team, word+" Analyst")[0]
	person := person(t, editor, map[string]any{"name": word + " Person"}).ID
	give(t, editor, map[string]any{"personId": person, "positionId": position, "from": day(-5)})
	role := orgRole(t, editor, word+" Approver")
	subject := inRealm(t, person)
	replicated(t)

	var meta struct{ Capabilities []string }
	partner.do(http.MethodGet, "/meta", nil, &meta)
	if !slices.Contains(meta.Capabilities, "org-embed@1") {
		t.Errorf("/meta does not list org-embed@1: %v", meta.Capabilities)
	}

	cardOf := func(path string) cardJSON {
		t.Helper()
		var got struct{ Card cardJSON }
		if status := partner.do(http.MethodGet, path, nil, &got); status != http.StatusOK {
			t.Fatalf("%s answered %d", path, status)
		}
		return got.Card
	}
	for _, c := range []struct {
		path               string
		kind, name, detail string
		page               string
	}{
		{"/cards/person/" + person, "person", word + " Person", word + " Analyst, " + word + " Team", "/people/" + person},
		{"/cards/position/" + position, "position", word + " Analyst", word + " Team", "/positions/" + position},
		{"/cards/unit/" + team, "unit", word + " Team", word + " Division", "/units/" + team},
		{"/cards/role/" + role.ID, "role", word + " Approver", "", "/roles/" + role.ID},
	} {
		got := cardOf(c.path)
		if got.Kind != c.kind || got.Name != c.name || got.Detail != c.detail || !strings.HasSuffix(got.URL, c.page) || !strings.HasPrefix(got.URL, "http") {
			t.Errorf("%s is %+v", c.path, got)
		}
	}
	if got := cardOf("/cards/unit/" + division); got.Detail == "" {
		t.Errorf("the division's card names no unit it is part of: %+v", got)
	}
	q := url.Values{"issuer": {realmIssuer}, "subject": {subject}}
	if got := cardOf("/cards/person/by-subject?" + q.Encode()); got.ID != person || got.Subject == nil || *got.Subject != subject || got.PhotoURL != nil {
		t.Errorf("the card by subject is %+v", got)
	}
	for path, want := range map[string]int{
		"/cards/person/" + uuid.NewString():        http.StatusNotFound,
		"/cards/process/" + person:                 http.StatusNotFound,
		"/cards/suggest?q=" + word + "&kinds=step": http.StatusUnprocessableEntity,
		"/cards/suggest?q=z":                       http.StatusUnprocessableEntity,
	} {
		if status := partner.do(http.MethodGet, path, nil, nil); status != want {
			t.Errorf("%s answered %d, want %d", path, status, want)
		}
	}

	var found struct{ Cards []cardJSON }
	if status := partner.do(http.MethodGet, "/cards/suggest?q="+word, nil, &found); status != http.StatusOK {
		t.Fatalf("suggesting answered %d", status)
	}
	var kinds []string
	for _, c := range found.Cards {
		kinds = append(kinds, c.Kind)
		if c.Kind == "person" && (c.Subject == nil || c.Detail != word+" Analyst, "+word+" Team") {
			t.Errorf("the person suggested is %+v", c)
		}
	}
	slices.Sort(kinds)
	if !slices.Equal(kinds, []string{"person", "position", "role", "unit", "unit"}) {
		t.Errorf("the cards suggested are %+v", found.Cards)
	}
	partner.do(http.MethodGet, "/cards/suggest?q="+word+"&kinds=person,role&limit=1", nil, &found)
	if len(found.Cards) != 1 || (found.Cards[0].Kind != "person" && found.Cards[0].Kind != "role") {
		t.Errorf("one person or role suggested is %+v", found.Cards)
	}
}
