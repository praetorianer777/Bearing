package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func TestCardsRefuseWhatTheyCannotShow(t *testing.T) {
	s := &Server{Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	for name, c := range map[string]struct {
		handle func(http.ResponseWriter, *http.Request)
		params map[string]string
		query  string
		want   int
	}{
		"a kind of no card":           {s.handleCard, map[string]string{"kind": "process", "cardID": uuid.NewString()}, "", http.StatusNotFound},
		"a card of no id":             {s.handleCard, map[string]string{"kind": "unit", "cardID": "x"}, "", http.StatusNotFound},
		"a card on no day":            {s.handleCard, map[string]string{"kind": "unit", "cardID": uuid.NewString()}, "?asOf=soon", http.StatusUnprocessableEntity},
		"a card of no subject":        {s.handleRealmPersonCard, nil, "", http.StatusUnprocessableEntity},
		"a subject on no day":         {s.handleRealmPersonCard, nil, "?issuer=i&subject=s&asOf=soon", http.StatusUnprocessableEntity},
		"one letter":                  {s.handleSuggestCards, nil, "?q=a", http.StatusUnprocessableEntity},
		"a kind of no card asked for": {s.handleSuggestCards, nil, "?q=ab&kinds=person,step", http.StatusUnprocessableEntity},
	} {
		r := httptest.NewRequest(http.MethodGet, "/x"+c.query, nil)
		rctx := chi.NewRouteContext()
		for k, v := range c.params {
			rctx.URLParams.Add(k, v)
		}
		w := httptest.NewRecorder()
		c.handle(w, r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx)))
		if w.Code != c.want {
			t.Errorf("%s answered %d, want %d", name, w.Code, c.want)
		}
	}
}

func TestACardClipsALongDescription(t *testing.T) {
	if got := clip("short", 10); got != "short" {
		t.Errorf("%q", got)
	}
	if got := clip("äöü two words", 5); got != "äöü t" {
		t.Errorf("%q", got)
	}
	s := &Server{AppURL: "https://bearing.example"}
	if got := s.cardURL("role", uuid.Nil); got != "https://bearing.example/roles/"+uuid.Nil.String() {
		t.Errorf("%q", got)
	}
}
