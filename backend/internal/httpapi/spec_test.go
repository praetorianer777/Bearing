package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/praetorianer777/bearing/backend/internal/db"
	"github.com/praetorianer777/bearing/backend/internal/openapi"
)

func TestTheCommittedDocumentIsCurrent(t *testing.T) {
	committed, err := os.ReadFile("../../../api/openapi.json")
	if err != nil {
		t.Fatalf("api/openapi.json: %v; run make openapi", err)
	}
	fresh, err := Spec().MarshalIndent()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(committed)) != strings.TrimSpace(string(fresh)) {
		t.Error("api/openapi.json is out of date; run make openapi and commit the result")
	}
}

func TestEveryOperationHasAUniqueID(t *testing.T) {
	seen := map[string]bool{}
	for _, op := range operations {
		if op.id == "" || seen[op.id] {
			t.Errorf("%s %s has id %q, empty or taken", op.method, op.path, op.id)
		}
		seen[op.id] = true
	}
}

// What the handlers answer fits what the document says they answer.
func TestAnswersFitTheDocument(t *testing.T) {
	doc := Spec()
	cases := []struct {
		name, path string
		db         fakeDB
	}{
		{"meta", APIPrefix + "/meta", fakeDB{}},
		{"ready", "/readyz", fakeDB{replicas: []db.ReplicaStatus{{Name: "replica-0", Healthy: true, LastError: ""}}}},
		{"not ready", "/readyz", fakeDB{pingErr: errors.New("down")}},
		{"openapi", APIPrefix + "/openapi.json", fakeDB{}},
		{"liveness", "/healthz", fakeDB{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := serve(t, newServer(c.db), http.MethodGet, c.path)
			schema := responseSchema(t, doc, strings.TrimPrefix(c.path, APIPrefix), rec.Code)
			if err := doc.ValidateJSON(schema, rec.Body.Bytes()); err != nil {
				t.Errorf("%d %s does not fit the document: %v", rec.Code, rec.Body, err)
			}
		})
	}
}

func responseSchema(t *testing.T, doc *openapi.Document, path string, status int) *openapi.Schema {
	t.Helper()
	op := doc.Paths[path]["get"]
	if op == nil {
		t.Fatalf("the document has no GET %s", path)
	}
	r := op.Responses[strconv.Itoa(status)]
	if r == nil {
		r = op.Responses["default"]
	}
	return r.Content["application/json"].Schema
}

func TestTheDocumentServedIsValidJSON(t *testing.T) {
	rec := serve(t, newServer(fakeDB{}), http.MethodGet, APIPrefix+"/openapi.json")
	var doc map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil || doc["openapi"] != "3.1.0" {
		t.Errorf("%v %v", err, doc["openapi"])
	}
}
