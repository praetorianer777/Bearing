//go:build integration

package test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/armature"
	"github.com/praetorianer777/bearing/backend/internal/db"
)

// connectedTo has Bearing read fake as each account, as their realm sign-ins would have it, and forgets it after.
func connectedTo(t *testing.T, admin *browser, fake *fakeArmature, accounts ...uuid.UUID) {
	t.Helper()
	super := superuser(t, "BEARING_TEST_SUPERUSER_URL")
	t.Cleanup(func() {
		_, _ = super.Exec(context.Background(), `DELETE FROM armature_connection`)
		_, _ = super.Exec(context.Background(), `DELETE FROM armature_grant`)
	})
	if status := admin.do(http.MethodPut, "/armature/connection", map[string]string{"baseUrl": fake.URL, "clientId": "bearing", "clientSecret": "partner secret"}, nil); status != http.StatusOK {
		t.Fatalf("connecting answered %d", status)
	}
	cfg := loadConfig(t)
	cluster, err := db.Open(context.Background(), cfg.DB, quietLog())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cluster.Close)
	signIn, err := armature.FromConfig(cfg.Outbound, cluster, nil, quietLog())
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range accounts {
		signIn.ConnectAtSignIn(context.Background(), a, "realm token")
	}
	replicated(t)
}

type absencesJSON struct {
	Partner struct{ Status string }
	Hidden  bool
	Gaps    []struct {
		SubjectKind, SubjectID, Name, StepKey, Kind, From, To string
		Days                                                  int
	}
}

// Hal holds the head's position and the approver role it carries, with Dee's position as its deputy. Next week Hal is
// away Monday to Wednesday and Dee on Tuesday, so on Tuesday nobody is there: not for the position, not for the role,
// and not for the step the role is responsible for and the head accountable.
func TestADayWithNeitherHolderNorDeputyThereIsAGap(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	editor, editorID := withRole(t, admin, "editor")
	member, memberID := withRole(t, admin, "member")
	unit := units(t, editor, unitType(t, admin), rootUnit(t, admin), "Covered")[0]
	ids := positions(t, editor, unit, "Covered head", "Covered deputy")
	head, deputy := ids[0], ids[1]
	editor.do(http.MethodPut, "/positions/"+head+"/deputies", map[string]any{"deputyIds": []string{deputy}}, nil)
	approver := orgRole(t, editor, "Covered approver")
	editor.do(http.MethodPost, "/positions/"+head+"/roles", map[string]any{"roleId": approver.ID, "from": day(-30)}, nil)
	hal, dee := somebody(t, editor), somebody(t, editor)
	give(t, editor, map[string]any{"personId": hal, "positionId": head, "from": day(-30)})
	give(t, editor, map[string]any{"personId": dee, "positionId": deputy, "from": day(-30)})
	halSubject, deeSubject := inRealm(t, hal), inRealm(t, dee)

	made := process(t, editor, "Covered approval", nil)
	nodes := []nodeJSON{
		{Key: "s", Kind: "start"},
		{Key: "approve", Kind: "step", Name: "Approve", Responsibilities: []responsibilityJSON{
			{Kind: "accountable", PositionID: head}, {Kind: "responsible", RoleID: approver.ID}, {Kind: "informed", PositionID: deputy},
		}},
		{Key: "e", Kind: "end"},
	}
	saveDraft(editor, made.ID, made.Draft.Revision, nodes, []arrowJSON{{Key: "a1", From: "s", To: "approve"}, {Key: "a2", From: "approve", To: "e"}}, nil)
	if status := editor.do(http.MethodPost, "/processes/"+made.ID+"/publish", map[string]any{}, nil); status != http.StatusCreated {
		t.Fatalf("publishing answered %d", status)
	}

	now := time.Now()
	next := now.AddDate(0, 0, 7-((int(now.Weekday())+6)%7))
	on := func(n int) string { return next.AddDate(0, 0, n).Format(time.DateOnly) }
	fake := newFakeArmature(t)
	fake.weeks[halSubject] = armature.Week{NominalHours: 40, DaysAway: 3, Away: []string{on(0), on(1), on(2)}}
	fake.weeks[deeSubject] = armature.Week{NominalHours: 40, DaysAway: 1, Away: []string{on(1)}}
	week := "?from=" + on(0) + "&to=" + on(6)
	read := func(b *browser, path string) absencesJSON {
		t.Helper()
		var got absencesJSON
		if status := b.do(http.MethodGet, path+week, nil, &got); status != http.StatusOK {
			t.Fatalf("%s answered %d", path, status)
		}
		return got
	}

	if got := read(editor, "/roles/"+approver.ID+"/absences"); got.Partner.Status != "not_configured" || len(got.Gaps) != 0 {
		t.Errorf("without Armature the role says %+v", got)
	}
	connectedTo(t, admin, fake, editorID, memberID)

	role := read(editor, "/roles/"+approver.ID+"/absences")
	if role.Partner.Status != "ok" || len(role.Gaps) != 1 || role.Gaps[0].From != on(1) || role.Gaps[0].To != on(1) || role.Gaps[0].Days != 1 ||
		role.Gaps[0].SubjectID != approver.ID {
		t.Errorf("the role's days with nobody there are %+v", role)
	}
	steps := read(editor, "/processes/"+made.ID+"/absences")
	kinds := map[string]string{}
	for _, g := range steps.Gaps {
		if g.StepKey != "approve" || g.From != on(1) {
			t.Errorf("a step's gap is %+v", g)
		}
		kinds[g.Kind] = g.SubjectID
	}
	if len(steps.Gaps) != 2 || kinds["accountable"] != head || kinds["responsible"] != approver.ID {
		t.Errorf("the step's days with nobody there are %+v", steps.Gaps)
	}
	positions := map[string]string{}
	for _, g := range read(editor, "/absences").Gaps {
		positions[g.SubjectID] = g.From + ".." + g.To
	}
	if positions[head] != on(1)+".."+on(1) || positions[deputy] != on(1)+".."+on(1) {
		t.Errorf("the positions' days with nobody there are %v", positions)
	}
	var inUnit absencesJSON
	editor.do(http.MethodGet, "/absences"+week+"&unitId="+unit, nil, &inUnit)
	if len(inUnit.Gaps) != 2 {
		t.Errorf("the unit's days with nobody there are %+v", inUnit.Gaps)
	}
	if got := read(member, "/roles/"+approver.ID+"/absences"); !got.Hidden || len(got.Gaps) != 0 {
		t.Errorf("a member sees %+v", got)
	}
	if status := editor.do(http.MethodGet, "/roles/"+approver.ID+"/absences?from="+on(0)+"&to="+on(400), nil, nil); status != http.StatusUnprocessableEntity {
		t.Errorf("more than 26 weeks answered %d", status)
	}
}
