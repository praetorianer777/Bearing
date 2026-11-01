//go:build integration

package test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type toolAnswer struct {
	Result struct {
		Content           []struct{ Text string }
		StructuredContent map[string]any
		IsError           bool
	}
	Error *struct{ Message string }
}

// tool calls an MCP tool as b, with its session.
func tool(t *testing.T, b *browser, name string, args map[string]any) toolAnswer {
	t.Helper()
	var out toolAnswer
	if status := b.do(http.MethodPost, "/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": name, "arguments": args}}, &out); status != http.StatusOK || out.Error != nil {
		t.Fatalf("%s answered %d %+v", name, status, out.Error)
	}
	return out
}

// found is what a tool found, failing the test when it could not.
func found(t *testing.T, b *browser, name string, args map[string]any) map[string]any {
	t.Helper()
	out := tool(t, b, name, args)
	if out.Result.IsError || out.Result.StructuredContent == nil {
		t.Fatalf("%s %v: %+v", name, args, out.Result.Content)
	}
	return out.Result.StructuredContent
}

func list(v any) []map[string]any {
	items, _ := v.([]any)
	out := make([]map[string]any, 0, len(items))
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		out = append(out, item)
	}
	return out
}

func named(items []map[string]any, field, value string) map[string]any {
	for _, item := range items {
		if item[field] == value {
			return item
		}
	}
	return nil
}

func TestEveryMCPToolAnswersFromTheDemoOrganisation(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)

	me := found(t, admin, "whoami", nil)
	if account, _ := me["account"].(map[string]any); account["email"] != adminEmail {
		t.Errorf("whoami: %v", me)
	}

	hits := found(t, admin, "search", map[string]any{"query": "finance"})
	unit := named(list(hits["units"]), "name", "Finance and People")
	head := named(list(hits["positions"]), "title", "Head of Finance and People")
	if unit == nil || head == nil {
		t.Fatalf("search for finance: %v", hits)
	}
	invoice := named(list(found(t, admin, "search", map[string]any{"query": "invoice"})["processes"]), "name", "Supplier invoice")
	approver := named(list(found(t, admin, "search", map[string]any{"query": "approver"})["roles"]), "name", "Invoice approver")
	if invoice == nil || approver == nil {
		t.Fatal("search found neither the supplier invoice nor the invoice approver")
	}

	position := found(t, admin, "get_position", map[string]any{"positionId": head["id"]})
	holders := list(position["position"].(map[string]any)["holders"])
	if len(holders) == 0 {
		t.Fatalf("the head of finance has no holder: %v", position)
	}
	personID := holders[0]["personId"].(string)
	person := found(t, admin, "get_person", map[string]any{"personId": personID})
	if person["person"] == nil {
		t.Errorf("get_person: %v", person)
	}
	withMembers := found(t, admin, "get_unit", map[string]any{"unitId": unit["id"], "subUnits": true})
	if withMembers["unit"] == nil || len(list(withMembers["members"])) == 0 {
		t.Errorf("get_unit: %v", withMembers)
	}

	chain := list(found(t, admin, "management_chain", map[string]any{"positionId": head["id"]})["positions"])
	if len(chain) == 0 {
		t.Errorf("the head of finance reports to nobody")
	}
	if list(found(t, admin, "management_chain", map[string]any{"personId": personID})["positions"]) == nil {
		t.Errorf("a person's chain")
	}
	direct := list(found(t, admin, "reports", map[string]any{"positionId": head["id"]})["positions"])
	all := list(found(t, admin, "reports", map[string]any{"personId": personID, "all": true})["positions"])
	if len(direct) == 0 || len(all) < len(direct) {
		t.Errorf("reports: %d direct, %d in all", len(direct), len(all))
	}

	duties := list(found(t, admin, "responsibilities_of", map[string]any{"positionId": head["id"]})["responsibilities"])
	if named(duties, "processName", "Supplier invoice") == nil {
		t.Errorf("the head of finance answers for nothing of the supplier invoice: %v", duties)
	}
	if roleDuties := list(found(t, admin, "responsibilities_of", map[string]any{"roleId": approver["id"]})["responsibilities"]); len(roleDuties) == 0 {
		t.Error("the invoice approver answers for nothing")
	}

	steps := list(found(t, admin, "who_is_responsible", map[string]any{"query": "approve the invoice"})["steps"])
	if len(steps) == 0 || steps[0]["name"] != "Approve the invoice" {
		t.Fatalf("who approves the invoice: %v", steps)
	}

	process := found(t, admin, "get_process", map[string]any{"processId": invoice["id"]})
	published, _ := process["published"].(map[string]any)
	if process["name"] != "Supplier invoice" || published == nil || len(list(published["nodes"])) == 0 || len(list(process["people"])) == 0 {
		t.Errorf("get_process: %v", process)
	}
	if again := found(t, admin, "get_process", map[string]any{"processId": invoice["id"], "version": 1}); again["published"] == nil {
		t.Errorf("version 1: %v", again)
	}
}

func TestAToolAnswersOnlyWhatThePersonMayRead(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	reader, _ := withRole(t, admin, "reader")
	missing := tool(t, reader, "get_person", map[string]any{"personId": "0195a3c4-0000-7000-8000-000000000000"})
	if !missing.Result.IsError || len(missing.Result.Content) != 1 || missing.Result.Content[0].Text == "" {
		t.Errorf("a person nobody has: %+v", missing.Result)
	}
	if out := found(t, reader, "whoami", nil); out["account"].(map[string]any)["role"] != "reader" {
		t.Errorf("whoami as a reader: %v", out)
	}
}

func TestAnAppConnectedOnTheConsentPageUsesTheTools(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	app := register(t, "Assistant for MCP", "none")
	verifier, challenge := pkce()
	query := authorizeQuery(app.ClientID, challenge, "read")
	code := connect(t, admin, query, "")
	_, granted := token(t, url.Values{"grant_type": {"authorization_code"}, "client_id": {app.ClientID}, "code": {code},
		"redirect_uri": {callback}, "code_verifier": {verifier}}, nil)

	req, _ := http.NewRequest(http.MethodPost, inProcessAPI(t)+"/api/v1/mcp",
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"search","arguments":{"query":"quality"}}}`))
	req.Header.Set("Authorization", "Bearer "+granted.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out toolAnswer
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode != http.StatusOK || out.Result.IsError || named(list(out.Result.StructuredContent["units"]), "name", "Quality") == nil {
		t.Errorf("a connected app searching: %d %+v", resp.StatusCode, out.Result.Content)
	}

	resp, err = http.Post(inProcessAPI(t)+"/api/v1/mcp", "application/json", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized || !strings.Contains(resp.Header.Get("WWW-Authenticate"), "/.well-known/oauth-protected-resource/api/v1/mcp") {
		t.Errorf("signed out: %d %q", resp.StatusCode, resp.Header.Get("WWW-Authenticate"))
	}
}
