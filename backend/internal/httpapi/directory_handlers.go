package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/org"
)

type entryPositionResponse struct {
	PositionID uuid.UUID `json:"positionId"`
	Title      string    `json:"title"`
	UnitID     uuid.UUID `json:"unitId"`
	UnitName   string    `json:"unitName"`
	HeadOfUnit bool      `json:"headOfUnit"`
	Share      int       `json:"share"`
}

type entryRoleResponse struct {
	RoleID uuid.UUID `json:"roleId"`
	Key    string    `json:"key"`
	Name   string    `json:"name"`
	// Through is the position they have it through; null when they hold it directly.
	Through *uuid.UUID `json:"through"`
}

type entryResponse struct {
	AsOf      org.Day                 `json:"asOf"`
	Person    personResponse          `json:"person"`
	Positions []entryPositionResponse `json:"positions"`
	Roles     []entryRoleResponse     `json:"roles"`
}

type subtreeUnitResponse struct {
	unitResponse
	// Depth is how far below the unit asked about it sits, zero for that unit.
	Depth int `json:"depth"`
}

type managerResponse struct {
	linkResponse
	// People are its holders, or when it is vacant the holders of its first deputy, standing in; none when nobody is
	// there.
	People []resolvedResponse `json:"people"`
}

const (
	subjectQuery     = "issuer and subject name the person as the realm knows them."
	entryDescription = "asOf is a day written 2006-01-02, today when left out. Positions come heads of units first; roles held directly come before those through a position."
	// managersDescription explains what partners routing an approval to a manager need to know.
	managersDescription = "levels is how many solid lines up, one when left out, the direct manager; asOf is a day written 2006-01-02, today when left out. " +
		"A person holding several positions has a manager for each chain. A vacant manager's position comes down to the holders of its first deputy with any; " +
		"none when the chain ends sooner or nobody is there."
)

var managersBody = env{"asOf": org.Day{}, "levels": 0, "managers": []managerResponse{}}

// realmPerson is the person the query names by the realm's issuer and subject; it answers the refusal itself.
func (s *Server) realmPerson(w http.ResponseWriter, r *http.Request) (org.Person, bool) {
	issuer, subject := r.URL.Query().Get("issuer"), r.URL.Query().Get("subject")
	if issuer == "" || subject == "" {
		respondError(w, r, s.Log, ErrValidation(map[string]string{"subject": "Name the person by issuer and subject, as the realm does."}))
		return org.Person{}, false
	}
	p, err := s.Org.PersonInRealm(r.Context(), issuer, subject)
	if err != nil {
		respondError(w, r, s.Log, personError(err))
		return org.Person{}, false
	}
	return p, true
}

func (s *Server) handlePersonEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "personID")
	if !ok {
		return
	}
	p, err := s.Org.Person(r.Context(), id)
	if err != nil {
		respondError(w, r, s.Log, personError(err))
		return
	}
	s.respondEntry(w, r, p)
}

func (s *Server) handleRealmPersonEntry(w http.ResponseWriter, r *http.Request) {
	if p, ok := s.realmPerson(w, r); ok {
		s.respondEntry(w, r, p)
	}
}

func (s *Server) respondEntry(w http.ResponseWriter, r *http.Request, p org.Person) {
	day, ok := s.asOf(w, r)
	if !ok {
		return
	}
	e, err := s.Org.Entry(r.Context(), p, day)
	if err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	out := entryResponse{AsOf: day, Person: toPerson(e.Person), Positions: make([]entryPositionResponse, len(e.Positions)),
		Roles: make([]entryRoleResponse, len(e.Roles))}
	for i, p := range e.Positions {
		out.Positions[i] = entryPositionResponse(p)
	}
	for i, role := range e.Roles {
		out.Roles[i] = entryRoleResponse(role)
	}
	respondJSON(w, http.StatusOK, out)
}

func (s *Server) handleRealmPersonChain(w http.ResponseWriter, r *http.Request) {
	if p, ok := s.realmPerson(w, r); ok {
		s.respondWalkFrom(w, r, p.ID, s.Org.ChainOfPerson, personError)
	}
}

func (s *Server) handleRealmPersonReports(w http.ResponseWriter, r *http.Request) {
	if p, ok := s.realmPerson(w, r); ok {
		s.respondWalkFrom(w, r, p.ID, s.Org.ReportsOfPerson, personError)
	}
}

func (s *Server) handleSubtree(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "unitID")
	if !ok {
		return
	}
	day, ok := s.asOf(w, r)
	if !ok {
		return
	}
	units, depths, err := s.Org.Subtree(r.Context(), id, day)
	if err != nil {
		respondError(w, r, s.Log, orgError(err))
		return
	}
	out := make([]subtreeUnitResponse, len(units))
	for i, u := range units {
		out[i] = subtreeUnitResponse{unitResponse: toUnit(u), Depth: depths[i]}
	}
	respondJSON(w, http.StatusOK, env{"asOf": day, "units": out})
}

func (s *Server) handleRolePeople(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "roleID")
	if !ok {
		return
	}
	day, ok := s.asOf(w, r)
	if !ok {
		return
	}
	people, err := s.Org.RolePeople(r.Context(), id, day)
	if err != nil {
		respondError(w, r, s.Log, roleError(err))
		return
	}
	out := make([]resolvedResponse, len(people))
	for i, p := range people {
		out[i] = resolvedResponse(p)
	}
	respondJSON(w, http.StatusOK, env{"asOf": day, "people": out})
}

func (s *Server) handlePersonManagers(w http.ResponseWriter, r *http.Request) {
	if id, ok := s.pathID(w, r, "personID"); ok {
		s.respondManagers(w, r, id)
	}
}

func (s *Server) handleRealmPersonManagers(w http.ResponseWriter, r *http.Request) {
	if p, ok := s.realmPerson(w, r); ok {
		s.respondManagers(w, r, p.ID)
	}
}

func (s *Server) respondManagers(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	day, ok := s.asOf(w, r)
	if !ok {
		return
	}
	levels := 1
	if raw := r.URL.Query().Get("levels"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			respondError(w, r, s.Log, ErrValidation(map[string]string{"levels": "Count the levels up from one."}))
			return
		}
		levels = n
	}
	managers, err := s.Org.Managers(r.Context(), id, day, levels)
	if errors.Is(err, org.ErrNoLevels) {
		err = ErrValidation(map[string]string{"levels": sentence(err.Error())})
	}
	if err != nil {
		respondError(w, r, s.Log, personError(err))
		return
	}
	out := make([]managerResponse, len(managers))
	for i, m := range managers {
		out[i] = managerResponse{linkResponse: toLink(m.Link), People: make([]resolvedResponse, len(m.People))}
		for j, p := range m.People {
			out[i].People[j] = resolvedResponse(p)
		}
	}
	respondJSON(w, http.StatusOK, env{"asOf": day, "levels": levels, "managers": out})
}
