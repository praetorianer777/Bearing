//go:build integration

package test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/db"
	"github.com/praetorianer777/bearing/backend/internal/org"
	"github.com/praetorianer777/bearing/backend/internal/orgcsv"
)

type importJSON struct {
	ID      string
	Status  string
	Counts  map[string]int
	Changes []struct {
		File, Action, Name string
		Row                int
	}
	Errors []struct {
		File, Message string
		Row           int
	}
	Result *struct {
		Error *struct {
			File, Message string
			Row           int
		}
	}
}

func exported(t *testing.T, b *browser, kind string) string {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, b.base+"/export/"+kind, nil)
	resp, err := b.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/csv") {
		t.Fatalf("exporting %s answered %d %s", kind, resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	return string(body)
}

func preview(t *testing.T, b *browser, files map[string]any) importJSON {
	t.Helper()
	var out struct{ Import importJSON }
	if status := b.do(http.MethodPost, "/imports", files, &out); status != http.StatusCreated {
		t.Fatalf("previewing answered %d", status)
	}
	return out.Import
}

// runJob runs a queued import as the worker does, and waits until it has run: the stack's worker may claim it first,
// when it is built from this revision, and is missing when it is not.
func runJob(t *testing.T, b *browser, id string, actor uuid.UUID) importJSON {
	t.Helper()
	cluster, err := db.Open(context.Background(), loadConfig(t).DB, quietLog())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cluster.Close)
	imports := &orgcsv.Imports{DB: cluster, Org: &org.Service{DB: cluster}}
	payload, _ := json.Marshal(map[string]any{"id": id, "actor": actor})
	if err := imports.Handler()(context.Background(), payload); err != nil {
		t.Fatalf("running the import: %v", err)
	}
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(100 * time.Millisecond) {
		var out struct{ Import importJSON }
		b.do(http.MethodGet, "/imports/"+id, nil, &out)
		if out.Import.Status == "done" || out.Import.Status == "failed" || time.Now().After(deadline) {
			return out.Import
		}
	}
}

func TestTheOrganisationGoesOutAndComesBackUnchanged(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	editor, _ := withRole(t, admin, "editor")
	files := map[string]any{}
	for _, kind := range orgcsv.Kinds {
		files[kind] = exported(t, editor, kind)
	}
	if lines := strings.Count(files["units"].(string), "\n"); lines < 5 {
		t.Fatalf("the demo's units export %d lines", lines)
	}
	if !strings.HasPrefix(files["holdings"].(string), "person_ref,position_ref,share,from,to\n") {
		t.Errorf("the holdings' header: %q", strings.SplitN(files["holdings"].(string), "\n", 2)[0])
	}
	got := preview(t, editor, files)
	if len(got.Errors) != 0 || got.Counts["create"] != 0 || got.Counts["update"] != 0 || got.Counts["unchanged"] < 20 {
		t.Errorf("importing what was exported: %v, %+v", got.Counts, got.Errors)
	}
}

func TestAnImportIsPreviewedThenRunInOneGo(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	editor, editorID := withRole(t, admin, "editor")
	typeID := unitType(t, admin)
	var types struct{ UnitTypes []struct{ ID, Name string } }
	replicated(t)
	editor.do(http.MethodGet, "/unit-types", nil, &types)
	typeName := ""
	for _, ty := range types.UnitTypes {
		if ty.ID == typeID {
			typeName = ty.Name
		}
	}
	parent := units(t, editor, typeID, rootUnit(t, admin), "Imports")[0]
	boss := positions(t, editor, parent, "Head of imports")[0]
	stamp := uuid.NewString()[:8]
	email := "imported-" + stamp + "@bearing.test"
	t.Cleanup(func() {
		super := superuser(t, "BEARING_TEST_SUPERUSER_URL")
		ctx := context.Background()
		_, _ = super.Exec(ctx, `DELETE FROM holding WHERE person_id IN (SELECT id FROM person WHERE email = $1)`, email)
		_, _ = super.Exec(ctx, `DELETE FROM person WHERE email = $1`, email)
		_, _ = super.Exec(ctx, `DELETE FROM reporting_line WHERE position_id IN (SELECT p.id FROM position p JOIN unit u ON u.id = p.unit_id WHERE u.name = $1)`, "Lab "+stamp)
		_, _ = super.Exec(ctx, `DELETE FROM position WHERE unit_id IN (SELECT id FROM unit WHERE name = $1)`, "Lab "+stamp)
		_, _ = super.Exec(ctx, `DELETE FROM unit_placement WHERE unit_id IN (SELECT id FROM unit WHERE name = $1)`, "Lab "+stamp)
		_, _ = super.Exec(ctx, `DELETE FROM unit WHERE name = $1`, "Lab "+stamp)
	})
	files := map[string]any{
		"units":     "ref,name,type,parent_ref\nlab,Lab " + stamp + "," + typeName + "," + parent + "\n",
		"positions": "ref,unit_ref,title,seats,reports_to_ref,head_of_unit\ntester,lab,Tester " + stamp + ",2," + boss + ",false\n",
		"people":    "ref,name,email\nnia,Nia " + stamp + "," + email + "\n",
		"holdings":  "person_ref,position_ref,share,from\nnia,tester,60," + day(0) + "\n",
		"from":      day(0),
	}
	got := preview(t, editor, files)
	if len(got.Errors) != 0 || got.Counts["create"] != 4 || got.Status != "previewed" {
		t.Fatalf("the preview: %+v", got)
	}
	var queued struct{ Import importJSON }
	if status := editor.do(http.MethodPost, "/imports/"+got.ID+"/run", nil, &queued); status != http.StatusAccepted || queued.Import.Status != "queued" {
		t.Fatalf("running answered %d %+v", status, queued.Import)
	}
	if status := editor.do(http.MethodPost, "/imports/"+got.ID+"/run", nil, nil); status != http.StatusConflict {
		t.Errorf("running twice answered %d", status)
	}
	if done := runJob(t, editor, got.ID, editorID); done.Status != "done" || done.Result == nil || done.Result.Error != nil {
		t.Fatalf("after running: %+v", done)
	}

	replicated(t)
	var people struct{ People []personJSON }
	editor.do(http.MethodGet, "/people?q="+email, nil, &people)
	if len(people.People) != 1 {
		t.Fatalf("the imported person: %+v", people.People)
	}
	var held struct{ Holdings []holdingJSON }
	editor.do(http.MethodGet, "/holdings?personId="+people.People[0].ID, nil, &held)
	if len(held.Holdings) != 1 || held.Holdings[0].Share != 60 {
		t.Fatalf("the imported holding: %+v", held.Holdings)
	}
	tester := positionOn(t, editor, held.Holdings[0].PositionID, day(0))
	if tester.Seats != 2 || tester.ReportsToID == nil || *tester.ReportsToID != boss {
		t.Errorf("the imported position: %+v", tester)
	}

	again := preview(t, editor, files)
	// Refs the files make name rows of these files only, so the same files make the unit, its position and the
	// holding again; the person is found by address.
	if again.Counts["create"] != 3 || again.Counts["unchanged"] != 1 || again.Changes[2].File != "people" || again.Changes[2].Action != "unchanged" {
		t.Errorf("the same files again: %v %+v", again.Counts, again.Changes)
	}
}

func TestAnImportListsTheRowsItCannotTake(t *testing.T) {
	editor, unit := orgFixture(t)
	got := preview(t, editor, map[string]any{
		"units":     "ref,name,type,parent_ref,colour\na,,Team," + unit + ",red\nb,Bench,No such type," + unit + "\nc,Bench,,nowhere\n",
		"positions": "ref,unit_ref,title,seats\np,nowhere,Tester,1\nq," + unit + ",Tester,many\nr," + unit + ",,1\n",
		"holdings":  "person_ref,position_ref,from\nnobody,p,2026-01-01\n",
	})
	want := []string{
		"units 1 There is no column \"colour\"; the columns are ref, name, short_name, type, parent_ref, description.",
		"units 2 Name the unit.",
		"units 3 There is no unit type \"No such type\".",
		"units 4 No unit has the ref \"nowhere\".",
		"positions 2 No unit has the ref \"nowhere\".",
		"positions 3 Give the seats as a whole number from 1.",
		"positions 4 Give the position a title.",
		"holdings 2 No person has the ref or address \"nobody\".",
	}
	var errs []string
	for _, e := range got.Errors {
		errs = append(errs, fmt.Sprintf("%s %d %s", e.File, e.Row, e.Message))
	}
	if strings.Join(errs, "\n") != strings.Join(want, "\n") {
		t.Errorf("errors:\n%s\nwant\n%s", strings.Join(errs, "\n"), strings.Join(want, "\n"))
	}
	var r refusal
	if status := editor.do(http.MethodPost, "/imports/"+got.ID+"/run", nil, &r); status != http.StatusConflict || r.Error.Code != "rows_refused" {
		t.Errorf("running a refused preview: %d %+v", status, r)
	}
}

func TestARefusalWhileRunningLeavesEverythingAsItWas(t *testing.T) {
	editor, unit := orgFixture(t)
	var me struct{ Account struct{ ID string } }
	editor.do(http.MethodGet, "/auth/me", nil, &me)
	editorID := uuid.MustParse(me.Account.ID)
	seat := positions(t, editor, unit, "Only seat")[0]
	give(t, editor, map[string]any{"personId": somebody(t, editor), "positionId": seat, "from": day(-5)})
	other := somebody(t, editor)
	got := preview(t, editor, map[string]any{
		"positions": "ref,unit_ref,title\nextra," + unit + ",Extra seat\n",
		"holdings":  "person_ref,position_ref,from\n" + other + "," + seat + "," + day(0) + "\n",
	})
	if len(got.Errors) != 0 || got.Counts["create"] != 2 {
		t.Fatalf("the preview: %+v", got)
	}
	editor.do(http.MethodPost, "/imports/"+got.ID+"/run", nil, nil)
	if failed := runJob(t, editor, got.ID, editorID); failed.Status != "failed" || failed.Result == nil || failed.Result.Error == nil ||
		failed.Result.Error.File != "holdings" || failed.Result.Error.Row != 2 {
		t.Fatalf("after a refusal: %+v", failed)
	}
	replicated(t)
	var list struct{ Positions []positionJSON }
	editor.do(http.MethodGet, "/positions?unitId="+unit, nil, &list)
	for _, p := range list.Positions {
		if p.Title == "Extra seat" {
			t.Errorf("the position the import made before its refusal is there")
		}
	}
}

func TestOnlyEditorsImportAndExport(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	member, _ := withRole(t, admin, "member")
	if status := member.do(http.MethodGet, "/export/units", nil, nil); status != http.StatusForbidden {
		t.Errorf("a member exported: %d", status)
	}
	if status := member.do(http.MethodPost, "/imports", map[string]any{"units": "ref,name\n"}, nil); status != http.StatusForbidden {
		t.Errorf("a member imported: %d", status)
	}
	editor, _ := withRole(t, admin, "editor")
	if status := editor.do(http.MethodGet, "/export/accounts", nil, nil); status != http.StatusNotFound {
		t.Errorf("exporting accounts: %d", status)
	}
	if status := editor.do(http.MethodPost, "/imports", map[string]any{}, nil); status != http.StatusUnprocessableEntity {
		t.Errorf("an import of nothing: %d", status)
	}
}
