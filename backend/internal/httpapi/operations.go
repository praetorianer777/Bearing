package httpapi

import (
	"net/http"

	"github.com/praetorianer777/bearing/backend/internal/openapi"
)

// env is a response envelope: the one or two keys a handler wraps its result in.
type env map[string]any

// operation is one row of the table the router is built from, and the OpenAPI document after it.
type operation struct {
	method, path string
	// id is what clients call the operation.
	id          string
	handle      func(*Server, http.ResponseWriter, *http.Request)
	summary     string
	description string
	tag         string
	// request is the JSON body type the handler decodes; nil is none.
	request any
	// responses maps a status to its envelope or bare type; nil is no body.
	responses map[int]any
	// public operations need no session.
	public bool
	// root operations are served outside APIPrefix, where a probe expects them.
	root bool
}

// operations is the table. Order is by area, then by path; paths are relative to APIPrefix except the root ones.
var operations = []operation{
	{method: http.MethodGet, path: "/healthz", id: "liveness", handle: (*Server).handleLiveness, tag: "health",
		summary: "Whether the process is running.", public: true, root: true,
		responses: map[int]any{http.StatusOK: statusResponse{}}},
	{method: http.MethodGet, path: "/readyz", id: "readiness", handle: (*Server).handleReadiness, tag: "health",
		summary: "Whether the process can serve, and how reads are routed.", public: true, root: true,
		responses: map[int]any{http.StatusOK: readinessResponse{}, http.StatusServiceUnavailable: errorEnvelope{}}},
	{method: http.MethodGet, path: "/meta", id: "meta", handle: (*Server).handleMeta, tag: "health",
		summary: "Which product and build answers here, and the capabilities it offers partners.", public: true,
		responses: map[int]any{http.StatusOK: metaResponse{}}},
}

// The document's own row joins at init: it is built from the table, so the table cannot name it in its initialiser.
func init() {
	operations = append(operations, operation{method: http.MethodGet, path: "/openapi.json", id: "openapi",
		handle: (*Server).handleOpenAPI, tag: "health", summary: "This document.", public: true,
		responses: map[int]any{http.StatusOK: openapi.Document{}}})
}
