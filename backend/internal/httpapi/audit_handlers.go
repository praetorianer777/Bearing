package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/org"
)

type eventResponse struct {
	ID uuid.UUID `json:"id"`
	At time.Time `json:"at"`
	// ActorID and ActorName are whom the change was made for; null when Bearing made it itself.
	ActorID   *uuid.UUID `json:"actorId"`
	ActorName *string    `json:"actorName"`
	// Table is what changed: unit, unit_placement, position, reporting_line, person, person_photo, holding, role,
	// position_role, deputy or unit_type.
	Table     string `json:"table"`
	Operation string `json:"operation"`
	// Before and After are the row as it was and as it became; null before an insert and after a delete.
	Before map[string]any `json:"before"`
	After  map[string]any `json:"after"`
}

const auditDescription = "Pages run newest first: before names the last entry of the page read, limit how many to answer, 50 when left out and 200 at most."

func auditSummary(of string) string {
	return "The changes to a " + of + " and to every relation naming it, newest first: who made each, when, and the row before and after."
}

var (
	auditBody    = env{"events": []eventResponse{}, "next": (*uuid.UUID)(nil)}
	personOnBody = env{"asOf": org.Day{}, "person": personResponse{}, "holdings": []holdingResponse{}}
)

func (s *Server) respondAudit(w http.ResponseWriter, r *http.Request, subject *uuid.UUID) {
	f := org.AuditFilter{Subject: subject, Limit: 50}
	fields := map[string]string{}
	if raw := r.URL.Query().Get("before"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			fields["before"] = "Name the last entry read by its id."
		}
		f.Before = &id
	}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > org.MaxAuditPage {
			fields["limit"] = "Ask for 1 to 200 entries."
		}
		f.Limit = n
	}
	if len(fields) > 0 {
		respondError(w, r, s.Log, ErrValidation(fields))
		return
	}
	events, err := s.Org.Audit(r.Context(), f)
	if err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	out := make([]eventResponse, len(events))
	for i, e := range events {
		out[i] = eventResponse{ID: e.ID, At: e.At, ActorID: e.ActorID, ActorName: e.ActorName, Table: e.Table, Operation: e.Operation}
		if err := unmarshalRow(e.Before, &out[i].Before); err != nil {
			respondError(w, r, s.Log, err)
			return
		}
		if err := unmarshalRow(e.After, &out[i].After); err != nil {
			respondError(w, r, s.Log, err)
			return
		}
	}
	// A full page may have more behind it; a short one is the last.
	var next *uuid.UUID
	if len(events) == f.Limit {
		next = &events[len(events)-1].ID
	}
	respondJSON(w, http.StatusOK, env{"events": out, "next": next})
}

func unmarshalRow(raw json.RawMessage, into *map[string]any) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, into)
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	s.respondAudit(w, r, nil)
}

func (s *Server) handleSubjectAudit(w http.ResponseWriter, r *http.Request, param string) {
	id, ok := s.pathID(w, r, param)
	if !ok {
		return
	}
	s.respondAudit(w, r, &id)
}

func (s *Server) handleUnitAudit(w http.ResponseWriter, r *http.Request) {
	s.handleSubjectAudit(w, r, "unitID")
}

func (s *Server) handlePositionAudit(w http.ResponseWriter, r *http.Request) {
	s.handleSubjectAudit(w, r, "positionID")
}

func (s *Server) handlePersonAudit(w http.ResponseWriter, r *http.Request) {
	s.handleSubjectAudit(w, r, "personID")
}

func (s *Server) handleRoleAudit(w http.ResponseWriter, r *http.Request) {
	s.handleSubjectAudit(w, r, "roleID")
}
