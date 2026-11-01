//go:build integration

package test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/webhook"
)

// received is one post the receiver took, with what it needs to check the signatures.
type received struct {
	header   http.Header
	body     []byte
	envelope struct {
		Version    int
		ID         string
		Source     string
		Topic      string
		OccurredAt time.Time
		Payload    map[string]any
	}
}

// receiver listens in the test container, which the stack's worker reaches as webhook-receiver; it answers fail
// with 500 for the topics named there.
type receiver struct {
	url  string
	mu   sync.Mutex
	got  []received
	fail map[string]bool
}

func listen(t *testing.T) *receiver {
	t.Helper()
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	rc := &receiver{url: fmt.Sprintf("http://webhook-receiver:%d/hook", l.Addr().(*net.TCPAddr).Port), fail: map[string]bool{}}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var got received
		got.header, got.body = r.Header.Clone(), body
		_ = json.Unmarshal(body, &got.envelope)
		rc.mu.Lock()
		rc.got = append(rc.got, got)
		failing := rc.fail[got.envelope.Topic]
		rc.mu.Unlock()
		if failing {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close() })
	return rc
}

func (rc *receiver) failing(topic string, on bool) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.fail[topic] = on
}

// await is the first post that matches, waiting for the worker, which looks every two seconds.
func (rc *receiver) await(t *testing.T, what string, match func(received) bool) received {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		rc.mu.Lock()
		for _, g := range rc.got {
			if match(g) {
				rc.mu.Unlock()
				return g
			}
		}
		rc.mu.Unlock()
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("no post of %s arrived", what)
	return received{}
}

func signedBy(t *testing.T, g received, secret string) {
	t.Helper()
	if got := g.header.Get(webhook.SignatureHeader); got != webhook.Sign(g.body, secret) {
		t.Errorf("the signature %q does not match the body", got)
	}
	var at int64
	fmt.Sscan(g.header.Get(webhook.TimestampHeader), &at)
	if got := g.header.Get(webhook.TimestampedSignatureHeader); got != webhook.SignTimestamped(g.body, secret, time.Unix(at, 0)) {
		t.Errorf("the timestamped signature %q does not match", got)
	}
	if g.header.Get(webhook.EventHeader) != g.envelope.Topic || g.header.Get(webhook.VersionHeader) != "1" || g.header.Get(webhook.DeliveryHeader) == "" {
		t.Errorf("the headers are %v", g.header)
	}
	if g.envelope.Version != 1 || g.envelope.Source != "bearing" || g.envelope.ID == "" || g.envelope.OccurredAt.IsZero() {
		t.Errorf("the envelope is %+v", g.envelope)
	}
}

type webhookJSON struct {
	ID       string
	Topics   []string
	Enabled  bool
	Failures int
	Owner    *struct{ ID string }
	Secret   string
}

type deliveryJSON struct {
	ID      string
	EventID string
	Topic   string
	Attempt int
	Manual  bool
	State   string
	Status  *int
	Error   string
}

func payloadID(g received, key string) string {
	part, _ := g.envelope.Payload[key].(map[string]any)
	id, _ := part["id"].(string)
	return id
}

func TestWebhooksPostChangesSignedAndRetryWhatFails(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	editor, _ := withRole(t, admin, "editor")
	rc := listen(t)

	var made struct{ Webhook webhookJSON }
	topics := []string{"responsibility.changed", "position.vacated", "position.holder_changed", "position.holder_changed"}
	if status := admin.do(http.MethodPost, "/webhooks", map[string]any{"name": "Receiver " + uuid.NewString()[:8], "url": rc.url, "topics": topics}, &made); status != http.StatusCreated {
		t.Fatalf("making the webhook answered %d %+v", status, made)
	}
	hook := made.Webhook
	super := superuser(t, "BEARING_TEST_SUPERUSER_URL")
	t.Cleanup(func() { _, _ = super.Exec(context.Background(), `DELETE FROM webhook_endpoint WHERE id = $1`, hook.ID) })
	if !strings.HasPrefix(hook.Secret, webhook.SecretPrefix) || !hook.Enabled || hook.Owner == nil ||
		strings.Join(hook.Topics, ",") != "position.holder_changed,position.vacated,responsibility.changed" {
		t.Errorf("the webhook is %+v", hook)
	}
	var listed struct{ Webhooks []webhookJSON }
	admin.do(http.MethodGet, "/webhooks", nil, &listed)
	for _, w := range listed.Webhooks {
		if w.Secret != "" {
			t.Errorf("the list shows a secret: %+v", w)
		}
	}

	var tested struct{ Delivery deliveryJSON }
	if status := admin.do(http.MethodPost, "/webhooks/"+hook.ID+"/test", nil, &tested); status != http.StatusOK || tested.Delivery.State != "delivered" || !tested.Delivery.Manual {
		t.Fatalf("the test answered %d %+v", status, tested.Delivery)
	}
	signedBy(t, rc.await(t, "the ping", func(g received) bool { return g.envelope.Topic == "ping" }), hook.Secret)

	unit := units(t, editor, unitType(t, admin), rootUnit(t, admin), "Hooked")[0]
	position := positions(t, editor, unit, "Hooked analyst")[0]
	someone := somebody(t, editor)
	held := give(t, editor, map[string]any{"personId": someone, "positionId": position, "from": day(-5)})
	added := rc.await(t, "the holder added", func(g received) bool {
		return g.envelope.Topic == "position.holder_changed" && payloadID(g, "position") == position && g.envelope.Payload["change"] == "added"
	})
	signedBy(t, added, hook.Secret)
	if payloadID(added, "person") != someone || payloadID(added, "holding") != held.ID {
		t.Errorf("the holder added is %+v", added.envelope.Payload)
	}
	if actor, _ := added.envelope.Payload["actor"].(map[string]any); actor == nil || actor["id"] == nil {
		t.Errorf("the change names no actor: %+v", added.envelope.Payload)
	}

	editor.do(http.MethodPost, "/holdings/"+held.ID+"/end", map[string]any{"on": day(3)}, nil)
	vacated := rc.await(t, "the position vacated", func(g received) bool {
		return g.envelope.Topic == "position.vacated" && payloadID(g, "position") == position
	})
	if vacated.envelope.Payload["on"] != day(3) || payloadID(vacated, "person") != someone {
		t.Errorf("the vacancy is %+v", vacated.envelope.Payload)
	}

	rc.failing("responsibility.changed", true)
	approver := orgRole(t, editor, "Hooked approver")
	editor.do(http.MethodPost, "/positions/"+position+"/roles", map[string]any{"roleId": approver.ID, "from": day(-1)}, nil)
	first := rc.await(t, "the role carried", func(g received) bool {
		return g.envelope.Topic == "responsibility.changed" && payloadID(g, "role") == approver.ID
	})
	var log struct{ Deliveries []deliveryJSON }
	var failed, retry deliveryJSON
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && retry.ID == "" {
		admin.do(http.MethodGet, "/webhooks/"+hook.ID+"/deliveries", nil, &log)
		for _, d := range log.Deliveries {
			if d.EventID == first.envelope.ID && d.Attempt == 1 && d.State == "failed" {
				failed = d
			}
			if d.EventID == first.envelope.ID && d.Attempt == 2 && d.State == "pending" {
				retry = d
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	if failed.Status == nil || *failed.Status != http.StatusInternalServerError || failed.Error == "" || retry.ID == "" {
		t.Fatalf("after a failure the log is %+v", log.Deliveries)
	}
	var got struct{ Webhooks []webhookJSON }
	admin.do(http.MethodGet, "/webhooks", nil, &got)
	for _, w := range got.Webhooks {
		if w.ID == hook.ID && w.Failures != 1 {
			t.Errorf("the webhook counts %d failures", w.Failures)
		}
	}

	rc.failing("responsibility.changed", false)
	var again struct{ Delivery deliveryJSON }
	if status := admin.do(http.MethodPost, "/webhooks/"+hook.ID+"/deliveries/"+failed.ID+"/redeliver", nil, &again); status != http.StatusOK ||
		again.Delivery.State != "delivered" || !again.Delivery.Manual || again.Delivery.EventID != first.envelope.ID {
		t.Errorf("the redelivery answered %d %+v", status, again.Delivery)
	}

	var rotated struct{ Webhook webhookJSON }
	if status := admin.do(http.MethodPost, "/webhooks/"+hook.ID+"/rotate-secret", nil, &rotated); status != http.StatusOK || rotated.Webhook.Secret == hook.Secret ||
		!strings.HasPrefix(rotated.Webhook.Secret, webhook.SecretPrefix) {
		t.Fatalf("rotating answered %d %+v", status, rotated.Webhook)
	}
	admin.do(http.MethodPost, "/webhooks/"+hook.ID+"/test", nil, &tested)
	latest := rc.await(t, "the ping after rotating", func(g received) bool { return g.header.Get(webhook.DeliveryHeader) == tested.Delivery.ID })
	signedBy(t, latest, rotated.Webhook.Secret)

	if status := admin.do(http.MethodDelete, "/webhooks/"+hook.ID, nil, nil); status != http.StatusNoContent {
		t.Errorf("deleting answered %d", status)
	}
	if status := admin.do(http.MethodGet, "/webhooks/"+hook.ID+"/deliveries", nil, nil); status != http.StatusNotFound {
		t.Errorf("the log of a deleted webhook answered %d", status)
	}
}

func TestWebhooksAreTheAdministratorsAndPostOnlyOutward(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	editor, _ := withRole(t, admin, "editor")
	if status := editor.do(http.MethodGet, "/webhooks", nil, nil); status != http.StatusForbidden {
		t.Errorf("an editor listing webhooks answered %d", status)
	}
	var r refusal
	for name, body := range map[string]map[string]any{
		"the server itself": {"name": "Inside", "url": "http://127.0.0.1:9000/hook", "topics": []string{"*"}},
		"localhost":         {"name": "Inside", "url": "http://localhost/hook", "topics": []string{"*"}},
		"no topics":         {"name": "Empty", "url": "https://example.com/hook", "topics": []string{}},
		"an unknown topic":  {"name": "Odd", "url": "https://example.com/hook", "topics": []string{"page.published"}},
		"no name":           {"name": " ", "url": "https://example.com/hook", "topics": []string{"*"}},
	} {
		if status := admin.do(http.MethodPost, "/webhooks", body, &r); status != http.StatusUnprocessableEntity {
			t.Errorf("%s answered %d", name, status)
		}
	}
	var meta struct{ Capabilities []string }
	admin.do(http.MethodGet, "/meta", nil, &meta)
	if !strings.Contains(strings.Join(meta.Capabilities, " "), "webhooks@1") {
		t.Errorf("/meta does not list webhooks@1: %v", meta.Capabilities)
	}
}

// hooked makes a webhook taking topics that posts to rc, removed after the test.
func hooked(t *testing.T, admin *browser, rc *receiver, topics ...string) webhookJSON {
	t.Helper()
	var made struct{ Webhook webhookJSON }
	if status := admin.do(http.MethodPost, "/webhooks", map[string]any{"name": "Receiver " + uuid.NewString()[:8], "url": rc.url, "topics": topics}, &made); status != http.StatusCreated {
		t.Fatalf("making the webhook answered %d", status)
	}
	super := superuser(t, "BEARING_TEST_SUPERUSER_URL")
	t.Cleanup(func() {
		_, _ = super.Exec(context.Background(), `DELETE FROM webhook_endpoint WHERE id = $1`, made.Webhook.ID)
	})
	return made.Webhook
}

// count is how many posts so far match.
func (rc *receiver) count(match func(received) bool) int {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	n := 0
	for _, g := range rc.got {
		if match(g) {
			n++
		}
	}
	return n
}

func TestHoldingsArePlannedStartedAndEndedOnTheirDayAndPeopleLeave(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	editor, _ := withRole(t, admin, "editor")
	rc := listen(t)
	hook := hooked(t, admin, rc, "holding.planned", "holding.started", "holding.ended", "person.left")
	unit := units(t, editor, unitType(t, admin), rootUnit(t, admin), "Planned")[0]
	ids := positions(t, editor, unit, "Starter", "Ender", "Newcomer")
	starter, ender, newcomer := ids[0], ids[1], ids[2]
	ana, ben, cleo := somebody(t, editor), somebody(t, editor), somebody(t, editor)

	planned := give(t, editor, map[string]any{"personId": cleo, "positionId": newcomer, "from": day(10)})
	got := rc.await(t, "the holding planned", func(g received) bool {
		return g.envelope.Topic == "holding.planned" && payloadID(g, "holding") == planned.ID
	})
	signedBy(t, got, hook.Secret)
	if payloadID(got, "position") != newcomer || payloadID(got, "person") != cleo {
		t.Errorf("the plan says %+v", got.envelope.Payload)
	}

	// Written yesterday for today, as moving them onto today unseen makes them: the worker announces them today, as
	// nobody writes on the day itself.
	starting := give(t, editor, map[string]any{"personId": ana, "positionId": starter, "from": day(1)})
	ending := give(t, editor, map[string]any{"personId": ben, "positionId": ender, "from": day(-20), "to": day(1)})
	super := superuser(t, "BEARING_TEST_SUPERUSER_URL")
	ctx := context.Background()
	tx, err := super.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		`SET LOCAL session_replication_role = replica`,
		`UPDATE holding SET valid_from = current_date, created_at = now() - interval '1 day', updated_at = now() - interval '1 day' WHERE id = '` + starting.ID + `'`,
		`UPDATE holding SET valid_to = current_date, created_at = now() - interval '1 day', updated_at = now() - interval '1 day' WHERE id = '` + ending.ID + `'`,
		`DELETE FROM webhook_sweep`,
		`INSERT INTO webhook_sweep (day) VALUES (current_date - 1)`,
	} {
		if _, err := tx.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	started := func(g received) bool {
		return g.envelope.Topic == "holding.started" && payloadID(g, "holding") == starting.ID
	}
	ended := func(g received) bool {
		return g.envelope.Topic == "holding.ended" && payloadID(g, "holding") == ending.ID
	}
	if g := rc.await(t, "the holding started today", started); g.envelope.Payload["actor"] != nil || payloadID(g, "person") != ana {
		t.Errorf("the start says %+v", g.envelope.Payload)
	}
	rc.await(t, "the holding ended today", ended)

	editor.do(http.MethodPost, "/people/"+ben+"/leave", map[string]any{"on": day(5)}, nil)
	left := rc.await(t, "the person leaving", func(g received) bool { return g.envelope.Topic == "person.left" && payloadID(g, "person") == ben })
	if left.envelope.Payload["on"] != day(5) {
		t.Errorf("the leaving says %+v", left.envelope.Payload)
	}

	// Long enough for a second sweep and a second fan out to have run, had either posted anything again.
	time.Sleep(5 * time.Second)
	for name, match := range map[string]func(received) bool{"started": started, "ended": ended,
		"planned": func(g received) bool {
			return g.envelope.Topic == "holding.planned" && payloadID(g, "holding") == planned.ID
		}} {
		if n := rc.count(match); n != 1 {
			t.Errorf("%s was posted %d times", name, n)
		}
	}
}

func TestAGapChangeIsPostedWhenAHoldingIsPlannedIntoIt(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	editor, _ := withRole(t, admin, "editor")
	rc := listen(t)
	hooked(t, admin, rc, "capacity.gap_changed")
	super := superuser(t, "BEARING_TEST_SUPERUSER_URL")
	deadline := time.Now().Add(30 * time.Second)
	for {
		var seen bool
		if err := super.QueryRow(context.Background(), `SELECT EXISTS (SELECT 1 FROM webhook_gap_window)`).Scan(&seen); err != nil {
			t.Fatal(err)
		}
		if seen {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the worker never read the gaps")
		}
		time.Sleep(200 * time.Millisecond)
	}

	unit := units(t, editor, unitType(t, admin), rootUnit(t, admin), "Gappy")[0]
	position := positions(t, editor, unit, "Wanted")[0]
	gapOf := func(g received) map[string]any {
		gap, _ := g.envelope.Payload["gap"].(map[string]any)
		return gap
	}
	opened := rc.await(t, "the vacancy opened", func(g received) bool {
		gap := gapOf(g)
		return g.envelope.Topic == "capacity.gap_changed" && gap["subjectId"] == position && gap["kind"] == "vacant" && g.envelope.Payload["change"] == "opened"
	})
	if gapOf(opened)["to"] != nil || gapOf(opened)["size"] != 1.0 {
		t.Errorf("the vacancy opened as %+v", opened.envelope.Payload)
	}

	give(t, editor, map[string]any{"personId": somebody(t, editor), "positionId": position, "from": day(10)})
	changed := rc.await(t, "the vacancy cut short", func(g received) bool {
		gap := gapOf(g)
		return g.envelope.Topic == "capacity.gap_changed" && gap["subjectId"] == position && g.envelope.Payload["change"] == "changed"
	})
	if gapOf(changed)["to"] != day(9) {
		t.Errorf("the vacancy now says %+v", changed.envelope.Payload)
	}
}
