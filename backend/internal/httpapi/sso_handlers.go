package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/auth"
	"github.com/praetorianer777/bearing/backend/internal/sso"
)

// SingleSignOn is the provider's side of a sign-in; sso.Service is the real one.
type SingleSignOn interface {
	Start(ctx context.Context, next string) (string, error)
	Finish(ctx context.Context, state, code string) (sso.Identity, string, error)
}

type joinRequestResponse struct {
	ID          uuid.UUID `json:"id"`
	Email       string    `json:"email"`
	Name        string    `json:"name"`
	Groups      []string  `json:"groups"`
	RequestedAt time.Time `json:"requestedAt"`
}

type admitRequest struct {
	Role auth.Role `json:"role"`
}

type groupAdmission struct {
	Group string    `json:"group"`
	Role  auth.Role `json:"role"`
}

type groupAdmissionsBody struct {
	Admissions []groupAdmission `json:"admissions"`
}

// safeNext is where a sign-in may go back to: a path of the web client's own, never another site.
func safeNext(next string) string {
	if strings.HasPrefix(next, "/") && !strings.HasPrefix(next, "//") && !strings.HasPrefix(next, "/\\") {
		return next
	}
	return "/"
}

func (s *Server) handleSSOStart(w http.ResponseWriter, r *http.Request) {
	if s.SSO == nil {
		respondError(w, r, s.Log, ErrNotFound())
		return
	}
	target, err := s.SSO.Start(r.Context(), safeNext(r.URL.Query().Get("next")))
	if err != nil {
		respondError(w, r, s.Log, ErrUnavailable("The identity provider cannot be reached right now.", err))
		return
	}
	http.Redirect(w, r, target, http.StatusFound)
}

// handleSSOCallback ends a sign-in at the provider. It always sends the browser on: to where it was headed, or back
// to the sign-in page saying what happened, since a person, not a script, is on the other end.
func (s *Server) handleSSOCallback(w http.ResponseWriter, r *http.Request) {
	back := func(outcome string) {
		http.Redirect(w, r, s.AppURL+"/sign-in?sso="+url.QueryEscape(outcome), http.StatusFound)
	}
	if s.SSO == nil {
		back("failed")
		return
	}
	q := r.URL.Query()
	if q.Get("error") != "" {
		back("cancelled")
		return
	}
	id, next, err := s.SSO.Finish(r.Context(), q.Get("state"), q.Get("code"))
	if err != nil {
		s.Log.Warn("single sign-on failed", "error", err)
		back("failed")
		return
	}
	a, token, lsn, err := s.Auth.SignInWithIdentity(r.Context(), auth.Identity{Issuer: id.Issuer, Subject: id.Subject, Email: id.Email, Name: id.Name, Groups: id.Groups})
	switch {
	case errors.Is(err, auth.ErrAwaitingAdmission):
		back("pending")
		return
	case errors.Is(err, auth.ErrDisabled):
		back("disabled")
		return
	case err != nil:
		s.Log.Error("single sign-on could not sign in", "error", err, "email", id.Email)
		back("failed")
		return
	}
	// The sign-in wrote the person's groups; their first reads must not miss them on a replica behind.
	if s.Fresh != nil {
		s.Fresh.Note(r.Context(), "s:"+auth.SessionKey(token), lsn)
	}
	s.setSession(w, token)
	s.Log.Info("signed in through single sign-on", "account", a.ID)
	http.Redirect(w, r, s.AppURL+safeNext(next), http.StatusFound)
}

func (s *Server) handleJoinRequests(w http.ResponseWriter, r *http.Request) {
	requests, err := s.Auth.JoinRequests(r.Context())
	if err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	out := make([]joinRequestResponse, len(requests))
	for i, j := range requests {
		out[i] = joinRequestResponse{ID: j.ID, Email: j.Email, Name: j.Name, Groups: j.Groups, RequestedAt: j.RequestedAt}
	}
	respondJSON(w, http.StatusOK, env{"joinRequests": out})
}

func (s *Server) requestID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "requestID"))
	if err != nil {
		respondError(w, r, s.Log, ErrNotFound())
		return uuid.Nil, false
	}
	return id, true
}

func (s *Server) handleAdmit(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requestID(w, r)
	if !ok {
		return
	}
	var req admitRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	f := fields{}
	f.role("role", req.Role)
	if err := f.err(); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	a, lsn, err := s.Auth.Admit(r.Context(), id, req.Role)
	noteWrite(r.Context(), lsn)
	if err != nil {
		respondError(w, r, s.Log, authError(err))
		return
	}
	respondJSON(w, http.StatusCreated, env{"account": toAccount(a)})
}

func (s *Server) handleDecline(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requestID(w, r)
	if !ok {
		return
	}
	lsn, err := s.Auth.Decline(r.Context(), id)
	noteWrite(r.Context(), lsn)
	if err != nil {
		respondError(w, r, s.Log, authError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGroupAdmissions(w http.ResponseWriter, r *http.Request) {
	admissions, err := s.Auth.GroupAdmissions(r.Context())
	if err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	out := make([]groupAdmission, len(admissions))
	for i, a := range admissions {
		out[i] = groupAdmission{Group: a.Group, Role: a.Role}
	}
	respondJSON(w, http.StatusOK, groupAdmissionsBody{Admissions: out})
}

func (s *Server) handleSetGroupAdmissions(w http.ResponseWriter, r *http.Request) {
	var req groupAdmissionsBody
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	f := fields{}
	admissions := make([]auth.GroupAdmission, 0, len(req.Admissions))
	seen := map[string]bool{}
	for i, a := range req.Admissions {
		group := strings.TrimSpace(a.Group)
		key := "admissions[" + strconv.Itoa(i) + "]"
		switch {
		case group == "":
			f[key+".group"] = "Name the group."
		case seen[group]:
			f[key+".group"] = "This group is listed twice."
		}
		seen[group] = true
		f.role(key+".role", a.Role)
		admissions = append(admissions, auth.GroupAdmission{Group: group, Role: a.Role})
	}
	if err := f.err(); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	lsn, err := s.Auth.SetGroupAdmissions(r.Context(), admissions)
	noteWrite(r.Context(), lsn)
	if err != nil {
		respondError(w, r, s.Log, authError(err))
		return
	}
	respondJSON(w, http.StatusOK, req)
}
