package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/org"
)

type processRequest struct {
	// Key is what other tools name the process by: small letters, digits and dashes.
	Key         *string `json:"key,omitempty"`
	Name        *string `json:"name,omitempty"`
	Area        *string `json:"area,omitempty"`
	Description *string `json:"description,omitempty"`
	// OwnerID is the position answering for the process; an empty one takes the owner away.
	OwnerID *string `json:"ownerId,omitempty"`
}

type responsibilityBody struct {
	Kind org.RACI `json:"kind"`
	// RoleID, PositionID and UnitID name the party; exactly one of them.
	RoleID     *uuid.UUID `json:"roleId,omitempty"`
	PositionID *uuid.UUID `json:"positionId,omitempty"`
	UnitID     *uuid.UUID `json:"unitId,omitempty"`
}

type nodeBody struct {
	Key         string       `json:"key"`
	Kind        org.NodeKind `json:"kind"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	X           float64      `json:"x"`
	Y           float64      `json:"y"`
	// SubprocessID is the process a subprocess node stands for.
	SubprocessID *uuid.UUID `json:"subprocessId"`
	// Responsibilities are on steps, decisions and subprocesses; a start or an end has none.
	Responsibilities []responsibilityBody `json:"responsibilities"`
}

type arrowBody struct {
	Key  string `json:"key"`
	From string `json:"from"`
	To   string `json:"to"`
	// Label is the condition the way is taken on, out of a decision.
	Label string `json:"label"`
}

type problemResponse struct {
	// Code says which rule: no_start, many_starts, no_end, into_start, out_of_end, unreachable, dead_end, unnamed,
	// decision_ways, unlabelled_way, no_subprocess or own_subprocess; and as warnings no_accountable,
	// many_accountable or no_responsible.
	Code    string `json:"code"`
	Message string `json:"message"`
	// Node and Arrow name what the problem is on, when it is on one.
	Node  string `json:"node,omitempty"`
	Arrow string `json:"arrow,omitempty"`
	// Warning says the problem does not keep the draft from being published.
	Warning bool `json:"warning"`
}

type draftResponse struct {
	Nodes  []nodeBody  `json:"nodes"`
	Arrows []arrowBody `json:"arrows"`
	// Revision is what the next save names.
	Revision int       `json:"revision"`
	SavedAt  time.Time `json:"savedAt"`
	// Problems keep the draft from being published, but for the warnings, which are worth a look first.
	Problems []problemResponse `json:"problems"`
}

type draftRequest struct {
	// Revision is the draft's revision the save was made from.
	Revision int         `json:"revision"`
	Nodes    []nodeBody  `json:"nodes"`
	Arrows   []arrowBody `json:"arrows"`
}

type versionResponse struct {
	Number      int       `json:"number"`
	PublishedAt time.Time `json:"publishedAt"`
	// PublishedBy and PublisherName are the account that published it; null once it is gone.
	PublishedBy   *uuid.UUID `json:"publishedBy"`
	PublisherName *string    `json:"publisherName"`
	Note          string     `json:"note"`
	// Nodes and Arrows are the version's graph; only when one version is asked for.
	Nodes  []nodeBody  `json:"nodes,omitempty"`
	Arrows []arrowBody `json:"arrows,omitempty"`
}

type processResponse struct {
	ID          uuid.UUID  `json:"id"`
	Key         string     `json:"key"`
	Name        string     `json:"name"`
	Area        string     `json:"area"`
	Description string     `json:"description"`
	OwnerID     *uuid.UUID `json:"ownerId"`
	// Latest is the newest published version's number and PublishedAt when; null before the first.
	Latest      *int       `json:"latest"`
	PublishedAt *time.Time `json:"publishedAt"`
	// Changed says the draft was saved since the newest version was published, or that none is.
	Changed bool `json:"changed"`
	// Draft and Versions, newest first, come only when one process is asked for.
	Draft    *draftResponse    `json:"draft,omitempty"`
	Versions []versionResponse `json:"versions,omitempty"`
}

type publishRequest struct {
	// Note says what changed.
	Note string `json:"note"`
}

func toNodes(nodes []org.Node) []nodeBody {
	out := make([]nodeBody, len(nodes))
	for i, n := range nodes {
		out[i] = nodeBody{Key: n.Key, Kind: n.Kind, Name: n.Name, Description: n.Description, X: n.X, Y: n.Y, SubprocessID: n.SubprocessID,
			Responsibilities: make([]responsibilityBody, len(n.Responsibilities))}
		for j, r := range n.Responsibilities {
			out[i].Responsibilities[j] = responsibilityBody(r)
		}
	}
	return out
}

func toArrows(arrows []org.Arrow) []arrowBody {
	out := make([]arrowBody, len(arrows))
	for i, a := range arrows {
		out[i] = arrowBody{Key: a.Key, From: a.From, To: a.To, Label: a.Label}
	}
	return out
}

func toProblems(problems []org.Problem) []problemResponse {
	out := make([]problemResponse, len(problems))
	for i, p := range problems {
		out[i] = problemResponse{Code: p.Code, Message: p.Message, Node: p.Node, Arrow: p.Arrow, Warning: p.Warning}
	}
	return out
}

func toVersion(v org.Version) versionResponse {
	out := versionResponse{Number: v.Number, PublishedAt: v.PublishedAt, PublishedBy: v.PublishedBy, PublisherName: v.PublisherName, Note: v.Note}
	if v.Graph != nil {
		out.Nodes, out.Arrows = toNodes(v.Graph.Nodes), toArrows(v.Graph.Arrows)
	}
	return out
}

func toProcess(p org.Process) processResponse {
	out := processResponse{ID: p.ID, Key: p.Key, Name: p.Name, Area: p.Area, Description: p.Description, OwnerID: p.OwnerID,
		Latest: p.Latest, PublishedAt: p.PublishedAt, Changed: p.Changed}
	if p.Draft != nil {
		out.Draft = &draftResponse{Nodes: toNodes(p.Draft.Nodes), Arrows: toArrows(p.Draft.Arrows), Revision: p.Draft.Revision,
			SavedAt: p.Draft.SavedAt, Problems: toProblems(org.Check(p.ID, p.Draft.Graph))}
	}
	for _, v := range p.Versions {
		out.Versions = append(out.Versions, toVersion(v))
	}
	return out
}

func (req processRequest) input() (org.ProcessInput, error) {
	in := org.ProcessInput{Key: req.Key, Name: req.Name, Area: req.Area, Description: req.Description}
	if req.OwnerID != nil {
		if *req.OwnerID == "" {
			in.ClearOwner = true
			return in, nil
		}
		id, err := uuid.Parse(*req.OwnerID)
		if err != nil {
			return in, ErrValidation(map[string]string{"ownerId": "Name the owning position by its id."})
		}
		in.OwnerID = &id
	}
	return in, nil
}

// processError says a refusal about processes in the API's terms.
func processError(err error) error {
	var refused org.Unpublishable
	switch {
	case errors.As(err, &refused):
		return &APIError{Status: http.StatusUnprocessableEntity, Code: "not_publishable",
			Message: "Some things keep the draft from being published; the draft lists them."}
	case errors.Is(err, org.ErrProcessNotFound), errors.Is(err, org.ErrVersionNotFound):
		return ErrNotFound()
	case errors.Is(err, org.ErrProcessKeyTaken):
		return &APIError{Status: http.StatusConflict, Code: "key_taken", Message: sentence(err.Error()),
			Fields: map[string]string{"key": sentence(err.Error())}}
	case errors.Is(err, org.ErrOwnerNotThere):
		return ErrValidation(map[string]string{"ownerId": sentence(err.Error())})
	case errors.Is(err, org.ErrSubprocessGone), errors.Is(err, org.ErrPartyGone):
		return ErrValidation(map[string]string{"nodes": sentence(err.Error())})
	}
	for code, refusal := range map[string]error{
		"process_published": org.ErrProcessPublished, "process_in_use": org.ErrProcessInUse,
		"draft_stale": org.ErrDraftStale, "nothing_changed": org.ErrNothingChanged,
	} {
		if errors.Is(err, refusal) {
			return ErrConflict(code, sentence(err.Error()))
		}
	}
	return orgError(err)
}

func (s *Server) respondProcess(w http.ResponseWriter, r *http.Request, status int, p org.Process, err error) {
	if err != nil {
		respondError(w, r, s.Log, processError(err))
		return
	}
	respondJSON(w, status, env{"process": toProcess(p)})
}

func (s *Server) handleProcesses(w http.ResponseWriter, r *http.Request) {
	processes, err := s.Org.Processes(r.Context())
	if err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	out := make([]processResponse, len(processes))
	for i, p := range processes {
		out[i] = toProcess(p)
	}
	respondJSON(w, http.StatusOK, env{"processes": out})
}

func (s *Server) handleProcess(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "processID")
	if !ok {
		return
	}
	p, err := s.Org.Process(r.Context(), id)
	s.respondProcess(w, r, http.StatusOK, p, err)
}

func (s *Server) handleCreateProcess(w http.ResponseWriter, r *http.Request) {
	var req processRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	in, err := req.input()
	if err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	p, lsn, err := s.Org.CreateProcess(r.Context(), in)
	noteWrite(r.Context(), lsn)
	s.respondProcess(w, r, http.StatusCreated, p, err)
}

func (s *Server) handleUpdateProcess(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "processID")
	if !ok {
		return
	}
	var req processRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	in, err := req.input()
	if err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	p, lsn, err := s.Org.UpdateProcess(r.Context(), id, in)
	noteWrite(r.Context(), lsn)
	s.respondProcess(w, r, http.StatusOK, p, err)
}

func (s *Server) handleDeleteProcess(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "processID")
	if !ok {
		return
	}
	lsn, err := s.Org.DeleteProcess(r.Context(), id)
	noteWrite(r.Context(), lsn)
	if err != nil {
		respondError(w, r, s.Log, processError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSaveDraft(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "processID")
	if !ok {
		return
	}
	var req draftRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	g := org.Graph{Nodes: make([]org.Node, len(req.Nodes)), Arrows: make([]org.Arrow, len(req.Arrows))}
	for i, n := range req.Nodes {
		g.Nodes[i] = org.Node{Key: n.Key, Kind: n.Kind, Name: n.Name, Description: n.Description, X: n.X, Y: n.Y, SubprocessID: n.SubprocessID}
		for _, r := range n.Responsibilities {
			g.Nodes[i].Responsibilities = append(g.Nodes[i].Responsibilities, org.Responsibility(r))
		}
	}
	for i, a := range req.Arrows {
		g.Arrows[i] = org.Arrow{Key: a.Key, From: a.From, To: a.To, Label: a.Label}
	}
	p, lsn, err := s.Org.SaveDraft(r.Context(), id, req.Revision, g)
	noteWrite(r.Context(), lsn)
	s.respondProcess(w, r, http.StatusOK, p, err)
}

func (s *Server) handlePublish(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "processID")
	if !ok {
		return
	}
	var req publishRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	v, lsn, err := s.Org.Publish(r.Context(), id, req.Note)
	noteWrite(r.Context(), lsn)
	if err != nil {
		respondError(w, r, s.Log, processError(err))
		return
	}
	respondJSON(w, http.StatusCreated, env{"version": toVersion(v)})
}

func (s *Server) handleProcessVersion(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r, "processID")
	if !ok {
		return
	}
	number, err := strconv.Atoi(chi.URLParam(r, "number"))
	if err != nil || number < 1 {
		respondError(w, r, s.Log, ErrNotFound())
		return
	}
	v, err := s.Org.ProcessVersion(r.Context(), id, number)
	if err != nil {
		respondError(w, r, s.Log, processError(err))
		return
	}
	respondJSON(w, http.StatusOK, env{"version": toVersion(v)})
}

func (s *Server) handleProcessAudit(w http.ResponseWriter, r *http.Request) {
	s.handleSubjectAudit(w, r, "processID")
}
