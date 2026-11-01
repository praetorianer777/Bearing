package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/org"
)

func TestPartnersNameAPersonByIssuerAndSubject(t *testing.T) {
	s := &Server{Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	for _, query := range []string{"", "?issuer=https://realm", "?subject=sub"} {
		w := httptest.NewRecorder()
		if _, ok := s.realmPerson(w, httptest.NewRequest(http.MethodGet, "/people/by-subject"+query, nil)); ok || w.Code != http.StatusUnprocessableEntity {
			t.Errorf("%q answered %d", query, w.Code)
		}
	}
}

func TestManagersCountLevelsFromOne(t *testing.T) {
	s := &Server{Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	for _, levels := range []string{"0", "-1", "two"} {
		r := httptest.NewRequest(http.MethodGet, "/people/x/managers?levels="+levels, nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("personID", uuid.NewString())
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
		w := httptest.NewRecorder()
		s.handlePersonManagers(w, r)
		var got struct {
			Error struct{ Fields map[string]string }
		}
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || w.Code != http.StatusUnprocessableEntity || got.Error.Fields["levels"] == "" {
			t.Errorf("levels=%s answered %d %s", levels, w.Code, w.Body)
		}
	}
}

func TestTheDirectoryRefusesWhatItCannotRead(t *testing.T) {
	s := &Server{Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	id := uuid.NewString()
	for name, c := range map[string]struct {
		handle    func(http.ResponseWriter, *http.Request)
		param, id string
		query     string
		want      int
	}{
		"entry by subject without one":    {s.handleRealmPersonEntry, "", "", "", http.StatusUnprocessableEntity},
		"chain by subject without one":    {s.handleRealmPersonChain, "", "", "?issuer=x", http.StatusUnprocessableEntity},
		"reports by subject without one":  {s.handleRealmPersonReports, "", "", "?subject=x", http.StatusUnprocessableEntity},
		"managers by subject without one": {s.handleRealmPersonManagers, "", "", "", http.StatusUnprocessableEntity},
		"an entry of no id":               {s.handlePersonEntry, "personID", "x", "", http.StatusNotFound},
		"the tree of no id":               {s.handleSubtree, "unitID", "x", "", http.StatusNotFound},
		"a tree on no day":                {s.handleSubtree, "unitID", id, "?asOf=soon", http.StatusUnprocessableEntity},
		"a role of no id":                 {s.handleRolePeople, "roleID", "x", "", http.StatusNotFound},
		"a role on no day":                {s.handleRolePeople, "roleID", id, "?asOf=soon", http.StatusUnprocessableEntity},
		"managers on no day":              {s.handlePersonManagers, "personID", id, "?asOf=soon", http.StatusUnprocessableEntity},
	} {
		r := httptest.NewRequest(http.MethodGet, "/x"+c.query, nil)
		rctx := chi.NewRouteContext()
		if c.param != "" {
			rctx.URLParams.Add(c.param, c.id)
		}
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
		w := httptest.NewRecorder()
		c.handle(w, r)
		if w.Code != c.want {
			t.Errorf("%s answered %d, want %d", name, w.Code, c.want)
		}
	}
}

func TestALinkCarriesItsHoldersRealmNames(t *testing.T) {
	subject := "sub"
	l := toLink(org.Link{Title: "Lead", Depth: 2, Holders: []org.Holder{{Name: "Lena", Share: 50, Subject: &subject}}})
	if l.Title != "Lead" || l.Depth != 2 || len(l.Holders) != 1 || l.Holders[0].Subject == nil || *l.Holders[0].Subject != "sub" {
		t.Errorf("the link is %+v", l)
	}
}
