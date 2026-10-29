package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/praetorianer777/bearing/backend/internal/db"
)

// readinessTimeout bounds the readiness probe's own round trip to the primary.
const readinessTimeout = 2 * time.Second

type statusResponse struct {
	Status string `json:"status"`
}

type readinessResponse struct {
	Status   string             `json:"status"`
	Replicas []db.ReplicaStatus `json:"replicas"`
}

func (s *Server) handleLiveness(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, statusResponse{Status: "ok"})
}

// handleReadiness is ready while the primary answers; a replica that is down only sends reads to the primary.
func (s *Server) handleReadiness(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
	defer cancel()
	if err := s.DB.Ping(ctx); err != nil {
		respondError(w, r, s.Log, ErrUnavailable("The primary database does not answer.", err))
		return
	}
	replicas := s.DB.Stats().Replicas
	if replicas == nil {
		replicas = []db.ReplicaStatus{}
	}
	respondJSON(w, http.StatusOK, readinessResponse{Status: "ok", Replicas: replicas})
}
