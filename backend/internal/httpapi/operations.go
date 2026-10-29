package httpapi

import (
	"net/http"

	"github.com/praetorianer777/bearing/backend/internal/auth"
	"github.com/praetorianer777/bearing/backend/internal/openapi"
	"github.com/praetorianer777/bearing/backend/internal/theme"
)

const (
	administrator = auth.RoleAdministrator
	member        = auth.RoleMember
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
	// query names the query parameters the handler reads, each a string.
	query []string
	// responses maps a status to its envelope or bare type; nil is no body.
	responses map[int]any
	// public operations need no session; every other needs somebody signed in who holds role or a higher one, any
	// role when it is empty.
	public bool
	role   auth.Role
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
	{method: http.MethodGet, path: "/auth/oidc/start", id: "startSingleSignOn", handle: (*Server).handleSSOStart, tag: "accounts",
		summary: "Send the browser to the identity provider; next is where it comes back to.", public: true,
		query:     []string{"next"},
		responses: map[int]any{http.StatusFound: nil}},
	{method: http.MethodGet, path: "/auth/oidc/callback", id: "finishSingleSignOn", handle: (*Server).handleSSOCallback, tag: "accounts",
		summary: "Where the identity provider sends the browser back. It goes on to the web client: where it was headed, or the sign-in page with sso set to pending, disabled, cancelled or failed.",
		public:  true, query: []string{"state", "code", "error"}, responses: map[int]any{http.StatusFound: nil}},
	{method: http.MethodGet, path: "/join-requests", id: "joinRequests", handle: (*Server).handleJoinRequests, tag: "accounts",
		summary: "Who the identity provider vouches for and asked to join.", role: administrator,
		responses: map[int]any{http.StatusOK: env{"joinRequests": []joinRequestResponse{}}}},
	{method: http.MethodPost, path: "/join-requests/{requestID}/admit", id: "admit", handle: (*Server).handleAdmit, tag: "accounts",
		summary: "Let somebody in who asked to join, with a role.", role: administrator,
		request: admitRequest{}, responses: map[int]any{http.StatusCreated: env{"account": accountResponse{}}}},
	{method: http.MethodDelete, path: "/join-requests/{requestID}", id: "decline", handle: (*Server).handleDecline, tag: "accounts",
		summary: "Turn a request to join down.", role: administrator, responses: map[int]any{http.StatusNoContent: nil}},
	{method: http.MethodGet, path: "/group-grants", id: "groupGrants", handle: (*Server).handleGroupGrants, tag: "accounts",
		summary: "The identity provider's groups whose members hold a role.", role: administrator,
		responses: map[int]any{http.StatusOK: groupGrantsBody{}}},
	{method: http.MethodPut, path: "/group-grants", id: "setGroupGrants", handle: (*Server).handleSetGroupGrants, tag: "accounts",
		summary:     "Replace the group grants. Members of a granted group come in at once and hold its role while they are in it; somebody in several holds the highest.",
		description: "Groups are what the identity provider names at sign-in, so a change to somebody's groups there takes effect at their next sign-in.",
		role:        administrator,
		request:     groupGrantsBody{}, responses: map[int]any{http.StatusOK: groupGrantsBody{}}},
	{method: http.MethodGet, path: "/auth/me", id: "me", handle: (*Server).handleMe, tag: "accounts",
		summary: "Who is signed in.", responses: map[int]any{http.StatusOK: env{"account": accountResponse{}}}},
	{method: http.MethodPatch, path: "/auth/me", id: "updateMe", handle: (*Server).handleUpdateMe, tag: "accounts",
		summary: "Change your own profile.", role: member,
		request: profileRequest{}, responses: map[int]any{http.StatusOK: env{"account": accountResponse{}}}},
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
		summary: "Every account.", role: administrator, responses: map[int]any{http.StatusOK: env{"accounts": []accountResponse{}}}},
	{method: http.MethodPost, path: "/accounts", id: "createAccount", handle: (*Server).handleCreateAccount, tag: "accounts",
		summary: "Make a local account with a password.", role: administrator,
		request: createAccountRequest{}, responses: map[int]any{http.StatusCreated: env{"account": accountResponse{}}}},
	{method: http.MethodPost, path: "/accounts/{accountID}/password", id: "setAccountPassword", handle: (*Server).handleSetAccountPassword, tag: "accounts",
		summary: "Give an account a new password; its sessions end.", role: administrator,
		request: passwordRequest{}, responses: map[int]any{http.StatusNoContent: nil}},
	{method: http.MethodPut, path: "/accounts/{accountID}/role", id: "setAccountRole", handle: (*Server).handleSetAccountRole, tag: "accounts",
		summary: "Grant an account a role of its own. Its groups may grant it a higher one; nobody changes their own.", role: administrator,
		request: roleRequest{}, responses: map[int]any{http.StatusNoContent: nil}},
	{method: http.MethodPost, path: "/accounts/{accountID}/disable", id: "disableAccount", handle: (*Server).handleDisableAccount, tag: "accounts",
		summary: "Switch an account off; its sessions end.", role: administrator, responses: map[int]any{http.StatusNoContent: nil}},
	{method: http.MethodPost, path: "/accounts/{accountID}/enable", id: "enableAccount", handle: (*Server).handleEnableAccount, tag: "accounts",
		summary: "Switch an account back on.", role: administrator, responses: map[int]any{http.StatusNoContent: nil}},
	{method: http.MethodGet, path: "/invitations", id: "invitations", handle: (*Server).handleInvitations, tag: "accounts",
		summary: "The open invitations.", role: administrator, responses: map[int]any{http.StatusOK: env{"invitations": []invitationResponse{}}}},
	{method: http.MethodPost, path: "/invitations", id: "invite", handle: (*Server).handleInvite, tag: "accounts",
		summary: "Invite an address with a role. The answer carries the link, which is also mailed when the installation sends mail.", role: administrator,
		request: inviteRequest{}, responses: map[int]any{http.StatusCreated: env{"invitation": invitationResponse{}, "link": ""}}},
	{method: http.MethodDelete, path: "/invitations/{invitationID}", id: "revokeInvitation", handle: (*Server).handleRevokeInvitation, tag: "accounts",
		summary: "Take back an open invitation.", role: administrator, responses: map[int]any{http.StatusNoContent: nil}},
	{method: http.MethodGet, path: "/themes", id: "themes", handle: (*Server).handleThemes, tag: "themes",
		summary:   "The installation's themes, to choose from, with the one everybody starts from marked.",
		responses: map[int]any{http.StatusOK: env{"themes": []themeResponse{}}}},
	{method: http.MethodPost, path: "/themes/import", id: "importTheme", handle: (*Server).handleImportTheme, tag: "themes",
		summary:     "Add a theme from an armature-theme/1 file, as Armature, Exciter, Stator and Bearing export it.",
		description: "The files get new ids, which the theme is rewritten to name. What each file is comes from its bytes. A taken name gets a number.",
		role:        administrator, request: theme.Package{}, responses: map[int]any{http.StatusCreated: env{"theme": themeResponse{}}}},
	{method: http.MethodPut, path: "/themes/default", id: "setDefaultTheme", handle: (*Server).handleSetDefaultTheme, tag: "themes",
		summary: "Name the theme everybody starts from; null is the built-in look.", role: administrator,
		request: defaultThemeRequest{}, responses: map[int]any{http.StatusNoContent: nil}},
	{method: http.MethodDelete, path: "/themes/{themeID}", id: "deleteTheme", handle: (*Server).handleDeleteTheme, tag: "themes",
		summary: "Remove a theme and its files. Whoever had chosen it falls back to the installation's.", role: administrator,
		responses: map[int]any{http.StatusNoContent: nil}},
	{method: http.MethodGet, path: "/themes/{themeID}/export", id: "exportTheme", handle: (*Server).handleExportTheme, tag: "themes",
		summary: "A theme as one armature-theme/1 file, its files inline.", responses: map[int]any{http.StatusOK: theme.Package{}}},
	{method: http.MethodGet, path: "/themes/{themeID}/assets/{assetID}", id: "themeFile", handle: (*Server).handleThemeFile, tag: "themes",
		summary: "One of a theme's files, as a download that never changes.", responses: map[int]any{http.StatusOK: nil}},
}

// The document's own row joins at init: it is built from the table, so the table cannot name it in its initialiser.
func init() {
	operations = append(operations, operation{method: http.MethodGet, path: "/openapi.json", id: "openapi",
		handle: (*Server).handleOpenAPI, tag: "health", summary: "This document.", public: true,
		responses: map[int]any{http.StatusOK: openapi.Document{}}})
}
