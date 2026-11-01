package armature

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/config"
	"github.com/praetorianer777/bearing/backend/internal/db"
	"github.com/praetorianer777/bearing/backend/internal/netguard"
)

func serving(t *testing.T, h http.HandlerFunc) (caller, string) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return caller{http: netguard.Client(CallTimeout, netguard.ParseAllow("127.0.0.1"))}, srv.URL
}

func TestMetaIsWhatArmatureListsAndNothingWithoutIt(t *testing.T) {
	c, base := serving(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/meta" {
			t.Errorf("asked %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"capabilities":["resource-load@1"]}`))
	})
	if got, err := c.meta(t.Context(), base+"/"); err != nil || !slices.Equal(got, []string{"resource-load@1"}) {
		t.Errorf("%v %v", got, err)
	}
	c, base = serving(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) })
	if got, err := c.meta(t.Context(), base); err != nil || len(got) != 0 {
		t.Errorf("an Armature without /meta: %v %v", got, err)
	}
	c, base = serving(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) })
	if _, err := c.meta(t.Context(), base); err == nil {
		t.Error("a failing /meta was believed")
	}
}

func TestATokenIsTradedAndRenewedAtTheTokenEndpoint(t *testing.T) {
	var forms []string
	c, base := serving(t, func(w http.ResponseWriter, r *http.Request) {
		id, secret, _ := r.BasicAuth()
		_ = r.ParseForm()
		forms = append(forms, id+"|"+secret+"|"+r.Form.Get("grant_type")+"|"+r.Form.Get("subject_token")+r.Form.Get("refresh_token")+"|"+r.Form.Get("scope"))
		if r.Form.Get("refresh_token") == "spent" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(tokens{AccessToken: "a", RefreshToken: "r", ExpiresIn: 3600})
	})
	if got, err := c.exchange(t.Context(), base, "bearing app", "s&s", "realm"); err != nil || got.AccessToken != "a" || got.ExpiresIn != 3600 {
		t.Errorf("%+v %v", got, err)
	}
	if _, err := c.refresh(t.Context(), base, "bearing app", "s&s", "old"); err != nil {
		t.Error(err)
	}
	_, err := c.refresh(t.Context(), base, "bearing app", "s&s", "spent")
	var status *errStatus
	if !errors.As(err, &status) || status.Status != http.StatusBadRequest || status.Code != "invalid_grant" {
		t.Errorf("a spent refresh token: %v", err)
	}
	if want := []string{"bearing+app|s%26s|" + grantTokenExchange + "|realm|read", "bearing+app|s%26s|refresh_token|old|", "bearing+app|s%26s|refresh_token|spent|"}; !slices.Equal(forms, want) {
		t.Errorf("the forms are %v", forms)
	}
	c, base = serving(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"access_token":"a"}`)) })
	if _, err := c.exchange(t.Context(), base, "x", "y", "z"); err == nil {
		t.Error("an answer without a refresh token was taken")
	}
}

func TestTheLoadIsAskedAsTheReader(t *testing.T) {
	c, base := serving(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer access" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"code":"unauthorized"}}`))
			return
		}
		var q LoadQuery
		_ = json.NewDecoder(r.Body).Decode(&q)
		_ = json.NewEncoder(w).Encode(loadAnswer{People: []PersonLoad{{Subject: q.People[0], Found: true, Weeks: []Week{{Start: q.From, LoadHours: 8}}}}})
	})
	q := LoadQuery{From: "2026-11-02", To: "2026-11-08", People: []Subject{{Issuer: "i", Subject: "s"}}}
	got, err := c.resourceLoad(t.Context(), base, "access", q)
	if err != nil || len(got) != 1 || got[0].Subject.Subject != "s" || got[0].Weeks[0].LoadHours != 8 {
		t.Errorf("%+v %v", got, err)
	}
	_, err = c.resourceLoad(t.Context(), base, "other", q)
	var status *errStatus
	if !errors.As(err, &status) || status.Status != http.StatusUnauthorized || status.Code != "unauthorized" {
		t.Errorf("another token: %v", err)
	}
}

func TestCapabilitiesAreKeptAWhileAndAMissingOneNamesArmature(t *testing.T) {
	asked := 0
	c, base := serving(t, func(w http.ResponseWriter, _ *http.Request) {
		asked++
		_, _ = w.Write([]byte(`{"capabilities":["oauth.token-exchange@1"]}`))
	})
	s := New(nil, nil, netguard.ParseAllow("127.0.0.1"), nil, nil)
	s.call = c
	now := time.Now()
	s.now = func() time.Time { return now }
	for range 3 {
		if err := s.need(t.Context(), base, CapabilityTokenExchange); err != nil {
			t.Fatal(err)
		}
	}
	var outdated *OutdatedError
	if err := s.need(t.Context(), base, CapabilityResourceLoad); !errors.As(err, &outdated) || outdated.Capability != CapabilityResourceLoad {
		t.Errorf("a missing capability: %v", err)
	}
	if asked != 1 {
		t.Errorf("/meta was asked %d times", asked)
	}
	now = now.Add(CapabilitiesTTL + time.Second)
	_ = s.need(t.Context(), base, CapabilityTokenExchange)
	if asked != 2 {
		t.Errorf("after a while /meta was asked %d times", asked)
	}
	if err := s.need(t.Context(), "http://127.0.0.1:1", CapabilityTokenExchange); !errors.Is(err, ErrUnreachable) {
		t.Errorf("an Armature not answering: %v", err)
	}
}

func TestWithoutAKeyOrAReaderNothingIsAsked(t *testing.T) {
	s, err := FromConfig(config.Outbound{}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.ConnectAtSignIn(context.Background(), uuid.New(), "realm")
	if _, err := s.ResourceLoad(t.Context(), LoadQuery{}); !errors.Is(err, ErrNotConnected) {
		t.Errorf("nobody reading: %v", err)
	}
	if _, err := s.ResourceLoad(db.WithUser(t.Context(), uuid.New()), LoadQuery{}); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("without a key: %v", err)
	}
	var field *FieldError
	for _, in := range []ConnectionInput{{BaseURL: "ftp://x", ClientID: "c", ClientSecret: "s"}, {BaseURL: "https://a.example"}, {BaseURL: "https://a.example", ClientID: "c"}} {
		if _, _, err := s.Connect(t.Context(), in); !errors.As(err, &field) {
			t.Errorf("%+v: %v", in, err)
		}
	}
	if _, _, err := s.Connect(t.Context(), ConnectionInput{BaseURL: "https://a.example", ClientID: "c", ClientSecret: "s"}); !errors.Is(err, ErrNoKey) {
		t.Errorf("connecting without a key: %v", err)
	}
	if _, err := FromConfig(config.Outbound{SecretKey: []byte("short")}, nil, nil, nil); err == nil {
		t.Error("a short key was taken")
	}
	if msg := (&OutdatedError{Capability: "x@1"}).Error(); !strings.Contains(msg, "x@1") || !strings.Contains(msg, "update Armature") {
		t.Errorf("an outdated Armature says %q", msg)
	}
}
