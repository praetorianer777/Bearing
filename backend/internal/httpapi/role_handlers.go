package httpapi

import (
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/org"
)

type orgRoleRequest struct {
	// Key is what other tools name the role by: small letters, digits and dashes.
	Key         *string `json:"key,omitempty"`
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

type orgRoleHolderResponse struct {
	PersonID  uuid.UUID `json:"personId"`
	HoldingID uuid.UUID `json:"holdingId"`
	Share     int       `json:"share"`
	// Through is the position they have the role through; null when they hold it directly.
	Through *uuid.UUID `json:"through"`
}

type orgRoleResponse struct {
	ID          uuid.UUID `json:"id"`
	Key         string    `json:"key"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	// Positions are those carrying it on the day asked about; only when one role is asked for.
	Positions []uuid.UUID `json:"positions,omitempty"`
	// Holders are who has it on that day, directly or through a position; only when one role is asked for.
	Holders []orgRoleHolderResponse `json:"holders,omitempty"`
}

type carryRoleRequest struct {
	RoleID uuid.UUID `json:"roleId"`
	// From is the day the position starts to carry it, today when left out.
	From *org.Day `json:"from,omitempty"`
}

type deputiesRequest struct {
	// DeputyIDs are the positions standing in, first first; an empty list takes them all away.
	DeputyIDs []uuid.UUID `json:"deputyIds"`
}

func toRole(r org.Role) orgRoleResponse {
	out := orgRoleResponse{ID: r.ID, Key: r.Key, Name: r.Name, Description: r.Description, Positions: r.Positions}
	for _, h := range r.Holders {
		out.Holders = append(out.Holders, orgRoleHolderResponse{PersonID: h.PersonID, HoldingID: h.HoldingID, Share: h.Share, Through: h.Through})
	}
	return out
}

// roleError says a refusal about roles in the API's terms.
func roleError(err error) error {
	switch {
	case errors.Is(err, org.ErrRoleNotFound):
		return ErrNotFound()
	case errors.Is(err, org.ErrRoleKeyTaken):
		return &APIError{Status: http.StatusConflict, Code: "key_taken", Message: sentence(err.Error()),
			Fields: map[string]string{"key": sentence(err.Error())}}
	case errors.Is(err, org.ErrRoleInUse):
		return ErrConflict("role_in_use", sentence(err.Error()))
	}
	return orgError(err)
}

// carriedRoleError says a refusal about the roles a position carries, where an unknown role is a field to fix.
func carriedRoleError(err error) error {
	switch {
	case errors.Is(err, org.ErrRoleNotFound):
		return ErrValidation(map[string]string{"roleId": sentence(err.Error())})
	case errors.Is(err, org.ErrRoleCarried):
		return ErrConflict("role_carried", sentence(err.Error()))
	case errors.Is(err, org.ErrRoleNotCarried):
		return ErrConflict("role_not_carried", sentence(err.Error()))
	case errors.Is(err, org.ErrDeputyNotThere):
		return ErrValidation(map[string]string{"deputyIds": sentence(err.Error())})
	}
	return positionError(err)
}

func (s *Server) respondRole(w http.ResponseWriter, r *http.Request, status int, role org.Role, err error) {
	if err != nil {
		respondError(w, r, s.Log, roleError(err))
		return
	}
	respondJSON(w, status, env{"role": toRole(role)})
}

func (s *Server) respondCarried(w http.ResponseWriter, r *http.Request, status int, p org.Position, err error) {
	if err != nil {
		respondError(w, r, s.Log, carriedRoleError(err))
		return
	}
	respondJSON(w, status, env{"position": toPosition(p)})
}

func (s *Server) handleRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := s.Org.Roles(r.Context())
	if err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	out := make([]orgRoleResponse, len(roles))
	for i, role := range roles {
		out[i] = toRole(role)
	}
	respondJSON(w, http.StatusOK, env{"roles": out})
}

func (s *Server) handleRole(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "roleID")
	if !ok {
		return
	}
	day, ok := s.asOf(w, r)
	if !ok {
		return
	}
	role, err := s.Org.Role(r.Context(), id, day)
	if err != nil {
		respondError(w, r, s.Log, roleError(err))
		return
	}
	respondJSON(w, http.StatusOK, env{"asOf": day, "role": toRole(role)})
}

func (s *Server) handleCreateRole(w http.ResponseWriter, r *http.Request) {
	var req orgRoleRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	role, lsn, err := s.Org.CreateRole(r.Context(), org.RoleInput(req))
	noteWrite(r.Context(), lsn)
	s.respondRole(w, r, http.StatusCreated, role, err)
}

func (s *Server) handleUpdateRole(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "roleID")
	if !ok {
		return
	}
	var req orgRoleRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	role, lsn, err := s.Org.UpdateRole(r.Context(), id, org.RoleInput(req), today())
	noteWrite(r.Context(), lsn)
	s.respondRole(w, r, http.StatusOK, role, err)
}

func (s *Server) handleDeleteRole(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "roleID")
	if !ok {
		return
	}
	lsn, err := s.Org.DeleteRole(r.Context(), id)
	noteWrite(r.Context(), lsn)
	if err != nil {
		respondError(w, r, s.Log, roleError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCarryRole(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "positionID")
	if !ok {
		return
	}
	var req carryRoleRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	p, lsn, err := s.Org.CarryRole(r.Context(), id, req.RoleID, dayOr(req.From))
	noteWrite(r.Context(), lsn)
	s.respondCarried(w, r, http.StatusCreated, p, err)
}

func (s *Server) handleDropRole(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "positionID")
	if !ok {
		return
	}
	role, ok := s.pathID(w, r, "roleID")
	if !ok {
		return
	}
	var req endRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	p, lsn, err := s.Org.DropRole(r.Context(), id, role, dayOr(req.On))
	noteWrite(r.Context(), lsn)
	s.respondCarried(w, r, http.StatusOK, p, err)
}

func (s *Server) handleSetDeputies(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "positionID")
	if !ok {
		return
	}
	var req deputiesRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	p, lsn, err := s.Org.SetDeputies(r.Context(), id, req.DeputyIDs)
	noteWrite(r.Context(), lsn)
	s.respondCarried(w, r, http.StatusOK, p, err)
}
