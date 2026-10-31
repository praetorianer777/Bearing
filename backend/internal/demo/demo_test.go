package demo

import (
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/org"
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

// Every published version of a demo process passes the rules, and the one left a draft shows what they catch.
func TestTheDemoProcessesHangTogether(t *testing.T) {
	ids := map[string]uuid.UUID{}
	titles := map[string]bool{}
	known := parties{positions: map[string]uuid.UUID{}, units: map[string]uuid.UUID{}, roles: map[string]uuid.UUID{}}
	for _, p := range positions {
		titles[p.title] = true
		known.positions[p.title] = uuid.New()
	}
	for _, u := range units {
		known.units[u.name] = uuid.New()
	}
	for _, r := range roles {
		known.roles[r.key] = uuid.New()
	}
	graph := func(f flow) org.Graph {
		g, err := f.graph(ids, known)
		if err != nil {
			t.Fatal(err)
		}
		return g
	}
	for _, p := range processes {
		ids[p.key] = uuid.New()
		if !titles[p.owner] {
			t.Errorf("process %s is owned by an unknown position %s", p.key, p.owner)
		}
	}
	unpublished := 0
	for _, p := range processes {
		for i, f := range p.versions {
			if problems := org.Check(ids[p.key], graph(f)); len(problems) > 0 {
				t.Errorf("version %d of %s has %+v", i+1, p.key, problems)
			}
		}
		if len(p.versions) == 0 {
			unpublished++
			problems := org.Check(ids[p.key], graph(*p.draft))
			if !org.Blocking(problems) || !slices.ContainsFunc(problems, func(pr org.Problem) bool { return pr.Code == "no_accountable" }) {
				t.Errorf("the draft-only %s should both be kept from publishing and warn of a step nobody answers for: %+v", p.key, problems)
			}
		}
	}
	if unpublished != 1 {
		t.Errorf("%d processes are never published, want 1", unpublished)
	}
	g := graph(processes[1].versions[1])
	for _, n := range g.Nodes {
		if n.Kind == org.Subprocess && (n.SubprocessID == nil || *n.SubprocessID != ids["production-run"]) {
			t.Errorf("the subprocess node %s stands for %v", n.Key, n.SubprocessID)
		}
	}
	rows := map[float64][]float64{}
	for _, n := range g.Nodes {
		rows[n.Y] = append(rows[n.Y], n.X)
	}
	if len(rows[0]) != 1 || rows[0][0] != -widths[org.Start]/2 {
		t.Errorf("the start is not alone at the top middle: %v", rows[0])
	}
	for y, xs := range rows {
		if len(xs) != len(slices.Compact(slices.Sorted(slices.Values(xs)))) {
			t.Errorf("nodes on the row at %v lie on each other: %v", y, xs)
		}
	}
}
