package demo

import (
	"testing"
)

// The demo is data; these hold it to what the model would accept and to what the issue promises, before any
// database sees it.
func TestTheDemoOrganisationHangsTogether(t *testing.T) {
	kindNames := map[string]bool{}
	for _, k := range kinds {
		kindNames[k.name] = true
	}
	unitKeys, depth := map[string]bool{}, map[string]int{}
	for _, u := range units {
		if !kindNames[u.kind] {
			t.Errorf("unit %s is of an unknown kind %s", u.key, u.kind)
		}
		if u.parent != "" && !unitKeys[u.parent] {
			t.Errorf("unit %s comes before its parent %s", u.key, u.parent)
		}
		if u.parent != "" {
			depth[u.key] = depth[u.parent] + 1
		}
		unitKeys[u.key] = true
	}
	if levels := maxOf(depth) + 1; levels != 4 {
		t.Errorf("the units are %d levels deep, want 4", levels)
	}

	positionKeys, heads, seats := map[string]bool{}, map[string]int{}, map[string]int{}
	for _, p := range positions {
		if !unitKeys[p.unit] {
			t.Errorf("position %s is in an unknown unit %s", p.key, p.unit)
		}
		if p.head {
			heads[p.unit]++
		}
		seats[p.key] = max(p.seats, 1)
		positionKeys[p.key] = true
	}
	for unit, n := range heads {
		if n > 1 {
			t.Errorf("unit %s has %d heads", unit, n)
		}
	}
	for _, u := range units {
		if heads[u.key] == 0 {
			t.Errorf("unit %s has no head for its people to report to", u.key)
		}
	}
	dotted := 0
	for _, p := range positions {
		for _, d := range p.dottedTo {
			dotted++
			if !positionKeys[d] {
				t.Errorf("position %s has a dotted line to an unknown %s", p.key, d)
			}
		}
	}
	if dotted < 3 {
		t.Errorf("the matrix has %d dotted lines", dotted)
	}

	personKeys := map[string]bool{}
	for _, p := range people {
		if personKeys[p.key] {
			t.Errorf("person %s is there twice", p.key)
		}
		personKeys[p.key] = true
	}
	if n := len(people); n < 55 || n > 65 {
		t.Errorf("the demo has %d people, want about sixty", n)
	}
	for _, realm := range []string{"alice", "bob", "carol", "dave"} {
		if !personKeys[realm] {
			t.Errorf("the realm's %s is not in the demo", realm)
		}
	}

	roleKeys := map[string]bool{}
	for _, r := range roles {
		roleKeys[r.key] = true
		for _, p := range r.carriedBy {
			if !positionKeys[p] {
				t.Errorf("role %s is carried by an unknown %s", r.key, p)
			}
		}
	}
	// Holdings of a position are counted where they overlap: from the start, or from the handover on.
	taken := map[string]int{}
	for _, h := range holdings {
		if !personKeys[h.person] {
			t.Errorf("an unknown %s holds %s%s", h.person, h.position, h.role)
		}
		switch {
		case (h.position == "") == (h.role == ""):
			t.Errorf("%s holds a position and a role at once, or neither", h.person)
		case h.position != "" && !positionKeys[h.position]:
			t.Errorf("%s holds an unknown position %s", h.person, h.position)
		case h.role != "" && !roleKeys[h.role]:
			t.Errorf("%s holds an unknown role %s", h.person, h.role)
		case h.position != "" && h.to == "":
			taken[h.position]++
		}
		for _, d := range []string{h.from, h.to} {
			if d != "" {
				mustDay(d)
			}
		}
	}
	var vacant []string
	for _, p := range positions {
		if taken[p.key] > seats[p.key] {
			t.Errorf("%s has %d holders for %d seats", p.key, taken[p.key], seats[p.key])
		}
		if taken[p.key] == 0 {
			vacant = append(vacant, p.key)
		}
	}
	if len(vacant) != 2 {
		t.Errorf("the vacancies are %v, want two", vacant)
	}
	for person := range leavers {
		if !personKeys[person] {
			t.Errorf("an unknown %s leaves", person)
		}
	}
	for position, stand := range deputies {
		for _, d := range append([]string{position}, stand...) {
			if !positionKeys[d] {
				t.Errorf("deputies name an unknown position %s", d)
			}
		}
	}
	if *ptr(3) != 3 {
		t.Error("ptr")
	}
}

func maxOf(m map[string]int) int {
	out := 0
	for _, v := range m {
		out = max(out, v)
	}
	return out
}
