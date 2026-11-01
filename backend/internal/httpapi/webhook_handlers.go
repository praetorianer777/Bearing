package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/praetorianer777/bearing/backend/internal/webhook"
)

// webhookError says a refusal about webhooks in the API's terms.
func webhookError(err error) error {
	var field *webhook.FieldError
	switch {
	case errors.As(err, &field):
		return ErrValidation(map[string]string{field.Field: field.Message})
	case errors.Is(err, webhook.ErrNotFound):
		return ErrNotFound()
	case errors.Is(err, webhook.ErrNoKey):
		return &APIError{Status: http.StatusServiceUnavailable, Code: "no_secret_key", Message: sentence(err.Error())}
	}
	return err
}

func (s *Server) handleWebhooks(w http.ResponseWriter, r *http.Request) {
	hooks, err := s.Webhooks.List(r.Context())
	if err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	respondJSON(w, http.StatusOK, env{"webhooks": hooks})
}

func (s *Server) handleCreateWebhook(w http.ResponseWriter, r *http.Request) {
	var in webhook.Input
	if err := decode(r, &in); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	made, lsn, err := s.Webhooks.Create(r.Context(), in)
	noteWrite(r.Context(), lsn)
	if err != nil {
		respondError(w, r, s.Log, webhookError(err))
		return
	}
	respondJSON(w, http.StatusCreated, env{"webhook": made})
}

func (s *Server) handleUpdateWebhook(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "webhookID")
	if !ok {
		return
	}
	var in webhook.Input
	if err := decode(r, &in); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	changed, lsn, err := s.Webhooks.Update(r.Context(), id, in)
	noteWrite(r.Context(), lsn)
	if err != nil {
		respondError(w, r, s.Log, webhookError(err))
		return
	}
	respondJSON(w, http.StatusOK, env{"webhook": changed})
}

func (s *Server) handleDeleteWebhook(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "webhookID")
	if !ok {
		return
	}
	lsn, err := s.Webhooks.Delete(r.Context(), id)
	noteWrite(r.Context(), lsn)
	if err != nil {
		respondError(w, r, s.Log, webhookError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRotateWebhookSecret(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "webhookID")
	if !ok {
		return
	}
	rotated, lsn, err := s.Webhooks.RotateSecret(r.Context(), id)
	noteWrite(r.Context(), lsn)
	if err != nil {
		respondError(w, r, s.Log, webhookError(err))
		return
	}
	respondJSON(w, http.StatusOK, env{"webhook": rotated})
}

func (s *Server) handleTestWebhook(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "webhookID")
	if !ok {
		return
	}
	sent, lsn, err := s.Webhooks.Test(r.Context(), id)
	noteWrite(r.Context(), lsn)
	if err != nil {
		respondError(w, r, s.Log, webhookError(err))
		return
	}
	respondJSON(w, http.StatusOK, env{"delivery": sent})
}

func (s *Server) handleWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "webhookID")
	if !ok {
		return
	}
	limit := webhook.DefaultDeliveries
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			respondError(w, r, s.Log, ErrValidation(map[string]string{"limit": "Ask for at least one delivery."}))
			return
		}
		limit = n
	}
	deliveries, err := s.Webhooks.Deliveries(r.Context(), id, limit)
	if err != nil {
		respondError(w, r, s.Log, webhookError(err))
		return
	}
	respondJSON(w, http.StatusOK, env{"deliveries": deliveries})
}

func (s *Server) handleRedeliverWebhook(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "webhookID")
	if !ok {
		return
	}
	delivery, ok := s.pathID(w, r, "deliveryID")
	if !ok {
		return
	}
	sent, lsn, err := s.Webhooks.Redeliver(r.Context(), id, delivery)
	noteWrite(r.Context(), lsn)
	if err != nil {
		respondError(w, r, s.Log, webhookError(err))
		return
	}
	respondJSON(w, http.StatusOK, env{"delivery": sent})
}
