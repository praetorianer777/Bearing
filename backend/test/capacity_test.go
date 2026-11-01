//go:build integration

package test

import (
	"net/http"
	"testing"
)

type capacityPeriod struct {
	From, To              string
	FTE                   float64
	Seats, Filled, Vacant *float64
	People                int
}

type capacityBody struct {
	From, To, By string
	SubUnits     bool
	Periods      []capacityPeriod
}

func capacityOf(t *testing.T, b *browser, path string) capacityBody {
	t.Helper()
	var out capacityBody
	if status := b.do(http.MethodGet, path, nil, &out); status != http.StatusOK {
		t.Fatalf("%s answered %d", path, status)
	}
	return out
}

// on is the period of a day's capacity, by day.
func on(t *testing.T, c capacityBody, d string) capacityPeriod {
	t.Helper()
	for _, p := range c.Periods {
		if p.From == d {
			return p
		}
	}
	t.Fatalf("no period starts on %s in %+v", d, c.Periods)
	return capacityPeriod{}
}

func seats(p capacityPeriod) [3]float64 {
	if p.Seats == nil || p.Filled == nil || p.Vacant == nil {
		return [3]float64{-1, -1, -1}
	}
	return [3]float64{*p.Seats, *p.Filled, *p.Vacant}
}

// Sales with a head, and inside it a team with a rep position of two seats: one held at half a week until it ends,
// another started later; the head leaves; a role held directly and carried by the rep position from a day.
func TestCapacityAddsUpHoldingsDayByDay(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	editor, _ := withRole(t, admin, "editor")
	chain := units(t, editor, unitType(t, admin), rootUnit(t, admin), "Sales", "Inside sales")
	sales, inside := chain[0], chain[1]
	head := positions(t, editor, sales, "Head of Sales")[0]
	rep := positions(t, editor, inside, "Sales rep")[0]
	editor.do(http.MethodPatch, "/positions/"+rep, map[string]any{"seats": 2}, nil)
	ann, bo, cy, di := somebody(t, editor), somebody(t, editor), somebody(t, editor), somebody(t, editor)
	give(t, editor, map[string]any{"personId": ann, "positionId": head, "from": day(-10)})
	give(t, editor, map[string]any{"personId": bo, "positionId": rep, "share": 50, "from": day(-20), "to": day(-5)})
	give(t, editor, map[string]any{"personId": cy, "positionId": rep, "from": day(-3)})
	if status := editor.do(http.MethodPost, "/people/"+ann+"/leave", map[string]any{"on": day(-2)}, nil); status != http.StatusOK {
		t.Fatalf("leaving answered %d", status)
	}
	approver := orgRole(t, editor, "Deal approver")
	give(t, editor, map[string]any{"personId": di, "roleId": approver.ID, "share": 20, "from": day(-20)})
	if status := editor.do(http.MethodPost, "/positions/"+rep+"/roles", map[string]any{"roleId": approver.ID, "from": day(-10)}, nil); status != http.StatusCreated {
		t.Fatalf("carrying the role answered %d", status)
	}
	byDay := "?by=day&from=" + day(-20) + "&to=" + day(-1)

	whole := capacityOf(t, editor, "/units/"+sales+"/capacity"+byDay+"&subUnits=true")
	if len(whole.Periods) != 20 || !whole.SubUnits || whole.By != "day" {
		t.Fatalf("twenty days with the sub-units: %+v", whole)
	}
	for _, c := range []struct {
		day    int
		fte    float64
		seats  [3]float64
		people int
	}{
		{-15, 0.5, [3]float64{3, 0.5, 2.5}, 1},
		{-8, 1.5, [3]float64{3, 1.5, 1.5}, 2},
		{-4, 1, [3]float64{3, 1, 2}, 1},
		{-1, 1, [3]float64{3, 1, 2}, 1},
	} {
		if p := on(t, whole, day(c.day)); p.FTE != c.fte || seats(p) != c.seats || p.People != c.people {
			t.Errorf("Sales with its team on day %d: %+v %v", c.day, p, seats(p))
		}
	}
	own := capacityOf(t, editor, "/units/"+sales+"/capacity"+byDay)
	if p := on(t, own, day(-8)); p.FTE != 1 || seats(p) != [3]float64{1, 1, 0} {
		t.Errorf("Sales alone: %+v", p)
	}
	if p := on(t, own, day(-15)); p.FTE != 0 || seats(p) != [3]float64{1, 0, 1} {
		t.Errorf("Sales alone before its head came: %+v", p)
	}

	repCap := capacityOf(t, editor, "/positions/"+rep+"/capacity"+byDay)
	if p := on(t, repCap, day(-15)); p.FTE != 0.5 || seats(p) != [3]float64{2, 0.5, 1.5} {
		t.Errorf("a seat held at half a week: %+v", p)
	}

	role := capacityOf(t, editor, "/roles/"+approver.ID+"/capacity"+byDay)
	for _, c := range []struct {
		day    int
		fte    float64
		people int
	}{{-15, 0.2, 1}, {-8, 0.7, 2}, {-4, 0.2, 1}, {-1, 1.2, 2}} {
		if p := on(t, role, day(c.day)); p.FTE != c.fte || p.People != c.people || p.Seats != nil {
			t.Errorf("the role on day %d: %+v", c.day, p)
		}
	}

	leaver := capacityOf(t, editor, "/people/"+ann+"/capacity"+byDay)
	if p := on(t, leaver, day(-3)); p.FTE != 1 || p.People != 1 {
		t.Errorf("the head before leaving: %+v", p)
	}
	if p := on(t, leaver, day(-2)); p.FTE != 0 || p.People != 0 {
		t.Errorf("the head on the day they left: %+v", p)
	}
}

func TestCapacityFollowsTheTreeAsItStandsEachDay(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	editor, _ := withRole(t, admin, "editor")
	typeID := unitType(t, admin)
	root := rootUnit(t, admin)
	sales := units(t, editor, typeID, root, "Sales")[0]
	marketing := units(t, editor, typeID, root, "Marketing")[0]
	team := units(t, editor, typeID, sales, "Campaigns")[0]
	lead := positions(t, editor, team, "Campaign lead")[0]
	give(t, editor, map[string]any{"personId": somebody(t, editor), "positionId": lead, "from": day(-20)})
	if status := editor.do(http.MethodPost, "/units/"+team+"/move", map[string]any{"parentId": marketing, "from": day(-5)}, nil); status != http.StatusOK {
		t.Fatalf("moving answered %d", status)
	}
	byDay := "/capacity?by=day&subUnits=true&from=" + day(-10) + "&to=" + day(-1)
	before, after := day(-6), day(-5)
	if s, m := capacityOf(t, editor, "/units/"+sales+byDay), capacityOf(t, editor, "/units/"+marketing+byDay); on(t, s, before).FTE != 1 ||
		on(t, s, after).FTE != 0 || on(t, m, before).FTE != 0 || on(t, m, after).FTE != 1 {
		t.Errorf("the team counts where it sits: sales %+v, marketing %+v", s.Periods, m.Periods)
	}
}

func TestCapacityByWeekAndMonthIsTheMeanOfWorkingDays(t *testing.T) {
	editor, unit := orgFixture(t)
	position := positions(t, editor, unit, "Analyst")[0]
	give(t, editor, map[string]any{"personId": somebody(t, editor), "positionId": position, "share": 80, "from": day(-30)})
	weeks := capacityOf(t, editor, "/positions/"+position+"/capacity?from="+day(-20)+"&to="+day(-1))
	if weeks.By != "week" || len(weeks.Periods) < 3 || weeks.Periods[0].From != day(-20) || weeks.Periods[len(weeks.Periods)-1].To != day(-1) {
		t.Fatalf("weeks cut by the range: %+v", weeks)
	}
	for _, p := range weeks.Periods {
		if p.FTE != 0.8 || seats(p) != [3]float64{1, 0.8, 0.2} {
			t.Errorf("a week: %+v", p)
		}
	}
	months := capacityOf(t, editor, "/positions/"+position+"/capacity?by=month&from="+day(-20)+"&to="+day(-1))
	if len(months.Periods) < 1 || len(months.Periods) > 2 || months.Periods[0].FTE != 0.8 {
		t.Errorf("months: %+v", months)
	}
	if def := capacityOf(t, editor, "/positions/"+position+"/capacity"); def.From != day(0) || def.To != day(90) {
		t.Errorf("thirteen weeks from today when nothing is given: %s to %s", def.From, def.To)
	}
}

func TestCapacityRefusesWhatItCannotCount(t *testing.T) {
	editor, unit := orgFixture(t)
	for path, want := range map[string]int{
		"/units/" + unit + "/capacity?by=year":                            http.StatusUnprocessableEntity,
		"/units/" + unit + "/capacity?from=soon":                          http.StatusUnprocessableEntity,
		"/units/" + unit + "/capacity?from=" + day(0) + "&to=" + day(-1):  http.StatusUnprocessableEntity,
		"/units/" + unit + "/capacity?from=" + day(0) + "&to=" + day(731): http.StatusUnprocessableEntity,
		"/units/0195a3c4-0000-7000-8000-000000000000/capacity":            http.StatusNotFound,
		"/roles/0195a3c4-0000-7000-8000-000000000000/capacity":            http.StatusNotFound,
		"/people/0195a3c4-0000-7000-8000-000000000000/capacity":           http.StatusNotFound,
		"/positions/0195a3c4-0000-7000-8000-000000000000/capacity":        http.StatusNotFound,
		"/units/" + unit + "/capacity?from=" + day(0) + "&to=" + day(730): http.StatusOK,
	} {
		if status := editor.do(http.MethodGet, path, nil, nil); status != want {
			t.Errorf("%s answered %d, want %d", path, status, want)
		}
	}
}
