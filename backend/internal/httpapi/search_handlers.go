package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/org"
)

type searchHitResponse struct {
	Kind   string    `json:"kind"`
	ID     uuid.UUID `json:"id"`
	Name   string    `json:"name"`
	Detail string    `json:"detail"`
	// StepKey and Version name a step in the process ID is; Version is null for a draft not published yet.
	StepKey string `json:"stepKey,omitempty"`
	Version *int   `json:"version,omitempty"`
}

type searchResponse struct {
	AsOf      org.Day             `json:"asOf"`
	Query     string              `json:"query"`
	People    []searchHitResponse `json:"people"`
	Positions []searchHitResponse `json:"positions"`
	Units     []searchHitResponse `json:"units"`
	Roles     []searchHitResponse `json:"roles"`
	Processes []searchHitResponse `json:"processes"`
	Steps     []searchHitResponse `json:"steps"`
}

func toSearchHit(h org.SearchHit) searchHitResponse {
	return searchHitResponse{Kind: h.Kind, ID: h.ID, Name: h.Name, Detail: h.Detail, StepKey: h.StepKey, Version: h.Version}
}

// searchArgs reads q, asOf and limit, answering the request itself when one does not do.
func (s *Server) searchArgs(w http.ResponseWriter, r *http.Request, fallback, most int) (string, org.Day, int, bool) {
	day, ok := s.asOf(w, r)
	if !ok {
		return "", org.Day{}, 0, false
	}
	typed := strings.TrimSpace(r.URL.Query().Get("q"))
	switch n := utf8.RuneCountInString(typed); {
	case n < org.MinSearchLength || len(org.SearchWords(typed)) == 0:
		respondError(w, r, s.Log, ErrValidation(map[string]string{"q": "Give two letters or more."}))
		return "", org.Day{}, 0, false
	case n > org.MaxSearchLength:
		respondError(w, r, s.Log, ErrValidation(map[string]string{"q": fmt.Sprintf("Keep the search to %d characters.", org.MaxSearchLength)}))
		return "", org.Day{}, 0, false
	}
	limit := fallback
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > most {
			respondError(w, r, s.Log, ErrValidation(map[string]string{"limit": fmt.Sprintf("Give a number from 1 to %d.", most)}))
			return "", org.Day{}, 0, false
		}
		limit = n
	}
	return typed, day, limit, true
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	typed, day, limit, ok := s.searchArgs(w, r, org.DefaultSearchLimit, org.MaxSearchLimit)
	if !ok {
		return
	}
	hits, err := s.Org.Search(r.Context(), typed, day, limit)
	if err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	out := searchResponse{AsOf: day, Query: typed}
	groups := map[string]*[]searchHitResponse{org.SearchKindPerson: &out.People, org.SearchKindPosition: &out.Positions,
		org.SearchKindUnit: &out.Units, org.SearchKindRole: &out.Roles, org.SearchKindProcess: &out.Processes, org.SearchKindStep: &out.Steps}
	for _, group := range groups {
		*group = []searchHitResponse{}
	}
	for _, h := range hits {
		if group, ok := groups[h.Kind]; ok {
			*group = append(*group, toSearchHit(h))
		}
	}
	respondJSON(w, http.StatusOK, out)
}

func (s *Server) handleQuickSearch(w http.ResponseWriter, r *http.Request) {
	typed, day, limit, ok := s.searchArgs(w, r, org.DefaultQuickLimit, org.MaxQuickLimit)
	if !ok {
		return
	}
	hits, err := s.Org.Quick(r.Context(), typed, day, limit)
	if err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	out := make([]searchHitResponse, len(hits))
	for i, h := range hits {
		out[i] = toSearchHit(h)
	}
	respondJSON(w, http.StatusOK, env{"asOf": day, "query": typed, "hits": out})
}
