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
}

type arrowJSON struct {
	Key   string `json:"key"`
	From  string `json:"from"`
	To    string `json:"to"`
	Label string `json:"label"`
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
		Problems []struct{ Code, Node, Arrow string }
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

func problemCodes(p processJSON) []string {
	var out []string
	for _, pr := range p.Draft.Problems {
		out = append(out, pr.Code)
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
	if status := saveDraft(editor, outer.ID, outer.Draft.Revision, nodes, arrows, &saved); status != http.StatusOK || len(saved.Process.Draft.Problems) != 0 {
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
}
