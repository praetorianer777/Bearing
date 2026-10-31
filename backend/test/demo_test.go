//go:build integration

package test

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/auth"
	"github.com/praetorianer777/bearing/backend/internal/db"
	"github.com/praetorianer777/bearing/backend/internal/demo"
	"github.com/praetorianer777/bearing/backend/internal/org"
)

type demoJSON struct{ Organisation, Processes struct{ Loaded, Empty bool } }

// The stack's seed loads the demo organisation; asked again, by the seed or an administrator, nothing changes.
func TestTheDemoOrganisationLoadsOnce(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	var state demoJSON
	if status := admin.do(http.MethodGet, "/demo", nil, &state); status != http.StatusOK || !state.Organisation.Loaded || state.Organisation.Empty ||
		!state.Processes.Loaded || state.Processes.Empty {
		t.Fatalf("the demo answered %d %+v, want it loaded by the seed", status, state)
	}
	count := func() (units, people int) {
		super := superuser(t, "BEARING_TEST_SUPERUSER_URL")
		if err := super.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM unit), (SELECT count(*) FROM person)`).Scan(&units, &people); err != nil {
			t.Fatal(err)
		}
		return units, people
	}
	units, people := count()

	var r refusal
	if status := admin.do(http.MethodPost, "/demo", nil, &r); status != http.StatusConflict || r.Error.Code != "demo_loaded" {
		t.Errorf("loading it again answered %d %+v", status, r)
	}
	cluster, err := db.Open(context.Background(), loadConfig(t).DB, quietLog())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cluster.Close)
	var me struct{ Account struct{ ID string } }
	admin.do(http.MethodGet, "/auth/me", nil, &me)
	if _, err := demo.Load(db.WithUser(context.Background(), uuid.MustParse(me.Account.ID)), &org.Service{DB: cluster}); !errors.Is(err, demo.ErrLoaded) {
		t.Errorf("the seed's second load = %v, want it refused as loaded", err)
	}
	if u, p := count(); u != units || p != people {
		t.Errorf("loading again changed the organisation: %d units and %d people, were %d and %d", u, p, units, people)
	}
	member, _ := withRole(t, admin, "member")
	if status := member.do(http.MethodPost, "/demo", nil, nil); status != http.StatusForbidden {
		t.Errorf("a member loaded the demo: %d", status)
	}
}

// Into an installation of its own, the demo goes once, all of it; into one with an organisation, not at all.
func TestTheDemoGoesIntoAnEmptyInstallationOnly(t *testing.T) {
	ctx := context.Background()
	cluster := freshCluster(t, "bearing_demo")
	accounts := &auth.Service{DB: cluster, Params: auth.DefaultPasswordParams()}
	ada, err := accounts.SetUp(ctx, "Demo", "Ada", "ada@bearing.test", "ada long password")
	if err != nil {
		t.Fatal(err)
	}
	as := db.WithUser(ctx, ada.ID)
	s := &org.Service{DB: cluster}
	if state, err := s.DemoState(as); err != nil || state.Organisation != (org.DemoPart{Empty: true}) || state.Processes != (org.DemoPart{Empty: true}) {
		t.Fatalf("a fresh installation's demo state is %+v %v", state, err)
	}
	if _, err := demo.Load(as, s); err != nil {
		t.Fatalf("loading the demo: %v", err)
	}
	var units, people, holdings, events, processes, versions int
	count := func() [6]int {
		_ = cluster.ReadAdmin(ctx, func(ctx context.Context, q db.DBTX) error {
			return q.QueryRow(ctx, `SELECT (SELECT count(*) FROM unit), (SELECT count(*) FROM person), (SELECT count(*) FROM holding),
				(SELECT count(*) FROM audit_event WHERE actor_id = $1), (SELECT count(*) FROM process),
				(SELECT count(*) FROM process_version WHERE number IS NOT NULL)`, ada.ID).
				Scan(&units, &people, &holdings, &events, &processes, &versions)
		})
		return [6]int{units, people, holdings, events, processes, versions}
	}
	before := count()
	if units != 19 || people < 55 || holdings < people || events == 0 || processes != 6 || versions != 6 {
		t.Errorf("the demo brought %d units, %d people, %d holdings, %d audited changes, %d processes and %d versions",
			units, people, holdings, events, processes, versions)
	}
	if _, err := demo.Load(as, s); !errors.Is(err, demo.ErrLoaded) {
		t.Errorf("a second load = %v", err)
	}
	if after := count(); after != before {
		t.Errorf("a second load changed things: %v, were %v", after, before)
	}

	// An installation whose demo organisation came before the processes did gets them on asking.
	if _, err := cluster.WriteAdmin(ctx, func(ctx context.Context, q db.DBTX) error {
		if _, err := q.Exec(ctx, `DELETE FROM process`); err != nil {
			return err
		}
		_, err := q.Exec(ctx, `UPDATE installation SET demo_processes_loaded_at = NULL`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := demo.Load(as, s); err != nil {
		t.Fatalf("loading the processes after the organisation: %v", err)
	}
	if after := count(); after[0] != before[0] || after[1] != before[1] || after[4] != 6 || after[5] != 6 {
		t.Errorf("loading the processes alone left %v, were %v", after, before)
	}

	other := freshCluster(t, "bearing_demo_own")
	accounts = &auth.Service{DB: other, Params: auth.DefaultPasswordParams()}
	bo, err := accounts.SetUp(ctx, "Own", "Bo", "bo@bearing.test", "bo long password")
	if err != nil {
		t.Fatal(err)
	}
	as = db.WithUser(ctx, bo.ID)
	own := &org.Service{DB: other}
	kind, _, err := own.CreateUnitType(as, org.UnitTypeInput{Name: ptrTo("Company"), Icon: ptrTo("organisation"), Colour: ptrTo("#123456")})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := own.CreateUnit(as, org.UnitInput{TypeID: &kind.ID, Name: ptrTo("Own company")}, nil, org.DayOf(time.Now())); err != nil {
		t.Fatal(err)
	}
	if _, err := demo.Load(as, own); !errors.Is(err, demo.ErrNotEmpty) {
		t.Errorf("loading into an installation with its own organisation = %v", err)
	}
}

func ptrTo[T any](v T) *T { return &v }

func TestTheDemoOrganisationHasWhatItPromises(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	var list struct{ People []personJSON }
	admin.do(http.MethodGet, "/people?q=kestrel.example", nil, &list)
	if n := len(list.People); n < 50 {
		t.Errorf("the demo has %d people of its own", n)
	}
	var positions struct{ Positions []positionJSON }
	admin.do(http.MethodGet, "/positions?vacant=true", nil, &positions)
	var vacant []string
	for _, p := range positions.Positions {
		vacant = append(vacant, p.Title)
	}
	for _, title := range []string{"Security engineer", "Head of Quality"} {
		if !slices.Contains(vacant, title) {
			t.Errorf("%s is not vacant: %v", title, vacant)
		}
	}
	var units struct{ Units []unitJSON }
	admin.do(http.MethodGet, "/units", nil, &units)
	var root string
	for _, u := range units.Units {
		if u.ParentID == nil {
			root = u.Name
		}
	}
	if root != "Kestrel Works" {
		t.Errorf("the root is %q", root)
	}

	var roles struct{ Roles []orgRoleJSON }
	admin.do(http.MethodGet, "/roles", nil, &roles)
	for _, r := range roles.Roles {
		if r.Key != "invoice-approver" {
			continue
		}
		got := roleOn(t, admin, r.ID, day(0))
		// The realm sign-in test enters alice anew, without the Head of Engineering's chair.
		if len(got.Positions) != 5 || len(got.Holders) < 4 {
			t.Errorf("invoice approvers are carried by %d positions and held by %d people", len(got.Positions), len(got.Holders))
		}
	}

	headOfProduct := func(on string) string {
		var all struct{ Positions []positionJSON }
		admin.do(http.MethodGet, "/positions?asOf="+on, nil, &all)
		for _, p := range all.Positions {
			if p.Title == "Head of Product" && len(p.Holders) == 1 {
				var who personBody
				admin.do(http.MethodGet, "/people/"+p.Holders[0].PersonID, nil, &who)
				return who.Person.Name
			}
		}
		return ""
	}
	if before, after := headOfProduct("2026-03-31"), headOfProduct("2026-04-01"); before != "Nadia Petrova" || after != "Elif Kaya" {
		t.Errorf("the Head of Product went from %q to %q", before, after)
	}
	var quality positionJSON
	for _, p := range positions.Positions {
		if p.Title == "Head of Quality" {
			quality = positionOn(t, admin, p.ID, day(0))
		}
	}
	if len(quality.Deputies) != 1 {
		t.Errorf("the empty Head of Quality chair has deputies %v", quality.Deputies)
	}
	dotted := 0
	var all struct{ Positions []positionJSON }
	admin.do(http.MethodGet, "/positions", nil, &all)
	for _, p := range all.Positions {
		if strings.HasPrefix(p.Title, "Product manager") || p.Title == "Quality engineer" || p.Title == "People partner" {
			dotted += len(p.DottedTo)
		}
	}
	if dotted != 3 {
		t.Errorf("the matrix has %d dotted lines where the demo draws three", dotted)
	}
}
