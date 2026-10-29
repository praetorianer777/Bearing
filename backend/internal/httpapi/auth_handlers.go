package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/auth"
)

// maxBody bounds a JSON request; nothing the API takes comes near it.
const maxBody = 1 << 20

type accountResponse struct {
	ID    uuid.UUID `json:"id"`
	Email string    `json:"email"`
	Name  string    `json:"name"`
	// Role is what the account may do: the highest of OwnRole and the roles granted to its groups.
	Role auth.Role `json:"role"`
	// OwnRole is the role granted to the account itself.
	OwnRole     auth.Role `json:"ownRole"`
	Disabled    bool      `json:"disabled"`
	HasPassword bool      `json:"hasPassword"`
	// Groups are what the identity provider last said the person belongs to.
	Groups    []string  `json:"groups"`
	CreatedAt time.Time `json:"createdAt"`
}

func toAccount(a auth.Account) accountResponse {
	groups := a.Groups
	if groups == nil {
		groups = []string{}
	}
	return accountResponse{ID: a.ID, Email: a.Email, Name: a.Name, Role: a.Role, OwnRole: a.OwnRole, Disabled: a.Disabled, HasPassword: a.HasPassword,
		Groups: groups, CreatedAt: a.CreatedAt}
}

type installationResponse struct {
	Name  string `json:"name"`
	SetUp bool   `json:"setUp"`
	// Mail says whether the installation sends mail, so the sign-in page knows whether to offer a reset.
	Mail bool `json:"mail"`
	// SSO says whether people may sign in through the identity provider.
	SSO bool `json:"sso"`
}

type invitationResponse struct {
	ID        uuid.UUID  `json:"id"`
	Email     string     `json:"email"`
	Role      auth.Role  `json:"role"`
	InvitedBy *uuid.UUID `json:"invitedBy"`
	CreatedAt time.Time  `json:"createdAt"`
	ExpiresAt time.Time  `json:"expiresAt"`
}

func toInvitation(in auth.Invitation) invitationResponse {
	return invitationResponse{ID: in.ID, Email: in.Email, Role: in.Role, InvitedBy: in.InvitedBy, CreatedAt: in.CreatedAt, ExpiresAt: in.ExpiresAt}
}

type invitationLinkResponse struct {
	Email string    `json:"email"`
	Role  auth.Role `json:"role"`
}

type setUpRequest struct {
	Name      string `json:"name"`
	AdminName string `json:"adminName"`
	Email     string `json:"email"`
	Password  string `json:"password"`
}

type signInRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type passwordResetRequest struct {
	Email string `json:"email"`
}

type passwordRequest struct {
	Password string `json:"password"`
}

type acceptInvitationRequest struct {
	Name     string `json:"name"`
	Password string `json:"password"`
}

type createAccountRequest struct {
	Name     string    `json:"name"`
	Email    string    `json:"email"`
	Role     auth.Role `json:"role"`
	Password string    `json:"password"`
}

type roleRequest struct {
	Role auth.Role `json:"role"`
}

type profileRequest struct {
	Name string `json:"name"`
}

type inviteRequest struct {
	Email string    `json:"email"`
	Role  auth.Role `json:"role"`
}

// decode reads a JSON body into v, refusing fields v does not have, so a misspelt field is an error and not a
// silent default.
func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return ErrBadRequest("The request body is not the JSON this address takes: " + err.Error())
	}
	return nil
}

// fields collects what a request got wrong, by field, and is nil when nothing is.
type fields map[string]string

func (f fields) need(name, value, message string) {
	if strings.TrimSpace(value) == "" {
		f[name] = message
	}
}

// name needs a name an account can hold.
func (f fields) name(name, value string) {
	f.need(name, value, "Give your name.")
	if utf8.RuneCountInString(strings.TrimSpace(value)) > 200 {
		f[name] = "Keep the name to 200 characters."
	}
}

func (f fields) email(name, value string) string {
	email, err := auth.NormaliseEmail(value)
	if err != nil {
		f[name] = "Give an email address."
	}
	return email
}

func (f fields) password(name, value string) {
	if err := auth.ValidatePassword(value); err != nil {
		f[name] = "Use at least 12 characters."
	}
}

func (f fields) role(name string, value auth.Role) {
	if !value.Valid() {
		f[name] = "Choose administrator, editor, member or reader."
	}
}

func (f fields) err() error {
	if len(f) == 0 {
		return nil
	}
	return ErrValidation(f)
}

// authError says an auth refusal in the API's terms.
func authError(err error) error {
	switch {
	case errors.Is(err, auth.ErrAlreadySetUp):
		return ErrConflict("already_set_up", "This installation is set up already; sign in instead.")
	case errors.Is(err, auth.ErrBadCredentials):
		return &APIError{Status: http.StatusUnauthorized, Code: "bad_credentials", Message: "That email and password do not match an account."}
	case errors.Is(err, auth.ErrDisabled):
		return ErrForbidden("This account is switched off. Ask an administrator.")
	case errors.Is(err, auth.ErrEmailTaken):
		return &APIError{Status: http.StatusConflict, Code: "email_taken", Message: "An account with this email exists already.",
			Fields: map[string]string{"email": "An account with this email exists already."}}
	case errors.Is(err, auth.ErrLinkInvalid):
		return &APIError{Status: http.StatusGone, Code: "link_invalid", Message: "This link is unknown, used or expired. Ask for a new one."}
	case errors.Is(err, auth.ErrNotFound):
		return ErrNotFound()
	case errors.Is(err, auth.ErrOwnRole):
		return ErrConflict("own_role", "You cannot change your own role; another administrator can.")
	case errors.Is(err, auth.ErrNotPermitted):
		return ErrForbidden("Your role does not allow that.")
	case errors.Is(err, auth.ErrInvalidRole):
		return ErrValidation(map[string]string{"role": "Choose administrator, editor, member or reader."})
	case errors.Is(err, auth.ErrSelf):
		return ErrConflict("self", "You cannot switch off your own account.")
	case errors.Is(err, auth.ErrNoPasswordToMail):
		return ErrConflict("no_mail", "This installation sends no mail, so an administrator sets a new password instead.")
	case errors.Is(err, auth.ErrPasswordTooShort):
		return ErrValidation(map[string]string{"password": "Use at least 12 characters."})
	}
	return err
}

func (s *Server) handleInstallation(w http.ResponseWriter, r *http.Request) {
	in, err := s.Auth.Installation(r.Context())
	if err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	respondJSON(w, http.StatusOK, installationResponse{Name: in.Name, SetUp: in.SetUp, Mail: s.MailOn, SSO: s.SSO != nil})
}

func (s *Server) handleSetUp(w http.ResponseWriter, r *http.Request) {
	var req setUpRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	f := fields{}
	f.need("name", req.Name, "Name the organisation.")
	f.need("adminName", req.AdminName, "Give your name.")
	email := f.email("email", req.Email)
	f.password("password", req.Password)
	if err := f.err(); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	if _, err := s.Auth.SetUp(r.Context(), req.Name, req.AdminName, email, req.Password); err != nil {
		respondError(w, r, s.Log, authError(err))
		return
	}
	s.signIn(w, r, email, req.Password, http.StatusCreated)
}

func (s *Server) handleSignIn(w http.ResponseWriter, r *http.Request) {
	var req signInRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	s.signIn(w, r, req.Email, req.Password, http.StatusOK)
}

func (s *Server) signIn(w http.ResponseWriter, r *http.Request, email, password string, status int) {
	a, token, err := s.Auth.SignIn(r.Context(), email, password)
	if err != nil {
		respondError(w, r, s.Log, authError(err))
		return
	}
	s.setSession(w, token)
	respondJSON(w, status, env{"account": toAccount(a)})
}

func (s *Server) handleSignOut(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(s.cookieName()); err == nil {
		if err := s.Auth.SignOut(r.Context(), c.Value); err != nil {
			respondError(w, r, s.Log, err)
			return
		}
	}
	s.clearSession(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, env{"account": toAccount(PrincipalFrom(r.Context()).Account)})
}

func (s *Server) handleUpdateMe(w http.ResponseWriter, r *http.Request) {
	var req profileRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	f := fields{}
	f.name("name", req.Name)
	if err := f.err(); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	a, lsn, err := s.Auth.Rename(r.Context(), req.Name)
	noteWrite(r.Context(), lsn)
	if err != nil {
		respondError(w, r, s.Log, authError(err))
		return
	}
	respondJSON(w, http.StatusOK, env{"account": toAccount(a)})
}

func (s *Server) handleRequestPasswordReset(w http.ResponseWriter, r *http.Request) {
	var req passwordResetRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	if err := s.Auth.RequestPasswordReset(r.Context(), req.Email); err != nil {
		respondError(w, r, s.Log, authError(err))
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	var req passwordRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	if err := s.Auth.ResetPassword(r.Context(), chi.URLParam(r, "token"), req.Password); err != nil {
		respondError(w, r, s.Log, authError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleInvitationLink(w http.ResponseWriter, r *http.Request) {
	in, err := s.Auth.InvitationFor(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		respondError(w, r, s.Log, authError(err))
		return
	}
	respondJSON(w, http.StatusOK, invitationLinkResponse{Email: in.Email, Role: in.Role})
}

func (s *Server) handleAcceptInvitation(w http.ResponseWriter, r *http.Request) {
	var req acceptInvitationRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	f := fields{}
	f.need("name", req.Name, "Give your name.")
	f.password("password", req.Password)
	if err := f.err(); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	a, token, err := s.Auth.AcceptInvitation(r.Context(), chi.URLParam(r, "token"), req.Name, req.Password)
	if err != nil {
		respondError(w, r, s.Log, authError(err))
		return
	}
	s.setSession(w, token)
	respondJSON(w, http.StatusCreated, env{"account": toAccount(a)})
}

func (s *Server) handleAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := s.Auth.Accounts(r.Context())
	if err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	out := make([]accountResponse, len(accounts))
	for i, a := range accounts {
		out[i] = toAccount(a)
	}
	respondJSON(w, http.StatusOK, env{"accounts": out})
}

func (s *Server) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	var req createAccountRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	f := fields{}
	f.need("name", req.Name, "Give a name.")
	email := f.email("email", req.Email)
	f.role("role", req.Role)
	f.password("password", req.Password)
	if err := f.err(); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	a, lsn, err := s.Auth.CreateAccount(r.Context(), req.Name, email, req.Role, req.Password)
	noteWrite(r.Context(), lsn)
	if err != nil {
		respondError(w, r, s.Log, authError(err))
		return
	}
	respondJSON(w, http.StatusCreated, env{"account": toAccount(a)})
}

func (s *Server) accountID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "accountID"))
	if err != nil {
		respondError(w, r, s.Log, ErrNotFound())
		return uuid.Nil, false
	}
	return id, true
}

func (s *Server) handleSetAccountPassword(w http.ResponseWriter, r *http.Request) {
	id, ok := s.accountID(w, r)
	if !ok {
		return
	}
	var req passwordRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	lsn, err := s.Auth.SetPassword(r.Context(), id, req.Password)
	noteWrite(r.Context(), lsn)
	if err != nil {
		respondError(w, r, s.Log, authError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSetAccountRole(w http.ResponseWriter, r *http.Request) {
	id, ok := s.accountID(w, r)
	if !ok {
		return
	}
	var req roleRequest
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
	lsn, err := s.Auth.SetRole(r.Context(), id, req.Role)
	noteWrite(r.Context(), lsn)
	if err != nil {
		respondError(w, r, s.Log, authError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDisableAccount(w http.ResponseWriter, r *http.Request) {
	s.setDisabled(w, r, true)
}
func (s *Server) handleEnableAccount(w http.ResponseWriter, r *http.Request) {
	s.setDisabled(w, r, false)
}

func (s *Server) setDisabled(w http.ResponseWriter, r *http.Request, disabled bool) {
	id, ok := s.accountID(w, r)
	if !ok {
		return
	}
	lsn, err := s.Auth.SetDisabled(r.Context(), id, disabled)
	noteWrite(r.Context(), lsn)
	if err != nil {
		respondError(w, r, s.Log, authError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleInvitations(w http.ResponseWriter, r *http.Request) {
	invitations, err := s.Auth.Invitations(r.Context())
	if err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	out := make([]invitationResponse, len(invitations))
	for i, in := range invitations {
		out[i] = toInvitation(in)
	}
	respondJSON(w, http.StatusOK, env{"invitations": out})
}

func (s *Server) handleInvite(w http.ResponseWriter, r *http.Request) {
	var req inviteRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	f := fields{}
	email := f.email("email", req.Email)
	f.role("role", req.Role)
	if err := f.err(); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	in, link, lsn, err := s.Auth.Invite(r.Context(), email, req.Role)
	noteWrite(r.Context(), lsn)
	if err != nil {
		respondError(w, r, s.Log, authError(err))
		return
	}
	respondJSON(w, http.StatusCreated, env{"invitation": toInvitation(in), "link": link})
}

func (s *Server) handleRevokeInvitation(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "invitationID"))
	if err != nil {
		respondError(w, r, s.Log, ErrNotFound())
		return
	}
	lsn, err := s.Auth.RevokeInvitation(r.Context(), id)
	noteWrite(r.Context(), lsn)
	if err != nil {
		respondError(w, r, s.Log, authError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
