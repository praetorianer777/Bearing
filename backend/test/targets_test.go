//go:build integration

package test

import (
	"fmt"
	"net/http"
	"slices"
	"testing"
)

type targetJSON struct {
	ID                         string
	UnitID, PositionID, RoleID *string
	FTE                        float64
	From                       string
	To                         *string
}

func setTarget(t *testing.T, b *browser, fields map[string]any) targetJSON {
	t.Helper()
	var made struct{ Target targetJSON }
	if status := b.do(http.MethodPost, "/targets", fields, &made); status != http.StatusCreated {
		t.Fatalf("setting %v answered %d", fields, status)
	}
	return made.Target
}

// spans are a subject's targets as "fte from–to", oldest first, to written "" for good.
func spans(t *testing.T, b *browser, query string) []string {
	t.Helper()
	var list struct{ Targets []targetJSON }
	if status := b.do(http.MethodGet, "/targets?"+query, nil, &list); status != http.StatusOK {
		t.Fatalf("the targets answered %d", status)
	}
	var out []string
	for _, x := range list.Targets {
		to := ""
		if x.To != nil {
			to = *x.To
		}
		out = append(out, fmt.Sprintf("%g %s-%s", x.FTE, x.From, to))
	}
	return out
}

func TestATargetSetForASpanReplacesWhatWasSetForItsDays(t *testing.T) {
	editor, unit := orgFixture(t)
	q := "unitId=" + unit
	setTarget(t, editor, map[string]any{"unitId": unit, "fte": 4, "from": day(-30)})
	setTarget(t, editor, map[string]any{"unitId": unit, "fte": 6, "from": day(0)})
	if got, want := spans(t, editor, q), []string{"4 " + day(-30) + "-" + day(0), "6 " + day(0) + "-"}; !slices.Equal(got, want) {
		t.Errorf("a change from a day: %v, want %v", got, want)
	}

	setTarget(t, editor, map[string]any{"unitId": unit, "fte": 5.5, "from": day(10), "to": day(20)})
	if got, want := spans(t, editor, q), []string{"4 " + day(-30) + "-" + day(0), "6 " + day(0) + "-" + day(10),
		"5.5 " + day(10) + "-" + day(20), "6 " + day(20) + "-"}; !slices.Equal(got, want) {
		t.Errorf("a span inside one for good splits it: %v, want %v", got, want)
	}

	setTarget(t, editor, map[string]any{"unitId": unit, "fte": 3.333, "from": day(-5), "to": day(15)})
	if got, want := spans(t, editor, q), []string{"4 " + day(-30) + "-" + day(-5), "3.33 " + day(-5) + "-" + day(15),
		"5.5 " + day(15) + "-" + day(20), "6 " + day(20) + "-"}; !slices.Equal(got, want) {
		t.Errorf("a span over two ends cuts both: %v, want %v", got, want)
	}

	if got := spans(t, editor, q+"&asOf="+day(16)); !slices.Equal(got, []string{"5.5 " + day(15) + "-" + day(20)}) {
		t.Errorf("the target on a day: %v", got)
	}

	all := setTarget(t, editor, map[string]any{"unitId": unit, "fte": 2, "from": day(-40)})
	if got := spans(t, editor, q); !slices.Equal(got, []string{"2 " + day(-40) + "-"}) {
		t.Errorf("a target for good from before them all replaces them all: %v", got)
	}
	if status := editor.do(http.MethodDelete, "/targets/"+all.ID, nil, nil); status != http.StatusNoContent {
		t.Errorf("removing answered %d", status)
	}
	if got := spans(t, editor, q); len(got) != 0 {
		t.Errorf("after removing: %v", got)
	}
}

func TestTargetsComeWithCapacityAndOnlyEditorsSetThem(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	editor, unit := orgFixture(t)
	member, _ := withRole(t, admin, "member")
	position := positions(t, editor, unit, "Engineer")[0]
	give(t, editor, map[string]any{"personId": somebody(t, editor), "positionId": position, "from": day(-30)})
	role := orgRole(t, editor, "Reviewer")
	setTarget(t, editor, map[string]any{"positionId": position, "fte": 1, "from": day(-10)})
	setTarget(t, editor, map[string]any{"roleId": role.ID, "fte": 2, "from": day(-30)})
	setTarget(t, editor, map[string]any{"unitId": unit, "fte": 3, "from": day(-30), "to": day(-5)})

	byDay := "/capacity?by=day&from=" + day(-12) + "&to=" + day(-1)
	pos := capacityOf(t, editor, "/positions/"+position+byDay)
	if p := on(t, pos, day(-11)); p.Target != nil {
		t.Errorf("no target before it was set: %+v", p)
	}
	if p := on(t, pos, day(-10)); p.Target == nil || *p.Target != 1 || p.FTE != 1 {
		t.Errorf("the position's target: %+v", p)
	}
	if p := on(t, capacityOf(t, editor, "/roles/"+role.ID+byDay), day(-1)); p.Target == nil || *p.Target != 2 {
		t.Errorf("the role's target: %+v", p)
	}
	unitCap := capacityOf(t, editor, "/units/"+unit+byDay)
	if p := on(t, unitCap, day(-6)); p.Target == nil || *p.Target != 3 {
		t.Errorf("the unit's target: %+v", p)
	}
	if p := on(t, unitCap, day(-5)); p.Target != nil {
		t.Errorf("the unit's target has ended: %+v", p)
	}

	if status := member.do(http.MethodPost, "/targets", map[string]any{"unitId": unit, "fte": 9}, nil); status != http.StatusForbidden {
		t.Errorf("a member set a target: %d", status)
	}
	var log auditJSON
	editor.do(http.MethodGet, "/units/"+unit+"/audit", nil, &log)
	if len(log.Events) == 0 || log.Events[0].Table != "target" || log.Events[0].Operation != "insert" || log.Events[0].After["fte"] != 3.0 {
		t.Errorf("the unit's log: %+v", log.Events)
	}
}

func TestATargetIsRefusedWithoutOneSubjectOrASensibleNumber(t *testing.T) {
	editor, unit := orgFixture(t)
	for _, fields := range []map[string]any{
		{"fte": 1},
		{"unitId": unit, "roleId": unit, "fte": 1},
		{"unitId": unit, "fte": -1},
		{"unitId": unit, "fte": 100001},
		{"unitId": unit, "fte": 1, "from": day(5), "to": day(5)},
		{"unitId": "0195a3c4-0000-7000-8000-000000000000", "fte": 1},
		{"positionId": "0195a3c4-0000-7000-8000-000000000000", "fte": 1},
		{"roleId": "0195a3c4-0000-7000-8000-000000000000", "fte": 1},
	} {
		if status := editor.do(http.MethodPost, "/targets", fields, nil); status != http.StatusUnprocessableEntity {
			t.Errorf("%v answered %d", fields, status)
		}
	}
	for _, query := range []string{"", "unitId=nope", "unitId=" + unit + "&asOf=soon"} {
		if status := editor.do(http.MethodGet, "/targets?"+query, nil, nil); status != http.StatusUnprocessableEntity {
			t.Errorf("listing %q answered %d", query, status)
		}
	}
	if status := editor.do(http.MethodDelete, "/targets/0195a3c4-0000-7000-8000-000000000000", nil, nil); status != http.StatusNotFound {
		t.Errorf("removing nothing answered %d", status)
	}
}
