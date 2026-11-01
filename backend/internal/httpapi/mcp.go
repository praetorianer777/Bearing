package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"
)

// The MCP endpoint (mcp@1): an assistant asks the organisation questions as the person whose token it holds. Each
// tool reads through the API's own operations, run in this process as that person, so it answers exactly what the
// person may read, by their role and the database's policies.

// MCPProtocolVersions are the protocol revisions served, newest first; a client asking for another is answered the
// newest.
var MCPProtocolVersions = []string{"2025-11-25", "2025-06-18"}

const (
	mcpServerName = "bearing"
	// mcpMaxResult bounds what one tool answers, so a large organisation cannot flood the assistant's context.
	mcpMaxResult = 64 << 10
)

// JSON-RPC's error codes.
const (
	rpcInvalidRequest = -32600
	rpcMethodNotFound = -32601
	rpcInvalidParams  = -32602
)

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// mcpRequest is a JSON-RPC 2.0 request, as the endpoint takes it; one at a time.
type mcpRequest struct {
	JSONRPC string `json:"jsonrpc"`
	// ID is the caller's, a string or a number; a notification has none.
	ID     any    `json:"id,omitempty"`
	Method string `json:"method"`
	// Params are the method's: initialize, ping, tools/list and tools/call are served.
	Params any `json:"params,omitempty"`
}

type toolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolResult struct {
	Content           []toolContent `json:"content"`
	StructuredContent any           `json:"structuredContent,omitempty"`
	IsError           bool          `json:"isError,omitempty"`
}

func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		respondError(w, r, s.Log, ErrBadRequest("The request is too large. Send one JSON-RPC request."))
		return
	}
	if trimmed := bytes.TrimSpace(body); len(trimmed) > 0 && trimmed[0] == '[' {
		respondJSON(w, http.StatusOK, rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"),
			Error: &rpcError{Code: rpcInvalidRequest, Message: "Send one request at a time; batches are not taken."}})
		return
	}
	var req rpcRequest
	if err := json.Unmarshal(body, &req); err != nil {
		respondError(w, r, s.Log, ErrBadRequest("The request is not JSON. Send one JSON-RPC 2.0 request."))
		return
	}
	id := req.ID
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	if req.JSONRPC != "2.0" || req.Method == "" {
		respondJSON(w, http.StatusOK, rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: rpcInvalidRequest, Message: "A request says jsonrpc 2.0 and names a method."}})
		return
	}
	// A notification wants no answer.
	if len(req.ID) == 0 || strings.HasPrefix(req.Method, "notifications/") {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	result, failure := s.dispatchMCP(r, req)
	respondJSON(w, http.StatusOK, rpcResponse{JSONRPC: "2.0", ID: id, Result: result, Error: failure})
}

func (s *Server) dispatchMCP(r *http.Request, req rpcRequest) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		var params struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &params)
		version := MCPProtocolVersions[0]
		if slices.Contains(MCPProtocolVersions, params.ProtocolVersion) {
			version = params.ProtocolVersion
		}
		return map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": mcpServerName, "title": "Bearing", "version": APIVersion},
			"instructions": "Bearing models an organisation: units, positions with their reporting lines, people holding them, roles, " +
				"and processes whose steps say who is responsible, accountable, consulted and informed. Every answer is as of a day, today " +
				"unless asOf names another, written 2006-01-02. Start with search to find ids.",
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		tools := make([]map[string]any, 0, len(mcpTools))
		for _, t := range mcpTools {
			tools = append(tools, map[string]any{
				"name": t.name, "title": t.title, "description": t.description, "inputSchema": t.input(),
				"annotations": map[string]any{"title": t.title, "readOnlyHint": true, "openWorldHint": false},
			})
		}
		return map[string]any{"tools": tools}, nil
	case "tools/call":
		var params struct {
			Name      string                     `json:"name"`
			Arguments map[string]json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil || params.Name == "" {
			return nil, &rpcError{Code: rpcInvalidParams, Message: "A tool call names a tool and gives its arguments as an object."}
		}
		for _, t := range mcpTools {
			if t.name == params.Name {
				return s.callTool(r, t, params.Arguments), nil
			}
		}
		return nil, &rpcError{Code: rpcInvalidParams, Message: "There is no tool " + params.Name + ". tools/list names them."}
	}
	return nil, &rpcError{Code: rpcMethodNotFound, Message: "There is no method " + req.Method + "."}
}

// callTool runs a tool and says what it found, or why it could not, as the result an assistant reads.
func (s *Server) callTool(r *http.Request, t mcpTool, args map[string]json.RawMessage) toolResult {
	call := &toolCall{s: s, outer: r, args: args}
	err := call.known(t)
	var out any
	if err == nil {
		out, err = t.run(call)
	}
	if err != nil {
		return toolResult{Content: []toolContent{{Type: "text", Text: err.Error()}}, IsError: true}
	}
	text, _ := json.Marshal(out)
	if len(text) > mcpMaxResult {
		return toolResult{Content: []toolContent{{Type: "text", Text: string(text[:mcpMaxResult]) +
			"\n... The answer was cut here. Ask about fewer things at a time."}}}
	}
	return toolResult{Content: []toolContent{{Type: "text", Text: string(text)}}, StructuredContent: out}
}

type mcpInnerKey struct{}

// withinMCP marks a request a tool runs in this process, which a token held to the MCP endpoint may make.
func withinMCP(ctx context.Context) context.Context {
	return context.WithValue(ctx, mcpInnerKey{}, true)
}

func fromMCP(ctx context.Context) bool { v, _ := ctx.Value(mcpInnerKey{}).(bool); return v }

// capture keeps what an operation run for a tool answered.
type capture struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (c *capture) Header() http.Header         { return c.header }
func (c *capture) Write(b []byte) (int, error) { return c.body.Write(b) }
func (c *capture) WriteHeader(status int) {
	if c.status == 0 {
		c.status = status
	}
}

// get runs a GET operation as the person the tool acts for and decodes what it answered.
func (c *toolCall) get(path string, query map[string]string) (map[string]any, error) {
	ctx := withinMCP(context.WithValue(c.outer.Context(), chi.RouteCtxKey, chi.NewRouteContext()))
	inner := c.outer.Clone(ctx)
	inner.Method = http.MethodGet
	q := url.Values{}
	for k, v := range query {
		if v != "" {
			q.Set(k, v)
		}
	}
	inner.URL = &url.URL{Path: APIPrefix + path, RawQuery: q.Encode()}
	inner.RequestURI = ""
	inner.Body = http.NoBody
	inner.ContentLength = 0
	rec := &capture{header: http.Header{}}
	c.s.router.ServeHTTP(rec, inner)
	if rec.status == 0 {
		rec.status = http.StatusOK
	}
	var out map[string]any
	if rec.status >= http.StatusBadRequest {
		var e errorEnvelope
		if json.Unmarshal(rec.body.Bytes(), &e) == nil && e.Error.Message != "" {
			return nil, &toolError{status: rec.status, message: e.Error.Message}
		}
		return nil, &toolError{status: rec.status, message: http.StatusText(rec.status) + "."}
	}
	if err := json.Unmarshal(rec.body.Bytes(), &out); err != nil {
		return nil, &toolError{status: http.StatusInternalServerError, message: "Bearing answered something that is not JSON."}
	}
	return out, nil
}

// toolError is an operation a tool ran refusing, in its own words.
type toolError struct {
	status  int
	message string
}

func (e *toolError) Error() string { return e.message }
