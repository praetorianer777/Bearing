//go:build integration

package test

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"testing"

	"github.com/praetorianer777/bearing/backend/internal/httpapi"
)

func get(t *testing.T, path string, into any) int {
	t.Helper()
	resp, err := http.Get(env(t, "BEARING_TEST_API_URL") + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode
}

func TestTheRunningAPINamesItself(t *testing.T) {
	var meta struct {
		Product      string
		APIVersion   string
		Capabilities []string
		Build        struct{ Version string }
	}
	if status := get(t, "/api/v1/meta", &meta); status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	if meta.Product != "bearing" || meta.APIVersion != "1" || meta.Build.Version == "" || meta.Capabilities == nil {
		t.Errorf("meta %+v", meta)
	}
}

func TestTheRunningAPIReadsFromItsReplica(t *testing.T) {
	var ready struct {
		Status   string
		Replicas []struct{ Healthy bool }
	}
	if status := get(t, "/readyz", &ready); status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	if len(ready.Replicas) != 1 || !ready.Replicas[0].Healthy {
		t.Errorf("replicas %+v, want one healthy", ready.Replicas)
	}
}

// The running api serves the document the code generates, and its answers fit it.
func TestTheRunningAPIKeepsToItsDocument(t *testing.T) {
	served, err := http.Get(env(t, "BEARING_TEST_API_URL") + "/api/v1/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	defer served.Body.Close()
	body, err := io.ReadAll(served.Body)
	if err != nil {
		t.Fatal(err)
	}
	want, err := httpapi.Spec().MarshalIndent()
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != string(want) {
		t.Error("the running api serves another document than this code generates; rebuild the stack")
	}
	doc := httpapi.Spec()
	for _, path := range []string{"/meta", "/readyz"} {
		url := env(t, "BEARING_TEST_API_URL") + "/api/v1" + path
		if path == "/readyz" {
			url = env(t, "BEARING_TEST_API_URL") + path
		}
		resp, err := http.Get(url)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		schema := doc.Paths[path]["get"].Responses[strconv.Itoa(resp.StatusCode)].Content["application/json"].Schema
		if err := doc.ValidateJSON(schema, raw); err != nil {
			t.Errorf("%s answered %s, which does not fit the document: %v", path, raw, err)
		}
	}
}
