package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/praetorianer777/bearing/backend/internal/db"
)

type fakeDB struct {
	pingErr  error
	replicas []db.ReplicaStatus
}

func (f fakeDB) Ping(context.Context) error { return f.pingErr }
func (f fakeDB) Stats() db.Stats            { return db.Stats{Replicas: f.replicas} }

func newServer(d Database) *Server {
	return &Server{DB: d, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func serve(t *testing.T, s *Server, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestMetaNamesTheProductAndAnEmptyListNotNull(t *testing.T) {
	rec := serve(t, newServer(fakeDB{}), http.MethodGet, APIPrefix+"/meta")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["product"] != "bearing" || body["apiVersion"] != "1" {
		t.Errorf("meta %v", body)
	}
	if caps, ok := body["capabilities"].([]any); !ok || caps == nil {
		t.Errorf("capabilities %v, want a list", body["capabilities"])
	}
	if build, _ := body["build"].(map[string]any); build["version"] == "" {
		t.Errorf("build %v has no version", body["build"])
	}
}

func TestReadinessFollowsThePrimary(t *testing.T) {
	ok := serve(t, newServer(fakeDB{}), http.MethodGet, "/readyz")
	if ok.Code != http.StatusOK || !strings.Contains(ok.Body.String(), `"replicas":[]`) {
		t.Errorf("ready: %d %s", ok.Code, ok.Body)
	}
	down := serve(t, newServer(fakeDB{pingErr: errors.New("refused")}), http.MethodGet, "/readyz")
	if down.Code != http.StatusServiceUnavailable || !strings.Contains(down.Body.String(), `"code":"unavailable"`) {
		t.Errorf("not ready: %d %s", down.Code, down.Body)
	}
	if strings.Contains(down.Body.String(), "refused") {
		t.Error("the cause reached the caller")
	}
}

func TestUnknownPathsAndMethodsAnswerInTheEnvelope(t *testing.T) {
	s := newServer(fakeDB{})
	for _, c := range []struct {
		method, path, code string
		status             int
	}{
		{http.MethodGet, APIPrefix + "/nothing", "not_found", http.StatusNotFound},
		{http.MethodGet, "/nothing", "not_found", http.StatusNotFound},
		{http.MethodPost, APIPrefix + "/meta", "method_not_allowed", http.StatusMethodNotAllowed},
	} {
		rec := serve(t, s, c.method, c.path)
		var body errorEnvelope
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if rec.Code != c.status || body.Error.Code != c.code || body.Error.RequestID == "" {
			t.Errorf("%s %s: %d %s", c.method, c.path, rec.Code, rec.Body)
		}
	}
}

func TestAPanicAnswersInternal(t *testing.T) {
	s := newServer(fakeDB{})
	h := s.recoverPanics(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), `"code":"internal"`) {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
}

func TestEveryOperationIsRoutedOnce(t *testing.T) {
	seen := map[string]bool{}
	for _, op := range operations {
		key := op.method + " " + op.path
		if seen[key] {
			t.Errorf("%s is in the table twice", key)
		}
		seen[key] = true
		if op.handle == nil || op.summary == "" || op.tag == "" || len(op.responses) == 0 {
			t.Errorf("%s lacks a handler, summary, tag or responses", key)
		}
	}
}

var capabilityName = regexp.MustCompile(`^[a-z]+(-[a-z]+)*(\.[a-z]+(-[a-z]+)*)*@[1-9][0-9]*$`)

func TestCapabilitiesNameServedOperations(t *testing.T) {
	served := map[string]bool{}
	for _, op := range operations {
		served[op.method+" "+op.path] = true
	}
	names := capabilities()
	if !slices.IsSorted(names) || len(slices.Compact(slices.Clone(names))) != len(names) {
		t.Errorf("capabilities %v are not sorted and unique", names)
	}
	for _, c := range integrationCapabilities {
		if !capabilityName.MatchString(c.name) {
			t.Errorf("%q is not name@version", c.name)
		}
		for _, op := range c.operations {
			if !served[op] {
				t.Errorf("%s promises %s, which is not served", c.name, op)
			}
		}
	}
}
