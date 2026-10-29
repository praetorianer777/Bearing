//go:build integration

package test

import (
	"context"
	"net/http"
	"testing"
)

type positionJSON struct {
	ID          string
	Title       string
	HeadOfUnit  bool
	ReportsToID *string
	DottedTo    []string
	ArchivedOn  *string
	Lines       []struct {
		ReportsToID string
		Kind        string
		From        string
		To          *string
	}
}

type positionBody struct{ Position positionJSON }

// positions makes positions in a unit from a month ago and removes them after.
func positions(t *testing.T, b *browser, unit string, titles ...string) []string {
	t.Helper()
	var ids []string
	for _, title := range titles {
		var made positionBody
		if status := b.do(http.MethodPost, "/positions", map[string]any{"unitId": unit, "title": title, "from": day(-30)}, &made); status != http.StatusCreated {
			t.Fatalf("making %s answered %d", title, status)
		}
		ids = append(ids, made.Position.ID)
	}
	super := superuser(t, "BEARING_TEST_SUPERUSER_URL")
	t.Cleanup(func() {
		for _, id := range ids {
			_, _ = super.Exec(context.Background(), `DELETE FROM reporting_line WHERE position_id = $1 OR reports_to_id = $1`, id)
		}
		for _, id := range ids {
			_, _ = super.Exec(context.Background(), `DELETE FROM position WHERE id = $1`, id)
		}
	})
	return ids
}

func positionOn(t *testing.T, b *browser, id, asOf string) positionJSON {
	t.Helper()
	var got positionBody
	if status := b.do(http.MethodGet, "/positions/"+id+"?asOf="+asOf, nil, &got); status != http.StatusOK {
		t.Fatalf("reading the position answered %d", status)
	}
	return got.Position
}

// orgFixture is an editor and a unit to put positions in.
func orgFixture(t *testing.T) (*browser, string) {
	t.Helper()
	admin := signedIn(t, adminEmail, adminPassword)
	editor, _ := withRole(t, admin, "editor")
	unit := units(t, editor, unitType(t, admin), rootUnit(t, admin), "Quality")[0]
	return editor, unit
}

func TestAPositionHasOneSolidLineAndAnyNumberOfDottedOnes(t *testing.T) {
	editor, unit := orgFixture(t)
	ids := positions(t, editor, unit, "Quality engineer", "Head of Quality", "Head of Engineering", "Project lead")
	engineer, head, engineering, project := ids[0], ids[1], ids[2], ids[3]

	if status := editor.do(http.MethodPut, "/positions/"+engineer+"/reports-to", map[string]any{"reportsToId": head, "from": day(-30)}, nil); status != http.StatusOK {
		t.Fatalf("the solid line answered %d", status)
	}
	// A second solid line replaces the first from its day, so there is never more than one.
	if status := editor.do(http.MethodPut, "/positions/"+engineer+"/reports-to", map[string]any{"reportsToId": engineering, "from": day(10)}, nil); status != http.StatusOK {
		t.Fatalf("the planned solid line answered %d", status)
	}
	if p := positionOn(t, editor, engineer, day(0)); p.ReportsToID == nil || *p.ReportsToID != head {
		t.Errorf("today the engineer reports to %v, want the head of quality", p.ReportsToID)
	}
	later := positionOn(t, editor, engineer, day(10))
	if later.ReportsToID == nil || *later.ReportsToID != engineering {
		t.Errorf("later the engineer reports to %v, want the head of engineering", later.ReportsToID)
	}
	solid := 0
	for _, l := range later.Lines {
		if l.Kind == "solid" {
			solid++
		}
	}
	if solid != 2 || later.Lines[0].To == nil || *later.Lines[0].To != day(10) {
		t.Errorf("lines %+v", later.Lines)
	}

	for _, to := range []string{engineering, project} {
		if status := editor.do(http.MethodPost, "/positions/"+engineer+"/dotted-lines", map[string]any{"reportsToId": to}, nil); status != http.StatusCreated {
			t.Errorf("a dotted line answered %d", status)
		}
	}
	if p := positionOn(t, editor, engineer, day(0)); len(p.DottedTo) != 2 {
		t.Errorf("today the engineer has dotted lines to %v, want two", p.DottedTo)
	}
	var r refusal
	if status := editor.do(http.MethodPost, "/positions/"+engineer+"/dotted-lines", map[string]any{"reportsToId": project}, &r); status != http.StatusConflict || r.Error.Code != "line_exists" {
		t.Errorf("the same dotted line twice answered %d %+v", status, r)
	}
	if status := editor.do(http.MethodPost, "/positions/"+engineer+"/dotted-lines/"+project+"/end", map[string]any{"on": day(5)}, nil); status != http.StatusOK {
		t.Errorf("ending a dotted line answered %d", status)
	}
	if p := positionOn(t, editor, engineer, day(5)); len(p.DottedTo) != 1 {
		t.Errorf("after it ends the engineer has dotted lines to %v, want one", p.DottedTo)
	}
	if status := editor.do(http.MethodPost, "/positions/"+engineer+"/dotted-lines/"+project+"/end", map[string]any{"on": day(6)}, &r); status != http.StatusConflict || r.Error.Code != "line_not_found" {
		t.Errorf("ending it again answered %d %+v", status, r)
	}
	if status := editor.do(http.MethodPut, "/positions/"+engineer+"/reports-to", map[string]any{"from": day(20)}, nil); status != http.StatusOK {
		t.Errorf("ending the solid line answered %d", status)
	}
	if p := positionOn(t, editor, engineer, day(20)); p.ReportsToID != nil {
		t.Errorf("after its end the engineer reports to %v", *p.ReportsToID)
	}
}

func TestSolidLinesMayNotRunInACircleButDottedOnesMay(t *testing.T) {
	editor, unit := orgFixture(t)
	ids := positions(t, editor, unit, "Analyst", "Lead", "Director")
	analyst, lead, director := ids[0], ids[1], ids[2]
	editor.do(http.MethodPut, "/positions/"+analyst+"/reports-to", map[string]any{"reportsToId": lead, "from": day(-30)}, nil)
	editor.do(http.MethodPut, "/positions/"+lead+"/reports-to", map[string]any{"reportsToId": director, "from": day(-30)}, nil)

	for name, c := range map[string]struct{ from, to, on string }{
		"directly":         {director, director, day(0)},
		"through a chain":  {director, analyst, day(0)},
		"planned for then": {director, lead, day(30)},
	} {
		var r refusal
		if status := editor.do(http.MethodPut, "/positions/"+c.from+"/reports-to", map[string]any{"reportsToId": c.to, "from": c.on}, &r); status != http.StatusConflict || r.Error.Code != "cycle" {
			t.Errorf("a circle %s answered %d %+v", name, status, r)
		}
	}
	if status := editor.do(http.MethodPost, "/positions/"+director+"/dotted-lines", map[string]any{"reportsToId": analyst}, nil); status != http.StatusCreated {
		t.Errorf("a dotted line against the solid ones answered %d", status)
	}
	// Once the chain is broken, the line that would have closed it is fine.
	editor.do(http.MethodPut, "/positions/"+lead+"/reports-to", map[string]any{"from": day(1)}, nil)
	if status := editor.do(http.MethodPut, "/positions/"+director+"/reports-to", map[string]any{"reportsToId": analyst, "from": day(1)}, nil); status != http.StatusOK {
		t.Errorf("the line after the chain was broken answered %d", status)
	}
}

func TestArchivingAPositionKeepsItsPast(t *testing.T) {
	editor, unit := orgFixture(t)
	ids := positions(t, editor, unit, "Clerk", "Office manager", "Head of Office")
	clerk, manager, head := ids[0], ids[1], ids[2]
	editor.do(http.MethodPut, "/positions/"+clerk+"/reports-to", map[string]any{"reportsToId": manager, "from": day(-30)}, nil)
	editor.do(http.MethodPut, "/positions/"+manager+"/reports-to", map[string]any{"reportsToId": head, "from": day(-30)}, nil)

	var r refusal
	if status := editor.do(http.MethodPost, "/positions/"+manager+"/archive", map[string]any{}, &r); status != http.StatusConflict || r.Error.Code != "has_reports" {
		t.Errorf("archiving a position others report to answered %d %+v", status, r)
	}
	editor.do(http.MethodPut, "/positions/"+clerk+"/reports-to", map[string]any{"reportsToId": head}, nil)
	if status := editor.do(http.MethodPost, "/positions/"+manager+"/archive", map[string]any{}, nil); status != http.StatusOK {
		t.Fatalf("archiving answered %d", status)
	}
	var list struct{ Positions []positionJSON }
	editor.do(http.MethodGet, "/positions?unitId="+unit, nil, &list)
	for _, p := range list.Positions {
		if p.ID == manager {
			t.Error("an archived position is still listed today")
		}
	}
	if p := positionOn(t, editor, manager, day(-1)); p.ReportsToID == nil || *p.ReportsToID != head {
		t.Errorf("yesterday the manager reported to %v, want the head", p.ReportsToID)
	}
	if status := editor.do(http.MethodPut, "/positions/"+clerk+"/reports-to", map[string]any{"reportsToId": manager, "from": day(1)}, &r); status != http.StatusConflict || r.Error.Code != "target_not_there" {
		t.Errorf("reporting to an archived position answered %d %+v", status, r)
	}
	var restored positionBody
	if status := editor.do(http.MethodPost, "/positions/"+manager+"/restore", nil, &restored); status != http.StatusOK || restored.Position.ReportsToID == nil {
		t.Errorf("restoring answered %d %+v", status, restored.Position)
	}

	if status := editor.do(http.MethodPost, "/units/"+unit+"/archive", map[string]any{}, &r); status != http.StatusConflict || r.Error.Code != "has_positions" {
		t.Errorf("archiving a unit with positions answered %d %+v", status, r)
	}
}

func TestAUnitHasOneHead(t *testing.T) {
	editor, unit := orgFixture(t)
	ids := positions(t, editor, unit, "Head", "Deputy head")
	if status := editor.do(http.MethodPatch, "/positions/"+ids[0], map[string]any{"headOfUnit": true}, nil); status != http.StatusOK {
		t.Fatalf("making the head answered %d", status)
	}
	var r refusal
	if status := editor.do(http.MethodPatch, "/positions/"+ids[1], map[string]any{"headOfUnit": true}, &r); status != http.StatusConflict || r.Error.Code != "head_taken" {
		t.Errorf("a second head answered %d %+v", status, r)
	}
	var list struct{ Positions []positionJSON }
	editor.do(http.MethodGet, "/positions?unitId="+unit, nil, &list)
	if len(list.Positions) != 2 || list.Positions[0].ID != ids[0] {
		t.Errorf("the head is not listed first: %+v", list.Positions)
	}
	if status := editor.do(http.MethodPost, "/positions", map[string]any{"unitId": unit, "title": "Too early", "from": day(-90)}, &r); status != http.StatusUnprocessableEntity {
		t.Errorf("a position before its unit existed answered %d", status)
	}
}
