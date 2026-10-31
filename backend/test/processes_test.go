//go:build integration

package test

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/google/uuid"
)

type nodeJSON struct {
	Key          string  `json:"key"`
	Kind         string  `json:"kind"`
	Name         string  `json:"name"`
	Description  string  `json:"description"`
	X            float64 `json:"x"`
	Y            float64 `json:"y"`
	SubprocessID *string `json:"subprocessId"`
	// Responsibilities left out of a save are none.
	Responsibilities []responsibilityJSON `json:"responsibilities,omitempty"`
}

type responsibilityJSON struct {
	Kind       string `json:"kind"`
	RoleID     string `json:"roleId,omitempty"`
	PositionID string `json:"positionId,omitempty"`
	UnitID     string `json:"unitId,omitempty"`
}

type arrowJSON struct {
	Key   string    `json:"key"`
	From  string    `json:"from"`
	To    string    `json:"to"`
	Label string    `json:"label"`
	Route []float64 `json:"route,omitempty"`
}

type processJSON struct {
	ID      string
	Key     string
	Name    string
	OwnerID *string
	Latest  *int
	Changed bool
	Draft   struct {
		Nodes    []nodeJSON
		Arrows   []arrowJSON
		Revision int
		Problems []struct {
			Code, Node, Arrow string
			Warning           bool
		}
	}
	Versions []struct {
		Number        int
		Note          string
		PublisherName *string
	}
}

type processBody struct{ Process processJSON }

type versionJSON struct {
	Number int
	Note   string
	Nodes  []nodeJSON
	Arrows []arrowJSON
}

// process adds a process and removes it, with its versions, after.
func process(t *testing.T, b *browser, name string, extra map[string]any) processJSON {
	t.Helper()
	body := map[string]any{"key": "process-" + uuid.NewString()[:8], "name": name}
	for k, v := range extra {
		body[k] = v
	}
	var made processBody
	if status := b.do(http.MethodPost, "/processes", body, &made); status != http.StatusCreated {
		t.Fatalf("adding the process %s answered %d", name, status)
	}
	super := superuser(t, "BEARING_TEST_SUPERUSER_URL")
	t.Cleanup(func() {
		_, _ = super.Exec(context.Background(), `DELETE FROM process WHERE id = $1`, made.Process.ID)
	})
	return made.Process
}

func saveDraft(b *browser, id string, revision int, nodes []nodeJSON, arrows []arrowJSON, into any) int {
	return b.do(http.MethodPut, "/processes/"+id+"/draft", map[string]any{"revision": revision, "nodes": nodes, "arrows": arrows}, into)
}

// problemCodes are the problems keeping a draft from being published; warnings are left out.
func problemCodes(p processJSON) []string {
	var out []string
	for _, pr := range p.Draft.Problems {
		if !pr.Warning {
			out = append(out, pr.Code)
		}
	}
	return out
}

// The case the issue names: a draft drawn, refused while it has problems, published as numbered versions that stay.
func TestAProcessIsDrawnThenPublishedAsVersionsThatStay(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	editor, _ := withRole(t, admin, "editor")
	made := process(t, editor, "Invoice approval", map[string]any{"area": "Finance"})
	if made.Latest != nil || !made.Changed || len(made.Draft.Nodes) != 2 || len(made.Draft.Arrows) != 1 || len(made.Draft.Problems) != 0 {
		t.Fatalf("a new process is %+v", made)
	}

	nodes := []nodeJSON{
		{Key: "in", Kind: "start", Name: "Invoice arrives"},
		{Key: "big", Kind: "decision", Name: "Over the limit?", Y: 100},
		{Key: "approve", Kind: "step", Name: "Approve", X: -120, Y: 200},
		{Key: "out", Kind: "end", Name: "Paid", Y: 300},
	}
	oneWay := []arrowJSON{{Key: "a1", From: "in", To: "big"}, {Key: "a2", From: "big", To: "approve", Label: "Yes"}, {Key: "a3", From: "approve", To: "out"}}
	var saved processBody
	if status := saveDraft(editor, made.ID, made.Draft.Revision, nodes, oneWay, &saved); status != http.StatusOK {
		t.Fatalf("saving the draft answered %d", status)
	}
	if got := problemCodes(saved.Process); !slices.Equal(got, []string{"decision_ways"}) || saved.Process.Draft.Revision != made.Draft.Revision+1 {
		t.Errorf("the saved draft has problems %v at revision %d", got, saved.Process.Draft.Revision)
	}
	var r refusal
	if status := saveDraft(editor, made.ID, made.Draft.Revision, nodes, oneWay, &r); status != http.StatusConflict || r.Error.Code != "draft_stale" {
		t.Errorf("a save from an older revision answered %d %+v", status, r)
	}
	if status := editor.do(http.MethodPost, "/processes/"+made.ID+"/publish", map[string]any{"note": "First"}, &r); status != http.StatusUnprocessableEntity || r.Error.Code != "not_publishable" {
		t.Errorf("publishing a draft with problems answered %d %+v", status, r)
	}

	twoWays := append(slices.Clone(oneWay), arrowJSON{Key: "a4", From: "big", To: "out", Label: "No"})
	saveDraft(editor, made.ID, saved.Process.Draft.Revision, nodes, twoWays, &saved)
	var published struct{ Version versionJSON }
	if status := editor.do(http.MethodPost, "/processes/"+made.ID+"/publish", map[string]any{"note": "First"}, &published); status != http.StatusCreated || published.Version.Number != 1 {
		t.Fatalf("publishing answered %d %+v", status, published)
	}
	if status := editor.do(http.MethodPost, "/processes/"+made.ID+"/publish", map[string]any{}, &r); status != http.StatusConflict || r.Error.Code != "nothing_changed" {
		t.Errorf("publishing the same again answered %d %+v", status, r)
	}
	var now processBody
	editor.do(http.MethodGet, "/processes/"+made.ID, nil, &now)
	if now.Process.Latest == nil || *now.Process.Latest != 1 || now.Process.Changed || len(now.Process.Draft.Arrows) != 4 {
		t.Errorf("after publishing the process is %+v", now.Process)
	}

	renamed := slices.Clone(nodes)
	renamed[2].Name = "Approve as invoice approver"
	saveDraft(editor, made.ID, now.Process.Draft.Revision, renamed, twoWays, &saved)
	if !saved.Process.Changed {
		t.Error("a draft changed since publishing is not marked so")
	}
	editor.do(http.MethodPost, "/processes/"+made.ID+"/publish", map[string]any{"note": "Named the approver"}, &published)
	editor.do(http.MethodGet, "/processes/"+made.ID, nil, &now)
	if len(now.Process.Versions) != 2 || now.Process.Versions[0].Number != 2 || now.Process.Versions[0].Note != "Named the approver" ||
		now.Process.Versions[0].PublisherName == nil {
		t.Errorf("the versions are %+v", now.Process.Versions)
	}

	var first struct{ Version versionJSON }
	if status := editor.do(http.MethodGet, "/processes/"+made.ID+"/versions/1", nil, &first); status != http.StatusOK {
		t.Fatalf("reading version 1 answered %d", status)
	}
	for _, n := range first.Version.Nodes {
		if n.Key == "approve" && n.Name != "Approve" {
			t.Errorf("version 1 changed with the draft: %q", n.Name)
		}
	}
	if status := editor.do(http.MethodGet, "/processes/"+made.ID+"/versions/3", nil, nil); status != http.StatusNotFound {
		t.Errorf("a version not published answered %d", status)
	}
	member, _ := withRole(t, admin, "member")
	if status := saveDraft(member, made.ID, now.Process.Draft.Revision, nodes, twoWays, nil); status != http.StatusForbidden {
		t.Errorf("a member saved the draft: %d", status)
	}
	if status := editor.do(http.MethodDelete, "/processes/"+made.ID, nil, &r); status != http.StatusConflict || r.Error.Code != "process_published" {
		t.Errorf("removing a published process answered %d %+v", status, r)
	}

	var log struct {
		Events []struct{ Table, Operation string }
	}
	editor.do(http.MethodGet, "/processes/"+made.ID+"/audit", nil, &log)
	publishes := 0
	for _, e := range log.Events {
		if e.Table == "process_version" {
			publishes++
		}
	}
	if publishes != 2 || log.Events[len(log.Events)-1].Table != "process" {
		t.Errorf("the process's log is %+v", log.Events)
	}

	super := superuser(t, "BEARING_TEST_SUPERUSER_URL")
	if _, err := super.Exec(context.Background(), `UPDATE process_node SET name = 'Changed' FROM process_version v
		WHERE v.id = process_node.version_id AND v.process_id = $1 AND v.number = 1`, made.ID); err == nil {
		t.Error("a published version's node changed")
	}
}

func TestADraftHoldsOnlyAWellFormedGraph(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	editor, _ := withRole(t, admin, "editor")
	made := process(t, editor, "Shapes", nil)
	var r refusal
	nodes := []nodeJSON{{Key: "s", Kind: "start"}, {Key: "e", Kind: "end"}}
	if status := saveDraft(editor, made.ID, made.Draft.Revision, nodes, []arrowJSON{{Key: "a", From: "s", To: "nowhere"}}, &r); status != http.StatusUnprocessableEntity || r.Error.Fields["arrows"] == "" {
		t.Errorf("an arrow to nowhere answered %d %+v", status, r)
	}
	if status := saveDraft(editor, made.ID, made.Draft.Revision, []nodeJSON{{Key: "s", Kind: "gateway"}}, nil, &r); status != http.StatusUnprocessableEntity || r.Error.Fields["nodes"] == "" {
		t.Errorf("an unknown kind answered %d %+v", status, r)
	}
	gone := uuid.NewString()
	sub := []nodeJSON{{Key: "s", Kind: "start"}, {Key: "p", Kind: "subprocess", Name: "Gone", SubprocessID: &gone}, {Key: "e", Kind: "end"}}
	if status := saveDraft(editor, made.ID, made.Draft.Revision, sub, nil, &r); status != http.StatusUnprocessableEntity || r.Error.Fields["nodes"] == "" {
		t.Errorf("a subprocess that does not exist answered %d %+v", status, r)
	}
	var empty processBody
	if status := saveDraft(editor, made.ID, made.Draft.Revision, nil, nil, &empty); status != http.StatusOK ||
		!slices.Equal(problemCodes(empty.Process), []string{"no_start", "no_end"}) {
		t.Errorf("an empty draft answered %d with problems %v", status, problemCodes(empty.Process))
	}
	if status := editor.do(http.MethodDelete, "/processes/"+made.ID, nil, nil); status != http.StatusNoContent {
		t.Errorf("removing a process never published answered %d", status)
	}
	if status := editor.do(http.MethodGet, "/processes/"+made.ID, nil, nil); status != http.StatusNotFound {
		t.Errorf("a removed process answered %d", status)
	}
}

func TestAProcessHasAnOwnerAndOtherProcessesAsSubprocesses(t *testing.T) {
	editor, unit := orgFixture(t)
	head := positions(t, editor, unit, "Head of Quality")[0]
	inner := process(t, editor, "Root cause analysis", map[string]any{"area": "Quality", "ownerId": head})
	if inner.OwnerID == nil || *inner.OwnerID != head {
		t.Errorf("the owner is %v", inner.OwnerID)
	}
	outer := process(t, editor, "Complaint", nil)
	nodes := []nodeJSON{{Key: "s", Kind: "start"}, {Key: "p", Kind: "subprocess", Name: "Find the cause", SubprocessID: &inner.ID}, {Key: "e", Kind: "end"}}
	arrows := []arrowJSON{{Key: "a1", From: "s", To: "p"}, {Key: "a2", From: "p", To: "e"}}
	var saved processBody
	if status := saveDraft(editor, outer.ID, outer.Draft.Revision, nodes, arrows, &saved); status != http.StatusOK || len(problemCodes(saved.Process)) != 0 {
		t.Fatalf("saving a draft with a subprocess answered %d %+v", status, saved.Process.Draft.Problems)
	}
	var r refusal
	if status := editor.do(http.MethodDelete, "/processes/"+inner.ID, nil, &r); status != http.StatusConflict || r.Error.Code != "process_in_use" {
		t.Errorf("removing a subprocess in use answered %d %+v", status, r)
	}
	var changed processBody
	if status := editor.do(http.MethodPatch, "/processes/"+inner.ID, map[string]any{"ownerId": "", "name": "Cause analysis"}, &changed); status != http.StatusOK ||
		changed.Process.OwnerID != nil || changed.Process.Name != "Cause analysis" {
		t.Errorf("taking the owner away answered %d %+v", status, changed.Process)
	}
	if status := editor.do(http.MethodPatch, "/processes/"+inner.ID, map[string]any{"ownerId": uuid.NewString()}, &r); status != http.StatusUnprocessableEntity || r.Error.Fields["ownerId"] == "" {
		t.Errorf("an owner that does not exist answered %d %+v", status, r)
	}
	if status := editor.do(http.MethodPatch, "/processes/"+inner.ID, map[string]any{"key": outer.Key}, &r); status != http.StatusConflict || r.Error.Code != "key_taken" {
		t.Errorf("another process's key answered %d %+v", status, r)
	}
	var list struct{ Processes []processJSON }
	editor.do(http.MethodGet, "/processes", nil, &list)
	found := 0
	for _, p := range list.Processes {
		if p.ID == inner.ID || p.ID == outer.ID {
			found++
			if len(p.Draft.Nodes) != 0 {
				t.Errorf("the list carries the draft of %s", p.Name)
			}
		}
	}
	if found != 2 {
		t.Errorf("the list has %d of the two processes", found)
	}
}

func TestTheDemoHasItsProcesses(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	var list struct{ Processes []processJSON }
	admin.do(http.MethodGet, "/processes", nil, &list)
	byKey := map[string]processJSON{}
	for _, p := range list.Processes {
		byKey[p.Key] = p
	}
	order, release, onboarding := byKey["order-to-delivery"], byKey["product-release"], byKey["onboarding"]
	if order.Latest == nil || *order.Latest != 2 || order.Changed || order.OwnerID == nil {
		t.Errorf("order to delivery is %+v", order)
	}
	if release.Latest != nil {
		t.Errorf("the product release was published: %+v", release)
	}
	if !onboarding.Changed {
		t.Error("onboarding has no unpublished changes")
	}
	var one processBody
	admin.do(http.MethodGet, "/processes/"+release.ID, nil, &one)
	if !slices.Contains(problemCodes(one.Process), "decision_ways") {
		t.Errorf("the product release draft has problems %v", problemCodes(one.Process))
	}
	warned := false
	for _, p := range one.Process.Draft.Problems {
		warned = warned || p.Warning && p.Code == "no_accountable" && p.Node == "ship"
	}
	if !warned {
		t.Errorf("the product release does not warn that nobody is accountable for shipping: %+v", one.Process.Draft.Problems)
	}
	var invoice processBody
	admin.do(http.MethodGet, "/processes/"+byKey["supplier-invoice"].ID, nil, &invoice)
	for _, n := range invoice.Process.Draft.Nodes {
		if n.Key == "approve" && (len(n.Responsibilities) != 2 || n.Responsibilities[0].Kind != "responsible" || n.Responsibilities[0].RoleID == "") {
			t.Errorf("approving the invoice has responsibilities %+v", n.Responsibilities)
		}
	}
}

// The case the issue names: a step's parties are saved with the draft, kept by the published version, and warned of
// when a step lacks its accountable or responsible, without keeping the draft from being published.
func TestAStepSaysWhoIsResponsibleAccountableConsultedAndInformed(t *testing.T) {
	editor, unit := orgFixture(t)
	head := positions(t, editor, unit, "Head of Finance")[0]
	approver := orgRole(t, editor, "Invoice approver")
	made := process(t, editor, "Invoices", nil)
	nodes := []nodeJSON{
		{Key: "s", Kind: "start"},
		{Key: "approve", Kind: "step", Name: "Approve", Responsibilities: []responsibilityJSON{
			{Kind: "responsible", RoleID: approver.ID}, {Kind: "accountable", PositionID: head}, {Kind: "informed", UnitID: unit},
		}},
		{Key: "book", Kind: "step", Name: "Book", Responsibilities: []responsibilityJSON{{Kind: "consulted", PositionID: head}}},
		{Key: "e", Kind: "end"},
	}
	arrows := []arrowJSON{{Key: "a1", From: "s", To: "approve"}, {Key: "a2", From: "approve", To: "book"}, {Key: "a3", From: "book", To: "e"}}
	var saved processBody
	if status := saveDraft(editor, made.ID, made.Draft.Revision, nodes, arrows, &saved); status != http.StatusOK {
		t.Fatalf("saving responsibilities answered %d", status)
	}
	var warnings []string
	for _, p := range saved.Process.Draft.Problems {
		if !p.Warning {
			t.Errorf("a blocking problem: %+v", p)
		}
		warnings = append(warnings, p.Code+"@"+p.Node)
	}
	slices.Sort(warnings)
	if !slices.Equal(warnings, []string{"no_accountable@book", "no_responsible@book"}) {
		t.Errorf("the warnings are %v", warnings)
	}
	for _, n := range saved.Process.Draft.Nodes {
		if n.Key == "approve" && len(n.Responsibilities) != 3 {
			t.Errorf("approving has %+v", n.Responsibilities)
		}
	}

	var published struct{ Version versionJSON }
	if status := editor.do(http.MethodPost, "/processes/"+made.ID+"/publish", map[string]any{"note": "With warnings"}, &published); status != http.StatusCreated {
		t.Fatalf("publishing a draft with warnings only answered %d", status)
	}
	var first struct{ Version versionJSON }
	editor.do(http.MethodGet, "/processes/"+made.ID+"/versions/1", nil, &first)
	kept := 0
	for _, n := range first.Version.Nodes {
		kept += len(n.Responsibilities)
	}
	if kept != 4 {
		t.Errorf("the published version keeps %d responsibilities", kept)
	}

	var r refusal
	if status := editor.do(http.MethodDelete, "/roles/"+approver.ID, nil, &r); status != http.StatusConflict || r.Error.Code != "role_in_use" {
		t.Errorf("removing a role a process names answered %d %+v", status, r)
	}
	start := []nodeJSON{{Key: "s", Kind: "start", Responsibilities: []responsibilityJSON{{Kind: "informed", UnitID: unit}}}, {Key: "e", Kind: "end"}}
	if status := saveDraft(editor, made.ID, saved.Process.Draft.Revision+1, start, nil, &r); status != http.StatusUnprocessableEntity || r.Error.Fields["nodes"] == "" {
		t.Errorf("a start with a responsibility answered %d %+v", status, r)
	}
	gone := []nodeJSON{{Key: "s", Kind: "start"}, {Key: "x", Kind: "step", Name: "X", Responsibilities: []responsibilityJSON{{Kind: "responsible", RoleID: uuid.NewString()}}}}
	if status := saveDraft(editor, made.ID, saved.Process.Draft.Revision+1, gone, nil, &r); status != http.StatusUnprocessableEntity || r.Error.Fields["nodes"] == "" {
		t.Errorf("a role that does not exist answered %d %+v", status, r)
	}
	super := superuser(t, "BEARING_TEST_SUPERUSER_URL")
	if _, err := super.Exec(context.Background(), `DELETE FROM process_responsibility r USING process_version v
		WHERE v.id = r.version_id AND v.process_id = $1 AND v.number = 1`, made.ID); err == nil {
		t.Error("a published version's responsibility was removed")
	}
}

type resolvedJSON struct {
	PersonID   string
	Name       string
	PositionID *string
	DeputyFor  *string
}

type stepPeopleJSON struct {
	Key              string
	Responsibilities []struct {
		Kind                       string
		RoleID, PositionID, UnitID string
		People                     []resolvedJSON
	}
}

// who are the names a step's responsibility of a kind comes down to, deputies marked.
func who(step stepPeopleJSON, kind string) []string {
	out := []string{}
	for _, r := range step.Responsibilities {
		if r.Kind != kind {
			continue
		}
		for _, p := range r.People {
			name := p.Name
			if p.DeputyFor != nil {
				name += " (deputy)"
			}
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

// The case the issue names: a responsibility comes down to the people holding its party on a day, and a vacant
// position to its deputies, in order, saying so.
func TestResponsibilitiesComeDownToPeopleOnADayWithDeputiesForVacancies(t *testing.T) {
	editor, unit := orgFixture(t)
	ids := positions(t, editor, unit, "Head of Quality", "Quality clerk", "Quality engineer")
	head, clerk, engineer := ids[0], ids[1], ids[2]
	if status := editor.do(http.MethodPatch, "/positions/"+head, map[string]any{"headOfUnit": true}, nil); status != http.StatusOK {
		t.Fatalf("making the head answered %d", status)
	}
	if status := editor.do(http.MethodPut, "/positions/"+head+"/deputies", map[string]any{"deputyIds": []string{clerk, engineer}}, nil); status != http.StatusOK {
		t.Fatalf("naming deputies answered %d", status)
	}
	auditor := orgRole(t, editor, "Auditor")
	if status := editor.do(http.MethodPost, "/positions/"+engineer+"/roles", map[string]any{"roleId": auditor.ID, "from": day(-30)}, nil); status != http.StatusCreated {
		t.Fatalf("carrying the role answered %d", status)
	}
	ana := person(t, editor, map[string]any{"name": "Ana Head"}).ID
	dora := person(t, editor, map[string]any{"name": "Dora Engineer"}).ID
	rita := person(t, editor, map[string]any{"name": "Rita Auditor"}).ID
	give(t, editor, map[string]any{"personId": ana, "positionId": head, "from": day(-20), "to": day(-5)})
	give(t, editor, map[string]any{"personId": dora, "positionId": engineer, "from": day(-20)})
	give(t, editor, map[string]any{"personId": rita, "roleId": auditor.ID, "from": day(-20)})

	made := process(t, editor, "Audit", nil)
	nodes := []nodeJSON{
		{Key: "s", Kind: "start"},
		{Key: "audit", Kind: "step", Name: "Audit", Responsibilities: []responsibilityJSON{
			{Kind: "accountable", UnitID: unit}, {Kind: "responsible", RoleID: auditor.ID},
			{Kind: "consulted", PositionID: clerk}, {Kind: "informed", PositionID: engineer},
		}},
		{Key: "e", Kind: "end"},
	}
	arrows := []arrowJSON{{Key: "a1", From: "s", To: "audit"}, {Key: "a2", From: "audit", To: "e"}}
	var saved processBody
	saveDraft(editor, made.ID, made.Draft.Revision, nodes, arrows, &saved)
	if status := editor.do(http.MethodPost, "/processes/"+made.ID+"/publish", map[string]any{}, nil); status != http.StatusCreated {
		t.Fatalf("publishing answered %d", status)
	}

	step := func(query string) stepPeopleJSON {
		t.Helper()
		var got struct {
			Version *int
			Step    stepPeopleJSON
		}
		if status := editor.do(http.MethodGet, "/processes/"+made.ID+"/steps/audit/people"+query, nil, &got); status != http.StatusOK {
			t.Fatalf("reading the step's people%s answered %d", query, status)
		}
		return got.Step
	}
	now := step("")
	for kind, want := range map[string][]string{
		"accountable": {"Dora Engineer (deputy)"},
		"responsible": {"Dora Engineer", "Rita Auditor"},
		"consulted":   {},
		"informed":    {"Dora Engineer"},
	} {
		if got := who(now, kind); !slices.Equal(got, want) {
			t.Errorf("today %s comes down to %v, want %v", kind, got, want)
		}
	}
	for _, r := range now.Responsibilities {
		if r.Kind == "accountable" && (*r.People[0].DeputyFor != head || *r.People[0].PositionID != engineer) {
			t.Errorf("the deputy does not say whom they stand in for: %+v", r.People[0])
		}
	}
	if got := who(step("?asOf="+day(-10)), "accountable"); !slices.Equal(got, []string{"Ana Head"}) {
		t.Errorf("ten days ago the accountable was %v", got)
	}

	nodes[1].Responsibilities = nodes[1].Responsibilities[:1]
	saveDraft(editor, made.ID, saved.Process.Draft.Revision+1, nodes, arrows, nil)
	if got := who(step(""), "responsible"); len(got) != 2 {
		t.Errorf("the published version's responsible changed with the draft: %v", got)
	}
	if got := who(step("?version=draft"), "responsible"); len(got) != 0 {
		t.Errorf("the draft, without a responsible, comes down to %v", got)
	}
	var whole struct {
		Version *int
		Steps   []stepPeopleJSON
	}
	if status := editor.do(http.MethodGet, "/processes/"+made.ID+"/people?version=1", nil, &whole); status != http.StatusOK ||
		whole.Version == nil || *whole.Version != 1 || len(whole.Steps) != 1 || whole.Steps[0].Key != "audit" {
		t.Errorf("the whole version's people answered %d %+v", status, whole)
	}
	var r refusal
	for query, want := range map[string]int{
		"/steps/s/people":               http.StatusNotFound,
		"/steps/audit/people?version=9": http.StatusNotFound,
		"/people?version=zero":          http.StatusUnprocessableEntity,
	} {
		if status := editor.do(http.MethodGet, "/processes/"+made.ID+query, nil, &r); status != want {
			t.Errorf("%s answered %d, want %d", query, status, want)
		}
	}
	if status := editor.do(http.MethodGet, "/processes/"+uuid.NewString()+"/people", nil, nil); status != http.StatusNotFound {
		t.Errorf("an unknown process answered %d", status)
	}
}

type dutyJSON struct {
	ProcessKey string
	StepKey    string
	StepName   string
	Kind       string
	People     []resolvedJSON
}

func duties(t *testing.T, b *browser, query string) []dutyJSON {
	t.Helper()
	var got struct{ Responsibilities []dutyJSON }
	if status := b.do(http.MethodGet, "/responsibilities?"+query, nil, &got); status != http.StatusOK {
		t.Fatalf("reading the responsibilities of %s answered %d", query, status)
	}
	return got.Responsibilities
}

func dutyKeys(list []dutyJSON) []string {
	out := []string{}
	for _, d := range list {
		out = append(out, d.ProcessKey+"/"+d.StepKey+":"+d.Kind)
	}
	slices.Sort(out)
	return out
}

// The cases the issue names, on the demo processes: what somebody, a position, a role and a unit answer for, and who
// does a step found by a term.
func TestWhatSomebodyIsResponsibleForAndWhoDoesItInTheDemo(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	find := func(path, field, value string) string {
		var body map[string]any
		admin.do(http.MethodGet, path, nil, &body)
		for _, list := range body {
			items, _ := list.([]any)
			for _, raw := range items {
				if item, _ := raw.(map[string]any); item[field] == value {
					return item["id"].(string)
				}
			}
		}
		t.Fatalf("no %s %q at %s", field, value, path)
		return ""
	}
	carol := find("/people?q=Carol", "name", "Carol Checker")
	labLead := find("/positions", "title", "Test lab lead")
	approver := find("/roles", "key", "invoice-approver")
	sales := find("/units", "name", "Sales")

	got := dutyKeys(duties(t, admin, "personId="+carol))
	for _, want := range []string{
		"complaint-handling/cause:accountable", "complaint-handling/fault:consulted", "production-run/test:accountable",
	} {
		if !slices.Contains(got, want) {
			t.Errorf("Carol Checker is not %s: %v", want, got)
		}
	}
	for _, d := range duties(t, admin, "personId="+carol) {
		if d.StepKey == "cause" && (len(d.People) == 0 || d.People[0].DeputyFor == nil) {
			t.Errorf("Carol answers for finding the cause without standing in for the Head of Quality: %+v", d)
		}
	}
	if got := dutyKeys(duties(t, admin, "positionId="+labLead)); !slices.Contains(got, "complaint-handling/cause:accountable") ||
		!slices.Contains(got, "production-run/test:accountable") {
		t.Errorf("the Test lab lead answers for %v", got)
	}
	// Other tests and browser runs on the same stack name the role in processes of their own; the demo's are counted.
	demoOnly := func(list []dutyJSON) []dutyJSON {
		var out []dutyJSON
		for _, d := range list {
			if slices.Contains([]string{"production-run", "order-to-delivery", "supplier-invoice", "complaint-handling", "onboarding", "product-release"}, d.ProcessKey) {
				out = append(out, d)
			}
		}
		return out
	}
	if got := dutyKeys(demoOnly(duties(t, admin, "roleId="+approver))); !slices.Equal(got, []string{"supplier-invoice/approve:responsible", "supplier-invoice/pay:informed"}) {
		t.Errorf("the invoice approver answers for %v", got)
	}
	if got := dutyKeys(duties(t, admin, "unitId="+sales)); !slices.Contains(got, "order-to-delivery/produce:informed") ||
		!slices.Contains(got, "production-run/release:informed") {
		t.Errorf("Sales answers for %v", got)
	}

	var r refusal
	for query, field := range map[string]string{"": "personId", "personId=x": "personId", "personId=" + carol + "&roleId=" + approver: "personId"} {
		if status := admin.do(http.MethodGet, "/responsibilities?"+query, nil, &r); status != http.StatusUnprocessableEntity || r.Error.Fields[field] == "" {
			t.Errorf("%q answered %d %+v", query, status, r)
		}
	}

	var found struct {
		Steps []struct {
			ProcessKey string
			stepPeopleJSON
		}
	}
	if status := admin.do(http.MethodGet, "/steps?q=root+cause", nil, &found); status != http.StatusOK || len(found.Steps) != 1 {
		t.Fatalf("who finds the root cause answered %d %+v", status, found)
	}
	if got := who(found.Steps[0].stepPeopleJSON, "accountable"); !slices.Equal(got, []string{"Carol Checker (deputy)"}) {
		t.Errorf("accountable for finding the root cause: %v", got)
	}
	if status := admin.do(http.MethodGet, "/steps?q=invoice", nil, &found); status != http.StatusOK || len(found.Steps) < 5 {
		t.Errorf("the steps of the supplier invoice by its name answered %d with %d steps", status, len(found.Steps))
	}
	if status := admin.do(http.MethodGet, "/steps?q=x", nil, &r); status != http.StatusUnprocessableEntity || r.Error.Fields["q"] == "" {
		t.Errorf("a one-letter term answered %d %+v", status, r)
	}
	if status := admin.do(http.MethodGet, "/steps?q=100%25", nil, &found); status != http.StatusOK || len(found.Steps) != 0 {
		t.Errorf("a term with a wildcard in it answered %d %+v", status, found.Steps)
	}
}

// The case the issue names: an arrow's route is saved with the draft, kept by the published version, and refused
// when it is not pairs of points.
func TestAnArrowKeepsTheRouteTheLayoutGaveIt(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	editor, _ := withRole(t, admin, "editor")
	made := process(t, editor, "Routed", nil)
	nodes := []nodeJSON{{Key: "s", Kind: "start"}, {Key: "e", Kind: "end", Y: 200}}
	route := []float64{80, 44, 80, 120.5, 90, 120.5, 90, 200}
	var saved processBody
	if status := saveDraft(editor, made.ID, made.Draft.Revision, nodes, []arrowJSON{{Key: "a", From: "s", To: "e", Route: route}}, &saved); status != http.StatusOK {
		t.Fatalf("saving a routed arrow answered %d", status)
	}
	if got := saved.Process.Draft.Arrows[0].Route; !slices.Equal(got, route) {
		t.Errorf("the saved route is %v", got)
	}
	editor.do(http.MethodPost, "/processes/"+made.ID+"/publish", map[string]any{}, nil)
	var first struct{ Version versionJSON }
	editor.do(http.MethodGet, "/processes/"+made.ID+"/versions/1", nil, &first)
	if len(first.Version.Arrows) != 1 || !slices.Equal(first.Version.Arrows[0].Route, route) {
		t.Errorf("the published route is %+v", first.Version.Arrows)
	}
	var r refusal
	for name, bad := range map[string][]float64{"odd": {1, 2, 3}, "one point": {1, 2}, "too long": make([]float64, 202)} {
		if status := saveDraft(editor, made.ID, saved.Process.Draft.Revision+1, nodes, []arrowJSON{{Key: "a", From: "s", To: "e", Route: bad}}, &r); status != http.StatusUnprocessableEntity || r.Error.Fields["arrows"] == "" {
			t.Errorf("a route %s answered %d %+v", name, status, r)
		}
	}
	var unrouted processBody
	saveDraft(editor, made.ID, saved.Process.Draft.Revision+1, nodes, []arrowJSON{{Key: "a", From: "s", To: "e"}}, &unrouted)
	if got := unrouted.Process.Draft.Arrows[0].Route; got == nil || len(got) != 0 {
		t.Errorf("an arrow without a route answers %v, want an empty list", got)
	}
}
