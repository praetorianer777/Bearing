package httpapi

import (
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/org"
	"github.com/praetorianer777/bearing/backend/internal/orgcsv"
)

type importRequest struct {
	// Units, Positions, People and Holdings are the CSV texts; one left out changes nothing of its kind.
	orgcsv.Files
	// From is the day the changes take effect, today when left out.
	From *org.Day `json:"from,omitempty"`
}

type importResponse struct {
	ID     uuid.UUID `json:"id"`
	Status string    `json:"status"`
	From   org.Day   `json:"from"`
	// Counts are the preview's rows by action: create, update and unchanged.
	Counts  map[string]int    `json:"counts"`
	Changes []orgcsv.Change   `json:"changes"`
	Errors  []orgcsv.RowError `json:"errors"`
	// Result is what running it did; null until it has run.
	Result     *orgcsv.Result `json:"result"`
	CreatedAt  time.Time      `json:"createdAt"`
	FinishedAt *time.Time     `json:"finishedAt"`
}

func toImport(im orgcsv.Import) importResponse {
	return importResponse{ID: im.ID, Status: im.Status, From: im.Effective, Counts: im.Preview.Counts(), Changes: im.Preview.Changes,
		Errors: im.Preview.Errors, Result: im.Result, CreatedAt: im.CreatedAt, FinishedAt: im.FinishedAt}
}

func importError(err error) error {
	switch {
	case errors.Is(err, orgcsv.ErrImportNotFound):
		return ErrNotFound()
	case errors.Is(err, orgcsv.ErrImportRefused):
		return ErrConflict("rows_refused", sentence(err.Error()))
	case errors.Is(err, orgcsv.ErrImportStarted):
		return ErrConflict("started", sentence(err.Error()))
	}
	return err
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	kind := chi.URLParam(r, "kind")
	if !slices.Contains(orgcsv.Kinds, kind) {
		respondError(w, r, s.Log, ErrNotFound())
		return
	}
	day, ok := s.asOf(w, r)
	if !ok {
		return
	}
	body, err := orgcsv.Export(r.Context(), s.Org, kind, day)
	if err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+kind+"-"+day.String()+`.csv"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body)
}

func (s *Server) handlePreviewImport(w http.ResponseWriter, r *http.Request) {
	var req importRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	if req.Units == "" && req.Positions == "" && req.People == "" && req.Holdings == "" {
		respondError(w, r, s.Log, ErrValidation(map[string]string{"units": "Give at least one file."}))
		return
	}
	im, lsn, err := s.Imports.Preview(r.Context(), req.Files, dayOr(req.From))
	noteWrite(r.Context(), lsn)
	if err != nil {
		respondError(w, r, s.Log, importError(err))
		return
	}
	respondJSON(w, http.StatusCreated, env{"import": toImport(im)})
}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "importID")
	if !ok {
		return
	}
	im, err := s.Imports.Get(r.Context(), id)
	if err != nil {
		respondError(w, r, s.Log, importError(err))
		return
	}
	respondJSON(w, http.StatusOK, env{"import": toImport(im)})
}

func (s *Server) handleRunImport(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "importID")
	if !ok {
		return
	}
	im, lsn, err := s.Imports.Run(r.Context(), id)
	noteWrite(r.Context(), lsn)
	if err != nil {
		respondError(w, r, s.Log, importError(err))
		return
	}
	respondJSON(w, http.StatusAccepted, env{"import": toImport(im)})
}
