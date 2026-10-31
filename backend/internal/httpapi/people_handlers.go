package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/db"
	"github.com/praetorianer777/bearing/backend/internal/org"
)

type personRequest struct {
	Name *string `json:"name,omitempty"`
	// Email, Phone and Location are taken away by an empty string.
	Email    *string `json:"email,omitempty"`
	Phone    *string `json:"phone,omitempty"`
	Location *string `json:"location,omitempty"`
}

type linkAccountRequest struct {
	// AccountID is the account to link; left out, the person is unlinked.
	AccountID *uuid.UUID `json:"accountId,omitempty"`
}

type photoRequest struct {
	// Data is a PNG, JPEG, WebP or GIF picture of at most 1 MB.
	Data []byte `json:"data"`
}

type selfEditableBody struct {
	// Fields are those of their own entry people may change: name, email, phone, location and photo.
	Fields []string `json:"fields"`
}

type personResponse struct {
	ID       uuid.UUID      `json:"id"`
	Name     string         `json:"name"`
	Email    *string        `json:"email"`
	Phone    *string        `json:"phone"`
	Location *string        `json:"location"`
	Kind     org.PersonKind `json:"kind"`
	// AccountID is the account here the person signs in with.
	AccountID *uuid.UUID `json:"accountId"`
	// Issuer and Subject are how the realm knows the person.
	Issuer   *string  `json:"issuer"`
	Subject  *string  `json:"subject"`
	LeftOn   *org.Day `json:"leftOn"`
	HasPhoto bool     `json:"hasPhoto"`
}

func toPerson(p org.Person) personResponse {
	return personResponse{ID: p.ID, Name: p.Name, Email: p.Email, Phone: p.Phone, Location: p.Location, Kind: p.Kind(),
		AccountID: p.AccountID, Issuer: p.Issuer, Subject: p.Subject, LeftOn: p.LeftOn, HasPhoto: p.HasPhoto}
}

// personError says a refusal about people in the API's terms.
func personError(err error) error {
	switch {
	case errors.Is(err, org.ErrPersonNotFound):
		return ErrNotFound()
	case errors.Is(err, org.ErrNoEntry):
		return &APIError{Status: http.StatusNotFound, Code: "no_entry", Message: sentence(err.Error())}
	case errors.Is(err, org.ErrNotYours):
		return ErrForbidden(sentence(err.Error()))
	case errors.Is(err, org.ErrEmailTaken):
		return &APIError{Status: http.StatusConflict, Code: "email_taken", Message: sentence(err.Error()),
			Fields: map[string]string{"email": sentence(err.Error())}}
	case errors.Is(err, org.ErrBadPhoto):
		return ErrValidation(map[string]string{"data": sentence(err.Error())})
	case errors.Is(err, org.ErrAccountMissing):
		return ErrValidation(map[string]string{"accountId": sentence(err.Error())})
	}
	for code, refusal := range map[string]error{"account_linked": org.ErrAccountLinked, "left": org.ErrLeft, "not_left": org.ErrNotLeft} {
		if errors.Is(err, refusal) {
			return ErrConflict(code, sentence(err.Error()))
		}
	}
	return orgError(err)
}

func (s *Server) respondPerson(w http.ResponseWriter, r *http.Request, status int, p org.Person, err error) {
	if err != nil {
		respondError(w, r, s.Log, personError(err))
		return
	}
	respondJSON(w, status, env{"person": toPerson(p)})
}

func (s *Server) handlePeople(w http.ResponseWriter, r *http.Request) {
	day, ok := s.asOf(w, r)
	if !ok {
		return
	}
	people, err := s.Org.People(r.Context(), day, r.URL.Query().Get("left") == "true", r.URL.Query().Get("q"))
	if err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	out := make([]personResponse, len(people))
	for i, p := range people {
		out[i] = toPerson(p)
	}
	respondJSON(w, http.StatusOK, env{"asOf": day, "people": out})
}

func (s *Server) handlePerson(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "personID")
	if !ok {
		return
	}
	day, ok := s.asOf(w, r)
	if !ok {
		return
	}
	p, err := s.Org.Person(r.Context(), id)
	s.respondPersonOn(w, r, day, p, err)
}

// respondPersonOn answers a person with what they hold on a day.
func (s *Server) respondPersonOn(w http.ResponseWriter, r *http.Request, day org.Day, p org.Person, err error) {
	if err != nil {
		respondError(w, r, s.Log, personError(err))
		return
	}
	holdings, err := s.Org.Holdings(r.Context(), org.HoldingFilter{PersonID: &p.ID, AsOf: &day})
	if err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	out := make([]holdingResponse, len(holdings))
	for i, h := range holdings {
		out[i] = toHolding(h)
	}
	respondJSON(w, http.StatusOK, env{"asOf": day, "person": toPerson(p), "holdings": out})
}

func (s *Server) handleCreatePerson(w http.ResponseWriter, r *http.Request) {
	var req personRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	p, lsn, err := s.Org.CreatePerson(r.Context(), org.PersonInput(req))
	noteWrite(r.Context(), lsn)
	s.respondPerson(w, r, http.StatusCreated, p, err)
}

func (s *Server) handleUpdatePerson(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "personID")
	if !ok {
		return
	}
	var req personRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	p, lsn, err := s.Org.UpdatePerson(r.Context(), id, org.PersonInput(req))
	noteWrite(r.Context(), lsn)
	s.respondPerson(w, r, http.StatusOK, p, err)
}

func (s *Server) handleLinkAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "personID")
	if !ok {
		return
	}
	var req linkAccountRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	p, lsn, err := s.Org.LinkAccount(r.Context(), id, req.AccountID)
	noteWrite(r.Context(), lsn)
	s.respondPerson(w, r, http.StatusOK, p, err)
}

func (s *Server) handleLeave(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "personID")
	if !ok {
		return
	}
	var req endRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	p, lsn, err := s.Org.Leave(r.Context(), id, dayOr(req.On))
	noteWrite(r.Context(), lsn)
	s.respondPerson(w, r, http.StatusOK, p, err)
}

func (s *Server) handleReturn(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "personID")
	if !ok {
		return
	}
	p, lsn, err := s.Org.Return(r.Context(), id)
	noteWrite(r.Context(), lsn)
	s.respondPerson(w, r, http.StatusOK, p, err)
}

func (s *Server) handleMyEntry(w http.ResponseWriter, r *http.Request) {
	day, ok := s.asOf(w, r)
	if !ok {
		return
	}
	p, err := s.Org.Me(r.Context())
	s.respondPersonOn(w, r, day, p, err)
}

func (s *Server) handleUpdateMyEntry(w http.ResponseWriter, r *http.Request) {
	var req personRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	p, lsn, err := s.Org.UpdateMe(r.Context(), org.PersonInput(req))
	noteWrite(r.Context(), lsn)
	s.respondPerson(w, r, http.StatusOK, p, err)
}

func (s *Server) handlePhoto(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "personID")
	if !ok {
		return
	}
	kind, data, err := s.Org.Photo(r.Context(), id)
	if err != nil {
		respondError(w, r, s.Log, personError(err))
		return
	}
	h := w.Header()
	h.Set("Content-Type", kind)
	h.Set("Content-Length", strconv.Itoa(len(data)))
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	h.Set("Cache-Control", "private, no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) setPhoto(w http.ResponseWriter, r *http.Request, id *uuid.UUID, remove bool) {
	var data []byte
	if !remove {
		var req photoRequest
		if err := decodeUpTo(r, &req, 2*org.MaxPhotoBytes); err != nil {
			respondError(w, r, s.Log, err)
			return
		}
		if req.Data == nil {
			req.Data = []byte{}
		}
		data = req.Data
	}
	lsn, err := s.Org.SetPhoto(r.Context(), id, data)
	noteWrite(r.Context(), lsn)
	if err != nil {
		respondError(w, r, s.Log, personError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSetPhoto(w http.ResponseWriter, r *http.Request) {
	if id, ok := s.pathID(w, r, "personID"); ok {
		s.setPhoto(w, r, &id, false)
	}
}

func (s *Server) handleDeletePhoto(w http.ResponseWriter, r *http.Request) {
	if id, ok := s.pathID(w, r, "personID"); ok {
		s.setPhoto(w, r, &id, true)
	}
}

func (s *Server) handleSetMyPhoto(w http.ResponseWriter, r *http.Request) {
	s.setPhoto(w, r, nil, false)
}
func (s *Server) handleDeleteMyPhoto(w http.ResponseWriter, r *http.Request) {
	s.setPhoto(w, r, nil, true)
}

func (s *Server) handleSelfEditable(w http.ResponseWriter, r *http.Request) {
	fields, err := s.Org.SelfEditableFields(r.Context())
	if err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	respondJSON(w, http.StatusOK, selfEditableBody{Fields: fields})
}

func (s *Server) handleSetSelfEditable(w http.ResponseWriter, r *http.Request) {
	var req selfEditableBody
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	lsn, err := s.Org.SetSelfEditableFields(r.Context(), req.Fields)
	noteWrite(r.Context(), lsn)
	if err != nil {
		respondError(w, r, s.Log, personError(err))
		return
	}
	// The answer reads what was just written, which a lagging replica may not have yet.
	s.handleSelfEditable(w, r.WithContext(db.PinLSN(r.Context(), lsn)))
}
