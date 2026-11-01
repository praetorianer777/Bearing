//go:build integration

package test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/armature"
	"github.com/praetorianer777/bearing/backend/internal/db"
)

// fakeArmature offers resource-load@1 under / and an older Armature without it under /old; every person it knows
// has the same week, each week of the range.
type fakeArmature struct {
	*httptest.Server
	mu    sync.Mutex
	weeks map[string]armature.Week
	asked [][]armature.Subject
}

func newFakeArmature(t *testing.T) *fakeArmature {
	t.Helper()
	f := &fakeArmature{weeks: map[string]armature.Week{}}
	mux := http.NewServeMux()
	meta := func(capabilities ...string) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"product": "armature", "capabilities": capabilities})
		}
	}
	token := func(w http.ResponseWriter, r *http.Request) {
		// Client credentials come form encoded inside Basic, as RFC 6749 section 2.3.1 has them.
		id, secret, ok := r.BasicAuth()
		id, _ = url.QueryUnescape(id)
		secret, _ = url.QueryUnescape(secret)
		if !ok || id != "bearing" || secret != "partner secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = r.ParseForm()
		if r.Form.Get("subject_token") == "refused" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "armature_oat_" + uuid.NewString(), "refresh_token": "armature_ort_" + uuid.NewString(),
			"expires_in": 3600, "token_type": "Bearer"})
	}
	mux.HandleFunc("GET /api/v1/meta", meta("oauth.token-exchange@1", "resource-load@1"))
	mux.HandleFunc("POST /api/v1/oauth/token", token)
	mux.HandleFunc("GET /old/api/v1/meta", meta("oauth.token-exchange@1"))
	mux.HandleFunc("POST /old/api/v1/oauth/token", token)
	mux.HandleFunc("POST /api/v1/resource-load", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer armature_oat_") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var q armature.LoadQuery
		_ = json.NewDecoder(r.Body).Decode(&q)
		f.mu.Lock()
		defer f.mu.Unlock()
		f.asked = append(f.asked, q.People)
		from, _ := time.Parse(time.DateOnly, q.From)
		to, _ := time.Parse(time.DateOnly, q.To)
		var out []armature.PersonLoad
		for _, p := range q.People {
			week, ok := f.weeks[p.Subject]
			load := armature.PersonLoad{Subject: p, Found: ok}
			for day := from; ok && !day.After(to); day = day.AddDate(0, 0, 7) {
				week.Start = day.Format(time.DateOnly)
				load.Weeks = append(load.Weeks, week)
			}
			out = append(out, load)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"people": out})
	})
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	return f
}

type demandJSON struct {
	Partner struct{ Status, Message string }
	Weeks   []struct {
		Start        string
		FTE          float64
		People       int
		AllocatedFTE *float64
		LoadHours    *float64
		NominalHours *float64
		DaysAway     *float64
	}
	Over []struct {
		PersonID, Reason string
	}
	Warnings  int
	Unmatched []struct{ PersonID string }
}

// The development team: Dev works all week and is given out at 120 percent; Ann works half her week here and is
// loaded with 30 of her 40 hours; Ext has no realm name, so Armature cannot be asked about them.
func TestAUnitsDemandComesFromArmatureAsTheReader(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	editor, editorID := withRole(t, admin, "editor")
	member, memberID := withRole(t, admin, "member")
	unit := units(t, editor, unitType(t, admin), rootUnit(t, admin), "Demanded")[0]
	ids := positions(t, editor, unit, "Developer", "Analyst", "Writer")
	dev, ann, ext := somebody(t, editor), somebody(t, editor), somebody(t, editor)
	start := time.Now().AddDate(0, 0, -((int(time.Now().Weekday()) + 6) % 7))
	monday, sunday := start.Format(time.DateOnly), start.AddDate(0, 0, 13).Format(time.DateOnly)
	for i, p := range []struct {
		who   string
		share int
	}{{dev, 100}, {ann, 50}, {ext, 100}} {
		give(t, editor, map[string]any{"personId": p.who, "positionId": ids[i], "share": p.share, "from": day(-30)})
	}
	devSubject, annSubject := inRealm(t, dev), inRealm(t, ann)
	fake := newFakeArmature(t)
	fake.weeks[devSubject] = armature.Week{AllocationPercent: 120, NominalHours: 40, LoadHours: 30, DaysAway: 1, Away: []string{monday}}
	fake.weeks[annSubject] = armature.Week{AllocationPercent: 50, NominalHours: 40, LoadHours: 30}
	super := superuser(t, "BEARING_TEST_SUPERUSER_URL")
	t.Cleanup(func() {
		_, _ = super.Exec(context.Background(), `DELETE FROM armature_connection`)
		_, _ = super.Exec(context.Background(), `DELETE FROM armature_grant`)
	})
	path := "/units/" + unit + "/demand?from=" + monday + "&to=" + sunday
	read := func(b *browser) demandJSON {
		t.Helper()
		var got demandJSON
		if status := b.do(http.MethodGet, path, nil, &got); status != http.StatusOK {
			t.Fatalf("the demand answered %d", status)
		}
		return got
	}

	if got := read(editor); got.Partner.Status != "not_configured" || len(got.Weeks) != 2 || got.Weeks[0].FTE != 2.5 || got.Weeks[0].AllocatedFTE != nil {
		t.Errorf("without Armature the demand is %+v", got)
	}
	connect := func(base string) {
		t.Helper()
		if status := admin.do(http.MethodPut, "/armature/connection", map[string]string{"baseUrl": base, "clientId": "bearing", "clientSecret": "partner secret"}, nil); status != http.StatusOK {
			t.Fatalf("connecting %s answered %d", base, status)
		}
	}
	connect(fake.URL)
	if status := member.do(http.MethodGet, "/armature/connection", nil, nil); status != http.StatusForbidden {
		t.Errorf("a member reading the connection answered %d", status)
	}
	var mine struct{ Status string }
	if editor.do(http.MethodGet, "/armature/me", nil, &mine); mine.Status != "not_connected" {
		t.Errorf("before signing in through the realm the editor is %q", mine.Status)
	}
	if got := read(editor); got.Partner.Status != "not_connected" || got.Partner.Message == "" {
		t.Errorf("before the exchange the demand is %+v", got.Partner)
	}

	// What the realm sign-in does, against the same database.
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
	signIn.ConnectAtSignIn(context.Background(), editorID, "realm token of the editor")
	signIn.ConnectAtSignIn(context.Background(), memberID, "refused")
	replicated(t)
	if editor.do(http.MethodGet, "/armature/me", nil, &mine); mine.Status != "connected" {
		t.Errorf("after the exchange the editor is %q", mine.Status)
	}

	got := read(editor)
	if got.Partner.Status != "ok" || len(got.Weeks) != 2 {
		t.Fatalf("with Armature the demand is %+v", got)
	}
	w := got.Weeks[0]
	if w.People != 3 || w.AllocatedFTE == nil || *w.AllocatedFTE != 1.45 || *w.LoadHours != 45 || *w.NominalHours != 60 || *w.DaysAway != 1 {
		t.Errorf("the first week is %+v", w)
	}
	reasons := map[string]string{}
	for _, o := range got.Over {
		reasons[o.PersonID] = o.Reason
	}
	if got.Warnings != 4 || reasons[dev] != "over_allocated" || reasons[ann] != "over_loaded" {
		t.Errorf("the warnings are %d %+v", got.Warnings, got.Over)
	}
	if len(got.Unmatched) != 1 || got.Unmatched[0].PersonID != ext {
		t.Errorf("the people Armature was not asked about are %+v", got.Unmatched)
	}
	fake.mu.Lock()
	if last := fake.asked[len(fake.asked)-1]; len(last) != 2 {
		t.Errorf("Armature was asked about %+v", last)
	}
	fake.mu.Unlock()

	if got := read(member); got.Partner.Status != "not_connected" {
		t.Errorf("a member whose exchange was refused reads %+v", got.Partner)
	}
	signIn.ConnectAtSignIn(context.Background(), memberID, "realm token of the member")
	replicated(t)
	if got := read(member); got.Partner.Status != "ok" || got.Warnings != 4 || len(got.Over) != 0 || len(got.Unmatched) != 0 {
		t.Errorf("a member sees %+v", got)
	}

	connect(fake.URL + "/old")
	signIn.ConnectAtSignIn(context.Background(), editorID, "realm token of the editor")
	replicated(t)
	if got := read(editor); got.Partner.Status != "outdated" || !strings.Contains(got.Partner.Message, "resource-load@1") || !strings.Contains(got.Partner.Message, "update Armature") {
		t.Errorf("an Armature without resource-load@1 is %+v", got.Partner)
	}
	connect("http://127.0.0.1:1")
	if got := read(editor); got.Partner.Status != "unreachable" && got.Partner.Status != "not_connected" {
		t.Errorf("an Armature that does not answer is %+v", got.Partner)
	}
	if status := admin.do(http.MethodDelete, "/armature/connection", nil, nil); status != http.StatusNoContent {
		t.Errorf("disconnecting answered %d", status)
	}
	if status := editor.do(http.MethodGet, "/units/"+unit+"/demand?from="+monday+"&to="+day(400), nil, nil); status != http.StatusUnprocessableEntity {
		t.Errorf("a year of weeks answered %d", status)
	}
}
