package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestARangeReadsFromAndToOrThirteenWeeksFromToday(t *testing.T) {
	fields := map[string]string{}
	from, to := readRange(httptest.NewRequest("GET", "/x?from=2026-11-02&to=2026-12-01", nil), fields)
	if from.String() != "2026-11-02" || to.String() != "2026-12-01" || len(fields) != 0 {
		t.Errorf("given: %s to %s, %v", from, to, fields)
	}
	from, to = readRange(httptest.NewRequest("GET", "/x?from=2026-11-02", nil), fields)
	if to.String() != "2027-01-31" {
		t.Errorf("thirteen weeks on: %s to %s", from, to)
	}
	from, _ = readRange(httptest.NewRequest("GET", "/x", nil), fields)
	if from != today() {
		t.Errorf("today: %s", from)
	}
	readRange(httptest.NewRequest("GET", "/x?from=soon&to=later", nil), fields)
	if fields["from"] == "" || fields["to"] == "" {
		t.Errorf("unreadable days are noted: %v", fields)
	}
}
