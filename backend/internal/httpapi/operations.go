package httpapi

import (
	"net/http"

	"github.com/praetorianer777/bearing/backend/internal/openapi"
)

// env is a response envelope: the one or two keys a handler wraps its result in.
type env map[string]any

// operation is one row of the table the router is built from, and the OpenAPI document after it.
type operation struct {
	method, path string
	// id is what clients call the operation.
	id          string
	handle      func(*Server, http.ResponseWriter, *http.Request)
	summary     string
	description string
	tag         string
	// request is the JSON body type the handler decodes; nil is none.
	request any
	// responses maps a status to its envelope or bare type; nil is no body.
	responses map[int]any
	// public operations need no session; every other needs somebody signed in, and administrator ones an
	// administrator.
	public        bool
	administrator bool
	// root operations are served outside APIPrefix, where a probe expects them.
	root bool
}

// operations is the table. Order is by area, then by path; paths are relative to APIPrefix except the root ones.
var operations = []operation{
	{method: http.MethodGet, path: "/healthz", id: "liveness", handle: (*Server).handleLiveness, tag: "health",
		summary: "Whether the process is running.", public: true, root: true,
		responses: map[int]any{http.StatusOK: statusResponse{}}},
	{method: http.MethodGet, path: "/readyz", id: "readiness", handle: (*Server).handleReadiness, tag: "health",
		summary: "Whether the process can serve, and how reads are routed.", public: true, root: true,
		responses: map[int]any{http.StatusOK: readinessResponse{}, http.StatusServiceUnavailable: errorEnvelope{}}},
	{method: http.MethodGet, path: "/meta", id: "meta", handle: (*Server).handleMeta, tag: "health",
		summary: "Which product and build answers here, and the capabilities it offers partners.", public: true,
		responses: map[int]any{http.StatusOK: metaResponse{}}},
	{method: http.MethodGet, path: "/installation", id: "installation", handle: (*Server).handleInstallation, tag: "accounts",
		summary: "What the installation is called, whether it is set up, and whether it sends mail.", public: true,
		responses: map[int]any{http.StatusOK: installationResponse{}}},
	{method: http.MethodPost, path: "/installation/setup", id: "setUp", handle: (*Server).handleSetUp, tag: "accounts",
		summary: "Name the installation and make its first administrator, who is signed in. Once only.", public: true,
		request: setUpRequest{}, responses: map[int]any{http.StatusCreated: env{"account": accountResponse{}}}},
	{method: http.MethodPost, path: "/auth/sign-in", id: "signIn", handle: (*Server).handleSignIn, tag: "accounts",
		summary: "Sign in with an email and a password; the session is a cookie.", public: true,
		request: signInRequest{}, responses: map[int]any{http.StatusOK: env{"account": accountResponse{}}}},
	{method: http.MethodPost, path: "/auth/sign-out", id: "signOut", handle: (*Server).handleSignOut, tag: "accounts",
		summary: "End this session.", public: true, responses: map[int]any{http.StatusNoContent: nil}},
	{method: http.MethodGet, path: "/auth/me", id: "me", handle: (*Server).handleMe, tag: "accounts",
		summary: "Who is signed in.", responses: map[int]any{http.StatusOK: env{"account": accountResponse{}}}},
	{method: http.MethodPost, path: "/auth/password-reset", id: "requestPasswordReset", handle: (*Server).handleRequestPasswordReset, tag: "accounts",
		summary: "Mail a link to choose a new password. The answer is the same whether the address has an account or not.", public: true,
		request: passwordResetRequest{}, responses: map[int]any{http.StatusAccepted: nil}},
	{method: http.MethodPost, path: "/auth/password-reset/{token}", id: "resetPassword", handle: (*Server).handleResetPassword, tag: "accounts",
		summary: "Choose a new password with a mailed link; every session of the account ends.", public: true,
		request: passwordRequest{}, responses: map[int]any{http.StatusNoContent: nil}},
	{method: http.MethodGet, path: "/invitation-links/{token}", id: "invitationLink", handle: (*Server).handleInvitationLink, tag: "accounts",
		summary: "The address and role an invitation link brings in.", public: true,
		responses: map[int]any{http.StatusOK: invitationLinkResponse{}}},
	{method: http.MethodPost, path: "/invitation-links/{token}", id: "acceptInvitation", handle: (*Server).handleAcceptInvitation, tag: "accounts",
		summary: "Accept an invitation with a name and a password, and be signed in.", public: true,
		request: acceptInvitationRequest{}, responses: map[int]any{http.StatusCreated: env{"account": accountResponse{}}}},
	{method: http.MethodGet, path: "/accounts", id: "accounts", handle: (*Server).handleAccounts, tag: "accounts",
		summary: "Every account.", administrator: true, responses: map[int]any{http.StatusOK: env{"accounts": []accountResponse{}}}},
	{method: http.MethodPost, path: "/accounts", id: "createAccount", handle: (*Server).handleCreateAccount, tag: "accounts",
		summary: "Make a local account with a password.", administrator: true,
		request: createAccountRequest{}, responses: map[int]any{http.StatusCreated: env{"account": accountResponse{}}}},
	{method: http.MethodPost, path: "/accounts/{accountID}/password", id: "setAccountPassword", handle: (*Server).handleSetAccountPassword, tag: "accounts",
		summary: "Give an account a new password; its sessions end.", administrator: true,
		request: passwordRequest{}, responses: map[int]any{http.StatusNoContent: nil}},
	{method: http.MethodPost, path: "/accounts/{accountID}/disable", id: "disableAccount", handle: (*Server).handleDisableAccount, tag: "accounts",
		summary: "Switch an account off; its sessions end.", administrator: true, responses: map[int]any{http.StatusNoContent: nil}},
	{method: http.MethodPost, path: "/accounts/{accountID}/enable", id: "enableAccount", handle: (*Server).handleEnableAccount, tag: "accounts",
		summary: "Switch an account back on.", administrator: true, responses: map[int]any{http.StatusNoContent: nil}},
	{method: http.MethodGet, path: "/invitations", id: "invitations", handle: (*Server).handleInvitations, tag: "accounts",
		summary: "The open invitations.", administrator: true, responses: map[int]any{http.StatusOK: env{"invitations": []invitationResponse{}}}},
	{method: http.MethodPost, path: "/invitations", id: "invite", handle: (*Server).handleInvite, tag: "accounts",
		summary: "Invite an address with a role. The answer carries the link, which is also mailed when the installation sends mail.", administrator: true,
		request: inviteRequest{}, responses: map[int]any{http.StatusCreated: env{"invitation": invitationResponse{}, "link": ""}}},
	{method: http.MethodDelete, path: "/invitations/{invitationID}", id: "revokeInvitation", handle: (*Server).handleRevokeInvitation, tag: "accounts",
		summary: "Take back an open invitation.", administrator: true, responses: map[int]any{http.StatusNoContent: nil}},
}

// The document's own row joins at init: it is built from the table, so the table cannot name it in its initialiser.
func init() {
	operations = append(operations, operation{method: http.MethodGet, path: "/openapi.json", id: "openapi",
		handle: (*Server).handleOpenAPI, tag: "health", summary: "This document.", public: true,
		responses: map[int]any{http.StatusOK: openapi.Document{}}})
}
