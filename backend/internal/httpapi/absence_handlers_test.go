package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/org"
)

func TestOnlyWorkingDaysCount(t *testing.T) {
	from, _ := time.Parse(time.DateOnly, "2026-11-06")
	to, _ := time.Parse(time.DateOnly, "2026-11-10")
	var got []string
	for _, d := range workingDays(from, to) {
		got = append(got, d.Format(time.DateOnly))
	}
	if !slices.Equal(got, []string{"2026-11-06", "2026-11-09", "2026-11-10"}) {
		t.Errorf("the working days are %v", got)
	}
}

func TestNobodyIsThereOnlyWhenEveryHolderAndDeputyIsAway(t *testing.T) {
	who := func(name string) org.Resolved { return org.Resolved{PersonID: uuid.New(), Name: name} }
	hal, dee, ida := who("hal"), who("dee"), who("ida")
	days := make([]time.Time, 5)
	for i := range days {
		days[i] = time.Date(2026, 11, 9+i, 0, 0, 0, 0, time.UTC)
	}
	away := map[string][]int{"hal": {0, 1, 2, 4}, "dee": {1, 4}}
	isAway := func(p org.Resolved, day time.Time) bool { return slices.Contains(away[p.Name], day.Day()-9) }
	held := org.Cover{Holders: []org.Resolved{hal}, Deputies: []org.Resolved{dee}}
	unknown := org.Cover{Holders: []org.Resolved{ida}}
	covers := make([][]org.Cover, len(days))
	for d := range days {
		covers[d] = []org.Cover{held, {}, unknown}
	}
	got := nobodyThere(days, covers, isAway)
	if !slices.Equal(got[0], [][2]int{{1, 1}, {4, 4}}) {
		t.Errorf("the covered position has nobody there on %v", got[0])
	}
	if len(got[1]) != 0 {
		t.Errorf("a vacancy counts as nobody there: %v", got[1])
	}
	if len(got[2]) != 0 {
		t.Errorf("somebody who is never away is: %v", got[2])
	}
	if nobodyThere(nil, nil, isAway) != nil {
		t.Error("no days have gaps")
	}
}

func TestAbsencesRefuseWhatTheyCannotRead(t *testing.T) {
	s := &Server{Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	for name, c := range map[string]struct {
		handle func(http.ResponseWriter, *http.Request)
		params map[string]string
		query  string
	}{
		"a role of no id":          {s.handleRoleAbsences, map[string]string{"roleID": "x"}, ""},
		"a role over a year":       {s.handleRoleAbsences, map[string]string{"roleID": uuid.NewString()}, "?from=2026-11-02&to=2027-11-02"},
		"a process of no id":       {s.handleProcessAbsences, map[string]string{"processID": "x"}, ""},
		"a process of no version":  {s.handleProcessAbsences, map[string]string{"processID": uuid.NewString()}, "?version=draft"},
		"a process ending earlier": {s.handleProcessAbsences, map[string]string{"processID": uuid.NewString()}, "?from=2026-11-02&to=2026-11-01"},
		"a unit of no id":          {s.handleAbsences, nil, "?unitId=x"},
		"a day that is none":       {s.handleAbsences, nil, "?from=soon"},
	} {
		r := httptest.NewRequest(http.MethodGet, "/x"+c.query, nil)
		rctx := chi.NewRouteContext()
		for k, v := range c.params {
			rctx.URLParams.Add(k, v)
		}
		w := httptest.NewRecorder()
		c.handle(w, r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx)))
		if w.Code != http.StatusNotFound && w.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s answered %d", name, w.Code)
		}
	}
}
