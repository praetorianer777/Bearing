package org

import (
	"slices"
	"testing"

	"github.com/google/uuid"
)

func TestASubtreeComesBreadthFirstWithItsDepths(t *testing.T) {
	root, a, b, c, other := Unit{ID: uuid.New(), Name: "Root"}, uuid.New(), uuid.New(), uuid.New(), uuid.New()
	under := func(id uuid.UUID, name string, parent uuid.UUID) Unit {
		return Unit{ID: id, Name: name, ParentID: &parent}
	}
	all := []Unit{root, under(c, "C", b), under(a, "A", root.ID), under(b, "B", root.ID), under(uuid.New(), "Elsewhere", other)}
	units, depths := subtree(root, all)
	var got []string
	for _, u := range units {
		got = append(got, u.Name)
	}
	if !slices.Equal(got, []string{"Root", "A", "B", "C"}) || !slices.Equal(depths, []int{0, 1, 1, 2}) {
		t.Errorf("the subtree is %v at %v", got, depths)
	}
	if units, _ := subtree(Unit{ID: c}, all); len(units) != 1 {
		t.Errorf("a leaf's subtree is %v", units)
	}
}

func TestManagersAreThoseExactlySoManyLevelsUp(t *testing.T) {
	chain := []Link{{Title: "Lead", Depth: 1}, {Title: "Other lead", Depth: 1}, {Title: "CTO", Depth: 2}, {Title: "CEO", Depth: 3}}
	for levels, want := range map[int][]string{1: {"Lead", "Other lead"}, 2: {"CTO"}, 3: {"CEO"}, 4: nil} {
		var got []string
		for _, m := range atLevel(chain, levels) {
			got = append(got, m.Title)
		}
		if !slices.Equal(got, want) {
			t.Errorf("%d levels up: %v, want %v", levels, got, want)
		}
	}
}

func TestManagersCountFromOne(t *testing.T) {
	if _, err := (&Service{}).Managers(t.Context(), uuid.New(), Day{}, 0); err != ErrNoLevels {
		t.Errorf("no levels: %v", err)
	}
}
