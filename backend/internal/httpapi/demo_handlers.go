package httpapi

import (
	"errors"
	"net/http"

	"github.com/praetorianer777/bearing/backend/internal/demo"
)

type demoPartResponse struct {
	// Loaded says the part was loaded once; it is not loaded again.
	Loaded bool `json:"loaded"`
	// Empty says the installation has nothing of that part of its own: no units, or no processes.
	Empty bool `json:"empty"`
}

type demoResponse struct {
	Organisation demoPartResponse `json:"organisation"`
	// Processes go in where there are none and the organisation is the demo's.
	Processes demoPartResponse `json:"processes"`
}

func (s *Server) respondDemo(w http.ResponseWriter, r *http.Request, status int) {
	d, err := s.Org.DemoState(r.Context())
	if err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	respondJSON(w, status, demoResponse{Organisation: demoPartResponse(d.Organisation), Processes: demoPartResponse(d.Processes)})
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
