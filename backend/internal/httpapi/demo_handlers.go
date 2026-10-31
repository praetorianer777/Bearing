package httpapi

import (
	"errors"
	"net/http"

	"github.com/praetorianer777/bearing/backend/internal/demo"
)

type demoResponse struct {
	// Loaded says the demo organisation was loaded once; it is not loaded again.
	Loaded bool `json:"loaded"`
	// Empty says the installation has no units, so the demo may go in.
	Empty bool `json:"empty"`
}

func (s *Server) respondDemo(w http.ResponseWriter, r *http.Request, status int) {
	loaded, empty, err := s.Org.DemoState(r.Context())
	if err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	respondJSON(w, status, demoResponse{Loaded: loaded, Empty: empty})
}

func (s *Server) handleDemo(w http.ResponseWriter, r *http.Request) {
	s.respondDemo(w, r, http.StatusOK)
}

func (s *Server) handleLoadDemo(w http.ResponseWriter, r *http.Request) {
	lsn, err := demo.Load(r.Context(), s.Org)
	noteWrite(r.Context(), lsn)
	switch {
	case errors.Is(err, demo.ErrLoaded):
		respondError(w, r, s.Log, ErrConflict("demo_loaded", sentence(err.Error())))
	case errors.Is(err, demo.ErrNotEmpty):
		respondError(w, r, s.Log, ErrConflict("not_empty", sentence(err.Error())))
	case err != nil:
		respondError(w, r, s.Log, err)
	default:
		s.respondDemo(w, r, http.StatusCreated)
	}
}
