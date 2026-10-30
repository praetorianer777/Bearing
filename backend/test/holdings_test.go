//go:build integration

package test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

type holdingJSON struct {
	ID         string
	PersonID   string
	PositionID string
	RoleID     *string
	Share      int
	From       string
	To         *string
}

type holdingBody struct{ Holding holdingJSON }

func give(t *testing.T, b *browser, fields map[string]any) holdingJSON {
	t.Helper()
	var made holdingBody
	if status := b.do(http.MethodPost, "/holdings", fields, &made); status != http.StatusCreated {
		t.Fatalf("giving %v answered %d", fields, status)
	}
	return made.Holding
}

func vacant(t *testing.T, b *browser, unit, position, on string) bool {
	t.Helper()
	var list struct{ Positions []positionJSON }
	b.do(http.MethodGet, "/positions?vacant=true&unitId="+unit+"&asOf="+on, nil, &list)
	for _, p := range list.Positions {
		if p.ID == position {
			return true
		}
	}
	return false
}

func somebody(t *testing.T, b *browser) string {
	return person(t, b, map[string]any{"name": "Holder " + uuid.NewString()[:6]}).ID
}

func TestHoldingsMayNotOverlapBeyondTheSeats(t *testing.T) {
	editor, unit := orgFixture(t)
	position := positions(t, editor, unit, "Developer")[0]
	ana, ben := somebody(t, editor), somebody(t, editor)

	give(t, editor, map[string]any{"personId": ana, "positionId": position, "share": 50, "from": day(-10)})
	var r refusal
	if status := editor.do(http.MethodPost, "/holdings", map[string]any{"personId": ana, "positionId": position, "from": day(-5), "to": day(5)}, &r); status != http.StatusConflict || r.Error.Code != "holding_overlaps" {
		t.Errorf("holding the same position twice at once answered %d %+v", status, r)
	}
	if status := editor.do(http.MethodPost, "/holdings", map[string]any{"personId": ben, "positionId": position, "from": day(-5)}, &r); status != http.StatusConflict || r.Error.Code != "seats_taken" {
		t.Errorf("a second holder of a single seat answered %d %+v", status, r)
	}
	if status := editor.do(http.MethodPatch, "/positions/"+position, map[string]any{"seats": 2}, nil); status != http.StatusOK {
		t.Fatalf("a second seat answered %d", status)
	}
	give(t, editor, map[string]any{"personId": ben, "positionId": position, "share": 50, "from": day(-5)})
	if p := positionOn(t, editor, position, day(0)); len(p.Holders) != 2 || p.Seats != 2 {
		t.Errorf("the shared position has %d holders and %d seats", len(p.Holders), p.Seats)
	}
	if p := positionOn(t, editor, position, day(-7)); len(p.Holders) != 1 || p.Holders[0].PersonID != ana {
		t.Errorf("before ben came the position was held by %+v", p.Holders)
	}
	if status := editor.do(http.MethodPatch, "/positions/"+position, map[string]any{"seats": 1}, &r); status != http.StatusConflict || r.Error.Code != "too_few_seats" {
		t.Errorf("taking a seat both still hold answered %d %+v", status, r)
	}
	if status := editor.do(http.MethodPost, "/holdings", map[string]any{"personId": ana, "positionId": position, "share": 0}, &r); status != http.StatusUnprocessableEntity {
		t.Errorf("a share of nothing answered %d", status)
	}
}

func TestAPositionNobodyHoldsIsVacant(t *testing.T) {
	editor, unit := orgFixture(t)
	position := positions(t, editor, unit, "Tester")[0]
	if !vacant(t, editor, unit, position, day(0)) {
		t.Fatal("a position nobody ever held is not vacant")
	}
	held := give(t, editor, map[string]any{"personId": somebody(t, editor), "positionId": position, "from": day(-20)})
	if vacant(t, editor, unit, position, day(0)) {
		t.Error("a held position is vacant")
	}
	var ended holdingBody
	if status := editor.do(http.MethodPost, "/holdings/"+held.ID+"/end", map[string]any{"on": day(3)}, &ended); status != http.StatusOK || ended.Holding.To == nil {
		t.Fatalf("ending the holding answered %d %+v", status, ended.Holding)
	}
	if vacant(t, editor, unit, position, day(2)) || !vacant(t, editor, unit, position, day(3)) {
		t.Error("the position is not vacant from the day its holding ends, and only then")
	}
	if !vacant(t, editor, unit, position, day(-25)) {
		t.Error("the position is not vacant before anybody held it")
	}
	var r refusal
	if status := editor.do(http.MethodPost, "/holdings/"+held.ID+"/end", map[string]any{"on": day(5)}, &r); status != http.StatusConflict || r.Error.Code != "ended" {
		t.Errorf("ending it after it ended answered %d %+v", status, r)
	}
}

func TestAHoldingPlannedAheadStartsOnItsDay(t *testing.T) {
	editor, unit := orgFixture(t)
	position := positions(t, editor, unit, "Architect")[0]
	current, next := somebody(t, editor), somebody(t, editor)
	now := give(t, editor, map[string]any{"personId": current, "positionId": position, "from": day(-30), "to": day(14)})
	planned := give(t, editor, map[string]any{"personId": next, "positionId": position, "share": 80, "from": day(14)})

	if p := positionOn(t, editor, position, day(0)); len(p.Holders) != 1 || p.Holders[0].ID != now.ID {
		t.Errorf("today the position is held by %+v", p.Holders)
	}
	if p := positionOn(t, editor, position, day(14)); len(p.Holders) != 1 || p.Holders[0].ID != planned.ID || p.Holders[0].Share != 80 {
		t.Errorf("from the planned day the position is held by %+v", p.Holders)
	}
	var list struct{ Holdings []holdingJSON }
	editor.do(http.MethodGet, "/holdings?positionId="+position, nil, &list)
	if len(list.Holdings) != 2 || list.Holdings[0].ID != now.ID {
		t.Errorf("the position's holdings are %+v", list.Holdings)
	}
	editor.do(http.MethodGet, "/holdings?personId="+next+"&asOf="+day(0), nil, &list)
	if len(list.Holdings) != 0 {
		t.Errorf("the next holder holds %+v today", list.Holdings)
	}
	var r refusal
	if status := editor.do(http.MethodPost, "/holdings/"+planned.ID+"/end", map[string]any{"on": day(0)}, &r); status != http.StatusConflict || r.Error.Code != "not_started" {
		t.Errorf("ending a holding before it starts answered %d %+v", status, r)
	}
	var changed holdingBody
	if status := editor.do(http.MethodPatch, "/holdings/"+planned.ID, map[string]any{"share": 100}, &changed); status != http.StatusOK || changed.Holding.Share != 100 {
		t.Errorf("changing the share answered %d %+v", status, changed.Holding)
	}
	if status := editor.do(http.MethodDelete, "/holdings/"+planned.ID, nil, nil); status != http.StatusNoContent {
		t.Errorf("calling off the planned holding answered %d", status)
	}
	if !vacant(t, editor, unit, position, day(14)) {
		t.Error("the position is not vacant once the planned holding is called off")
	}
}

func TestHoldingsEndWithThePersonAndThePosition(t *testing.T) {
	editor, unit := orgFixture(t)
	ids := positions(t, editor, unit, "Buyer", "Seller")
	buyer, seller := ids[0], ids[1]
	leaving, staying := somebody(t, editor), somebody(t, editor)
	held := give(t, editor, map[string]any{"personId": leaving, "positionId": buyer, "from": day(-20)})
	later := give(t, editor, map[string]any{"personId": leaving, "positionId": seller, "from": day(20)})

	if status := editor.do(http.MethodPost, "/people/"+leaving+"/leave", map[string]any{"on": day(10)}, nil); status != http.StatusOK {
		t.Fatalf("leaving answered %d", status)
	}
	var got holdingBody
	editor.do(http.MethodGet, "/holdings/"+held.ID, nil, &got)
	if got.Holding.To == nil || *got.Holding.To != day(10) {
		t.Errorf("the leaver's holding ends %v, want the day they leave", got.Holding.To)
	}
	if status := editor.do(http.MethodGet, "/holdings/"+later.ID, nil, nil); status != http.StatusNotFound {
		t.Errorf("a holding planned after leaving is still there: %d", status)
	}
	var r refusal
	if status := editor.do(http.MethodPost, "/holdings", map[string]any{"personId": leaving, "positionId": seller, "from": day(12)}, &r); status != http.StatusConflict || r.Error.Code != "person_not_there" {
		t.Errorf("a holding after leaving answered %d %+v", status, r)
	}
	// Without an end, a holding runs until the person is gone.
	if h := give(t, editor, map[string]any{"personId": leaving, "positionId": seller, "from": day(0)}); h.To == nil || *h.To != day(10) {
		t.Errorf("an open holding of a leaver ends %v", h.To)
	}

	stays := give(t, editor, map[string]any{"personId": staying, "positionId": buyer, "from": day(10)})
	if status := editor.do(http.MethodPost, "/positions/"+buyer+"/archive", map[string]any{"on": day(15)}, nil); status != http.StatusOK {
		t.Fatalf("archiving answered %d", status)
	}
	editor.do(http.MethodGet, "/holdings/"+stays.ID, nil, &got)
	if got.Holding.To == nil || *got.Holding.To != day(15) {
		t.Errorf("the holding of an archived position ends %v, want the day it is archived", got.Holding.To)
	}
	if status := editor.do(http.MethodPost, "/holdings", map[string]any{"personId": staying, "positionId": buyer, "from": day(15)}, &r); status != http.StatusConflict || r.Error.Code != "position_not_there" {
		t.Errorf("holding an archived position answered %d %+v", status, r)
	}
	if status := editor.do(http.MethodPost, "/holdings", map[string]any{"personId": uuid.NewString(), "positionId": seller}, &r); status != http.StatusUnprocessableEntity {
		t.Errorf("a holding of nobody answered %d", status)
	}
}
