//go:build integration

package test

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/google/uuid"
)

type orgRoleJSON struct {
	ID        string
	Key       string
	Name      string
	Positions []string
	Holders   []struct {
		PersonID  string
		HoldingID string
		Share     int
		Through   *string
	}
}

type orgRoleBody struct{ Role orgRoleJSON }

// orgRole adds a role and removes it, with whatever refers to it, after.
func orgRole(t *testing.T, b *browser, name string) orgRoleJSON {
	t.Helper()
	var made orgRoleBody
	key := "role-" + uuid.NewString()[:8]
	if status := b.do(http.MethodPost, "/roles", map[string]any{"key": key, "name": name}, &made); status != http.StatusCreated {
		t.Fatalf("adding the role %s answered %d", name, status)
	}
	super := superuser(t, "BEARING_TEST_SUPERUSER_URL")
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = super.Exec(ctx, `DELETE FROM holding WHERE role_id = $1`, made.Role.ID)
		_, _ = super.Exec(ctx, `DELETE FROM position_role WHERE role_id = $1`, made.Role.ID)
		_, _ = super.Exec(ctx, `DELETE FROM role WHERE id = $1`, made.Role.ID)
	})
	return made.Role
}

func roleOn(t *testing.T, b *browser, id, on string) orgRoleJSON {
	t.Helper()
	var got orgRoleBody
	if status := b.do(http.MethodGet, "/roles/"+id+"?asOf="+on, nil, &got); status != http.StatusOK {
		t.Fatalf("reading the role answered %d", status)
	}
	return got.Role
}

// The case the issue names: a role held through two positions and directly.
func TestARoleIsHeldThroughPositionsAndDirectly(t *testing.T) {
	editor, unit := orgFixture(t)
	ids := positions(t, editor, unit, "Head of Finance", "Controller")
	head, controller := ids[0], ids[1]
	approver := orgRole(t, editor, "Invoice approver")
	finn, carla, dana := somebody(t, editor), somebody(t, editor), somebody(t, editor)
	give(t, editor, map[string]any{"personId": finn, "positionId": head, "from": day(-20)})
	give(t, editor, map[string]any{"personId": carla, "positionId": controller, "from": day(-20)})
	direct := give(t, editor, map[string]any{"personId": dana, "roleId": approver.ID, "share": 10, "from": day(-5)})
	if direct.RoleID == nil || *direct.RoleID != approver.ID || direct.PositionID != "" {
		t.Errorf("the direct holding is %+v", direct)
	}

	for _, p := range []string{head, controller} {
		var carried positionBody
		if status := editor.do(http.MethodPost, "/positions/"+p+"/roles", map[string]any{"roleId": approver.ID, "from": day(-10)}, &carried); status != http.StatusCreated || !slices.Contains(carried.Position.Roles, approver.ID) {
			t.Fatalf("carrying the role answered %d %+v", status, carried.Position.Roles)
		}
	}
	got := roleOn(t, editor, approver.ID, day(0))
	if len(got.Positions) != 2 || len(got.Holders) != 3 {
		t.Fatalf("the role is carried by %v and held by %+v", got.Positions, got.Holders)
	}
	if got.Holders[0].PersonID != dana || got.Holders[0].Through != nil || got.Holders[0].Share != 10 {
		t.Errorf("the direct holder is not listed first: %+v", got.Holders[0])
	}
	through := map[string]string{}
	for _, h := range got.Holders[1:] {
		if h.Through != nil {
			through[h.PersonID] = *h.Through
		}
	}
	if through[finn] != head || through[carla] != controller {
		t.Errorf("the holders through positions are %v", through)
	}
	if before := roleOn(t, editor, approver.ID, day(-15)); len(before.Holders) != 0 || len(before.Positions) != 0 {
		t.Errorf("before anybody had it, the role was held by %+v", before.Holders)
	}

	var r refusal
	if status := editor.do(http.MethodPost, "/positions/"+head+"/roles", map[string]any{"roleId": approver.ID}, &r); status != http.StatusConflict || r.Error.Code != "role_carried" {
		t.Errorf("carrying the role twice answered %d %+v", status, r)
	}
	if status := editor.do(http.MethodPost, "/positions/"+controller+"/roles/"+approver.ID+"/end", map[string]any{"on": day(1)}, nil); status != http.StatusOK {
		t.Errorf("dropping the role answered %d", status)
	}
	if later := roleOn(t, editor, approver.ID, day(1)); len(later.Holders) != 2 {
		t.Errorf("after the controller drops it the role is held by %+v", later.Holders)
	}
	if status := editor.do(http.MethodPost, "/positions/"+controller+"/roles/"+approver.ID+"/end", map[string]any{"on": day(2)}, &r); status != http.StatusConflict || r.Error.Code != "role_not_carried" {
		t.Errorf("dropping it again answered %d %+v", status, r)
	}
	if status := editor.do(http.MethodPost, "/holdings", map[string]any{"personId": dana, "roleId": approver.ID}, &r); status != http.StatusConflict || r.Error.Code != "holding_overlaps" {
		t.Errorf("holding the role twice at once answered %d %+v", status, r)
	}
	var held struct{ Holdings []holdingJSON }
	editor.do(http.MethodGet, "/holdings?roleId="+approver.ID, nil, &held)
	if len(held.Holdings) != 1 || held.Holdings[0].ID != direct.ID {
		t.Errorf("the role's direct holdings are %+v", held.Holdings)
	}
	if status := editor.do(http.MethodDelete, "/roles/"+approver.ID, nil, &r); status != http.StatusConflict || r.Error.Code != "role_in_use" {
		t.Errorf("removing a role in use answered %d %+v", status, r)
	}
}

func TestRolesAreKeptByEditors(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	editor, _ := withRole(t, admin, "editor")
	member, _ := withRole(t, admin, "member")
	approver := orgRole(t, editor, "Approver")

	var r refusal
	if status := editor.do(http.MethodPost, "/roles", map[string]any{"key": approver.Key, "name": "Another"}, &r); status != http.StatusConflict || r.Error.Code != "key_taken" {
		t.Errorf("a second role with the key answered %d %+v", status, r)
	}
	if status := member.do(http.MethodPost, "/roles", map[string]any{"key": "mine", "name": "Mine"}, nil); status != http.StatusForbidden {
		t.Errorf("a member added a role: %d", status)
	}
	var changed orgRoleBody
	if status := editor.do(http.MethodPatch, "/roles/"+approver.ID, map[string]any{"name": "Purchase approver"}, &changed); status != http.StatusOK || changed.Role.Name != "Purchase approver" {
		t.Errorf("renaming answered %d %+v", status, changed.Role)
	}
	var list struct{ Roles []orgRoleJSON }
	editor.do(http.MethodGet, "/roles", nil, &list)
	if !slices.ContainsFunc(list.Roles, func(r orgRoleJSON) bool { return r.ID == approver.ID }) {
		t.Error("the role is not listed")
	}
	unused := orgRole(t, editor, "Unused")
	if status := editor.do(http.MethodDelete, "/roles/"+unused.ID, nil, nil); status != http.StatusNoContent {
		t.Errorf("removing an unused role answered %d", status)
	}
	if status := editor.do(http.MethodPost, "/holdings", map[string]any{"personId": somebody(t, editor), "roleId": uuid.NewString()}, &r); status != http.StatusUnprocessableEntity {
		t.Errorf("holding a role that is none answered %d", status)
	}
}

func TestDeputiesStandInInOrder(t *testing.T) {
	editor, unit := orgFixture(t)
	ids := positions(t, editor, unit, "Head", "First deputy", "Second deputy")
	head, first, second := ids[0], ids[1], ids[2]

	var set positionBody
	if status := editor.do(http.MethodPut, "/positions/"+head+"/deputies", map[string]any{"deputyIds": []string{second, first}}, &set); status != http.StatusOK {
		t.Fatalf("setting deputies answered %d", status)
	}
	if !slices.Equal(set.Position.Deputies, []string{second, first}) {
		t.Errorf("the deputies are %v, want the order given", set.Position.Deputies)
	}
	editor.do(http.MethodPut, "/positions/"+head+"/deputies", map[string]any{"deputyIds": []string{first, second}}, &set)
	if !slices.Equal(set.Position.Deputies, []string{first, second}) {
		t.Errorf("the reordered deputies are %v", set.Position.Deputies)
	}
	var r refusal
	for name, deputies := range map[string][]string{"itself": {head}, "twice": {first, first}, "none such": {uuid.NewString()}} {
		if status := editor.do(http.MethodPut, "/positions/"+head+"/deputies", map[string]any{"deputyIds": deputies}, &r); status != http.StatusUnprocessableEntity {
			t.Errorf("deputies %s answered %d", name, status)
		}
	}
	if status := editor.do(http.MethodPost, "/positions/"+first+"/archive", map[string]any{"on": day(5)}, nil); status != http.StatusOK {
		t.Fatalf("archiving the first deputy answered %d", status)
	}
	if p := positionOn(t, editor, head, day(5)); !slices.Equal(p.Deputies, []string{second}) {
		t.Errorf("once the first deputy is archived the deputies are %v", p.Deputies)
	}
	if p := positionOn(t, editor, head, day(0)); !slices.Equal(p.Deputies, []string{first, second}) {
		t.Errorf("today the deputies are %v", p.Deputies)
	}
	if status := editor.do(http.MethodPut, "/positions/"+head+"/deputies", map[string]any{"deputyIds": []string{}}, &set); status != http.StatusOK || len(set.Position.Deputies) != 0 {
		t.Errorf("taking the deputies away answered %d %v", status, set.Position.Deputies)
	}
}
