//go:build integration

package test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

type personalTokenJSON struct {
	Token struct {
		ID, Name  string
		Write     bool
		ExpiresAt string
	}
	Secret string
}

func makeToken(t *testing.T, b *browser, name string, write bool, days int) personalTokenJSON {
	t.Helper()
	var out personalTokenJSON
	if status := b.do(http.MethodPost, "/tokens", map[string]any{"name": name, "write": write, "days": days}, &out); status != http.StatusCreated {
		t.Fatalf("making a token answered %d", status)
	}
	if !strings.HasPrefix(out.Secret, "bearing_pat_") {
		t.Fatalf("the secret: %q", out.Secret)
	}
	return out
}

func TestAPersonalTokenReadsOrWritesAsItsPersonUntilItEnds(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	member, _ := withRole(t, admin, "member")
	reading := makeToken(t, member, "Reporting script", false, 30)
	writing := makeToken(t, member, "Sync", true, 1)

	if got := bearer(t, http.MethodGet, "/people", reading.Secret); got != http.StatusOK {
		t.Errorf("a reading token reading: %d", got)
	}
	if got := bearer(t, http.MethodPatch, "/auth/me", reading.Secret); got != http.StatusForbidden {
		t.Errorf("a reading token writing: %d", got)
	}
	req, _ := http.NewRequest(http.MethodPatch, inProcessAPI(t)+"/api/v1/auth/me", strings.NewReader(`{"name":"Renamed by a token"}`))
	req.Header.Set("Authorization", "Bearer "+writing.Secret)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("a writing token writing: %d", resp.StatusCode)
	}
	if got := bearer(t, http.MethodGet, "/accounts", writing.Secret); got != http.StatusForbidden {
		t.Errorf("a member's token reaching past the member's role: %d", got)
	}
	if got := bearer(t, http.MethodPost, "/tokens", writing.Secret); got != http.StatusForbidden {
		t.Errorf("a token making a token: %d", got)
	}
	if got := bearer(t, http.MethodPost, "/mcp", reading.Secret); got == http.StatusUnauthorized || got == http.StatusForbidden {
		t.Errorf("a token at the MCP endpoint: %d", got)
	}

	var listed struct{ Tokens []struct{ ID, Name string } }
	for deadline := time.Now().Add(5 * time.Second); len(listed.Tokens) < 2 && time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		member.do(http.MethodGet, "/tokens", nil, &listed)
	}
	if len(listed.Tokens) != 2 || listed.Tokens[0].Name != "Sync" {
		t.Errorf("the member's tokens: %+v", listed.Tokens)
	}
	if status := admin.do(http.MethodDelete, "/tokens/"+reading.Token.ID, nil, nil); status != http.StatusNotFound {
		t.Errorf("somebody else revoking the token: %d", status)
	}

	// A token past its day acts for nobody.
	if _, err := superuser(t, "BEARING_TEST_SUPERUSER_URL").Exec(context.Background(),
		`UPDATE api_token SET expires_at = now() - interval '1 second' WHERE id = $1`, writing.Token.ID); err != nil {
		t.Fatal(err)
	}
	if got := bearer(t, http.MethodGet, "/people", writing.Secret); got != http.StatusUnauthorized {
		t.Errorf("an expired token: %d", got)
	}
	if status := member.do(http.MethodDelete, "/tokens/"+reading.Token.ID, nil, nil); status != http.StatusNoContent {
		t.Fatalf("revoking answered %d", status)
	}
	if got := bearer(t, http.MethodGet, "/people", reading.Secret); got != http.StatusUnauthorized {
		t.Errorf("a revoked token: %d", got)
	}
	var refused struct {
		Error struct{ Fields map[string]string }
	}
	if status := member.do(http.MethodPost, "/tokens", map[string]any{"name": "Forever", "days": 3650}, &refused); status != http.StatusUnprocessableEntity || refused.Error.Fields["days"] == "" {
		t.Errorf("a token for ten years: %d %+v", status, refused)
	}
}
