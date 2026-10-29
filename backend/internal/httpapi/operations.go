package httpapi

import "net/http"

// operation is one row of the table the router is built from, and the OpenAPI document after it.
type operation struct {
	method, path string
	handle       func(*Server, http.ResponseWriter, *http.Request)
	summary      string
	tag          string
	// responses maps a status to the body type it answers with; nil is no body.
	responses map[int]any
	// public operations need no session.
	public bool
	// root operations are served outside APIPrefix, where a probe expects them.
	root bool
}

// operations is the table. Order is by area, then by path; paths are relative to APIPrefix except the root ones.
var operations = []operation{
	{method: http.MethodGet, path: "/healthz", handle: (*Server).handleLiveness, tag: "health", summary: "Whether the process is running.",
		public: true, root: true, responses: map[int]any{http.StatusOK: statusResponse{}}},
	{method: http.MethodGet, path: "/readyz", handle: (*Server).handleReadiness, tag: "health", summary: "Whether the process can serve, and how reads are routed.",
		public: true, root: true, responses: map[int]any{http.StatusOK: readinessResponse{}, http.StatusServiceUnavailable: errorEnvelope{}}},
	{method: http.MethodGet, path: "/meta", handle: (*Server).handleMeta, tag: "health", summary: "Which product and build answers here, and the capabilities it offers partners.",
		public: true, responses: map[int]any{http.StatusOK: metaResponse{}}},
}
