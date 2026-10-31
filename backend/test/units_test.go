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

type unitJSON struct {
	ID         string
	Name       string
	ParentID   *string
	ArchivedOn *string
	Placements []struct {
		ParentID *string
		From     string
		To       *string
	}
}

type unitBody struct{ Unit unitJSON }

type refusal struct {
	Error struct {
		Code, Message string
		Fields        map[string]string
	}
}

func day(offset int) string { return time.Now().AddDate(0, 0, offset).Format("2006-01-02") }

// unitType makes a unit type for the test and removes it after.
func unitType(t *testing.T, admin *browser) string {
	t.Helper()
	var made struct{ UnitType struct{ ID string } }
	if status := admin.do(http.MethodPost, "/unit-types", map[string]any{"name": "Type " + uuid.NewString()[:8], "icon": "team", "colour": "#2F5FD0"}, &made); status != http.StatusCreated {
		t.Fatalf("making a unit type answered %d", status)
	}
	t.Cleanup(func() { admin.do(http.MethodDelete, "/unit-types/"+made.UnitType.ID, nil, nil) })
	return made.UnitType.ID
}

// rootUnit is the organisation's root, made from long ago when nothing has made it yet. It stays, as the seed's root
// would.
func rootUnit(t *testing.T, admin *browser) string {
	t.Helper()
	var list struct{ Units []unitJSON }
	admin.do(http.MethodGet, "/units", nil, &list)
	for _, u := range list.Units {
		if u.ParentID == nil {
			return u.ID
		}
	}
	var made struct{ UnitType struct{ ID string } }
	if status := admin.do(http.MethodPost, "/unit-types", map[string]any{"name": "Organisation " + uuid.NewString()[:8], "icon": "organisation", "rank": -1}, &made); status != http.StatusCreated {
		t.Fatalf("making the root's type answered %d", status)
	}
	var root unitBody
	if status := admin.do(http.MethodPost, "/units", map[string]any{"typeId": made.UnitType.ID, "name": "Organisation", "from": "2000-01-01"}, &root); status != http.StatusCreated {
		t.Fatalf("making the root answered %d", status)
	}
	return root.Unit.ID
}

// units makes a chain of units under parent from a month ago, each inside the one before, and removes them after.
func units(t *testing.T, b *browser, typeID, parent string, names ...string) []string {
	t.Helper()
	var ids []string
	for _, name := range names {
		var made unitBody
		if status := b.do(http.MethodPost, "/units", map[string]any{"typeId": typeID, "name": name, "parentId": parent, "from": day(-30)}, &made); status != http.StatusCreated {
			t.Fatalf("making %s answered %d", name, status)
		}
		ids = append(ids, made.Unit.ID)
		parent = made.Unit.ID
	}
	super := superuser(t, "BEARING_TEST_SUPERUSER_URL")
	t.Cleanup(func() {
		for _, id := range ids {
			_, _ = super.Exec(context.Background(), `DELETE FROM unit_placement WHERE unit_id = $1 OR parent_id = $1`, id)
		}
		for _, id := range ids {
			_, _ = super.Exec(context.Background(), `DELETE FROM unit WHERE id = $1`, id)
		}
	})
	return ids
}

// parentOn is where a unit sits on a day: its parent's id, "root", or nil when it does not exist then.
func parentOn(t *testing.T, b *browser, id, asOf string) *string {
	t.Helper()
	var list struct{ Units []unitJSON }
	b.do(http.MethodGet, "/units?asOf="+asOf, nil, &list)
	for _, u := range list.Units {
		if u.ID == id {
			if u.ParentID == nil {
				root := "root"
				return &root
			}
			return u.ParentID
		}
	}
	return nil
}

func TestAMoveThatWouldMakeACycleIsRefused(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	editor, _ := withRole(t, admin, "editor")
	typeID := unitType(t, admin)
	root := rootUnit(t, admin)
	ids := units(t, editor, typeID, root, "Division", "Department", "Team")
	division, team := ids[0], ids[2]

	for _, target := range []string{team, division} {
		var r refusal
		if status := editor.do(http.MethodPost, "/units/"+division+"/move", map[string]any{"parentId": target}, &r); status != http.StatusConflict || r.Error.Code != "cycle" {
			t.Errorf("moving the division under %s answered %d %+v", target, status, r)
		}
	}
	var r refusal
	if status := editor.do(http.MethodPost, "/units/"+division+"/move", map[string]any{"parentId": team, "from": day(60)}, &r); status != http.StatusConflict || r.Error.Code != "cycle" {
		t.Errorf("a planned cycle answered %d %+v", status, r)
	}
	if status := editor.do(http.MethodPost, "/units/"+team+"/move", map[string]any{"parentId": root}, nil); status != http.StatusOK {
		t.Fatalf("moving the team out answered %d", status)
	}
	if status := editor.do(http.MethodPost, "/units/"+division+"/move", map[string]any{"parentId": team, "from": day(1)}, nil); status != http.StatusOK {
		t.Errorf("moving the division under the team, now outside it, answered %d", status)
	}
	if status := editor.do(http.MethodPost, "/units", map[string]any{"typeId": typeID, "name": "Second root"}, &r); status != http.StatusConflict || r.Error.Code != "root_taken" {
		t.Errorf("a second root answered %d %+v", status, r)
	}
}

func TestAMoveTakesEffectFromItsDay(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	editor, _ := withRole(t, admin, "editor")
	typeID := unitType(t, admin)
	root := rootUnit(t, admin)
	sales := units(t, editor, typeID, root, "Sales")[0]
	marketing := units(t, editor, typeID, root, "Marketing")[0]
	team := units(t, editor, typeID, sales, "Campaigns")[0]

	var moved unitBody
	if status := editor.do(http.MethodPost, "/units/"+team+"/move", map[string]any{"parentId": marketing, "from": day(14)}, &moved); status != http.StatusOK {
		t.Fatalf("planning a move answered %d", status)
	}
	if p := parentOn(t, editor, team, day(0)); p == nil || *p != sales {
		t.Errorf("today the team is under %v, want sales", p)
	}
	if p := parentOn(t, editor, team, day(14)); p == nil || *p != marketing {
		t.Errorf("from the move the team is under %v, want marketing", p)
	}
	if len(moved.Unit.Placements) != 2 || moved.Unit.Placements[0].To == nil || *moved.Unit.Placements[0].To != day(14) {
		t.Errorf("placements %+v", moved.Unit.Placements)
	}
	if p := parentOn(t, editor, team, day(-31)); p != nil {
		t.Errorf("before it was made the team is under %v", *p)
	}
	var r refusal
	if status := editor.do(http.MethodPost, "/units/"+team+"/move", map[string]any{"parentId": marketing, "from": day(-60)}, &r); status != http.StatusConflict || r.Error.Code != "not_there" {
		t.Errorf("moving before it existed answered %d %+v", status, r)
	}
	if status := editor.do(http.MethodGet, "/units?asOf=yesterday", nil, &r); status != http.StatusUnprocessableEntity {
		t.Errorf("a day that is no day answered %d", status)
	}
}

func TestArchivingKeepsTheDaysBeforeAndRestoringBringsItBack(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	editor, _ := withRole(t, admin, "editor")
	typeID := unitType(t, admin)
	root := rootUnit(t, admin)
	ids := units(t, editor, typeID, root, "Logistics", "Warehouse")
	logistics, warehouse := ids[0], ids[1]

	var r refusal
	if status := editor.do(http.MethodPost, "/units/"+logistics+"/archive", map[string]any{}, &r); status != http.StatusConflict || r.Error.Code != "has_units" {
		t.Errorf("archiving a unit with units in it answered %d %+v", status, r)
	}
	editor.do(http.MethodPost, "/units/"+warehouse+"/move", map[string]any{"parentId": root}, nil)
	var archived unitBody
	if status := editor.do(http.MethodPost, "/units/"+logistics+"/archive", map[string]any{}, &archived); status != http.StatusOK || archived.Unit.ArchivedOn == nil {
		t.Fatalf("archiving answered %d %+v", status, archived.Unit)
	}
	if p := parentOn(t, editor, logistics, day(0)); p != nil {
		t.Error("an archived unit is still listed today")
	}
	if p := parentOn(t, editor, logistics, day(-1)); p == nil || *p != root {
		t.Errorf("yesterday the unit was under %v, want the root", p)
	}
	var gone struct{ Units []unitJSON }
	editor.do(http.MethodGet, "/units?archived=true", nil, &gone)
	found := false
	for _, u := range gone.Units {
		found = found || u.ID == logistics && u.ParentID != nil && *u.ParentID == root
	}
	if !found {
		t.Error("the archived list does not hold the unit with where it last was")
	}
	if status := editor.do(http.MethodPost, "/units/"+warehouse+"/move", map[string]any{"parentId": logistics}, &r); status != http.StatusConflict || r.Error.Code != "parent_not_there" {
		t.Errorf("moving into an archived unit answered %d %+v", status, r)
	}
	if status := editor.do(http.MethodPost, "/units/"+logistics+"/archive", map[string]any{}, &r); status != http.StatusConflict || r.Error.Code != "archived" {
		t.Errorf("archiving twice answered %d %+v", status, r)
	}

	var restored unitBody
	if status := editor.do(http.MethodPost, "/units/"+logistics+"/restore", nil, &restored); status != http.StatusOK || restored.Unit.ArchivedOn != nil {
		t.Fatalf("restoring answered %d %+v", status, restored.Unit)
	}
	if p := parentOn(t, editor, logistics, day(0)); p == nil || *p != root {
		t.Errorf("restored, the unit is under %v, want the root", p)
	}
	if status := editor.do(http.MethodPost, "/units/"+logistics+"/restore", nil, &r); status != http.StatusConflict || r.Error.Code != "not_archived" {
		t.Errorf("restoring twice answered %d %+v", status, r)
	}
}

func TestOnlyEditorsChangeUnitsAndOnlyAdministratorsTheirTypes(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	editor, _ := withRole(t, admin, "editor")
	member, memberID := withRole(t, admin, "member")
	typeID := unitType(t, admin)
	root := rootUnit(t, admin)

	if status := member.do(http.MethodPost, "/units", map[string]any{"typeId": typeID, "name": "Mine", "parentId": root}, nil); status != http.StatusForbidden {
		t.Errorf("a member made a unit: %d", status)
	}
	if status := editor.do(http.MethodPost, "/unit-types", map[string]any{"name": "Squad"}, nil); status != http.StatusForbidden {
		t.Errorf("an editor made a unit type: %d", status)
	}
	var list struct{ UnitTypes []struct{ ID string } }
	if status := member.do(http.MethodGet, "/unit-types", nil, &list); status != http.StatusOK || len(list.UnitTypes) == 0 {
		t.Errorf("a member reading unit types answered %d", status)
	}

	id := units(t, editor, typeID, root, "Finance")[0]
	var r refusal
	if status := admin.do(http.MethodDelete, "/unit-types/"+typeID, nil, &r); status != http.StatusConflict || r.Error.Code != "type_in_use" {
		t.Errorf("removing a type in use answered %d %+v", status, r)
	}
	var renamed unitBody
	if status := editor.do(http.MethodPatch, "/units/"+id, map[string]any{"name": "Finance and Controlling", "shortName": "FC"}, &renamed); status != http.StatusOK || renamed.Unit.Name != "Finance and Controlling" {
		t.Errorf("renaming answered %d %+v", status, renamed.Unit)
	}
	if status := editor.do(http.MethodPatch, "/units/"+id, map[string]any{"name": " "}, nil); status != http.StatusUnprocessableEntity {
		t.Errorf("an empty name answered %d", status)
	}

	cluster, err := db.Open(context.Background(), loadConfig(t).DB, quietLog())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cluster.Close)
	for _, stmt := range []string{
		`UPDATE unit SET name = 'Taken over' WHERE id = $1`,
		`INSERT INTO unit_placement (unit_id, parent_id, valid_from, valid_to) VALUES ($1, NULL, '1990-01-01', '1990-01-02')`,
	} {
		var n int64
		_, err = cluster.Write(db.WithUser(context.Background(), memberID), func(ctx context.Context, q db.DBTX) error {
			tag, err := q.Exec(ctx, stmt, id)
			n = tag.RowsAffected()
			return err
		})
		if n != 0 || err != nil && !strings.Contains(err.Error(), "row-level security") {
			t.Errorf("a member wrote %q: %d rows, %v", stmt, n, err)
		}
	}
}
