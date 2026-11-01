// Package armature is Bearing's side of Armature: a connection an administrator sets up, a grant per person traded
// for their realm token at sign-in (oauth.token-exchange@1), and the reads Bearing makes as the reader, each gated by
// what Armature lists at /meta. Adapted from Stator's internal/armature.
package armature

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// CapabilityTokenExchange is Armature trading a realm token for a grant of its own.
	CapabilityTokenExchange = "oauth.token-exchange@1"
	// CapabilityResourceLoad is Armature's allocations, load and days away per person and week.
	CapabilityResourceLoad = "resource-load@1"
	// Scope is what Bearing asks a grant for: it only reads.
	Scope = "read"
	// CallTimeout bounds one call to Armature; a sign-in waits for it at most once.
	CallTimeout = 5 * time.Second
	// maxAnswer is the most of an answer read.
	maxAnswer = 4 << 20
)

const (
	grantTokenExchange = "urn:ietf:params:oauth:grant-type:token-exchange"
	tokenTypeAccess    = "urn:ietf:params:oauth:token-type:access_token"
)

// errStatus is an answer Armature gave that is not a success.
type errStatus struct {
	Status int
	Code   string
}

func (e *errStatus) Error() string { return fmt.Sprintf("armature answered %d %s", e.Status, e.Code) }

// tokens are what Armature's token endpoint answers.
type tokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

// Subject names a person as the realm does.
type Subject struct {
	Issuer  string `json:"issuer"`
	Subject string `json:"subject"`
}

// Week is one person's week as Armature counts it.
type Week struct {
	// Start is the Monday, written 2006-01-02.
	Start string `json:"start"`
	// AllocationPercent is the share of the week projects are given, across them all; above 100 is given out twice.
	AllocationPercent float64 `json:"allocationPercent"`
	NominalHours      float64 `json:"nominalHours"`
	LoadHours         float64 `json:"loadHours"`
	DaysAway          float64 `json:"daysAway"`
	// Away are the days, written 2006-01-02, the person is away for at least half of; never with a reason.
	Away []string `json:"away"`
}

// PersonLoad is one person's weeks, or Found false for somebody Armature does not know.
type PersonLoad struct {
	Subject
	Found bool   `json:"found"`
	Weeks []Week `json:"weeks"`
}

// LoadQuery is what resource-load@1 is asked: the weeks from the Monday of From to that of To, of the people.
type LoadQuery struct {
	From   string    `json:"from"`
	To     string    `json:"to"`
	People []Subject `json:"people"`
}

type loadAnswer struct {
	People []PersonLoad `json:"people"`
}

// caller makes the HTTP calls; it holds no state of its own.
type caller struct{ http *http.Client }

func (c caller) do(ctx context.Context, req *http.Request, out any) error {
	ctx, cancel := context.WithTimeout(ctx, CallTimeout)
	defer cancel()
	resp, err := c.http.Do(req.WithContext(ctx))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAnswer))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var refusal struct {
			Error any `json:"error"`
		}
		_ = json.Unmarshal(body, &refusal)
		code := ""
		switch e := refusal.Error.(type) {
		case string:
			code = e
		case map[string]any:
			code, _ = e["code"].(string)
		}
		return &errStatus{Status: resp.StatusCode, Code: code}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(body, out)
}

func api(base, path string) string { return strings.TrimRight(base, "/") + "/api/v1" + path }

// meta is what Armature lists at /meta; an Armature without /meta offers nothing.
func (c caller) meta(ctx context.Context, base string) ([]string, error) {
	req, err := http.NewRequest(http.MethodGet, api(base, "/meta"), nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		Capabilities []string `json:"capabilities"`
	}
	err = c.do(ctx, req, &out)
	var status *errStatus
	if errors.As(err, &status) && (status.Status == http.StatusNotFound || status.Status == http.StatusMethodNotAllowed) {
		return []string{}, nil
	}
	if out.Capabilities == nil {
		out.Capabilities = []string{}
	}
	return out.Capabilities, err
}

func (c caller) token(ctx context.Context, base, clientID, secret string, form url.Values) (tokens, error) {
	req, err := http.NewRequest(http.MethodPost, api(base, "/oauth/token"), strings.NewReader(form.Encode()))
	if err != nil {
		return tokens{}, err
	}
	req.SetBasicAuth(url.QueryEscape(clientID), url.QueryEscape(secret))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var out tokens
	err = c.do(ctx, req, &out)
	if err == nil && (out.AccessToken == "" || out.RefreshToken == "") {
		err = errors.New("armature answered no tokens")
	}
	return out, err
}

// exchange trades a person's realm access token for a grant of theirs at Armature.
func (c caller) exchange(ctx context.Context, base, clientID, secret, realmToken string) (tokens, error) {
	return c.token(ctx, base, clientID, secret, url.Values{"grant_type": {grantTokenExchange}, "subject_token": {realmToken},
		"subject_token_type": {tokenTypeAccess}, "scope": {Scope}})
}

// refresh renews a grant; Armature rotates the refresh token every time.
func (c caller) refresh(ctx context.Context, base, clientID, secret, refreshToken string) (tokens, error) {
	return c.token(ctx, base, clientID, secret, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refreshToken}})
}

// resourceLoad asks resource-load@1 for the people's weeks, as the person whose access token it is.
func (c caller) resourceLoad(ctx context.Context, base, access string, q LoadQuery) ([]PersonLoad, error) {
	body, err := json.Marshal(q)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, api(base, "/resource-load"), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+access)
	req.Header.Set("Content-Type", "application/json")
	var out loadAnswer
	return out.People, c.do(ctx, req, &out)
}
