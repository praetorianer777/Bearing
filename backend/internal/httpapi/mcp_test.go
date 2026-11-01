package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type rpcAnswer struct {
	ID     any
	Result struct {
		ProtocolVersion string
		ServerInfo      struct{ Name string }
		Tools           []struct {
			Name        string
			InputSchema struct{ Required []string }
			Annotations struct{ ReadOnlyHint bool }
		}
		Content []struct{ Text string }
		IsError bool
	}
	Error *struct {
		Code    int
		Message string
	}
}

func rpc(t *testing.T, s *Server, cookie, body string) (int, rpcAnswer) {
	t.Helper()
	rec := call(s, http.MethodPost, APIPrefix+"/mcp", cookie, body, map[string]string{"Content-Type": "application/json"})
	var out rpcAnswer
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestTheMCPEndpointSpeaksTheProtocol(t *testing.T) {
	s := signedInServer()
	_, init := rpc(t, s, "bert", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`)
	if init.Result.ProtocolVersion != "2025-06-18" || init.Result.ServerInfo.Name != "bearing" {
		t.Errorf("an older client: %+v", init.Result)
	}
	_, init = rpc(t, s, "bert", `{"jsonrpc":"2.0","id":2,"method":"initialize","params":{"protocolVersion":"2099-01-01"}}`)
	if init.Result.ProtocolVersion != MCPProtocolVersions[0] {
		t.Errorf("an unknown version was answered %q", init.Result.ProtocolVersion)
	}
	if status, _ := rpc(t, s, "bert", `{"jsonrpc":"2.0","method":"notifications/initialized"}`); status != http.StatusAccepted {
		t.Errorf("a notification: %d", status)
	}
	if _, ping := rpc(t, s, "bert", `{"jsonrpc":"2.0","id":"p","method":"ping"}`); ping.Error != nil || ping.ID != "p" {
		t.Errorf("ping: %+v", ping)
	}
	_, list := rpc(t, s, "bert", `{"jsonrpc":"2.0","id":3,"method":"tools/list"}`)
	names := []string{}
	for _, tool := range list.Result.Tools {
		names = append(names, tool.Name)
		if !tool.Annotations.ReadOnlyHint {
			t.Errorf("%s does not say it only reads", tool.Name)
		}
	}
	if strings.Join(names, " ") != "whoami search get_person get_unit get_position management_chain reports responsibilities_of who_is_responsible get_process" {
		t.Errorf("the tools: %v", names)
	}
	for body, want := range map[string]int{
		`{"jsonrpc":"2.0","id":4,"method":"resources/list"}`:                                   rpcMethodNotFound,
		`{"jsonrpc":"1.0","id":5,"method":"ping"}`:                                             rpcInvalidRequest,
		`[{"jsonrpc":"2.0","id":6,"method":"ping"}]`:                                           rpcInvalidRequest,
		`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{}}`:                           rpcInvalidParams,
		`{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"delete_everything"}}`: rpcInvalidParams,
	} {
		if _, out := rpc(t, s, "bert", body); out.Error == nil || out.Error.Code != want {
			t.Errorf("%s: %+v", body, out.Error)
		}
	}
	if status, _ := rpc(t, s, "bert", `not json`); status != http.StatusBadRequest {
		t.Errorf("not JSON: %d", status)
	}
}

func TestAToolSaysWhatIsWrongWithItsArguments(t *testing.T) {
	s := signedInServer()
	for args, want := range map[string]string{
		`{"name":"get_person","arguments":{}}`:                              "needs personId",
		`{"name":"get_person","arguments":{"personId":"bob"}}`:              "an id such as search answers",
		`{"name":"get_person","arguments":{"personId":7}}`:                  "personId is a string",
		`{"name":"get_person","arguments":{"personId":"x","colour":"red"}}`: "takes no argument colour",
		`{"name":"management_chain","arguments":{}}`:                        "name one of personId, positionId",
		`{"name":"reports","arguments":{"personId":"0195a3c4-0000-7000-8000-000000000001","positionId":"0195a3c4-0000-7000-8000-000000000002"}}`: "not several",
		`{"name":"reports","arguments":{"positionId":"0195a3c4-0000-7000-8000-000000000002","all":"yes"}}`:                                       "all is true or false",
		`{"name":"search","arguments":{"query":"x"}}`:                                                         "two letters or more",
		`{"name":"who_is_responsible","arguments":{"query":" "}}`:                                             "two letters or more",
		`{"name":"get_process","arguments":{"processId":"0195a3c4-0000-7000-8000-000000000003","version":0}}`: "whole number",
		`{"name":"responsibilities_of","arguments":{}}`:                                                       "name one of personId, positionId, roleId, unitId",
	} {
		_, out := rpc(t, s, "bert", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":`+args+`}`)
		if !out.Result.IsError || len(out.Result.Content) != 1 || !strings.Contains(out.Result.Content[0].Text, want) {
			t.Errorf("%s: %+v", args, out.Result)
		}
	}
}

func TestAToolReadsAsThePersonThroughTheAPI(t *testing.T) {
	s := oauthServer()
	s.Routes()
	var me map[string]any
	var refused error
	h := s.identify(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		c := &toolCall{s: s, outer: r}
		me, _ = c.get("/auth/me", nil)
		_, refused = c.get("/accounts", nil)
	}))
	r := httptest.NewRequest(http.MethodPost, APIPrefix+"/mcp", nil)
	r.Header.Set("Authorization", "Bearer mcp")
	h.ServeHTTP(httptest.NewRecorder(), r)
	account, _ := me["account"].(map[string]any)
	if account["name"] != "Bert" {
		t.Errorf("a token held to MCP reading through a tool: %v", me)
	}
	var e *toolError
	if !errors.As(refused, &e) || e.status != http.StatusForbidden || e.Error() == "" {
		t.Errorf("a member reading the accounts through a tool: %v", refused)
	}
	if rec := bearerCall(s, http.MethodGet, "/auth/me", "mcp"); rec.Code != http.StatusForbidden {
		t.Errorf("the token outside MCP: %d", rec.Code)
	}
}

func TestTheMCPEndpointPointsSomebodySignedOutToTheMetadata(t *testing.T) {
	s := oauthServer()
	rec := call(s, http.MethodPost, APIPrefix+"/mcp", "", `{"jsonrpc":"2.0","id":1,"method":"ping"}`, nil)
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Header().Get("WWW-Authenticate"), `resource_metadata="https://bearing.example/.well-known/oauth-protected-resource/api/v1/mcp"`) {
		t.Errorf("%d %q", rec.Code, rec.Header().Get("WWW-Authenticate"))
	}
}
