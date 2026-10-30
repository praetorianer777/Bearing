package httpapi

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/org"
)

type holderResponse struct {
	PersonID uuid.UUID `json:"personId"`
	Name     string    `json:"name"`
	Share    int       `json:"share"`
}

type linkResponse struct {
	PositionID uuid.UUID `json:"positionId"`
	Title      string    `json:"title"`
	UnitID     uuid.UUID `json:"unitId"`
	// From is the position it was reached from: the report going up, the manager going down.
	From uuid.UUID    `json:"from"`
	Kind org.LineKind `json:"kind"`
	// Depth is how many lines away it is, one for the nearest.
	Depth int `json:"depth"`
	// Holders are who holds it on the day asked about; none means it is vacant then.
	Holders []holderResponse `json:"holders"`
}

type memberResponse struct {
	PersonID   uuid.UUID `json:"personId"`
	Name       string    `json:"name"`
	PositionID uuid.UUID `json:"positionId"`
	Title      string    `json:"title"`
	UnitID     uuid.UUID `json:"unitId"`
	HeadOfUnit bool      `json:"headOfUnit"`
	HoldingID  uuid.UUID `json:"holdingId"`
	Share      int       `json:"share"`
}

type walkFunc func(ctx context.Context, id uuid.UUID, asOf org.Day, w org.Walk) ([]org.Link, error)

// respondWalk answers the positions reached from the one in the path, over the lines the query asks for.
func (s *Server) respondWalk(w http.ResponseWriter, r *http.Request, param string, walk walkFunc, refusal func(error) error) {
	id, ok := s.pathID(w, r, param)
	if !ok {
		return
	}
	day, ok := s.asOf(w, r)
	if !ok {
		return
	}
	query := r.URL.Query()
	links, err := walk(r.Context(), id, day, org.Walk{Dotted: query.Get("dotted") == "true", All: query.Get("all") == "true"})
	if err != nil {
		respondError(w, r, s.Log, refusal(err))
		return
	}
	out := make([]linkResponse, len(links))
	for i, l := range links {
		out[i] = linkResponse{PositionID: l.PositionID, Title: l.Title, UnitID: l.UnitID, From: l.From, Kind: l.Kind, Depth: l.Depth,
			Holders: make([]holderResponse, len(l.Holders))}
		for j, h := range l.Holders {
			out[i].Holders[j] = holderResponse{PersonID: h.PersonID, Name: h.Name, Share: h.Share}
		}
	}
	respondJSON(w, http.StatusOK, env{"asOf": day, "positions": out})
}

func (s *Server) handlePersonChain(w http.ResponseWriter, r *http.Request) {
	s.respondWalk(w, r, "personID", s.Org.ChainOfPerson, personError)
}

func (s *Server) handlePersonReports(w http.ResponseWriter, r *http.Request) {
	s.respondWalk(w, r, "personID", s.Org.ReportsOfPerson, personError)
}

func (s *Server) handlePositionChain(w http.ResponseWriter, r *http.Request) {
	s.respondWalk(w, r, "positionID", s.Org.ChainOfPosition, positionError)
}

func (s *Server) handlePositionReports(w http.ResponseWriter, r *http.Request) {
	s.respondWalk(w, r, "positionID", s.Org.ReportsOfPosition, positionError)
}

func (s *Server) handleMembers(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "unitID")
	if !ok {
		return
	}
	day, ok := s.asOf(w, r)
	if !ok {
		return
	}
	members, err := s.Org.Members(r.Context(), id, day, r.URL.Query().Get("subUnits") == "true")
	if err != nil {
		respondError(w, r, s.Log, orgError(err))
		return
	}
	out := make([]memberResponse, len(members))
	for i, m := range members {
		out[i] = memberResponse(m)
	}
	respondJSON(w, http.StatusOK, env{"asOf": day, "members": out})
}
