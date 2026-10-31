package org

import (
	"slices"
	"testing"

	"github.com/google/uuid"
)

func TestASubjectsDutiesArePickedFromWhatComesDownToIt(t *testing.T) {
	ana, bo := uuid.New(), uuid.New()
	head, clerk, vacant, deputy := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	role, unit := uuid.New(), uuid.New()
	steps := []PublishedStep{{StepPeople: StepPeople{Key: "approve", Responsibilities: []ResolvedResponsibility{
		{Responsibility{Kind: Accountable, PositionID: &head}, []Resolved{{PersonID: ana, PositionID: &head}}},
		{Responsibility{Kind: Responsible, RoleID: &role}, []Resolved{{PersonID: bo}, {PersonID: ana, PositionID: &head}}},
		{Responsibility{Kind: Consulted, PositionID: &vacant}, []Resolved{{PersonID: bo, PositionID: &deputy, DeputyFor: &vacant}}},
		{Responsibility{Kind: Informed, UnitID: &unit}, nil},
		{Responsibility{Kind: Informed, PositionID: &clerk}, nil},
	}}}}
	kinds := func(s Subject, inUnit map[uuid.UUID]bool) []RACI {
		var out []RACI
		for _, d := range dutiesOf(steps, s, inUnit) {
			out = append(out, d.Kind)
		}
		return out
	}
	for name, c := range map[string]struct {
		s      Subject
		inUnit map[uuid.UUID]bool
		want   []RACI
	}{
		"a person by position and by role":           {Subject{PersonID: &ana}, nil, []RACI{Accountable, Responsible}},
		"a person standing in as deputy, and a role": {Subject{PersonID: &bo}, nil, []RACI{Responsible, Consulted}},
		"a position named and through a role":        {Subject{PositionID: &head}, nil, []RACI{Accountable, Responsible}},
		"a vacant position named, and its deputy":    {Subject{PositionID: &vacant}, nil, []RACI{Consulted}},
		"a deputy standing in":                       {Subject{PositionID: &deputy}, nil, []RACI{Consulted}},
		"a role named":                               {Subject{RoleID: &role}, nil, []RACI{Responsible}},
		"a unit named and a position in it":          {Subject{UnitID: &unit}, map[uuid.UUID]bool{clerk: true}, []RACI{Informed, Informed}},
		"nothing asked":                              {Subject{}, nil, nil},
	} {
		if got := kinds(c.s, c.inUnit); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", name, got, c.want)
		}
	}
}
