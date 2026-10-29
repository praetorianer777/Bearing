package httpapi

import (
	"net/http"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/praetorianer777/bearing/backend/internal/auth"
	"github.com/praetorianer777/bearing/backend/internal/config"
	"github.com/praetorianer777/bearing/backend/internal/openapi"
)

// ProductName is the API's title in its document.
const ProductName = "Bearing"

// specBuilder knows the types whose JSON is not what reflection would say.
func specBuilder() *openapi.Builder {
	b := openapi.NewBuilder()
	// The document describes itself as an object; its own schema would be the OpenAPI meta-schema.
	b.Overrides[reflect.TypeOf(openapi.Document{})] = &openapi.Schema{Type: "object", Description: "An OpenAPI 3.1 document."}
	roles := make([]string, len(auth.Roles))
	for i, r := range auth.Roles {
		roles[i] = string(r)
	}
	b.Enums[reflect.TypeOf(auth.Role(""))] = roles
	return b
}

// Spec is the API's OpenAPI document, derived from the operations table and the Go types the handlers answer with.
func Spec() *openapi.Document {
	b := specBuilder()
	doc := &openapi.Document{
		OpenAPI: "3.1.0",
		Info: openapi.Info{
			Title:   ProductName,
			Version: APIVersion,
			Description: "The organisation modeling HTTP API. Every endpoint answers JSON; errors share one envelope. " +
				"Partners check the capabilities at /meta rather than the build's version.",
		},
		Servers: []openapi.Server{{URL: APIPrefix, Description: "This deployment"}},
		Paths:   map[string]openapi.PathItem{},
		Components: openapi.Components{
			SecuritySchemes: map[string]openapi.SecurityScheme{
				"session": {Type: "apiKey", In: "cookie", Name: config.DefaultSessionCookie, Description: "The cookie a sign-in sets."},
			},
		},
		Security: []map[string][]string{{"session": {}}},
	}
	errorSchema := b.Schema(reflect.TypeOf(errorEnvelope{}))
	tags := map[string]bool{}
	for _, op := range operations {
		item := doc.Paths[op.path]
		if item == nil {
			item = openapi.PathItem{}
			doc.Paths[op.path] = item
		}
		item[strings.ToLower(op.method)] = op.describe(b, errorSchema)
		tags[op.tag] = true
	}
	for _, tag := range openapi.SortedKeys(tags) {
		doc.Tags = append(doc.Tags, openapi.Tag{Name: tag})
	}
	doc.Components.Schemas = b.Components()
	return doc
}

// describe is one row of the table as an OpenAPI operation.
func (op operation) describe(b *openapi.Builder, errorSchema *openapi.Schema) *openapi.Operation {
	o := &openapi.Operation{
		OperationID: op.id,
		Summary:     op.summary,
		Description: op.description,
		Role:        string(op.role),
		Tags:        []string{op.tag},
		Responses:   map[string]*openapi.Response{},
	}
	for _, name := range pathParams(op.path) {
		o.Parameters = append(o.Parameters, openapi.Parameter{Name: name, In: "path", Required: true, Schema: pathParamSchema(name)})
	}
	for _, name := range op.query {
		o.Parameters = append(o.Parameters, openapi.Parameter{Name: name, In: "query", Schema: &openapi.Schema{Type: "string"}})
	}
	if op.request != nil {
		o.RequestBody = &openapi.RequestBody{Required: true, Content: map[string]openapi.MediaType{
			"application/json": {Schema: b.SchemaOf(op.request)},
		}}
	}
	for status, body := range op.responses {
		r := &openapi.Response{Description: http.StatusText(status)}
		switch {
		case isErrorEnvelope(body):
			r.Content = map[string]openapi.MediaType{"application/json": {Schema: errorSchema}}
		case body != nil:
			r.Content = map[string]openapi.MediaType{"application/json": {Schema: envelopeSchema(b, body)}}
		}
		o.Responses[strconv.Itoa(status)] = r
	}
	o.Responses["default"] = &openapi.Response{Description: "An error, in the one shape every endpoint uses.",
		Content: map[string]openapi.MediaType{"application/json": {Schema: errorSchema}}}
	if op.public {
		o.Security = []map[string][]string{}
	}
	if op.root {
		o.Servers = []openapi.Server{{URL: "/", Description: "Outside the API's prefix"}}
	}
	return o
}

func isErrorEnvelope(body any) bool {
	_, ok := body.(errorEnvelope)
	return ok
}

// envelopeSchema describes a response body: an env becomes an inline object with every key required, anything
// else is the type itself.
func envelopeSchema(b *openapi.Builder, body any) *openapi.Schema {
	e, ok := body.(env)
	if !ok {
		return b.SchemaOf(body)
	}
	s := &openapi.Schema{Type: "object", Properties: map[string]*openapi.Schema{}}
	for _, key := range openapi.SortedKeys(e) {
		schema := b.SchemaOf(e[key])
		if schema == nil {
			schema = &openapi.Schema{Description: "A JSON value."}
		}
		s.Properties[key] = schema
		s.Required = append(s.Required, key)
	}
	return s
}

var pathParamPattern = regexp.MustCompile(`\{(\w+)\}`)

func pathParams(path string) []string {
	var out []string
	for _, m := range pathParamPattern.FindAllStringSubmatch(path, -1) {
		out = append(out, m[1])
	}
	return out
}

// pathParamSchema types a path parameter by its name: ids are uuids, the rest strings.
func pathParamSchema(name string) *openapi.Schema {
	if strings.HasSuffix(name, "ID") {
		return &openapi.Schema{Type: "string", Format: "uuid"}
	}
	return &openapi.Schema{Type: "string"}
}

var (
	specOnce    sync.Once
	specEncoded []byte
)

// handleOpenAPI serves the document the build was made with, encoded once.
func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	specOnce.Do(func() {
		encoded, err := Spec().MarshalIndent()
		if err != nil {
			panic(err)
		}
		specEncoded = encoded
	})
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(specEncoded)
}
