//go:build integration

package test

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"testing"

	"github.com/google/uuid"
)

const realmIssuer = "https://realm.test/realms/dynamo"

// inRealm links a person to the realm by a subject of their own, as signing in through it would.
func inRealm(t *testing.T, id string) string {
	t.Helper()
	subject := "sub-" + uuid.NewString()
	super := superuser(t, "BEARING_TEST_SUPERUSER_URL")
	if _, err := super.Exec(context.Background(), `UPDATE person SET issuer = $3, subject = $2 WHERE id = $1`, id, subject, realmIssuer); err != nil {
		t.Fatal(err)
	}
	return subject
}

// A partner reads the organisation as the person it acts for, by the realm's names for people: the developer
// reports to the lead, the lead to the CTO, whose chair is empty and whose deputy stands in, the CTO to the CEO. An
// approver role is held directly by Rita and carried by the CTO's and the developer's positions.
func TestAPartnerReadsTheDirectoryAndWhoAnswersForWhat(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	editor, _ := withRole(t, admin, "editor")
	partner, _ := withRole(t, admin, "member")
	kind := unitType(t, admin)
	made := units(t, editor, kind, rootUnit(t, admin), "Engineering", "Platform")
	unit, inner := made[0], made[1]
	ids := positions(t, editor, unit, "CEO", "CTO", "Lead", "Developer", "CTO deputy")
	ceo, cto, lead, dev, deputy := ids[0], ids[1], ids[2], ids[3], ids[4]
	for _, l := range [][2]string{{cto, ceo}, {lead, cto}, {dev, lead}} {
		editor.do(http.MethodPut, "/positions/"+l[0]+"/reports-to", map[string]any{"reportsToId": l[1], "from": day(-20)}, nil)
	}
	editor.do(http.MethodPut, "/positions/"+cto+"/deputies", map[string]any{"deputyIds": []string{deputy}}, nil)
	people := map[string]string{}
	for _, p := range []struct{ name, position string }{{"Cleo", ceo}, {"Lena", lead}, {"Dev", dev}, {"Dora", deputy}, {"Rita", ""}} {
		people[p.name] = person(t, editor, map[string]any{"name": p.name + " " + uuid.NewString()[:6]}).ID
		if p.position != "" {
			give(t, editor, map[string]any{"personId": people[p.name], "positionId": p.position, "share": 80, "from": day(-20)})
		}
	}
	approver := orgRole(t, editor, "Approver")
	give(t, editor, map[string]any{"personId": people["Rita"], "roleId": approver.ID, "share": 10, "from": day(-20)})
	for _, p := range []string{cto, dev} {
		editor.do(http.MethodPost, "/positions/"+p+"/roles", map[string]any{"roleId": approver.ID, "from": day(-20)}, nil)
	}
	devSubject, lenaSubject := inRealm(t, people["Dev"]), inRealm(t, people["Lena"])
	replicated(t)
	bySubject := func(path, subject string, more url.Values) string {
		q := url.Values{"issuer": {realmIssuer}, "subject": {subject}}
		for k, v := range more {
			q[k] = v
		}
		return path + "?" + q.Encode()
	}

	var meta struct{ Capabilities []string }
	partner.do(http.MethodGet, "/meta", nil, &meta)
	for _, c := range []string{"org-directory@1", "responsibility@1"} {
		if !slices.Contains(meta.Capabilities, c) {
			t.Errorf("/meta does not list %s: %v", c, meta.Capabilities)
		}
	}

	var entry struct {
		Person struct {
			ID      string
			Subject *string
		}
		Positions []struct {
			PositionID, Title, UnitID, UnitName string
			Share                               int
		}
		Roles []struct {
			RoleID  string
			Through *string
		}
	}
	if status := partner.do(http.MethodGet, bySubject("/people/by-subject", devSubject, nil), nil, &entry); status != http.StatusOK {
		t.Fatalf("the developer by subject answered %d", status)
	}
	if entry.Person.ID != people["Dev"] || len(entry.Positions) != 1 || entry.Positions[0].PositionID != dev || entry.Positions[0].UnitName != "Engineering" ||
		entry.Positions[0].Share != 80 {
		t.Errorf("the developer's entry is %+v", entry)
	}
	if len(entry.Roles) != 1 || entry.Roles[0].RoleID != approver.ID || entry.Roles[0].Through == nil || *entry.Roles[0].Through != dev {
		t.Errorf("the developer's roles are %+v", entry.Roles)
	}
	if status := partner.do(http.MethodGet, bySubject("/people/by-subject", devSubject, url.Values{"asOf": {day(-25)}}), nil, &entry); status != http.StatusOK ||
		len(entry.Positions) != 0 || len(entry.Roles) != 0 {
		t.Errorf("before the holding the entry is %d %+v", status, entry)
	}
	if status := partner.do(http.MethodGet, "/people/"+people["Rita"]+"/entry", nil, &entry); status != http.StatusOK || len(entry.Roles) != 1 || entry.Roles[0].Through != nil {
		t.Errorf("Rita's entry is %d %+v", status, entry)
	}
	if status := partner.do(http.MethodGet, bySubject("/people/by-subject", "nobody", nil), nil, nil); status != http.StatusNotFound {
		t.Errorf("a subject nobody has answered %d", status)
	}
	if status := partner.do(http.MethodGet, "/people/by-subject?issuer="+url.QueryEscape(realmIssuer), nil, nil); status != http.StatusUnprocessableEntity {
		t.Errorf("no subject answered %d", status)
	}

	var chain struct {
		Positions []struct {
			PositionID string
			Holders    []resolvedJSON
		}
	}
	partner.do(http.MethodGet, bySubject("/people/by-subject/chain", devSubject, nil), nil, &chain)
	if len(chain.Positions) != 3 || chain.Positions[0].PositionID != lead || len(chain.Positions[0].Holders) != 1 ||
		chain.Positions[0].Holders[0].Subject == nil || *chain.Positions[0].Holders[0].Subject != lenaSubject {
		t.Errorf("the developer's chain by subject is %+v", chain)
	}
	if got := walked(t, partner, bySubject("/people/by-subject/reports", lenaSubject, nil)); len(got) != 1 || got[0].PositionID != dev {
		t.Errorf("the lead's reports by subject are %+v", got)
	}

	var tree struct {
		Units []struct {
			ID       string
			ParentID *string
			Depth    int
		}
	}
	if status := partner.do(http.MethodGet, "/units/"+unit+"/tree", nil, &tree); status != http.StatusOK || len(tree.Units) != 2 ||
		tree.Units[0].ID != unit || tree.Units[0].Depth != 0 || tree.Units[1].ID != inner || tree.Units[1].Depth != 1 {
		t.Errorf("the unit's tree is %d %+v", status, tree.Units)
	}
	if status := partner.do(http.MethodGet, "/units/"+uuid.NewString()+"/tree", nil, nil); status != http.StatusNotFound {
		t.Errorf("the tree of no unit answered %d", status)
	}

	var holders struct{ People []resolvedJSON }
	partner.do(http.MethodGet, "/roles/"+approver.ID+"/people", nil, &holders)
	var got []string
	for _, p := range holders.People {
		name := p.PersonID
		if p.DeputyFor != nil {
			name += " for " + *p.DeputyFor
		}
		got = append(got, name)
	}
	if want := []string{people["Rita"], people["Dora"] + " for " + cto, people["Dev"]}; !slices.Equal(got, want) {
		t.Errorf("the approver comes down to %v, want %v", got, want)
	}

	type managers struct {
		Levels   int
		Managers []struct {
			PositionID string
			People     []resolvedJSON
		}
	}
	for levels, want := range map[string][2]string{"1": {lead, people["Lena"]}, "2": {cto, people["Dora"]}, "3": {ceo, people["Cleo"]}} {
		var m managers
		if status := partner.do(http.MethodGet, bySubject("/people/by-subject/managers", devSubject, url.Values{"levels": {levels}}), nil, &m); status != http.StatusOK ||
			len(m.Managers) != 1 || m.Managers[0].PositionID != want[0] || len(m.Managers[0].People) != 1 || m.Managers[0].People[0].PersonID != want[1] {
			t.Errorf("%s levels up from the developer: %d %+v", levels, status, m)
		}
	}
	var m managers
	if partner.do(http.MethodGet, "/people/"+people["Dev"]+"/managers", nil, &m); m.Levels != 1 || len(m.Managers) != 1 || m.Managers[0].PositionID != lead {
		t.Errorf("the developer's manager is %+v", m)
	}
	if partner.do(http.MethodGet, "/people/"+people["Dev"]+"/managers?levels=4", nil, &m); len(m.Managers) != 0 {
		t.Errorf("above the top the developer has managers %+v", m)
	}
	for _, levels := range []string{"0", "x"} {
		if status := partner.do(http.MethodGet, "/people/"+people["Dev"]+"/managers?levels="+levels, nil, nil); status != http.StatusUnprocessableEntity {
			t.Errorf("levels=%s answered %d", levels, status)
		}
	}
}
