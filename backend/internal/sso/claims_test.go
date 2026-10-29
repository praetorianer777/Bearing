package sso

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"testing"
)

func TestClaimsBecomeAnIdentity(t *testing.T) {
	claims := map[string]any{
		"email":          " Alice@Example.TEST ",
		"email_verified": true,
		"name":           "Alice Liddell",
		"groups":         []any{"engineering", " exciter-admins ", "engineering", "", 42},
	}
	got, err := identityFromClaims("https://id.test/realms/r", "sub-1", claims, "groups")
	if err != nil {
		t.Fatal(err)
	}
	want := &Identity{
		Issuer: "https://id.test/realms/r", Subject: "sub-1",
		Email: "alice@example.test", Name: "Alice Liddell",
		Groups: []string{"engineering", "exciter-admins"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("identity = %+v, want %+v", got, want)
	}
}

func TestTheGroupsClaimIsConfigurable(t *testing.T) {
	claims := map[string]any{
		"email":                        "bob@example.test",
		"roles":                        "marketing",
		"realm_access":                 map[string]any{"roles": []any{"reader", "writer"}},
		"https://example.test/groups":  []any{"by-url"},
		"https://example.test/nothing": nil,
	}
	for claim, want := range map[string][]string{
		"roles":                       {"marketing"},
		"realm_access.roles":          {"reader", "writer"},
		"https://example.test/groups": {"by-url"},
		"groups":                      {},
		"realm_access.missing.deeper": {},
	} {
		got, err := identityFromClaims("i", "s", claims, claim)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got.Groups, want) {
			t.Errorf("claim %q gave %q, want %q", claim, got.Groups, want)
		}
	}
}

func TestANameIsAlwaysFound(t *testing.T) {
	for _, tc := range []struct {
		claims map[string]any
		want   string
	}{
		{map[string]any{"given_name": "Bob", "family_name": "Builder"}, "Bob Builder"},
		{map[string]any{"preferred_username": "bobby"}, "bobby"},
		{map[string]any{}, "bob"},
	} {
		tc.claims["email"] = "bob@example.test"
		got, err := identityFromClaims("i", "s", tc.claims, "groups")
		if err != nil || got.Name != tc.want {
			t.Errorf("claims %v named %q (%v), want %q", tc.claims, got.Name, err, tc.want)
		}
	}
}

func TestAnIdentityNeedsAVerifiedEmail(t *testing.T) {
	if _, err := identityFromClaims("i", "s", map[string]any{"name": "Nobody"}, "groups"); !errors.Is(err, ErrNoEmail) {
		t.Errorf("no email = %v, want ErrNoEmail", err)
	}
	for _, verified := range []any{false, "false", "FALSE"} {
		claims := map[string]any{"email": "x@example.test", "email_verified": verified}
		if _, err := identityFromClaims("i", "s", claims, "groups"); !errors.Is(err, ErrEmailUnverified) {
			t.Errorf("email_verified %v = %v, want ErrEmailUnverified", verified, err)
		}
	}
}

func TestBackchannelReachesTheProviderUnderItsPublicName(t *testing.T) {
	var sawHost, sawPath string
	reachable := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawHost, sawPath = r.Host, r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer reachable.Close()

	client := Backchannel(map[string]string{"http://public.example:8180": reachable.URL})
	resp, err := client.Get("http://public.example:8180/realms/demo/.well-known/openid-configuration")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if sawHost != "public.example:8180" {
		t.Errorf("the provider was asked as %q, want the public name", sawHost)
	}
	if sawPath != "/realms/demo/.well-known/openid-configuration" {
		t.Errorf("path = %q", sawPath)
	}

	// Anywhere not named goes where it says.
	resp, err = client.Get(reachable.URL + "/direct")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if sawPath != "/direct" {
		t.Errorf("an unnamed address was rewritten: %q", sawPath)
	}
}
