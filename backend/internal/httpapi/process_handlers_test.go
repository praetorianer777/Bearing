package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/org"
)

func withParams(r *http.Request, params map[string]string) *http.Request {
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// What the process operations refuse before asking the model: they answer without a database.
func TestProcessOperationsRefuseWhatTheyCannotRead(t *testing.T) {
	s := &Server{Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	id := uuid.NewString()
	for name, c := range map[string]struct {
		handle func(*Server, http.ResponseWriter, *http.Request)
		target string
		params map[string]string
		want   int
	}{
		"duties of nobody":        {(*Server).handleDuties, "/responsibilities", nil, http.StatusUnprocessableEntity},
		"duties of a bad id":      {(*Server).handleDuties, "/responsibilities?personId=x", nil, http.StatusUnprocessableEntity},
		"duties of two":           {(*Server).handleDuties, "/responsibilities?personId=" + id + "&unitId=" + id, nil, http.StatusUnprocessableEntity},
		"duties on no day":        {(*Server).handleDuties, "/responsibilities?personId=" + id + "&asOf=soon", nil, http.StatusUnprocessableEntity},
		"who does a letter":       {(*Server).handleWhoDoes, "/steps?q=x", nil, http.StatusUnprocessableEntity},
		"who does on no day":      {(*Server).handleWhoDoes, "/steps?q=xy&asOf=soon", nil, http.StatusUnprocessableEntity},
		"people of no process":    {(*Server).handleProcessPeople, "/processes/x/people", map[string]string{"processID": "x"}, http.StatusNotFound},
		"people of version zero":  {(*Server).handleProcessPeople, "/processes/p/people?version=0", map[string]string{"processID": id}, http.StatusUnprocessableEntity},
		"people on no day":        {(*Server).handleProcessPeople, "/processes/p/people?asOf=soon", map[string]string{"processID": id}, http.StatusUnprocessableEntity},
		"a step of no process":    {(*Server).handleStepPeople, "/processes/x/steps/s/people", map[string]string{"processID": "x", "stepKey": "s"}, http.StatusNotFound},
		"version zero":            {(*Server).handleProcessVersion, "/processes/p/versions/0", map[string]string{"processID": id, "number": "0"}, http.StatusNotFound},
		"a version of no process": {(*Server).handleProcessVersion, "/processes/x/versions/1", map[string]string{"processID": "x", "number": "1"}, http.StatusNotFound},
		"no process":              {(*Server).handleProcess, "/processes/x", map[string]string{"processID": "x"}, http.StatusNotFound},
		"a draft of no process":   {(*Server).handleSaveDraft, "/processes/x/draft", map[string]string{"processID": "x"}, http.StatusNotFound},
		"publishing no process":   {(*Server).handlePublish, "/processes/x/publish", map[string]string{"processID": "x"}, http.StatusNotFound},
		"removing no process":     {(*Server).handleDeleteProcess, "/processes/x", map[string]string{"processID": "x"}, http.StatusNotFound},
		"changing no process":     {(*Server).handleUpdateProcess, "/processes/x", map[string]string{"processID": "x"}, http.StatusNotFound},
		"the log of no process":   {(*Server).handleProcessAudit, "/processes/x/audit", map[string]string{"processID": "x"}, http.StatusNotFound},
	} {
		r := withParams(httptest.NewRequest(http.MethodGet, c.target, nil), c.params)
		w := httptest.NewRecorder()
		c.handle(s, w, r)
		if w.Code != c.want || !json.Valid(w.Body.Bytes()) {
			t.Errorf("%s answered %d %s, want %d", name, w.Code, w.Body, c.want)
		}
	}
}

func TestProcessRefusalsSayWhatWentWrongInTheAPIsTerms(t *testing.T) {
	for _, c := range []struct {
		err  error
		want int
	}{
		{org.Unpublishable{}, http.StatusUnprocessableEntity},
		{org.ErrProcessNotFound, http.StatusNotFound},
		{org.ErrVersionNotFound, http.StatusNotFound},
		{org.ErrProcessKeyTaken, http.StatusConflict},
		{org.ErrOwnerNotThere, http.StatusUnprocessableEntity},
		{org.ErrSubprocessGone, http.StatusUnprocessableEntity},
		{org.ErrPartyGone, http.StatusUnprocessableEntity},
		{org.ErrProcessPublished, http.StatusConflict},
		{org.ErrProcessInUse, http.StatusConflict},
		{org.ErrDraftStale, http.StatusConflict},
		{org.ErrNothingChanged, http.StatusConflict},
		{org.Invalid{Field: "name", Message: "Give a name."}, http.StatusUnprocessableEntity},
	} {
		var apiErr *APIError
		if !errors.As(processError(c.err), &apiErr) || apiErr.Status != c.want {
			t.Errorf("%v became %v, want %d", c.err, processError(c.err), c.want)
		}
	}
	plain := errors.New("the disk is full")
	if processError(plain) != plain {
		t.Error("an error the API has no word for was changed")
	}
}

func TestAProcessIsWrittenWithItsDraftProblemsAndVersions(t *testing.T) {
	sub, role, owner := uuid.New(), uuid.New(), uuid.New()
	latest := 2
	p := org.Process{ID: uuid.New(), Key: "invoice", Name: "Invoice", Latest: &latest,
		Draft: &org.Draft{Revision: 3, SavedAt: time.Now(), Graph: org.Graph{
			Nodes: []org.Node{{Key: "s", Kind: org.Start}, {Key: "x", Kind: org.Subprocess, Name: "X", SubprocessID: &sub,
				Responsibilities: []org.Responsibility{{Kind: org.Accountable, RoleID: &role}}}},
			Arrows: []org.Arrow{{Key: "a", From: "s", To: "x"}},
		}},
		Versions: []org.Version{{Number: 2, Note: "Second"}, {Number: 1}},
	}
	out := toProcess(p)
	if out.Draft == nil || out.Draft.Revision != 3 || len(out.Draft.Nodes) != 2 || out.Draft.Nodes[1].Responsibilities[0].RoleID == nil {
		t.Fatalf("the draft is %+v", out.Draft)
	}
	if len(out.Draft.Problems) == 0 || len(out.Versions) != 2 || out.Versions[0].Note != "Second" {
		t.Errorf("problems %+v, versions %+v", out.Draft.Problems, out.Versions)
	}
	if v := toVersion(org.Version{Number: 1, Graph: &p.Draft.Graph}); len(v.Nodes) != 2 || len(v.Arrows) != 1 {
		t.Errorf("a version with its graph is %+v", v)
	}
	ownerID := owner.String()
	empty, bad := "", "nobody"
	if in, err := (processRequest{OwnerID: &ownerID}).input(); err != nil || *in.OwnerID != owner {
		t.Errorf("an owner by id became %+v %v", in, err)
	}
	if in, err := (processRequest{OwnerID: &empty}).input(); err != nil || !in.ClearOwner {
		t.Errorf("an empty owner became %+v %v", in, err)
	}
	if _, err := (processRequest{OwnerID: &bad}).input(); err == nil {
		t.Error("an owner not named by an id was taken")
	}
}
