package orgcsv

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/org"
)

func mustDay(t *testing.T, s string) org.Day {
	t.Helper()
	d, err := org.ParseDay(s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

type world struct {
	st                 state
	root, quality, lab uuid.UUID
	hoq, analyst       uuid.UUID
	ana                uuid.UUID
	dept, team         uuid.UUID
}

func demo(t *testing.T) world {
	w := world{root: uuid.New(), quality: uuid.New(), lab: uuid.New(), hoq: uuid.New(), analyst: uuid.New(), ana: uuid.New(), dept: uuid.New(), team: uuid.New()}
	short := "QA"
	email := "ana@kestrel.test"
	w.st = state{
		types: []org.UnitType{{ID: w.dept, Name: "Department"}, {ID: w.team, Name: "Team"}},
		units: []org.Unit{
			{ID: w.root, TypeID: w.dept, Name: "Kestrel"},
			{ID: w.quality, TypeID: w.dept, Name: "Quality", ShortName: &short, ParentID: &w.root},
			{ID: w.lab, TypeID: w.team, Name: "Lab", ParentID: &w.quality},
		},
		positions: []org.Position{
			{ID: w.hoq, UnitID: w.quality, Title: "Head of Quality", HeadOfUnit: true, Seats: 1},
			{ID: w.analyst, UnitID: w.quality, Title: "Analyst", Seats: 2, ReportsTo: &w.hoq},
		},
		people:   []org.Person{{ID: w.ana, Name: "Ana Analyst", Email: &email}},
		holdings: []org.Holding{{ID: uuid.New(), PersonID: w.ana, PositionID: &w.analyst, Share: 100, From: mustDay(t, "2025-01-01")}},
	}
	return w
}

func changes(p *plan) []string {
	var out []string
	for _, c := range p.Changes {
		out = append(out, fmt.Sprintf("%s %d %s %s", c.File, c.Row, c.Action, c.Name))
	}
	return out
}

func errorsOf(p *plan) []string {
	var out []string
	for _, e := range p.Errors {
		out = append(out, fmt.Sprintf("%s %d %s", e.File, e.Row, e.Message))
	}
	return out
}

func TestRowsNamingWhatExistsChangeItOrLeaveIt(t *testing.T) {
	w := demo(t)
	files := Files{
		Units: "ref,name,short_name,type,parent_ref\n" +
			w.quality.String() + ",Quality,QA,Department," + w.root.String() + "\n" +
			w.lab.String() + ",Test lab,,Team," + w.root.String() + "\n",
		Positions: "ref,unit_ref,title,seats,head_of_unit,reports_to_ref\n" +
			w.hoq.String() + "," + w.quality.String() + ",Head of Quality,1,true,\n" +
			w.analyst.String() + "," + w.quality.String() + ",Analyst,3,false," + w.hoq.String() + "\n",
		People:   "ref,name,email,location\nanyone,Ana Analyst,ANA@kestrel.test,Leeds\n",
		Holdings: "person_ref,position_ref,share,from\nana@kestrel.test," + w.analyst.String() + ",100,2025-01-01\n",
	}
	p := planned(w.st, files, mustDay(t, "2026-11-02"))
	if len(p.Errors) != 0 {
		t.Fatalf("errors: %v", errorsOf(p))
	}
	want := []string{
		"units 2 unchanged Quality", "units 3 update Test lab",
		"positions 2 unchanged Head of Quality", "positions 3 update Analyst",
		"people 2 update Ana Analyst",
		"holdings 2 unchanged ana@kestrel.test / " + w.analyst.String(),
	}
	if got := changes(p); !slices.Equal(got, want) {
		t.Errorf("changes:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	// The lab is renamed and moved, the analyst given a seat, Ana a location.
	if len(p.steps) != 4 {
		t.Errorf("%d steps, want 4", len(p.steps))
	}
	if counts := p.Counts(); counts["update"] != 3 || counts["unchanged"] != 3 {
		t.Errorf("counts: %v", counts)
	}
}

func TestRowsWithNewRefsMakeWhatTheyName(t *testing.T) {
	w := demo(t)
	files := Files{
		Units: "ref,name,type,parent_ref,description\n" +
			"bench,Bench,Team,field,On the bench.\n" +
			"field,Field,Department," + w.root.String() + ",\n",
		Positions: "ref,unit_ref,title,seats,reports_to_ref,from\n" +
			"tester,bench,Tester,2,lead,2026-12-01\n" +
			"lead,bench,Bench lead,,," + "\n",
		People:   "ref,name,email,phone\nnia,Nia New,nia@kestrel.test,123\n",
		Holdings: "person_ref,position_ref,share,from,to\nnia,tester,50,2026-12-01,2027-12-01\nnia@kestrel.test,lead,,2026-12-01,\n",
	}
	p := planned(w.st, files, mustDay(t, "2026-11-02"))
	if len(p.Errors) != 0 {
		t.Fatalf("errors: %v", errorsOf(p))
	}
	if counts := p.Counts(); counts["create"] != 7 {
		t.Errorf("counts: %v %v", counts, changes(p))
	}
	// Two units, two positions, the tester's line once both exist, a person and two holdings.
	if len(p.steps) != 8 {
		t.Errorf("%d steps, want 8", len(p.steps))
	}
}

func TestEveryRowThatCannotGoSaysWhy(t *testing.T) {
	w := demo(t)
	files := Files{
		Units: "ref,name,type,parent_ref\n" +
			"a,Twice,Team," + w.root.String() + "\n" +
			"a,Twice again,Team," + w.root.String() + "\n" +
			"b,Untyped,," + w.root.String() + "\n" +
			"c,A second root,Team,\n",
		Positions: "ref,unit_ref,title,head_of_unit,reports_to_ref,from\n" +
			"p," + w.quality.String() + ",Lead,maybe,,\n" +
			"q," + w.quality.String() + ",Lead,false,nowhere,\n" +
			"r," + w.quality.String() + ",Lead,false,,soon\n",
		People: "ref,name\nx,\n",
		Holdings: "person_ref,position_ref,share,from,to\n" +
			w.ana.String() + ",nowhere,100,2026-01-01,\n" +
			w.ana.String() + "," + w.hoq.String() + ",100,someday,\n" +
			w.ana.String() + "," + w.hoq.String() + ",100,2026-01-01,later\n" +
			w.ana.String() + "," + w.hoq.String() + ",120,2026-01-01,\n",
	}
	p := planned(w.st, files, mustDay(t, "2026-11-02"))
	want := []string{
		`units 3 Another row has the ref "a".`,
		"units 4 Give the new unit a type.",
		"units 5 The organisation has a root already; give the unit a parent_ref.",
		"positions 2 Write head_of_unit as true or false.",
		`positions 3 No position has the ref "nowhere".`,
		"positions 4 Write from as a day, such as 2026-01-31.",
		"people 2 Name the person.",
		`holdings 2 No position has the ref "nowhere".`,
		"holdings 3 Write from as a day, such as 2026-01-31.",
		"holdings 4 Write to as a day, such as 2026-01-31, or leave it empty.",
		"holdings 5 Give the share as a whole number from 1 to 100.",
	}
	if got := errorsOf(p); !slices.Equal(got, want) {
		t.Errorf("errors:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestTheFirstUnitOfAnEmptyOrganisationIsItsRoot(t *testing.T) {
	p := planned(state{types: []org.UnitType{{ID: uuid.New(), Name: "Organisation"}}},
		Files{Units: "ref,name,type\nroot,Kestrel,organisation\nother,Also root,Organisation\n"}, mustDay(t, "2026-11-02"))
	if got := changes(p); !slices.Equal(got, []string{"units 2 create Kestrel"}) || len(p.Errors) != 1 || p.Errors[0].Row != 3 {
		t.Errorf("changes %v, errors %v", got, errorsOf(p))
	}
}
