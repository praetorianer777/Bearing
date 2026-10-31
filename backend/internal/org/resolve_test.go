package org

import (
	"slices"
	"testing"

	"github.com/google/uuid"
)

func names(people []Resolved) []string {
	out := make([]string, len(people))
	for i, p := range people {
		out[i] = p.Name
		if p.DeputyFor != nil {
			out[i] += " (deputy)"
		}
	}
	return out
}

func TestAResponsibilityComesDownToPeopleWithDeputiesForVacancies(t *testing.T) {
	head, vacant, firstDeputy, secondDeputy, carrier, emptyCarrier := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	role, nobodysRole, unit, headless := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	held := func(position uuid.UUID, name string) Resolved {
		return Resolved{PersonID: uuid.New(), Name: name, Share: 100, PositionID: &position}
	}
	w := standing{
		holders: map[uuid.UUID][]Resolved{
			head:         {held(head, "Hana Head")},
			secondDeputy: {held(secondDeputy, "Dana Deputy"), held(secondDeputy, "Dora Deputy")},
			carrier:      {held(carrier, "Carl Carrier")},
		},
		deputies:    map[uuid.UUID][]uuid.UUID{vacant: {firstDeputy, secondDeputy}, emptyCarrier: {head}, firstDeputy: {head}},
		roleHolders: map[uuid.UUID][]Resolved{role: {{PersonID: uuid.New(), Name: "Rita Direct", Share: 20}}},
		carriers:    map[uuid.UUID][]uuid.UUID{role: {carrier, emptyCarrier}},
		heads:       map[uuid.UUID]uuid.UUID{unit: vacant},
	}
	for name, c := range map[string]struct {
		r    Responsibility
		want []string
	}{
		"a held position":                          {Responsibility{PositionID: &head}, []string{"Hana Head"}},
		"a vacant position, its first held deputy": {Responsibility{PositionID: &vacant}, []string{"Dana Deputy (deputy)", "Dora Deputy (deputy)"}},
		"a vacant deputy, not its own deputies":    {Responsibility{PositionID: &firstDeputy}, []string{"Hana Head (deputy)"}},
		"a role, directly and through positions":   {Responsibility{RoleID: &role}, []string{"Rita Direct", "Carl Carrier", "Hana Head (deputy)"}},
		"a role nobody has":                        {Responsibility{RoleID: &nobodysRole}, []string{}},
		"a unit, by its head":                      {Responsibility{UnitID: &unit}, []string{"Dana Deputy (deputy)", "Dora Deputy (deputy)"}},
		"a unit without a head":                    {Responsibility{UnitID: &headless}, []string{}},
		"a deputy in their own position":           {Responsibility{PositionID: &secondDeputy}, []string{"Dana Deputy", "Dora Deputy"}},
		"nothing named":                            {Responsibility{}, []string{}},
	} {
		if got := names(w.resolve(c.r)); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", name, got, c.want)
		}
	}
	if got := w.position(vacant); *got[0].DeputyFor != vacant || *got[0].PositionID != secondDeputy {
		t.Errorf("a deputy standing in names the vacant position and their own: %+v", got[0])
	}
	if w.holders[secondDeputy][0].DeputyFor != nil {
		t.Error("standing in changed the deputy's own holding")
	}
}
