//go:build integration

package test

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/db"
	"github.com/praetorianer777/bearing/backend/internal/org"
)

type linkJSON struct {
	PositionID string
	Title      string
	From       string
	Kind       string
	Depth      int
	Holders    []struct {
		PersonID string
		Name     string
	}
}

func walked(t *testing.T, b *browser, path string) []linkJSON {
	t.Helper()
	var got struct{ Positions []linkJSON }
	if status := b.do(http.MethodGet, path, nil, &got); status != http.StatusOK {
		t.Fatalf("%s answered %d", path, status)
	}
	return got.Positions
}

func titles(links []linkJSON) []string {
	out := make([]string, len(links))
	for i, l := range links {
		out[i] = fmt.Sprintf("%s@%d", l.Title, l.Depth)
	}
	return out
}

// A small company: the developer reports to the lead, the lead to the CTO, the CTO to the CEO, and the developer
// has a dotted line to the product manager, who reports to the CEO. The CTO's chair is empty.
func TestTheChainUpAndTheReportsDown(t *testing.T) {
	editor, unit := orgFixture(t)
	ids := positions(t, editor, unit, "CEO", "CTO", "Lead", "Developer", "Product manager")
	ceo, cto, lead, dev, pm := ids[0], ids[1], ids[2], ids[3], ids[4]
	for _, l := range [][2]string{{cto, ceo}, {lead, cto}, {dev, lead}, {pm, ceo}} {
		editor.do(http.MethodPut, "/positions/"+l[0]+"/reports-to", map[string]any{"reportsToId": l[1], "from": day(-20)}, nil)
	}
	editor.do(http.MethodPost, "/positions/"+dev+"/dotted-lines", map[string]any{"reportsToId": pm, "from": day(-20)}, nil)
	people := map[string]string{}
	for _, p := range []struct{ name, position string }{{"Cleo", ceo}, {"Lena", lead}, {"Dev", dev}, {"Pam", pm}} {
		people[p.name] = person(t, editor, map[string]any{"name": p.name + " " + uuid.NewString()[:6]}).ID
		give(t, editor, map[string]any{"personId": people[p.name], "positionId": p.position, "from": day(-20)})
	}

	chain := walked(t, editor, "/people/"+people["Dev"]+"/chain")
	if got := titles(chain); !slices.Equal(got, []string{"Lead@1", "CTO@2", "CEO@3"}) {
		t.Errorf("the developer's chain is %v", got)
	}
	if len(chain[1].Holders) != 0 || len(chain[0].Holders) != 1 || chain[0].Holders[0].PersonID != people["Lena"] {
		t.Errorf("the chain's holders are %+v", chain)
	}
	if got := titles(walked(t, editor, "/people/"+people["Dev"]+"/chain?dotted=true")); !slices.Equal(got, []string{"Lead@1", "Product manager@1", "CEO@2", "CTO@2"}) {
		t.Errorf("the developer's chain with dotted lines is %v", got)
	}
	if got := walked(t, editor, "/people/"+people["Dev"]+"/chain?asOf="+day(-25)); len(got) != 0 {
		t.Errorf("before any line the developer had a chain %v", titles(got))
	}
	if got := titles(walked(t, editor, "/positions/"+cto+"/chain")); !slices.Equal(got, []string{"CEO@1"}) {
		t.Errorf("the empty CTO chair's chain is %v", got)
	}

	if got := titles(walked(t, editor, "/people/"+people["Cleo"]+"/reports")); !slices.Equal(got, []string{"CTO@1", "Product manager@1"}) {
		t.Errorf("the CEO's direct reports are %v", got)
	}
	if got := titles(walked(t, editor, "/people/"+people["Cleo"]+"/reports?all=true")); !slices.Equal(got, []string{"CTO@1", "Product manager@1", "Lead@2", "Developer@3"}) {
		t.Errorf("all the CEO's reports are %v", got)
	}
	if got := walked(t, editor, "/people/"+people["Pam"]+"/reports"); len(got) != 0 {
		t.Errorf("the product manager has solid reports %v", titles(got))
	}
	dotted := walked(t, editor, "/positions/"+pm+"/reports?dotted=true")
	if len(dotted) != 1 || dotted[0].PositionID != dev || dotted[0].Kind != "dotted" || dotted[0].From != pm {
		t.Errorf("the product manager's dotted reports are %+v", dotted)
	}
	if got := walked(t, editor, "/people/"+somebody(t, editor)+"/chain"); len(got) != 0 {
		t.Errorf("somebody holding nothing has a chain %v", titles(got))
	}
}

func TestTheMembersOfAUnitAndItsSubUnits(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	editor, _ := withRole(t, admin, "editor")
	chain := units(t, editor, unitType(t, admin), rootUnit(t, admin), "Sales", "Inside sales")
	sales, inside := chain[0], chain[1]
	head := positions(t, editor, sales, "Head of Sales")[0]
	editor.do(http.MethodPatch, "/positions/"+head, map[string]any{"headOfUnit": true}, nil)
	rep := positions(t, editor, inside, "Sales rep")[0]
	editor.do(http.MethodPatch, "/positions/"+rep, map[string]any{"seats": 2}, nil)
	for _, p := range []string{head, rep, rep} {
		give(t, editor, map[string]any{"personId": somebody(t, editor), "positionId": p, "from": day(-10)})
	}

	members := func(query string) []string {
		var got struct{ Members []struct{ PositionID string } }
		if status := editor.do(http.MethodGet, "/units/"+sales+"/members"+query, nil, &got); status != http.StatusOK {
			t.Fatalf("the members answered %d", status)
		}
		var out []string
		for _, m := range got.Members {
			out = append(out, m.PositionID)
		}
		return out
	}
	if got := members(""); !slices.Equal(got, []string{head}) {
		t.Errorf("the unit's own members hold %v", got)
	}
	if got := members("?subUnits=true"); len(got) != 3 || got[0] != head {
		t.Errorf("with its sub-units the members hold %v, the head first", got)
	}
	if got := members("?subUnits=true&asOf=" + day(-20)); len(got) != 0 {
		t.Errorf("before anybody held a position the unit had members %v", got)
	}
}

// A 5000-person organisation, eight reports to a manager and five levels deep, answers the hierarchy questions
// within 50 ms, measured in the model so the race detector's toll on encoding does not count.
func TestTheHierarchyOfFiveThousandAnswersWithin50ms(t *testing.T) {
	if raced {
		t.Skip("timed by make test-timing, without the race detector")
	}
	const people, units, span = 5000, 50, 8
	admin := signedIn(t, adminEmail, adminPassword)
	typeID, root := unitType(t, admin), rootUnit(t, admin)
	super := superuser(t, "BEARING_TEST_SUPERUSER_URL")
	ctx := context.Background()

	unitIDs := make([]uuid.UUID, units+1)
	positionIDs, personIDs := make([]uuid.UUID, people), make([]uuid.UUID, people)
	managers, unitOf := make([]*uuid.UUID, people), make([]uuid.UUID, people)
	parents := make([]uuid.UUID, units+1)
	for i := range unitIDs {
		unitIDs[i], parents[i] = uuid.New(), unitIDs[0]
	}
	parents[0] = uuid.MustParse(root)
	for i := range people {
		positionIDs[i], personIDs[i], unitOf[i] = uuid.New(), uuid.New(), unitIDs[1+i%units]
		if i > 0 {
			managers[i] = &positionIDs[(i-1)/span]
		}
	}
	from := time.Now().AddDate(0, 0, -30)
	t.Cleanup(func() {
		for _, stmt := range []string{
			`DELETE FROM reporting_line WHERE position_id = ANY ($1)`, `DELETE FROM position WHERE id = ANY ($1)`,
		} {
			_, _ = super.Exec(ctx, stmt, positionIDs)
		}
		_, _ = super.Exec(ctx, `DELETE FROM person WHERE id = ANY ($1)`, personIDs)
		_, _ = super.Exec(ctx, `DELETE FROM unit_placement WHERE unit_id = ANY ($1)`, unitIDs)
		_, _ = super.Exec(ctx, `DELETE FROM unit WHERE id = ANY ($1)`, unitIDs)
	})
	for _, step := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO unit (id, type_id, name) SELECT u, $2, 'Scale ' || n FROM unnest($1::uuid[]) WITH ORDINALITY AS t (u, n)`, []any{unitIDs, typeID}},
		{`INSERT INTO unit_placement (unit_id, parent_id, valid_from) SELECT * FROM unnest($1::uuid[], $2::uuid[]), (SELECT $3::date)`, []any{unitIDs, parents, from}},
		{`INSERT INTO position (id, unit_id, title, exists_from) SELECT p, u, 'Position ' || n, $3 FROM unnest($1::uuid[], $2::uuid[]) WITH ORDINALITY AS t (p, u, n)`, []any{positionIDs, unitOf, from}},
		{`INSERT INTO reporting_line (position_id, reports_to_id, kind, valid_from) SELECT p, m, 'solid', $3 FROM unnest($1::uuid[], $2::uuid[]) AS t (p, m) WHERE m IS NOT NULL`, []any{positionIDs, managers, from}},
		{`INSERT INTO person (id, name) SELECT p, 'Person ' || n FROM unnest($1::uuid[]) WITH ORDINALITY AS t (p, n)`, []any{personIDs}},
		{`INSERT INTO holding (person_id, position_id, valid_from) SELECT * FROM unnest($1::uuid[], $2::uuid[]), (SELECT $3::date)`, []any{personIDs, positionIDs, from}},
		// What autovacuum would do soon after a load this size; the planner needs it to choose well.
		{`ANALYZE unit, unit_placement, position, reporting_line, person, holding`, nil},
	} {
		if _, err := super.Exec(ctx, step.sql, step.args...); err != nil {
			t.Fatalf("building the organisation: %v", err)
		}
	}

	cluster, err := db.Open(ctx, loadConfig(t).DB, quietLog())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cluster.Close)
	var me struct{ Account struct{ ID string } }
	admin.do(http.MethodGet, "/auth/me", nil, &me)
	as := db.WithUser(ctx, uuid.MustParse(me.Account.ID))
	s := &org.Service{DB: cluster}
	today := org.DayOf(time.Now())

	// The replica has the organisation once the top's reports reach all the way down.
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(100 * time.Millisecond) {
		all, err := s.ReportsOfPosition(as, positionIDs[0], today, org.Walk{All: true})
		if err == nil && len(all) == people-1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the replica has %d of the reports: %v", len(all), err)
		}
	}
	for name, c := range map[string]struct {
		want int
		ask  func() (int, error)
	}{
		"the chain above the last person": {5, func() (int, error) {
			l, err := s.ChainOfPerson(as, personIDs[people-1], today, org.Walk{Dotted: true})
			return len(l), err
		}},
		"the top's direct reports": {span, func() (int, error) {
			l, err := s.ReportsOfPerson(as, personIDs[0], today, org.Walk{})
			return len(l), err
		}},
		"everybody below the top": {people - 1, func() (int, error) {
			l, err := s.ReportsOfPerson(as, personIDs[0], today, org.Walk{All: true})
			return len(l), err
		}},
		"everybody below the top, dotted lines too": {people - 1, func() (int, error) {
			l, err := s.ReportsOfPerson(as, personIDs[0], today, org.Walk{All: true, Dotted: true})
			return len(l), err
		}},
		"a unit's members": {people / units, func() (int, error) {
			m, err := s.Members(as, unitIDs[1], today, false)
			return len(m), err
		}},
		"every member with the sub-units": {people, func() (int, error) {
			m, err := s.Members(as, unitIDs[0], today, true)
			return len(m), err
		}},
	} {
		best := time.Hour
		for range 5 {
			start := time.Now()
			got, err := c.ask()
			if err != nil || got != c.want {
				t.Fatalf("%s: %d, want %d: %v", name, got, c.want, err)
			}
			best = min(best, time.Since(start))
		}
		t.Logf("%s: %s", name, best)
		if best > 50*time.Millisecond {
			t.Errorf("%s took %s at best, more than 50 ms", name, best)
		}
	}
}
