//go:build integration

package test

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"testing"

	"github.com/google/uuid"
)

// A worker reports to a boss; each is a member signed in, linked to their entry. The worker's capacity is theirs,
// their boss's and editors' to see; a colleague beside them sees the sums only.
func TestOnlyThePersonTheirManagersAndEditorsSeeTheirCapacity(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	editor, unit := orgFixture(t)
	ids := positions(t, editor, unit, "Boss", "Worker")
	bossPosition, workerPosition := ids[0], ids[1]
	if status := editor.do(http.MethodPut, "/positions/"+workerPosition+"/reports-to", map[string]any{"reportsToId": bossPosition, "from": day(-30)}, nil); status != http.StatusOK {
		t.Fatalf("the reporting line answered %d", status)
	}
	worker, workerAccount := withRole(t, admin, "member")
	boss, bossAccount := withRole(t, admin, "member")
	colleague, _ := withRole(t, admin, "member")
	linked := func(account uuid.UUID) personJSON {
		entry := person(t, editor, map[string]any{"name": "Linked " + uuid.NewString()[:6]})
		if status := admin.do(http.MethodPut, "/people/"+entry.ID+"/account", map[string]any{"accountId": account}, nil); status != http.StatusOK {
			t.Fatalf("linking answered %d", status)
		}
		return entry
	}
	workerEntry, bossEntry := linked(workerAccount), linked(bossAccount)
	give(t, editor, map[string]any{"personId": workerEntry.ID, "positionId": workerPosition, "share": 80, "from": day(-10)})
	give(t, editor, map[string]any{"personId": bossEntry.ID, "positionId": bossPosition, "from": day(-10)})
	replicated(t)

	path := "/people/" + workerEntry.ID + "/capacity?by=day&from=" + day(-1) + "&to=" + day(-1)
	for who, b := range map[string]*browser{"the worker": worker, "their boss": boss, "an editor": editor} {
		var got capacityBody
		if status := b.do(http.MethodGet, path, nil, &got); status != http.StatusOK || len(got.Periods) != 1 || got.Periods[0].FTE != 0.8 {
			t.Errorf("%s reading the worker's capacity: %d %+v", who, status, got.Periods)
		}
	}
	var refused struct {
		Error struct{ Code, Message string }
	}
	if status := colleague.do(http.MethodGet, path, nil, &refused); status != http.StatusForbidden || refused.Error.Message == "" {
		t.Errorf("a colleague read the worker's capacity: %d %+v", status, refused)
	}
	if status := worker.do(http.MethodGet, "/people/"+bossEntry.ID+"/capacity", nil, nil); status != http.StatusForbidden {
		t.Errorf("the worker read their boss's capacity: %d", status)
	}
	if status := colleague.do(http.MethodGet, "/positions/"+workerPosition+"/capacity", nil, nil); status != http.StatusOK {
		t.Errorf("a colleague reading the position's sum: %d", status)
	}
	if status := colleague.do(http.MethodGet, "/units/"+unit+"/capacity?subUnits=true", nil, nil); status != http.StatusOK {
		t.Errorf("a colleague reading the unit's sum: %d", status)
	}
}

func TestPartnersNameAPersonByTheRealmsSubject(t *testing.T) {
	editor, unit := orgFixture(t)
	position := positions(t, editor, unit, "Analyst")[0]
	someone := somebody(t, editor)
	give(t, editor, map[string]any{"personId": someone, "positionId": position, "share": 60, "from": day(-10)})
	subject := "sub-" + uuid.NewString()
	super := superuser(t, "BEARING_TEST_SUPERUSER_URL")
	if _, err := super.Exec(context.Background(), `UPDATE person SET issuer = 'https://realm.test/realms/dynamo', subject = $2 WHERE id = $1`, someone, subject); err != nil {
		t.Fatal(err)
	}
	replicated(t)

	q := url.Values{"issuer": {"https://realm.test/realms/dynamo"}, "subject": {subject}, "by": {"day"}, "from": {day(-1)}, "to": {day(-1)}}
	var got capacityBody
	if status := editor.do(http.MethodGet, "/people/by-subject/capacity?"+q.Encode(), nil, &got); status != http.StatusOK || len(got.Periods) != 1 || got.Periods[0].FTE != 0.6 {
		t.Errorf("by the realm's subject: %d %+v", status, got.Periods)
	}
	q.Set("subject", "nobody")
	if status := editor.do(http.MethodGet, "/people/by-subject/capacity?"+q.Encode(), nil, nil); status != http.StatusNotFound {
		t.Errorf("a subject nobody has: %d", status)
	}
	if status := editor.do(http.MethodGet, "/people/by-subject/capacity?issuer=x", nil, nil); status != http.StatusUnprocessableEntity {
		t.Errorf("without a subject: %d", status)
	}

	var meta struct{ Capabilities []string }
	if status := editor.do(http.MethodGet, "/meta", nil, &meta); status != http.StatusOK || !slices.Contains(meta.Capabilities, "capacity@1") {
		t.Errorf("meta offers %v", meta.Capabilities)
	}
}
