//go:build integration

package test

import (
	"encoding/json"
	"net/http"
	"testing"
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
