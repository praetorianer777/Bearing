package httpapi

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/armature"
	"github.com/praetorianer777/bearing/backend/internal/org"
)

func TestAWeekStartsOnMonday(t *testing.T) {
	for day, want := range map[string]string{"2026-11-02": "2026-11-02", "2026-11-01": "2026-10-26", "2026-11-07": "2026-11-02"} {
		d, _ := time.Parse(time.DateOnly, day)
		if got := monday(d).Format(time.DateOnly); got != want {
			t.Errorf("%s: %s", day, got)
		}
	}
}

func TestEveryRefusalOfArmatureIsSaid(t *testing.T) {
	for err, want := range map[error]string{
		nil:                       "ok",
		armature.ErrNotConfigured: "not_configured",
		armature.ErrNotConnected:  "not_connected",
		armature.ErrRejected:      "rejected",
		armature.ErrUnreachable:   "unreachable",
		&armature.OutdatedError{Capability: "resource-load@1"}: "outdated",
		errors.New("anything else"):                            "unreachable",
	} {
		if got := partnerOf(err); got.Status != want || (err != nil && got.Message == "") {
			t.Errorf("%v: %+v", err, got)
		}
	}
}

func TestDemandCountsEachPersonAtTheirShare(t *testing.T) {
	issuer, full, half, gone := "https://realm", "full", "half", "gone"
	week := org.Day{}
	start, _ := org.ParseDay("2026-11-02")
	week = start
	people := []weekPeople{{
		uuid.MustParse("00000000-0000-0000-0000-000000000001"): {PersonID: uuid.MustParse("00000000-0000-0000-0000-000000000001"), Name: "Full", Issuer: &issuer, Subject: &full, Share: 100},
		uuid.MustParse("00000000-0000-0000-0000-000000000002"): {PersonID: uuid.MustParse("00000000-0000-0000-0000-000000000002"), Name: "Half", Issuer: &issuer, Subject: &half, Share: 50},
		uuid.MustParse("00000000-0000-0000-0000-000000000003"): {PersonID: uuid.MustParse("00000000-0000-0000-0000-000000000003"), Name: "Gone", Issuer: &issuer, Subject: &gone, Share: 100},
		uuid.MustParse("00000000-0000-0000-0000-000000000004"): {PersonID: uuid.MustParse("00000000-0000-0000-0000-000000000004"), Name: "Local", Share: 100},
	}}
	loads := map[armature.Subject]armature.PersonLoad{
		{Issuer: issuer, Subject: full}: {Found: true, Weeks: []armature.Week{{Start: "2026-11-02", AllocationPercent: 80, NominalHours: 40, LoadHours: 32, DaysAway: 1}}},
		{Issuer: issuer, Subject: half}: {Found: true, Weeks: []armature.Week{{Start: "2026-11-02", AllocationPercent: 110, NominalHours: 40, LoadHours: 10}}},
		{Issuer: issuer, Subject: gone}: {Found: false},
	}
	weeks, over, unmatched := demandOf([]demandWeekResponse{{Start: week}}, people, loads)
	w := weeks[0]
	if w.People != 4 || *w.AllocatedFTE != 1.35 || *w.LoadHours != 37 || *w.NominalHours != 60 || *w.DaysAway != 1 {
		t.Errorf("the week is %+v", w)
	}
	if len(over) != 1 || over[0].Name != "Half" || over[0].Reason != "over_allocated" {
		t.Errorf("the warnings are %+v", over)
	}
	if len(unmatched) != 2 {
		t.Errorf("the unmatched are %+v", unmatched)
	}
	weeks, over, _ = demandOf([]demandWeekResponse{{Start: week}}, people, nil)
	if weeks[0].People != 4 || weeks[0].AllocatedFTE != nil || over != nil {
		t.Errorf("without Armature the week is %+v %+v", weeks[0], over)
	}
}
