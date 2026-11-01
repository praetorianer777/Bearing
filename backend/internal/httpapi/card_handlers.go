package httpapi

import (
	"net/http"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/org"
)

// cardKinds are what a card shows, as org-embed@1 names them.
var cardKinds = []string{"person", "position", "unit", "role"}

// cardPaths are where each kind's page is in the web client.
var cardPaths = map[string]string{"person": "/people/", "position": "/positions/", "unit": "/units/", "role": "/roles/"}

// maxCardDetail is how much of a role's description a card carries, as the search does.
const maxCardDetail = 120

type cardResponse struct {
	// Kind is person, position, unit or role.
	Kind string    `json:"kind"`
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
	// Detail is one line beneath the name: a person's position and unit, a position's unit, the unit a unit is part
	// of, the start of a role's description.
	Detail string `json:"detail"`
	// URL is the thing's page in Bearing.
	URL string `json:"url"`
	// PhotoURL is a person's photo, which needs the caller signed in; null without one, and for anything else.
	PhotoURL *string `json:"photoUrl"`
	// Issuer and Subject are how the realm knows a person; null for anything else.
	Issuer  *string  `json:"issuer"`
	Subject *string  `json:"subject"`
	LeftOn  *org.Day `json:"leftOn"`
}

const cardDescription = "asOf is a day written 2006-01-02, today when left out. A card is what a partner shows as a chip: the name, " +
	"one line beneath it and a link to the page in Bearing."

func (s *Server) cardURL(kind string, id uuid.UUID) string {
	return s.AppURL + cardPaths[kind] + id.String()
}

func (s *Server) personCard(r *http.Request, p org.Person, day org.Day) (cardResponse, error) {
	out := cardResponse{Kind: "person", ID: p.ID, Name: p.Name, URL: s.cardURL("person", p.ID), Issuer: p.Issuer, Subject: p.Subject, LeftOn: p.LeftOn}
	if p.HasPhoto {
		photo := s.AppURL + APIPrefix + "/people/" + p.ID.String() + "/photo"
		out.PhotoURL = &photo
	}
	e, err := s.Org.Entry(r.Context(), p, day)
	if err != nil {
		return out, err
	}
	if len(e.Positions) > 0 {
		out.Detail = e.Positions[0].Title + ", " + e.Positions[0].UnitName
	}
	return out, nil
}

// card is one thing's card by its kind and id.
func (s *Server) card(r *http.Request, kind string, id uuid.UUID, day org.Day) (cardResponse, error) {
	ctx := r.Context()
	switch kind {
	case "person":
		p, err := s.Org.Person(ctx, id)
		if err != nil {
			return cardResponse{}, personError(err)
		}
		return s.personCard(r, p, day)
	case "position":
		p, err := s.Org.Position(ctx, id, day)
		if err != nil {
			return cardResponse{}, positionError(err)
		}
		u, err := s.Org.Unit(ctx, p.UnitID, day)
		if err != nil {
			return cardResponse{}, orgError(err)
		}
		return cardResponse{Kind: kind, ID: id, Name: p.Title, Detail: u.Name, URL: s.cardURL(kind, id)}, nil
	case "unit":
		u, err := s.Org.Unit(ctx, id, day)
		if err != nil {
			return cardResponse{}, orgError(err)
		}
		out := cardResponse{Kind: kind, ID: id, Name: u.Name, URL: s.cardURL(kind, id)}
		if u.ParentID != nil {
			parent, err := s.Org.Unit(ctx, *u.ParentID, day)
			if err != nil {
				return cardResponse{}, orgError(err)
			}
			out.Detail = parent.Name
		}
		return out, nil
	case "role":
		role, err := s.Org.Role(ctx, id, day)
		if err != nil {
			return cardResponse{}, roleError(err)
		}
		return cardResponse{Kind: kind, ID: id, Name: role.Name, Detail: clip(role.Description, maxCardDetail), URL: s.cardURL(kind, id)}, nil
	}
	return cardResponse{}, ErrNotFound()
}

func clip(text string, most int) string {
	if r := []rune(text); len(r) > most {
		return strings.TrimSpace(string(r[:most]))
	}
	return text
}

func (s *Server) handleCard(w http.ResponseWriter, r *http.Request) {
	kind := chi.URLParam(r, "kind")
	if !slices.Contains(cardKinds, kind) {
		respondError(w, r, s.Log, ErrNotFound())
		return
	}
	id, ok := s.pathID(w, r, "cardID")
	if !ok {
		return
	}
	day, ok := s.asOf(w, r)
	if !ok {
		return
	}
	c, err := s.card(r, kind, id, day)
	if err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	respondJSON(w, http.StatusOK, env{"asOf": day, "card": c})
}

func (s *Server) handleRealmPersonCard(w http.ResponseWriter, r *http.Request) {
	day, ok := s.asOf(w, r)
	if !ok {
		return
	}
	p, ok := s.realmPerson(w, r)
	if !ok {
		return
	}
	c, err := s.personCard(r, p, day)
	if err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	respondJSON(w, http.StatusOK, env{"asOf": day, "card": c})
}

// handleSuggestCards is the cards whose names hold the words typed so far, best first, of the kinds asked for.
func (s *Server) handleSuggestCards(w http.ResponseWriter, r *http.Request) {
	typed, day, limit, ok := s.searchArgs(w, r, org.DefaultQuickLimit, org.MaxQuickLimit)
	if !ok {
		return
	}
	kinds := cardKinds
	if raw := r.URL.Query().Get("kinds"); raw != "" {
		kinds = nil
		for _, k := range strings.Split(raw, ",") {
			if k = strings.TrimSpace(k); !slices.Contains(cardKinds, k) {
				respondError(w, r, s.Log, ErrValidation(map[string]string{"kinds": "Name kinds among person, position, unit and role."}))
				return
			}
			kinds = append(kinds, k)
		}
	}
	// Processes and steps are found too, so more are asked for than are wanted.
	hits, err := s.Org.Quick(r.Context(), typed, day, org.MaxQuickLimit)
	if err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	out := []cardResponse{}
	for _, h := range hits {
		if len(out) == limit {
			break
		}
		if !slices.Contains(kinds, h.Kind) {
			continue
		}
		c := cardResponse{Kind: h.Kind, ID: h.ID, Name: h.Name, Detail: h.Detail, URL: s.cardURL(h.Kind, h.ID)}
		if h.Kind == "person" || h.Kind == "role" {
			if c, err = s.card(r, h.Kind, h.ID, day); err != nil {
				respondError(w, r, s.Log, err)
				return
			}
		}
		out = append(out, c)
	}
	respondJSON(w, http.StatusOK, env{"asOf": day, "query": typed, "cards": out})
}
