package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/praetorianer777/bearing/backend/internal/auth"
	"github.com/praetorianer777/bearing/backend/internal/org"
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
	"createTheme":          "",
	"activeTheme":          "",
	"chooseTheme":          "",
	"defaultTheme":         "",
	"setDefaultTheme":      auth.RoleAdministrator,
	"themeExamples":        "",
	"importTheme":          "",
	"theme":                "",
	"updateTheme":          "",
	"deleteTheme":          "",
	"addThemeAsset":        "",
	"themeFile":            "",
	"removeThemeAsset":     "",
	"exportTheme":          "",
	"unitTypes":            "",
	"createUnitType":       auth.RoleAdministrator,
	"updateUnitType":       auth.RoleAdministrator,
	"deleteUnitType":       auth.RoleAdministrator,
	"units":                "",
	"unit":                 "",
	"createUnit":           auth.RoleEditor,
	"updateUnit":           auth.RoleEditor,
	"moveUnit":             auth.RoleEditor,
	"archiveUnit":          auth.RoleEditor,
	"restoreUnit":          auth.RoleEditor,
	"positions":            "",
	"position":             "",
	"createPosition":       auth.RoleEditor,
	"updatePosition":       auth.RoleEditor,
	"setReportsTo":         auth.RoleEditor,
	"addDottedLine":        auth.RoleEditor,
	"endDottedLine":        auth.RoleEditor,
	"archivePosition":      auth.RoleEditor,
	"restorePosition":      auth.RoleEditor,
	"people":               "",
	"person":               "",
	"myEntry":              "",
	"photo":                "",
	"selfEditable":         "",
	"createPerson":         auth.RoleEditor,
	"updatePerson":         auth.RoleEditor,
	"leave":                auth.RoleEditor,
	"return":               auth.RoleEditor,
	"setPhoto":             auth.RoleEditor,
	"deletePhoto":          auth.RoleEditor,
	"updateMyEntry":        auth.RoleMember,
	"setMyPhoto":           auth.RoleMember,
	"deleteMyPhoto":        auth.RoleMember,
	"linkAccount":          auth.RoleAdministrator,
	"setSelfEditable":      auth.RoleAdministrator,
	"holdings":             "",
	"holding":              "",
	"giveHolding":          auth.RoleEditor,
	"updateHolding":        auth.RoleEditor,
	"endHolding":           auth.RoleEditor,
	"removeHolding":        auth.RoleEditor,
	"roles":                "",
	"role":                 "",
	"createRole":           auth.RoleEditor,
	"updateRole":           auth.RoleEditor,
	"deleteRole":           auth.RoleEditor,
	"carryRole":            auth.RoleEditor,
	"dropRole":             auth.RoleEditor,
	"setDeputies":          auth.RoleEditor,
	"members":              "",
	"positionChain":        "",
	"positionReports":      "",
	"personChain":          "",
	"personEntry":          "",
	"personManagers":       "",
	"realmPerson":          "",
	"realmPersonChain":     "",
	"realmPersonReports":   "",
	"realmPersonManagers":  "",
	"rolePeople":           "",
	"unitTree":             "",
	"personReports":        "",
	"audit":                auth.RoleEditor,
	"unitAudit":            auth.RoleEditor,
	"positionAudit":        auth.RoleEditor,
	"personAudit":          auth.RoleEditor,
	"roleAudit":            auth.RoleEditor,
	"processes":            "",
	"createProcess":        auth.RoleEditor,
	"process":              "",
	"updateProcess":        auth.RoleEditor,
	"deleteProcess":        auth.RoleEditor,
	"processAudit":         auth.RoleEditor,
	"saveDraft":            auth.RoleEditor,
	"publish":              auth.RoleEditor,
	"processVersion":       "",
	"processPeople":        "",
	"stepPeople":           "",
	"duties":               "",
	"whoDoes":              "",
	"search":               "",
	"exportCSV":            auth.RoleEditor,
	"previewImport":        auth.RoleEditor,
	"import":               auth.RoleEditor,
	"runImport":            auth.RoleEditor,
	"unitCapacity":         "",
	"positionCapacity":     "",
	"roleCapacity":         "",
	"personCapacity":       "",
	"realmPersonCapacity":  "",
	"targets":              "",
	"gaps":                 "",
	"setTarget":            auth.RoleEditor,
	"removeTarget":         auth.RoleEditor,
	"quickSearch":          "",
	"demo":                 auth.RoleAdministrator,
	"loadDemo":             auth.RoleAdministrator,
	"oauthMetadata":        anybody,
	"resourceMetadata":     anybody,
	"mcpResourceMetadata":  anybody,
	"registerApp":          anybody,
	"authorize":            anybody,
	"consent":              "",
	"decide":               "",
	"token":                anybody,
	"revoke":               anybody,
	"connectedApps":        "",
	"disconnect":           "",
	"apps":                 auth.RoleAdministrator,
	"removeApp":            auth.RoleAdministrator,
	"registerPartner":      auth.RoleAdministrator,
	"mcp":                  "",
	"personalTokens":       "",
	"makePersonalToken":    "",
	"revokePersonalToken":  "",
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

func TestUnitRefusalsSayWhatToDo(t *testing.T) {
	for err, want := range map[error]int{
		org.Invalid{Field: "name", Message: "Give a name."}: http.StatusUnprocessableEntity,
		org.ErrNotFound:       http.StatusNotFound,
		org.ErrTypeNotFound:   http.StatusUnprocessableEntity,
		org.ErrTypeNameTaken:  http.StatusConflict,
		org.ErrTypeInUse:      http.StatusConflict,
		org.ErrCycle:          http.StatusConflict,
		org.ErrRootTaken:      http.StatusConflict,
		org.ErrHasUnits:       http.StatusConflict,
		org.ErrParentArchived: http.StatusConflict,
	} {
		var apiErr *APIError
		if !errors.As(orgError(err), &apiErr) || apiErr.Status != want || apiErr.Message == "" {
			t.Errorf("%v became %v, want status %d", err, orgError(err), want)
		}
	}
	other := errors.New("boom")
	if orgError(other) != other {
		t.Error("an unknown error was dressed up")
	}
}

func TestHoldingRefusalsSayWhatToDo(t *testing.T) {
	for err, want := range map[error]int{
		org.ErrHoldingNotFound:   http.StatusNotFound,
		org.ErrPersonNotFound:    http.StatusUnprocessableEntity,
		org.ErrPositionNotFound:  http.StatusUnprocessableEntity,
		org.ErrPositionNotThere:  http.StatusConflict,
		org.ErrPersonNotThere:    http.StatusConflict,
		org.ErrHoldingOverlaps:   http.StatusConflict,
		org.ErrSeatsTaken:        http.StatusConflict,
		org.ErrHoldingNotStarted: http.StatusConflict,
		org.ErrHoldingEnded:      http.StatusConflict,
		org.ErrTooFewSeats:       http.StatusConflict,
	} {
		refusal := holdingError(err)
		if err == org.ErrTooFewSeats {
			refusal = positionError(err)
		}
		var apiErr *APIError
		if !errors.As(refusal, &apiErr) || apiErr.Status != want || apiErr.Message == "" {
			t.Errorf("%v became %v, want status %d", err, refusal, want)
		}
	}
	h := org.Holding{ID: uuid.New(), Share: 50}
	p := toPosition(org.Position{Seats: 2, Holders: []org.Holding{h}})
	if len(p.Holders) != 1 || p.Holders[0].ID != h.ID || p.Holders[0].Share != 50 || p.Seats != 2 {
		t.Errorf("the position's holders are %+v", p.Holders)
	}
	if empty := toPosition(org.Position{}); empty.Holders == nil {
		t.Error("a vacant position's holders are null, not empty")
	}
}

func TestRoleRefusalsSayWhatToDo(t *testing.T) {
	for err, want := range map[error]int{
		org.ErrRoleNotFound: http.StatusNotFound,
		org.ErrRoleKeyTaken: http.StatusConflict,
		org.ErrRoleInUse:    http.StatusConflict,
		org.Invalid{Field: "key", Message: "Give a key."}: http.StatusUnprocessableEntity,
	} {
		var apiErr *APIError
		if !errors.As(roleError(err), &apiErr) || apiErr.Status != want {
			t.Errorf("%v became %v, want status %d", err, roleError(err), want)
		}
	}
	for err, want := range map[error]int{
		org.ErrRoleNotFound:     http.StatusUnprocessableEntity,
		org.ErrRoleCarried:      http.StatusConflict,
		org.ErrRoleNotCarried:   http.StatusConflict,
		org.ErrDeputyNotThere:   http.StatusUnprocessableEntity,
		org.ErrPositionNotFound: http.StatusNotFound,
	} {
		var apiErr *APIError
		if !errors.As(carriedRoleError(err), &apiErr) || apiErr.Status != want {
			t.Errorf("%v became %v, want status %d", err, carriedRoleError(err), want)
		}
	}
	for _, err := range []error{org.ErrRoleNotFound, org.ErrHoldingWhat} {
		var apiErr *APIError
		if !errors.As(holdingError(err), &apiErr) || apiErr.Status != http.StatusUnprocessableEntity {
			t.Errorf("%v became %v", err, holdingError(err))
		}
	}
	through := uuid.New()
	r := toRole(org.Role{Key: "approver", Positions: []uuid.UUID{through}, Holders: []org.RoleHolder{{Share: 20, Through: &through}}})
	if r.Key != "approver" || len(r.Holders) != 1 || *r.Holders[0].Through != through || r.Holders[0].Share != 20 {
		t.Errorf("the role reads %+v", r)
	}
	if p := toPosition(org.Position{}); p.Roles == nil || p.Deputies == nil {
		t.Error("a position without roles or deputies has null lists")
	}
}

func TestUnitRequestsAreCheckedBeforeTheModelIsAsked(t *testing.T) {
	s := signedInServer()
	editorSessions := s.Accounts.(fakeSessions)
	editorSessions["eve"] = auth.Account{ID: uuid.New(), Name: "Eve", Role: auth.RoleEditor}
	for _, c := range []struct {
		method, path, cookie, body string
		want                       int
	}{
		{http.MethodGet, "/units?asOf=someday", "bert", "", http.StatusUnprocessableEntity},
		{http.MethodGet, "/units/nope", "bert", "", http.StatusNotFound},
		{http.MethodPatch, "/units/nope", "eve", `{}`, http.StatusNotFound},
		{http.MethodPost, "/units/nope/move", "eve", `{}`, http.StatusNotFound},
		{http.MethodPost, "/units/nope/archive", "eve", `{}`, http.StatusNotFound},
		{http.MethodPost, "/units/nope/restore", "eve", ``, http.StatusNotFound},
		{http.MethodPatch, "/unit-types/nope", "ada", `{}`, http.StatusNotFound},
		{http.MethodDelete, "/unit-types/nope", "ada", ``, http.StatusNotFound},
		{http.MethodPost, "/units", "eve", `{"from":"soon"}`, http.StatusBadRequest},
		{http.MethodPost, "/units/" + uuid.NewString() + "/move", "eve", `{"parent":"x"}`, http.StatusBadRequest},
		{http.MethodPost, "/units/" + uuid.NewString() + "/archive", "eve", `{"on":5}`, http.StatusBadRequest},
		{http.MethodPatch, "/units/" + uuid.NewString(), "eve", `{"nam":"x"}`, http.StatusBadRequest},
		{http.MethodPost, "/unit-types", "ada", `{"colour":`, http.StatusBadRequest},
		{http.MethodPatch, "/unit-types/" + uuid.NewString(), "ada", `[]`, http.StatusBadRequest},
	} {
		if rec := call(s, c.method, APIPrefix+c.path, c.cookie, c.body, nil); rec.Code != c.want {
			t.Errorf("%s %s %s: %d, want %d", c.method, c.path, c.body, rec.Code, c.want)
		}
	}
	if d := dayOr(nil); d != today() {
		t.Errorf("no day is %s, want today", d)
	}
}

func TestPositionRequestsAreCheckedBeforeTheModelIsAsked(t *testing.T) {
	s := signedInServer()
	s.Accounts.(fakeSessions)["eve"] = auth.Account{ID: uuid.New(), Name: "Eve", Role: auth.RoleEditor}
	id := uuid.NewString()
	for _, c := range []struct {
		method, path, body string
		want               int
	}{
		{http.MethodGet, "/positions?unitId=nope", "", http.StatusUnprocessableEntity},
		{http.MethodGet, "/positions?asOf=nope", "", http.StatusUnprocessableEntity},
		{http.MethodGet, "/positions/nope", "", http.StatusNotFound},
		{http.MethodGet, "/positions/" + id + "?asOf=x", "", http.StatusUnprocessableEntity},
		{http.MethodPatch, "/positions/nope", `{}`, http.StatusNotFound},
		{http.MethodPut, "/positions/nope/reports-to", `{}`, http.StatusNotFound},
		{http.MethodPost, "/positions/nope/dotted-lines", `{}`, http.StatusNotFound},
		{http.MethodPost, "/positions/" + id + "/dotted-lines/nope/end", `{}`, http.StatusNotFound},
		{http.MethodPost, "/positions/nope/dotted-lines/" + id + "/end", `{}`, http.StatusNotFound},
		{http.MethodPost, "/positions/nope/archive", `{}`, http.StatusNotFound},
		{http.MethodPost, "/positions/nope/restore", ``, http.StatusNotFound},
		{http.MethodPost, "/positions", `{"unit":"x"}`, http.StatusBadRequest},
		{http.MethodPatch, "/positions/" + id, `{"head":true}`, http.StatusBadRequest},
		{http.MethodPut, "/positions/" + id + "/reports-to", `{"to":"x"}`, http.StatusBadRequest},
		{http.MethodPost, "/positions/" + id + "/dotted-lines", `{"from":"soon"}`, http.StatusBadRequest},
		{http.MethodPost, "/positions/" + id + "/dotted-lines/" + id + "/end", `{"on":1}`, http.StatusBadRequest},
		{http.MethodPost, "/positions/" + id + "/archive", `{"on":1}`, http.StatusBadRequest},
	} {
		if rec := call(s, c.method, APIPrefix+c.path, "eve", c.body, nil); rec.Code != c.want {
			t.Errorf("%s %s %s: %d, want %d", c.method, c.path, c.body, rec.Code, c.want)
		}
	}
	for err, want := range map[error]int{
		org.ErrPositionNotFound: http.StatusNotFound,
		org.ErrUnitNotThere:     http.StatusUnprocessableEntity,
		org.ErrHasReports:       http.StatusConflict,
		org.ErrReportsToItself:  http.StatusConflict,
		org.ErrHasPositions:     http.StatusConflict,
	} {
		var apiErr *APIError
		if !errors.As(positionError(err), &apiErr) || apiErr.Status != want {
			t.Errorf("%v became %v, want status %d", err, positionError(err), want)
		}
	}
}

func TestPeopleRequestsAreCheckedBeforeTheModelIsAsked(t *testing.T) {
	s := signedInServer()
	s.Accounts.(fakeSessions)["eve"] = auth.Account{ID: uuid.New(), Name: "Eve", Role: auth.RoleEditor}
	for _, c := range []struct {
		method, path, cookie, body string
		want                       int
	}{
		{http.MethodGet, "/people?asOf=x", "bert", "", http.StatusUnprocessableEntity},
		{http.MethodGet, "/people/nope", "bert", "", http.StatusNotFound},
		{http.MethodGet, "/holdings", "bert", "", http.StatusUnprocessableEntity},
		{http.MethodGet, "/holdings?personId=x", "bert", "", http.StatusUnprocessableEntity},
		{http.MethodGet, "/holdings?positionId=" + uuid.NewString() + "&asOf=x", "bert", "", http.StatusUnprocessableEntity},
		{http.MethodGet, "/holdings/nope", "bert", "", http.StatusNotFound},
		{http.MethodGet, "/holdings?roleId=x", "bert", "", http.StatusUnprocessableEntity},
		{http.MethodGet, "/roles/nope", "bert", "", http.StatusNotFound},
		{http.MethodGet, "/people/" + uuid.NewString() + "?asOf=x", "bert", "", http.StatusUnprocessableEntity},
		{http.MethodGet, "/people/me?asOf=x", "bert", "", http.StatusUnprocessableEntity},
		{http.MethodGet, "/audit?before=x", "eve", "", http.StatusUnprocessableEntity},
		{http.MethodGet, "/audit?limit=x", "eve", "", http.StatusUnprocessableEntity},
		{http.MethodGet, "/audit?limit=500", "eve", "", http.StatusUnprocessableEntity},
		{http.MethodGet, "/units/nope/audit", "eve", "", http.StatusNotFound},
		{http.MethodGet, "/positions/nope/audit", "eve", "", http.StatusNotFound},
		{http.MethodGet, "/people/nope/audit", "eve", "", http.StatusNotFound},
		{http.MethodGet, "/roles/nope/audit", "eve", "", http.StatusNotFound},
		{http.MethodGet, "/units/nope/members", "bert", "", http.StatusNotFound},
		{http.MethodGet, "/units/" + uuid.NewString() + "/members?asOf=x", "bert", "", http.StatusUnprocessableEntity},
		{http.MethodGet, "/positions/nope/chain", "bert", "", http.StatusNotFound},
		{http.MethodGet, "/positions/" + uuid.NewString() + "/reports?asOf=x", "bert", "", http.StatusUnprocessableEntity},
		{http.MethodGet, "/people/nope/chain", "bert", "", http.StatusNotFound},
		{http.MethodGet, "/people/" + uuid.NewString() + "/reports?asOf=x", "bert", "", http.StatusUnprocessableEntity},
		{http.MethodGet, "/roles/" + uuid.NewString() + "?asOf=x", "bert", "", http.StatusUnprocessableEntity},
		{http.MethodPatch, "/roles/nope", "eve", `{}`, http.StatusNotFound},
		{http.MethodDelete, "/roles/nope", "eve", ``, http.StatusNotFound},
		{http.MethodPost, "/roles", "eve", `{"title":"x"}`, http.StatusBadRequest},
		{http.MethodPatch, "/roles/" + uuid.NewString(), "eve", `{"key":3}`, http.StatusBadRequest},
		{http.MethodPost, "/positions/nope/roles", "eve", `{}`, http.StatusNotFound},
		{http.MethodPost, "/positions/" + uuid.NewString() + "/roles", "eve", `{"role":"x"}`, http.StatusBadRequest},
		{http.MethodPost, "/positions/nope/roles/" + uuid.NewString() + "/end", "eve", `{}`, http.StatusNotFound},
		{http.MethodPost, "/positions/" + uuid.NewString() + "/roles/nope/end", "eve", `{}`, http.StatusNotFound},
		{http.MethodPost, "/positions/" + uuid.NewString() + "/roles/" + uuid.NewString() + "/end", "eve", `{"on":1}`, http.StatusBadRequest},
		{http.MethodPut, "/positions/nope/deputies", "eve", `{}`, http.StatusNotFound},
		{http.MethodPut, "/positions/" + uuid.NewString() + "/deputies", "eve", `{"deputies":[]}`, http.StatusBadRequest},
		{http.MethodPatch, "/holdings/nope", "eve", `{}`, http.StatusNotFound},
		{http.MethodPost, "/holdings/nope/end", "eve", `{}`, http.StatusNotFound},
		{http.MethodDelete, "/holdings/nope", "eve", ``, http.StatusNotFound},
		{http.MethodPost, "/holdings", "eve", `{"person":"x"}`, http.StatusBadRequest},
		{http.MethodPatch, "/holdings/" + uuid.NewString(), "eve", `{"share":"half"}`, http.StatusBadRequest},
		{http.MethodPost, "/holdings/" + uuid.NewString() + "/end", "eve", `{"on":"x"}`, http.StatusBadRequest},
		{http.MethodGet, "/people/nope/photo", "bert", "", http.StatusNotFound},
		{http.MethodPatch, "/people/nope", "eve", `{}`, http.StatusNotFound},
		{http.MethodPut, "/people/nope/account", "ada", `{}`, http.StatusNotFound},
		{http.MethodPost, "/people/nope/leave", "eve", `{}`, http.StatusNotFound},
		{http.MethodPost, "/people/nope/return", "eve", ``, http.StatusNotFound},
		{http.MethodPut, "/people/nope/photo", "eve", `{}`, http.StatusNotFound},
		{http.MethodDelete, "/people/nope/photo", "eve", ``, http.StatusNotFound},
		{http.MethodPost, "/people", "eve", `{"nme":"x"}`, http.StatusBadRequest},
		{http.MethodPatch, "/people/me", "bert", `{"left":true}`, http.StatusBadRequest},
		{http.MethodPatch, "/people/" + uuid.NewString(), "eve", `[]`, http.StatusBadRequest},
		{http.MethodPut, "/people/" + uuid.NewString() + "/account", "ada", `{"account":"x"}`, http.StatusBadRequest},
		{http.MethodPost, "/people/" + uuid.NewString() + "/leave", "eve", `{"on":"x"}`, http.StatusBadRequest},
		{http.MethodPut, "/people/me/photo", "bert", `{"data":"not base64!"}`, http.StatusBadRequest},
		{http.MethodPut, "/people/self-editable", "ada", `{"field":[]}`, http.StatusBadRequest},
	} {
		if rec := call(s, c.method, APIPrefix+c.path, c.cookie, c.body, nil); rec.Code != c.want {
			t.Errorf("%s %s %s: %d, want %d", c.method, c.path, c.body, rec.Code, c.want)
		}
	}
	for err, want := range map[error]int{
		org.ErrPersonNotFound: http.StatusNotFound,
		org.ErrNoEntry:        http.StatusNotFound,
		org.ErrNotYours:       http.StatusForbidden,
		org.ErrEmailTaken:     http.StatusConflict,
		org.ErrBadPhoto:       http.StatusUnprocessableEntity,
		org.ErrAccountMissing: http.StatusUnprocessableEntity,
		org.ErrAccountLinked:  http.StatusConflict,
		org.ErrLeft:           http.StatusConflict,
	} {
		var apiErr *APIError
		if !errors.As(personError(err), &apiErr) || apiErr.Status != want {
			t.Errorf("%v became %v, want status %d", err, personError(err), want)
		}
	}
}

func TestWalksAnswerEveryHolder(t *testing.T) {
	s := &Server{}
	called := false
	walk := func(_ context.Context, _ uuid.UUID, _ org.Day, w org.Walk) ([]org.Link, error) {
		called = w.Dotted && w.All
		return []org.Link{{Title: "Lead", Depth: 1, Kind: org.Solid, Holders: []org.Holder{{Name: "Lena", Share: 50}}}}, nil
	}
	r := httptest.NewRequest(http.MethodGet, "/positions/x/reports?all=true&dotted=true", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("positionID", uuid.NewString())
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	s.respondWalk(w, r, "positionID", walk, positionError)
	var got struct{ Positions []linkResponse }
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || w.Code != http.StatusOK || !called {
		t.Fatalf("%d %s %v", w.Code, w.Body, err)
	}
	if len(got.Positions) != 1 || got.Positions[0].Holders[0].Name != "Lena" || got.Positions[0].Holders[0].Share != 50 {
		t.Errorf("the walk answered %+v", got.Positions)
	}
}

func TestWalksRefuseWhatTheyCannotAnswer(t *testing.T) {
	s := &Server{Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	missing := func(context.Context, uuid.UUID, org.Day, org.Walk) ([]org.Link, error) {
		return nil, org.ErrPersonNotFound
	}
	for name, c := range map[string]struct {
		id, query string
		want      int
	}{
		"no id":      {"nope", "", http.StatusNotFound},
		"a bad day":  {uuid.NewString(), "?asOf=x", http.StatusUnprocessableEntity},
		"nobody":     {uuid.NewString(), "", http.StatusNotFound},
		"a good day": {uuid.NewString(), "?asOf=2026-01-01", http.StatusNotFound},
	} {
		r := httptest.NewRequest(http.MethodGet, "/people/x/chain"+c.query, nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("personID", c.id)
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
		w := httptest.NewRecorder()
		s.respondWalk(w, r, "personID", missing, personError)
		if w.Code != c.want {
			t.Errorf("%s answered %d, want %d", name, w.Code, c.want)
		}
	}
}

func TestOrgAnswersCarryTheirBodyOrTheirRefusal(t *testing.T) {
	s := &Server{Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	for name, c := range map[string]struct {
		respond func(w http.ResponseWriter)
		want    int
	}{
		"a holding": {func(w http.ResponseWriter) { s.respondHolding(w, r, http.StatusCreated, org.Holding{Share: 50}, nil) }, http.StatusCreated},
		"no holding": {func(w http.ResponseWriter) {
			s.respondHolding(w, r, http.StatusOK, org.Holding{}, org.ErrHoldingNotFound)
		}, http.StatusNotFound},
		"a role":    {func(w http.ResponseWriter) { s.respondRole(w, r, http.StatusOK, org.Role{Key: "approver"}, nil) }, http.StatusOK},
		"no role":   {func(w http.ResponseWriter) { s.respondRole(w, r, http.StatusOK, org.Role{}, org.ErrRoleKeyTaken) }, http.StatusConflict},
		"a carrier": {func(w http.ResponseWriter) { s.respondCarried(w, r, http.StatusCreated, org.Position{}, nil) }, http.StatusCreated},
		"no carrier": {func(w http.ResponseWriter) {
			s.respondCarried(w, r, http.StatusOK, org.Position{}, org.ErrRoleCarried)
		}, http.StatusConflict},
	} {
		w := httptest.NewRecorder()
		c.respond(w)
		if w.Code != c.want || !json.Valid(w.Body.Bytes()) {
			t.Errorf("%s answered %d %s", name, w.Code, w.Body)
		}
	}
}
