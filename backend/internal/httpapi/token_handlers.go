package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/auth"
)

type personalTokenResponse struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
	// Write is whether it acts as far as its person may, rather than only reading.
	Write     bool      `json:"write"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type personalTokenRequest struct {
	Name  string `json:"name"`
	Write bool   `json:"write"`
	// Days is how long the token lives, from 1 to 365.
	Days int `json:"days"`
}

func toPersonalToken(t auth.PersonalToken) personalTokenResponse {
	return personalTokenResponse{ID: t.ID, Name: t.Name, Write: t.Write, CreatedAt: t.CreatedAt, ExpiresAt: t.ExpiresAt}
}

func (s *Server) handlePersonalTokens(w http.ResponseWriter, r *http.Request) {
	tokens, err := s.Auth.PersonalTokens(r.Context())
	if err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	out := make([]personalTokenResponse, len(tokens))
	for i, t := range tokens {
		out[i] = toPersonalToken(t)
	}
	respondJSON(w, http.StatusOK, env{"tokens": out})
}

func (s *Server) handleMakePersonalToken(w http.ResponseWriter, r *http.Request) {
	var in personalTokenRequest
	if err := decode(r, &in); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	made, secret, lsn, err := s.Auth.MakePersonalToken(r.Context(), PrincipalFrom(r.Context()).Account.ID, auth.TokenInput{Name: in.Name, Write: in.Write, Days: in.Days})
	noteWrite(r.Context(), lsn)
	var invalid auth.TokenInvalid
	if errors.As(err, &invalid) {
		err = ErrValidation(invalid)
	}
	if err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	respondJSON(w, http.StatusCreated, env{"token": toPersonalToken(made), "secret": secret})
}

func (s *Server) handleRevokePersonalToken(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "tokenID"))
	if err != nil {
		respondError(w, r, s.Log, ErrNotFound())
		return
	}
	lsn, err := s.Auth.RevokePersonalToken(r.Context(), id)
	noteWrite(r.Context(), lsn)
	if errors.Is(err, auth.ErrTokenNotFound) {
		err = ErrNotFound()
	}
	if err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
