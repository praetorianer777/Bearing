//go:build integration

package test

import (
	"net/http"
	"slices"
	"testing"
)

type eventJSON struct {
	ID        string
	ActorID   *string
	ActorName *string
	Table     string
	Operation string
	Before    map[string]any
	After     map[string]any
}

type auditJSON struct {
	Events []eventJSON
	Next   *string
}

// The case the issue names: a reorganisation today, read as of yesterday, shows the structure as it was.
func TestAPastDayShowsTheOldStructure(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	editor, _ := withRole(t, admin, "editor")
	typeID, root := unitType(t, admin), rootUnit(t, admin)
	north := units(t, editor, typeID, root, "North")[0]
	south := units(t, editor, typeID, root, "South")[0]
	team := units(t, editor, typeID, north, "Field team")[0]
	ids := positions(t, editor, team, "Field engineer")
	engineer := ids[0]
	bosses := positions(t, editor, north, "Head of North", "Head of South")
	headNorth, headSouth := bosses[0], bosses[1]
	editor.do(http.MethodPut, "/positions/"+engineer+"/reports-to", map[string]any{"reportsToId": headNorth, "from": day(-30)}, nil)
	before, after := somebody(t, editor), somebody(t, editor)
	held := give(t, editor, map[string]any{"personId": before, "positionId": engineer, "from": day(-30)})

	// Today the team moves south, reports there, and changes hands.
	if status := editor.do(http.MethodPost, "/units/"+team+"/move", map[string]any{"parentId": south, "from": day(0)}, nil); status != http.StatusOK {
		t.Fatalf("moving the team answered %d", status)
	}
	editor.do(http.MethodPut, "/positions/"+engineer+"/reports-to", map[string]any{"reportsToId": headSouth, "from": day(0)}, nil)
	editor.do(http.MethodPost, "/holdings/"+held.ID+"/end", map[string]any{"on": day(0)}, nil)
	give(t, editor, map[string]any{"personId": after, "positionId": engineer, "from": day(0)})

	for _, c := range []struct {
		on                  string
		parent, boss, holds string
	}{{day(-1), north, headNorth, before}, {day(0), south, headSouth, after}} {
		if got := parentOn(t, editor, team, c.on); got == nil || *got != c.parent {
			t.Errorf("on %s the team sits in %v, want %s", c.on, got, c.parent)
		}
		p := positionOn(t, editor, engineer, c.on)
		if p.ReportsToID == nil || *p.ReportsToID != c.boss || len(p.Holders) != 1 || p.Holders[0].PersonID != c.holds {
			t.Errorf("on %s the engineer reports to %v and is held by %+v", c.on, p.ReportsToID, p.Holders)
		}
		if chain := walked(t, editor, "/positions/"+engineer+"/chain?asOf="+c.on); len(chain) == 0 || chain[0].PositionID != c.boss {
			t.Errorf("on %s the chain is %v", c.on, titles(chain))
		}
		var members struct{ Members []struct{ PersonID string } }
		editor.do(http.MethodGet, "/units/"+north+"/members?subUnits=true&asOf="+c.on, nil, &members)
		inNorth := slices.ContainsFunc(members.Members, func(m struct{ PersonID string }) bool { return m.PersonID == c.holds })
		if inNorth != (c.parent == north) {
			t.Errorf("on %s the holder is among North's members: %v", c.on, inNorth)
		}
		var entry struct {
			Holdings []holdingJSON
		}
		editor.do(http.MethodGet, "/people/"+before+"?asOf="+c.on, nil, &entry)
		if holds := len(entry.Holdings) == 1; holds != (c.holds == before) {
			t.Errorf("on %s the first holder holds %+v", c.on, entry.Holdings)
		}
	}
}

func TestEveryChangeIsAuditedWithWhoMadeIt(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	editor, editorID := withRole(t, admin, "editor")
	member, _ := withRole(t, admin, "member")
	unit := units(t, editor, unitType(t, admin), rootUnit(t, admin), "Audited")[0]
	position := positions(t, editor, unit, "Auditor")[0]
	editor.do(http.MethodPatch, "/positions/"+position, map[string]any{"title": "Chief auditor"}, nil)
	holder := somebody(t, editor)
	held := give(t, editor, map[string]any{"personId": holder, "positionId": position, "from": day(-5)})
	editor.do(http.MethodPost, "/holdings/"+held.ID+"/end", map[string]any{"on": day(5)}, nil)

	var log auditJSON
	if status := editor.do(http.MethodGet, "/positions/"+position+"/audit", nil, &log); status != http.StatusOK {
		t.Fatalf("the position's log answered %d", status)
	}
	var tables []string
	for _, e := range log.Events {
		tables = append(tables, e.Table+" "+e.Operation)
		if e.ActorID == nil || *e.ActorID != editorID.String() || e.ActorName == nil || *e.ActorName != "editor" {
			t.Errorf("%s %s was made by %v %v, want the editor", e.Table, e.Operation, e.ActorID, e.ActorName)
		}
	}
	if want := []string{"holding update", "holding insert", "position update", "position insert"}; !slices.Equal(tables, want) {
		t.Errorf("the position's log is %v, want %v", tables, want)
	}
	if len(log.Events) == 4 {
		retitled := log.Events[2]
		if retitled.Before["title"] != "Auditor" || retitled.After["title"] != "Chief auditor" {
			t.Errorf("the retitling reads %v → %v", retitled.Before["title"], retitled.After["title"])
		}
		if ended := log.Events[0]; ended.Before["valid_to"] != nil || ended.After["valid_to"] != day(5) {
			t.Errorf("ending the holding reads %v → %v", ended.Before["valid_to"], ended.After["valid_to"])
		}
	}
	var theirs auditJSON
	editor.do(http.MethodGet, "/people/"+holder+"/audit", nil, &theirs)
	if len(theirs.Events) != 3 || theirs.Events[2].Table != "person" {
		t.Errorf("the holder's log has %d entries: %+v", len(theirs.Events), theirs.Events)
	}

	var page auditJSON
	editor.do(http.MethodGet, "/positions/"+position+"/audit?limit=3", nil, &page)
	if len(page.Events) != 3 || page.Next == nil {
		t.Fatalf("a first page of three is %d entries, next %v", len(page.Events), page.Next)
	}
	editor.do(http.MethodGet, "/positions/"+position+"/audit?limit=3&before="+*page.Next, nil, &page)
	if len(page.Events) != 1 || page.Events[0].Operation != "insert" || page.Next != nil {
		t.Errorf("the second page is %+v, next %v", page.Events, page.Next)
	}
	var all auditJSON
	if status := editor.do(http.MethodGet, "/audit?limit=1", nil, &all); status != http.StatusOK || len(all.Events) != 1 {
		t.Errorf("the whole log answered %d with %d entries", status, len(all.Events))
	}
	if status := member.do(http.MethodGet, "/positions/"+position+"/audit", nil, nil); status != http.StatusForbidden {
		t.Errorf("a member read the log: %d", status)
	}
}
