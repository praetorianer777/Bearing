package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/praetorianer777/bearing/backend/internal/netguard"
	"github.com/praetorianer777/bearing/backend/internal/webhook"
)

func TestWebhookRefusalsAreSaidInTheAPIsTerms(t *testing.T) {
	for err, want := range map[error]int{
		&webhook.FieldError{Field: "url", Message: "No."}: http.StatusUnprocessableEntity,
		webhook.ErrNotFound: http.StatusNotFound,
		webhook.ErrNoKey:    http.StatusServiceUnavailable,
	} {
		var apiErr *APIError
		if !errors.As(webhookError(err), &apiErr) || apiErr.Status != want {
			t.Errorf("%v became %v, want %d", err, webhookError(err), want)
		}
	}
	other := errors.New("other")
	if webhookError(other) != other {
		t.Error("an error of nobody's is changed")
	}
}

func TestWebhookOperationsRefuseWhatTheyCannotRead(t *testing.T) {
	s := &Server{Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Webhooks: webhook.New(nil, nil, netguard.Allow{}, nil)}
	for name, c := range map[string]struct {
		handle func(http.ResponseWriter, *http.Request)
		params map[string]string
		body   string
		want   int
	}{
		"a change of no id":           {s.handleUpdateWebhook, map[string]string{"webhookID": "x"}, "{}", http.StatusNotFound},
		"a change that is no JSON":    {s.handleUpdateWebhook, map[string]string{"webhookID": "0199e1c2-7d4a-7c11-9b1e-3f0a2b4c5d6e"}, "[", http.StatusBadRequest},
		"a change without a name":     {s.handleUpdateWebhook, map[string]string{"webhookID": "0199e1c2-7d4a-7c11-9b1e-3f0a2b4c5d6e"}, "{}", http.StatusUnprocessableEntity},
		"a new one that is no JSON":   {s.handleCreateWebhook, nil, "[", http.StatusBadRequest},
		"a new one without a key":     {s.handleCreateWebhook, nil, `{"name":"x","url":"https://example.com","topics":["*"]}`, http.StatusServiceUnavailable},
		"a deletion of no id":         {s.handleDeleteWebhook, map[string]string{"webhookID": "x"}, "", http.StatusNotFound},
		"a new secret of no id":       {s.handleRotateWebhookSecret, map[string]string{"webhookID": "x"}, "", http.StatusNotFound},
		"a test of no id":             {s.handleTestWebhook, map[string]string{"webhookID": "x"}, "", http.StatusNotFound},
		"the log of no id":            {s.handleWebhookDeliveries, map[string]string{"webhookID": "x"}, "", http.StatusNotFound},
		"a redelivery of no delivery": {s.handleRedeliverWebhook, map[string]string{"webhookID": "0199e1c2-7d4a-7c11-9b1e-3f0a2b4c5d6e", "deliveryID": "x"}, "", http.StatusNotFound},
		"a redelivery of no webhook":  {s.handleRedeliverWebhook, map[string]string{"webhookID": "x"}, "", http.StatusNotFound},
	} {
		r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(c.body))
		r.Header.Set("Content-Type", "application/json")
		rctx := chi.NewRouteContext()
		for k, v := range c.params {
			rctx.URLParams.Add(k, v)
		}
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
		w := httptest.NewRecorder()
		c.handle(w, r)
		if w.Code != c.want {
			t.Errorf("%s answered %d, want %d: %s", name, w.Code, c.want, w.Body)
		}
	}
	r := httptest.NewRequest(http.MethodGet, "/x?limit=0", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("webhookID", "0199e1c2-7d4a-7c11-9b1e-3f0a2b4c5d6e")
	w := httptest.NewRecorder()
	s.handleWebhookDeliveries(w, r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx)))
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("limit=0 answered %d", w.Code)
	}
}
