package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/config"
	"github.com/praetorianer777/bearing/backend/internal/netguard"
	"github.com/praetorianer777/bearing/backend/internal/secret"
)

func TestASignatureIsTheHexHMACOfTheBody(t *testing.T) {
	mac := hmac.New(sha256.New, []byte("s"))
	mac.Write([]byte(`{"a":1}`))
	if got := Sign([]byte(`{"a":1}`), "s"); got != "sha256="+hex.EncodeToString(mac.Sum(nil)) {
		t.Errorf("the signature is %q", got)
	}
	at := time.Unix(1700000000, 0)
	if SignTimestamped([]byte("b"), "s", at) != Sign([]byte("1700000000.b"), "s") {
		t.Error("the timestamped signature does not cover the time, a dot and the body")
	}
	if Sign([]byte("b"), "s") == Sign([]byte("b"), "t") {
		t.Error("two secrets sign alike")
	}
}

func TestRetriesWaitLongerEachTimeAndAFailingDayTurnsAWebhookOff(t *testing.T) {
	var waits []time.Duration
	for attempt := range MaxAttempts {
		waits = append(waits, NextWait(attempt+1))
	}
	if !slices.Equal(waits, []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 12 * time.Hour, 12 * time.Hour}) {
		t.Errorf("the waits are %v", waits)
	}
	now := time.Now()
	for name, c := range map[string]struct {
		failures int
		since    time.Time
		want     bool
	}{
		"a day of six failures":    {6, now.Add(-DisableAfter), true},
		"six failures this hour":   {6, now.Add(-time.Hour), false},
		"one failure a day ago":    {1, now.Add(-2 * DisableAfter), false},
		"many failures for longer": {40, now.Add(-3 * DisableAfter), true},
	} {
		if got := ShouldDisable(c.failures, c.since, now); got != c.want {
			t.Errorf("%s: %v", name, got)
		}
	}
}

func TestAnInputIsCheckedAndItsTopicsPutInOrder(t *testing.T) {
	s := &Service{allow: netguard.ParseAllow("receiver,10.1.0.0/16")}
	in, err := s.clean(Input{Name: " Feed ", URL: " https://example.com/hook ", Topics: []string{TopicVacated, TopicAny, TopicVacated}})
	if err != nil || in.Name != "Feed" || in.URL != "https://example.com/hook" || !slices.Equal(in.Topics, []string{TopicAny, TopicVacated}) {
		t.Errorf("%+v %v", in, err)
	}
	for _, url := range []string{"http://receiver:9000/hook", "http://10.1.2.3/hook"} {
		if _, err := s.clean(Input{Name: "x", URL: url, Topics: []string{TopicAny}}); err != nil {
			t.Errorf("%s, allowed, is refused: %v", url, err)
		}
	}
	version := 2
	for name, c := range map[string]struct {
		in    Input
		field string
	}{
		"no name":         {Input{URL: "https://e.com", Topics: []string{TopicAny}}, "name"},
		"ftp":             {Input{Name: "x", URL: "ftp://e.com", Topics: []string{TopicAny}}, "url"},
		"a user name":     {Input{Name: "x", URL: "https://u:p@e.com", Topics: []string{TopicAny}}, "url"},
		"a fragment":      {Input{Name: "x", URL: "https://e.com/#a", Topics: []string{TopicAny}}, "url"},
		"loopback":        {Input{Name: "x", URL: "http://127.0.0.1/", Topics: []string{TopicAny}}, "url"},
		"a private range": {Input{Name: "x", URL: "http://192.168.1.1/", Topics: []string{TopicAny}}, "url"},
		"localhost":       {Input{Name: "x", URL: "http://api.localhost/", Topics: []string{TopicAny}}, "url"},
		"no topics":       {Input{Name: "x", URL: "https://e.com"}, "topics"},
		"a topic unknown": {Input{Name: "x", URL: "https://e.com", Topics: []string{"page.published"}}, "topics"},
		"a version":       {Input{Name: "x", URL: "https://e.com", Topics: []string{TopicAny}, Version: &version}, "version"},
	} {
		var field *FieldError
		if _, err := s.clean(c.in); !errors.As(err, &field) || field.Field != c.field {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestAFailureIsSaidWithoutNamingTheNetwork(t *testing.T) {
	for err, want := range map[error]string{
		fmt.Errorf("dial: %w", netguard.ErrBlocked): whyBlocked,
		context.DeadlineExceeded:                    whyTimeout,
		errors.New("i/o timeout"):                   whyTimeout,
		errors.New("connection refused 10.0.0.1"):   whyUnreached,
	} {
		if got := why(err); got != want {
			t.Errorf("%v: %q", err, got)
		}
	}
}

// fakeLookup names everything by its id and keeps positions vacant as told.
type fakeLookup struct{ vacant map[string]bool }

func (f fakeLookup) Position(id string) map[string]any { return map[string]any{"id": id} }
func (f fakeLookup) Person(id string) map[string]any   { return map[string]any{"id": id} }
func (f fakeLookup) Unit(id string) map[string]any     { return map[string]any{"id": id} }
func (f fakeLookup) Role(id string) map[string]any     { return map[string]any{"id": id} }
func (f fakeLookup) Process(id string) map[string]any  { return map[string]any{"id": id} }
func (f fakeLookup) Actor(id uuid.UUID) map[string]any { return map[string]any{"id": id.String()} }
func (f fakeLookup) Vacant(position, day string) bool  { return f.vacant[position+"@"+day] }

func topicsOf(events []Event) []string {
	var out []string
	for _, e := range events {
		out = append(out, e.Topic)
	}
	return out
}

func TestEachChangeIsAnnouncedAsTheTopicsItMeans(t *testing.T) {
	look := fakeLookup{vacant: map[string]bool{"p@2026-11-05": true, "p@2026-11-01": true}}
	if p := HoldingPayload(look, map[string]any{"role_id": "r", "person_id": "x"}, map[string]any{"id": "h"}); p["role"] == nil || p["position"] != nil {
		t.Errorf("a role's holding says %v", p)
	}
	held := map[string]any{"id": "h", "person_id": "x", "position_id": "p", "share": 100, "valid_from": "2026-10-01", "valid_to": nil}
	ended := map[string]any{"id": "h", "person_id": "x", "position_id": "p", "share": 100, "valid_from": "2026-10-01", "valid_to": "2026-11-05"}
	actor := uuid.New()
	for name, c := range map[string]struct {
		change Change
		want   []string
	}{
		"a holder given":         {Change{Table: "holding", Operation: "insert", After: held, ActorID: &actor}, []string{TopicHolderChanged, TopicHoldingStarted}},
		"a holder planned":       {Change{Table: "holding", Operation: "insert", After: map[string]any{"position_id": "p", "valid_from": "2026-12-01"}}, []string{TopicHolderChanged, TopicHoldingPlanned}},
		"a start moved ahead":    {Change{Table: "holding", Operation: "update", Before: held, After: map[string]any{"position_id": "q", "valid_from": "2026-12-01"}}, []string{TopicHolderChanged, TopicHoldingPlanned}},
		"a holding ended today":  {Change{Table: "holding", Operation: "update", Before: held, After: map[string]any{"position_id": "p", "valid_from": "2026-10-01", "valid_to": "2026-11-01"}}, []string{TopicHolderChanged, TopicVacated, TopicHoldingEnded}},
		"a person leaving":       {Change{Table: "person", Operation: "update", Before: map[string]any{"id": "x"}, After: map[string]any{"id": "x", "left_on": "2026-12-01"}}, []string{TopicPersonLeft}},
		"a person renamed":       {Change{Table: "person", Operation: "update", Before: map[string]any{"id": "x", "name": "A"}, After: map[string]any{"id": "x", "name": "B"}}, nil},
		"a holding cut short":    {Change{Table: "holding", Operation: "update", Before: held, After: ended}, []string{TopicHolderChanged, TopicVacated}},
		"a holding removed":      {Change{Table: "holding", Operation: "delete", Before: held}, []string{TopicHolderChanged, TopicVacated}},
		"a future one removed":   {Change{Table: "holding", Operation: "delete", Before: map[string]any{"position_id": "p", "valid_from": "2027-01-01"}}, []string{TopicHolderChanged}},
		"a role held":            {Change{Table: "holding", Operation: "insert", After: map[string]any{"role_id": "r", "person_id": "x"}}, []string{TopicResponsibilityChanged, TopicHoldingStarted}},
		"a line drawn":           {Change{Table: "reporting_line", Operation: "insert", After: map[string]any{"position_id": "p", "reports_to_id": "q"}}, []string{TopicReportingLineChanged}},
		"a unit renamed":         {Change{Table: "unit", Operation: "update", After: map[string]any{"id": "u", "name": "U"}}, []string{TopicUnitChanged}},
		"a unit moved":           {Change{Table: "unit_placement", Operation: "insert", After: map[string]any{"unit_id": "u", "parent_id": "v"}}, []string{TopicUnitChanged}},
		"a role carried":         {Change{Table: "position_role", Operation: "insert", After: map[string]any{"role_id": "r", "position_id": "p"}}, []string{TopicResponsibilityChanged}},
		"a deputy named":         {Change{Table: "deputy", Operation: "insert", After: map[string]any{"position_id": "p", "deputy_id": "d", "rank": 1}}, []string{TopicResponsibilityChanged}},
		"a process published":    {Change{Table: "process_version", Operation: "update", After: map[string]any{"process_id": "pr", "number": 2}}, []string{TopicResponsibilityChanged}},
		"a table nobody follows": {Change{Table: "target", Operation: "update", After: map[string]any{"id": "x"}}, nil},
	} {
		if got := topicsOf(Announce(c.change, look, "2026-11-01")); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", name, got, c.want)
		}
	}

	events := Announce(Change{Table: "holding", Operation: "update", Before: held, After: ended, ActorID: &actor}, look, "2026-11-01")
	if p := events[0].Payload; p["change"] != "changed" || p["position"].(map[string]any)["id"] != "p" || p["actor"].(map[string]any)["id"] != actor.String() {
		t.Errorf("the holder changed says %v", p)
	}
	if p := events[1].Payload; p["on"] != "2026-11-05" || p["person"].(map[string]any)["id"] != "x" {
		t.Errorf("the vacancy says %v", p)
	}
	if p := Announce(Change{Table: "unit", Operation: "delete", Before: map[string]any{"id": "u", "name": "Gone"}}, fakeLookup{}, "")[0].Payload; p["change"] != "removed" || p["actor"] != nil {
		t.Errorf("a unit removed says %v", p)
	}
}

// sending is a service posting to receivers on this machine, with a secret sealed for endpoint.
func sending(t *testing.T) (*Service, claimed, string) {
	t.Helper()
	box, err := secret.New(make([]byte, secret.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	s, err := FromConfig(config.Outbound{SecretKey: make([]byte, secret.KeySize), Allow: "127.0.0.1"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.box = box
	plain, err := newSecret()
	if err != nil || !strings.HasPrefix(plain, SecretPrefix) {
		t.Fatalf("%q %v", plain, err)
	}
	endpoint := uuid.New()
	sealed, err := box.Seal([]byte(plain), sealContext(endpoint))
	if err != nil {
		t.Fatal(err)
	}
	return s, claimed{endpointID: endpoint, eventID: uuid.New(), topic: TopicVacated, sealed: sealed, enabled: true, owned: true,
		payload: json.RawMessage(`{"on":"2026-12-01"}`), occurred: time.Now(), attempt: 1, state: StatePending, version: CurrentVersion}, plain
}

func TestASendIsSignedAndSaysHowTheReceiverAnswered(t *testing.T) {
	s, c, plain := sending(t)
	var got *http.Request
	var body []byte
	status := http.StatusNoContent
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, body = r, must(io.ReadAll(r.Body))
		w.WriteHeader(status)
	}))
	defer receiver.Close()
	c.url = receiver.URL
	id := uuid.New()

	if o := s.send(t.Context(), id, c); o.state != StateDelivered || o.status == nil || *o.status != http.StatusNoContent {
		t.Fatalf("a receiver answering 204: %+v", o)
	}
	var envelope Envelope
	if err := json.Unmarshal(body, &envelope); err != nil || envelope.ID != c.eventID || envelope.Source != Source || envelope.Topic != TopicVacated ||
		envelope.Version != 1 || string(envelope.Payload) != `{"on":"2026-12-01"}` {
		t.Errorf("the envelope is %s %v", body, err)
	}
	at, _ := strconv.ParseInt(got.Header.Get(TimestampHeader), 10, 64)
	if got.Header.Get(SignatureHeader) != Sign(body, plain) || got.Header.Get(TimestampedSignatureHeader) != SignTimestamped(body, plain, time.Unix(at, 0)) ||
		got.Header.Get(DeliveryHeader) != id.String() || got.Header.Get(EventHeader) != TopicVacated || got.Header.Get(VersionHeader) != "1" ||
		got.Header.Get("User-Agent") != userAgent {
		t.Errorf("the headers are %v", got.Header)
	}

	status = http.StatusInternalServerError
	if o := s.send(t.Context(), id, c); o.state != StateFailed || !o.counts || o.status == nil || !strings.Contains(o.why, "500") {
		t.Errorf("a receiver answering 500: %+v", o)
	}
}

func TestASendThatCannotBeMadeIsSaidSo(t *testing.T) {
	s, c, _ := sending(t)
	c.url = "http://127.0.0.1:1/hook"
	for name, change := range map[string]struct {
		edit func(*claimed)
		want outcome
	}{
		"an endpoint turned off":       {func(c *claimed) { c.enabled = false }, outcome{state: StateCancelled, why: whyOff}},
		"an owner no longer in charge": {func(c *claimed) { c.owned = false }, outcome{state: StateWithheld, why: whyNoOwner}},
		"a version not sent":           {func(c *claimed) { c.version = 2 }, outcome{state: StateFailed, why: fmt.Sprintf(whyVersion, 2, CurrentVersion)}},
		"a secret for another row":     {func(c *claimed) { c.endpointID = uuid.New() }, outcome{state: StateFailed, why: whyNoSecret, counts: true}},
		"nobody listening":             {func(*claimed) {}, outcome{state: StateFailed, why: whyUnreached, counts: true}},
	} {
		attempt := c
		change.edit(&attempt)
		if o := s.send(t.Context(), uuid.New(), attempt); o != change.want {
			t.Errorf("%s: %+v", name, o)
		}
	}
	c.owned, c.topic = false, TopicPing
	if o := s.send(t.Context(), uuid.New(), c); o.state != StateFailed || o.why != whyUnreached {
		t.Errorf("a ping is sent without an owner: %+v", o)
	}
	inside := New(nil, s.box, netguard.ParseAllow(""), nil)
	c.topic = TopicVacated
	c.owned = true
	if o := inside.send(t.Context(), uuid.New(), c); o.why != whyBlocked {
		t.Errorf("an address inside the network: %+v", o)
	}
}

func TestWithoutAKeyNoWebhookIsMade(t *testing.T) {
	s, err := FromConfig(config.Outbound{}, nil, nil)
	if err != nil || s.box != nil {
		t.Fatalf("%v", err)
	}
	if _, _, err := s.Create(t.Context(), Input{Name: "x", URL: "https://example.com", Topics: []string{TopicAny}}); !errors.Is(err, ErrNoKey) {
		t.Errorf("making one: %v", err)
	}
	if _, _, err := s.RotateSecret(t.Context(), uuid.New()); !errors.Is(err, ErrNoKey) {
		t.Errorf("rotating: %v", err)
	}
	if _, err := FromConfig(config.Outbound{SecretKey: []byte("short")}, nil, nil); err == nil {
		t.Error("a short key was taken")
	}
	var field *FieldError
	if _, _, err := s.Update(t.Context(), uuid.New(), Input{}); !errors.As(err, &field) {
		t.Errorf("an empty change: %v", err)
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func TestGapsAreComparedWithoutTheWindowMovingCountingAsAChange(t *testing.T) {
	position, role := uuid.New(), uuid.New()
	was := window{From: "2026-11-01", To: "2027-01-30"}
	now := window{From: "2026-11-02", To: "2027-01-31"}
	before := []Gap{
		{Kind: "vacant", SubjectKind: "position", SubjectID: position, From: "2026-11-01", To: "2027-01-30", Size: 1},
		{Kind: "unheld", SubjectKind: "role", SubjectID: role, From: "2026-11-20", To: "2026-12-01"},
		{Kind: "under_target", SubjectKind: "unit", SubjectID: role, From: "2026-11-01", To: "2026-11-01", Size: 0.5},
	}
	after := []Gap{
		{Kind: "vacant", SubjectKind: "position", SubjectID: position, From: "2026-11-02", To: "2027-01-31", Size: 1},
		{Kind: "unheld", SubjectKind: "role", SubjectID: role, From: "2026-11-20", To: "2026-11-25"},
		{Kind: "single_holder", SubjectKind: "role", SubjectID: role, From: "2026-11-26", To: "2027-01-31", Size: 1},
	}
	var got []string
	for _, e := range DiffGaps(before, was, after, now) {
		gap := e.Payload["gap"].(map[string]any)
		got = append(got, fmt.Sprintf("%s %s %v", e.Payload["change"], gap["kind"], gap["to"]))
	}
	if want := []string{"changed unheld 2026-11-25", "opened single_holder <nil>", "closed under_target 2026-11-01"}; !slices.Equal(got, want) {
		t.Errorf("the changes are %v, want %v", got, want)
	}
	if len(DiffGaps(after, now, after, now)) != 0 {
		t.Error("the same gaps changed")
	}
}
