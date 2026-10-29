package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/praetorianer777/bearing/backend/internal/auth"
	"github.com/praetorianer777/bearing/backend/internal/theme"
)

// anybody marks a public operation in mayCall.
const anybody auth.Role = "anybody"

// mayCall is the least role each operation needs, written out apart from the table, so that changing who may call
// something changes this test too. Empty is any role.
var mayCall = map[string]auth.Role{
	"liveness":             anybody,
	"readiness":            anybody,
	"meta":                 anybody,
	"openapi":              anybody,
	"installation":         anybody,
	"setUp":                anybody,
	"signIn":               anybody,
	"signOut":              anybody,
	"startSingleSignOn":    anybody,
	"finishSingleSignOn":   anybody,
	"requestPasswordReset": anybody,
	"resetPassword":        anybody,
	"invitationLink":       anybody,
	"acceptInvitation":     anybody,
	"me":                   "",
	"updateMe":             auth.RoleMember,
	"joinRequests":         auth.RoleAdministrator,
	"admit":                auth.RoleAdministrator,
	"decline":              auth.RoleAdministrator,
	"groupGrants":          auth.RoleAdministrator,
	"setGroupGrants":       auth.RoleAdministrator,
	"accounts":             auth.RoleAdministrator,
	"createAccount":        auth.RoleAdministrator,
	"setAccountPassword":   auth.RoleAdministrator,
	"setAccountRole":       auth.RoleAdministrator,
	"disableAccount":       auth.RoleAdministrator,
	"enableAccount":        auth.RoleAdministrator,
	"invitations":          auth.RoleAdministrator,
	"invite":               auth.RoleAdministrator,
	"revokeInvitation":     auth.RoleAdministrator,
	"themes":               "",
	"exportTheme":          "",
	"themeFile":            "",
	"importTheme":          auth.RoleAdministrator,
	"setDefaultTheme":      auth.RoleAdministrator,
	"deleteTheme":          auth.RoleAdministrator,
}

func TestEveryOperationAgainstEveryRole(t *testing.T) {
	s := newServer(fakeDB{})
	sessions := fakeSessions{}
	for _, role := range auth.Roles {
		sessions[string(role)] = auth.Account{ID: uuid.New(), Name: string(role), Role: role}
	}
	s.Accounts = sessions
	reached := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })

	seen := map[string]bool{}
	for _, op := range operations {
		seen[op.id] = true
		need, ok := mayCall[op.id]
		if !ok {
			t.Errorf("%s is not in mayCall; say who may call it", op.id)
			continue
		}
		h := s.identify(s.guard(op, reached))
		for _, caller := range append([]auth.Role{""}, auth.Roles...) {
			r := httptest.NewRequest(op.method, op.path, nil)
			if caller != "" {
				r.AddCookie(&http.Cookie{Name: s.cookieName(), Value: string(caller)})
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, r)

			want := http.StatusTeapot
			switch {
			case need == anybody:
			case caller == "":
				want = http.StatusUnauthorized
			case need != "" && !caller.AtLeast(need):
				want = http.StatusForbidden
			}
			if rec.Code != want {
				name := string(caller)
				if name == "" {
					name = "nobody"
				}
				t.Errorf("%s as %s: %d, want %d", op.id, name, rec.Code, want)
			}
		}
	}
	for id := range mayCall {
		if !seen[id] {
			t.Errorf("mayCall names %s, which is no operation", id)
		}
	}
}

func TestRolesOrderFromMostToLeast(t *testing.T) {
	for i, r := range auth.Roles {
		for j, other := range auth.Roles {
			if got := r.AtLeast(other); got != (i <= j) {
				t.Errorf("%s at least %s = %v", r, other, got)
			}
		}
	}
	if auth.RoleAdministrator.AtLeast("owner") || auth.Role("owner").AtLeast(auth.RoleReader) {
		t.Error("an unknown role compared as a known one")
	}
}

func TestTheDocumentNamesTheRoleEachOperationNeeds(t *testing.T) {
	doc := Spec()
	if got := doc.Paths["/accounts"]["get"].Role; got != "administrator" {
		t.Errorf("accounts needs %q", got)
	}
	if got := doc.Paths["/auth/me"]["patch"].Role; got != "member" {
		t.Errorf("updateMe needs %q", got)
	}
	if got := doc.Paths["/auth/me"]["get"].Role; got != "" {
		t.Errorf("me needs %q", got)
	}
}

func TestAPolicyRefusalIsForbidden(t *testing.T) {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	respondError(rec, r, newServer(fakeDB{}).Log, fmt.Errorf("renaming: %w", &pgconn.PgError{Code: "42501"}))
	if rec.Code != http.StatusForbidden || code(t, rec) != "forbidden" {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
	if roleNeeded(auth.RoleEditor) != "That needs the editor role or a higher one." {
		t.Errorf("an editor's operation says %q", roleNeeded(auth.RoleEditor))
	}
}

func TestProfilesAndRolesAreCheckedBeforeAnythingIsWritten(t *testing.T) {
	s := signedInServer()
	for _, c := range []struct{ method, path, cookie, body, field string }{
		{http.MethodPatch, "/auth/me", "bert", `{"name":"  "}`, "name"},
		{http.MethodPatch, "/auth/me", "bert", fmt.Sprintf(`{"name":"%0201d"}`, 0), "name"},
		{http.MethodPut, "/accounts/" + bert.ID.String() + "/role", "ada", `{"role":"owner"}`, "role"},
	} {
		rec := call(s, c.method, APIPrefix+c.path, c.cookie, c.body, nil)
		var body errorEnvelope
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if rec.Code != http.StatusUnprocessableEntity || body.Error.Fields[c.field] == "" {
			t.Errorf("%s %s %s: %d %s", c.method, c.path, c.body, rec.Code, rec.Body)
		}
	}
	if rec := call(s, http.MethodPatch, APIPrefix+"/auth/me", "bert", `{"name":"x","role":"administrator"}`, nil); rec.Code != http.StatusBadRequest {
		t.Errorf("a role slipped into a profile: %d", rec.Code)
	}
	if rec := call(s, http.MethodPut, APIPrefix+"/accounts/nope/role", "ada", `{"role":"member"}`, nil); rec.Code != http.StatusNotFound {
		t.Errorf("a malformed account id: %d", rec.Code)
	}
}

func TestThemeRefusalsSayWhatToDo(t *testing.T) {
	for err, want := range map[error]int{
		theme.ErrNotFound:                      http.StatusNotFound,
		theme.ErrDuplicateName:                 http.StatusConflict,
		theme.ErrNotAThemeFile:                 http.StatusUnprocessableEntity,
		theme.Invalid{Err: theme.ErrUnsafeSVG}: http.StatusUnprocessableEntity,
		fmt.Errorf("x: %w", theme.ErrNotFound): http.StatusNotFound,
	} {
		var apiErr *APIError
		if !errors.As(themeError(err), &apiErr) || apiErr.Status != want || !strings.HasSuffix(apiErr.Message, ".") {
			t.Errorf("%v became %v, want status %d", err, themeError(err), want)
		}
	}
	other := errors.New("boom")
	if themeError(other) != other {
		t.Error("an unknown error was dressed up")
	}
	if sentence("") != "" || sentence("done.") != "Done." {
		t.Error("sentence")
	}
}

func TestThemeFileNamesKeepToSafeLetters(t *testing.T) {
	for in, want := range map[string]string{
		"Harbour, from Armature": "harbour-from-armature",
		`"; rm -rf /`:            "rm-rf",
		"Ünïcode":                "n-code",
		"???":                    "theme",
	} {
		if got := fileName(in); got != want {
			t.Errorf("fileName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestThemeAddressesNeedAnID(t *testing.T) {
	s := signedInServer()
	for _, path := range []string{"/themes/nope/export", "/themes/" + uuid.NewString() + "/assets/nope"} {
		if rec := call(s, http.MethodGet, APIPrefix+path, "bert", "", nil); rec.Code != http.StatusNotFound {
			t.Errorf("%s: %d", path, rec.Code)
		}
	}
	if rec := call(s, http.MethodDelete, APIPrefix+"/themes/nope", "ada", "", nil); rec.Code != http.StatusNotFound {
		t.Errorf("delete: %d", rec.Code)
	}
	if rec := call(s, http.MethodPost, APIPrefix+"/themes/import", "ada", `{"format":`, nil); rec.Code != http.StatusBadRequest {
		t.Errorf("a broken file: %d", rec.Code)
	}
	if rec := call(s, http.MethodPut, APIPrefix+"/themes/default", "ada", `{"theme":"x"}`, nil); rec.Code != http.StatusBadRequest {
		t.Errorf("a misspelt default: %d", rec.Code)
	}
}
