package org

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/praetorianer777/bearing/backend/internal/db"
)

// Process is a flow graph whose steps say who does what: a draft edited freely and the versions published from it.
type Process struct {
	ID          uuid.UUID
	Key         string
	Name        string
	Area        string
	Description string
	// OwnerID is the position answering for the process as a whole.
	OwnerID *uuid.UUID
	// Latest is the newest published version's number, and PublishedAt when it was published; nil before the first.
	Latest      *int
	PublishedAt *time.Time
	// Changed says the draft was saved since the newest version was published, or that none is.
	Changed bool
	// Draft is the graph being edited and Versions every one published, newest first; filled when one process is
	// asked for.
	Draft    *Draft
	Versions []Version
}

// Draft is a process's graph as it is being edited.
type Draft struct {
	Graph
	// Revision counts the saves; a save names the revision it was made from.
	Revision int
	SavedAt  time.Time
}

// Version is a published version of a process, which never changes.
type Version struct {
	Number      int
	PublishedAt time.Time
	// PublishedBy is the account that published it and PublisherName what it is called; nil once it is gone.
	PublishedBy   *uuid.UUID
	PublisherName *string
	Note          string
	// Graph is filled when one version is asked for.
	Graph *Graph
}

// ProcessInput is what a process is made or changed with; a nil field is left as it is, and a nil OwnerID with
// ClearOwner takes the owner away.
type ProcessInput struct {
	Key         *string
	Name        *string
	Area        *string
	Description *string
	OwnerID     *uuid.UUID
	ClearOwner  bool
}

var (
	ErrProcessNotFound  = errors.New("there is no such process")
	ErrProcessKeyTaken  = errors.New("another process has that key")
	ErrProcessPublished = errors.New("the process has published versions; they stay for the history")
	ErrProcessInUse     = errors.New("another process has it as a subprocess")
	ErrVersionNotFound  = errors.New("the process has no such version")
	ErrDraftStale       = errors.New("the draft was saved by somebody else since; read it again")
	ErrNothingChanged   = errors.New("the draft is as the newest version has it")
	ErrOwnerNotThere    = errors.New("there is no such position")
	ErrSubprocessGone   = errors.New("a subprocess node names a process that does not exist")
	ErrPartyGone        = errors.New("a responsibility names a role, position or unit that does not exist")
)

// Unpublishable is a draft refused for publishing, with what keeps it from it.
type Unpublishable struct{ Problems []Problem }

func (Unpublishable) Error() string { return "the draft cannot be published yet" }

func (in *ProcessInput) check(creating bool) error {
	if creating && in.Key == nil {
		return Invalid{"key", "Give a key."}
	}
	if creating && in.Name == nil {
		return Invalid{"name", "Give a name."}
	}
	if in.Key != nil && !roleKey.MatchString(*in.Key) {
		return Invalid{"key", "Use small letters, digits and dashes, up to 63."}
	}
	if err := checkText("name", in.Name, 1, 200); err != nil {
		return err
	}
	if err := checkText("area", in.Area, 0, 100); err != nil {
		return err
	}
	if in.Description != nil && utf8.RuneCountInString(*in.Description) > 4000 {
		return Invalid{"description", "Keep it to 4000 characters."}
	}
	return nil
}

const processColumns = `p.id, p.key, p.name, p.area, p.description, p.owner_id, v.number, v.published_at,
	v.number IS NULL OR d.saved_at > v.published_at
	FROM process p
	JOIN process_version d ON d.process_id = p.id AND d.number IS NULL
	LEFT JOIN LATERAL (SELECT number, published_at FROM process_version
		WHERE process_id = p.id AND number IS NOT NULL ORDER BY number DESC LIMIT 1) v ON true`

func scanProcess(row pgx.CollectableRow) (Process, error) {
	var p Process
	err := row.Scan(&p.ID, &p.Key, &p.Name, &p.Area, &p.Description, &p.OwnerID, &p.Latest, &p.PublishedAt, &p.Changed)
	return p, err
}

// Processes lists every process by area, then by name.
func (s *Service) Processes(ctx context.Context) ([]Process, error) {
	var out []Process
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		rows, err := q.Query(ctx, `SELECT `+processColumns+` ORDER BY lower(p.area), lower(p.name), p.id`)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, scanProcess)
		return err
	})
	return out, err
}

// Process is one process with its draft and the versions published from it.
func (s *Service) Process(ctx context.Context, id uuid.UUID) (Process, error) {
	var out Process
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		return process(ctx, q, id, &out)
	})
	return out, err
}

func process(ctx context.Context, q db.DBTX, id uuid.UUID, out *Process) error {
	rows, err := q.Query(ctx, `SELECT `+processColumns+` WHERE p.id = $1`, id)
	if err != nil {
		return err
	}
	*out, err = pgx.CollectExactlyOneRow(rows, scanProcess)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrProcessNotFound
	}
	if err != nil {
		return err
	}
	var draft uuid.UUID
	out.Draft = &Draft{}
	if err := q.QueryRow(ctx, `SELECT id, revision, saved_at FROM process_version WHERE process_id = $1 AND number IS NULL`, id).
		Scan(&draft, &out.Draft.Revision, &out.Draft.SavedAt); err != nil {
		return err
	}
	if out.Draft.Graph, err = graph(ctx, q, draft); err != nil {
		return err
	}
	rows, err = q.Query(ctx, `SELECT v.number, v.published_at, v.published_by, a.name, v.note
		FROM process_version v LEFT JOIN account a ON a.id = v.published_by
		WHERE v.process_id = $1 AND v.number IS NOT NULL ORDER BY v.number DESC`, id)
	if err != nil {
		return err
	}
	out.Versions, err = pgx.CollectRows(rows, scanVersion)
	return err
}

func scanVersion(row pgx.CollectableRow) (Version, error) {
	var v Version
	err := row.Scan(&v.Number, &v.PublishedAt, &v.PublishedBy, &v.PublisherName, &v.Note)
	return v, err
}

func graph(ctx context.Context, q db.DBTX, version uuid.UUID) (Graph, error) {
	var g Graph
	rows, err := q.Query(ctx, `SELECT key, kind, name, description, x, y, subprocess_id FROM process_node
		WHERE version_id = $1 ORDER BY key`, version)
	if err != nil {
		return g, err
	}
	g.Nodes, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Node, error) {
		var n Node
		err := row.Scan(&n.Key, &n.Kind, &n.Name, &n.Description, &n.X, &n.Y, &n.SubprocessID)
		return n, err
	})
	if err != nil {
		return g, err
	}
	at := make(map[string]int, len(g.Nodes))
	for i, n := range g.Nodes {
		at[n.Key] = i
	}
	rows, err = q.Query(ctx, `SELECT node_key, kind, role_id, position_id, unit_id FROM process_responsibility
		WHERE version_id = $1 ORDER BY node_key, kind, role_id, position_id, unit_id`, version)
	if err != nil {
		return g, err
	}
	var key string
	var r Responsibility
	if _, err := pgx.ForEachRow(rows, []any{&key, &r.Kind, &r.RoleID, &r.PositionID, &r.UnitID}, func() error {
		n := &g.Nodes[at[key]]
		n.Responsibilities = append(n.Responsibilities, r)
		return nil
	}); err != nil {
		return g, err
	}
	rows, err = q.Query(ctx, `SELECT key, from_key, to_key, label, route FROM process_arrow WHERE version_id = $1 ORDER BY key`, version)
	if err != nil {
		return g, err
	}
	g.Arrows, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Arrow, error) {
		var a Arrow
		err := row.Scan(&a.Key, &a.From, &a.To, &a.Label, &a.Route)
		return a, err
	})
	return g, err
}

// ProcessVersion is one published version of a process with its graph.
func (s *Service) ProcessVersion(ctx context.Context, id uuid.UUID, number int) (Version, error) {
	var out Version
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		var version uuid.UUID
		err := q.QueryRow(ctx, `SELECT v.id, v.number, v.published_at, v.published_by, a.name, v.note
			FROM process_version v LEFT JOIN account a ON a.id = v.published_by
			WHERE v.process_id = $1 AND v.number = $2`, id, number).
			Scan(&version, &out.Number, &out.PublishedAt, &out.PublishedBy, &out.PublisherName, &out.Note)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrVersionNotFound
		}
		if err != nil {
			return err
		}
		g, err := graph(ctx, q, version)
		out.Graph = &g
		return err
	})
	return out, err
}

// CreateProcess adds a process, as an editor. Its draft begins as a start joined to an end.
func (s *Service) CreateProcess(ctx context.Context, in ProcessInput) (Process, db.LSN, error) {
	if err := in.check(true); err != nil {
		return Process{}, 0, err
	}
	var out Process
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		var id, draft uuid.UUID
		if err := q.QueryRow(ctx, `INSERT INTO process (key, name, area, description, owner_id)
			VALUES ($1, $2, coalesce($3, ''), coalesce($4, ''), $5) RETURNING id`,
			in.Key, in.Name, in.Area, in.Description, in.OwnerID).Scan(&id); err != nil {
			return processError(err)
		}
		if err := q.QueryRow(ctx, `INSERT INTO process_version (process_id) VALUES ($1) RETURNING id`, id).Scan(&draft); err != nil {
			return err
		}
		if err := writeGraph(ctx, q, draft, Graph{
			Nodes:  []Node{{Key: "start", Kind: Start, Name: "Start"}, {Key: "end", Kind: End, Name: "End", Y: 160}},
			Arrows: []Arrow{{Key: "start-end", From: "start", To: "end"}},
		}); err != nil {
			return err
		}
		return process(ctx, q, id, &out)
	})
	return out, lsn, err
}

// UpdateProcess changes a process's key, name, area, description or owner, as an editor.
func (s *Service) UpdateProcess(ctx context.Context, id uuid.UUID, in ProcessInput) (Process, db.LSN, error) {
	if err := in.check(false); err != nil {
		return Process{}, 0, err
	}
	var out Process
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		tag, err := q.Exec(ctx, `UPDATE process SET key = coalesce($2, key), name = coalesce($3, name), area = coalesce($4, area),
			description = coalesce($5, description), owner_id = CASE WHEN $7 THEN NULL ELSE coalesce($6, owner_id) END
			WHERE id = $1`, id, in.Key, in.Name, in.Area, in.Description, in.OwnerID, in.ClearOwner)
		if err != nil {
			return processError(err)
		}
		if tag.RowsAffected() == 0 {
			return ErrProcessNotFound
		}
		return process(ctx, q, id, &out)
	})
	return out, lsn, err
}

// DeleteProcess removes a process never published that no other process has as a subprocess, as an editor.
func (s *Service) DeleteProcess(ctx context.Context, id uuid.UUID) (db.LSN, error) {
	return s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		var published bool
		err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM process_version WHERE process_id = p.id AND number IS NOT NULL)
			FROM process p WHERE p.id = $1 FOR UPDATE`, id).Scan(&published)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrProcessNotFound
		}
		if err != nil {
			return err
		}
		if published {
			return ErrProcessPublished
		}
		var used bool
		if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM process_node n JOIN process_version v ON v.id = n.version_id
			WHERE n.subprocess_id = $1 AND v.process_id <> $1)`, id).Scan(&used); err != nil {
			return err
		}
		if used {
			return ErrProcessInUse
		}
		_, err = q.Exec(ctx, `DELETE FROM process WHERE id = $1`, id)
		return err
	})
}

// SaveDraft replaces a process's draft graph, as an editor. The save names the revision it was made from; one made
// from an older revision than the draft's is refused, so two editors do not overwrite each other unseen.
func (s *Service) SaveDraft(ctx context.Context, id uuid.UUID, revision int, g Graph) (Process, db.LSN, error) {
	if err := checkShape(g); err != nil {
		return Process{}, 0, err
	}
	var out Process
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		draft, current, err := lockDraft(ctx, q, id)
		if err != nil {
			return err
		}
		if current != revision {
			return ErrDraftStale
		}
		if err := subprocessesExist(ctx, q, g); err != nil {
			return err
		}
		if _, err := q.Exec(ctx, `DELETE FROM process_node WHERE version_id = $1`, draft); err != nil {
			return err
		}
		if err := writeGraph(ctx, q, draft, g); err != nil {
			return err
		}
		if _, err := q.Exec(ctx, `UPDATE process_version SET revision = revision + 1, saved_at = clock_timestamp() WHERE id = $1`, draft); err != nil {
			return err
		}
		return process(ctx, q, id, &out)
	})
	return out, lsn, err
}

func lockDraft(ctx context.Context, q db.DBTX, id uuid.UUID) (draft uuid.UUID, revision int, err error) {
	err = q.QueryRow(ctx, `SELECT d.id, d.revision FROM process p
		JOIN process_version d ON d.process_id = p.id AND d.number IS NULL
		WHERE p.id = $1 FOR UPDATE OF p, d`, id).Scan(&draft, &revision)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrProcessNotFound
	}
	return draft, revision, err
}

func subprocessesExist(ctx context.Context, q db.DBTX, g Graph) error {
	var named []uuid.UUID
	for _, n := range g.Nodes {
		if n.SubprocessID != nil {
			named = append(named, *n.SubprocessID)
		}
	}
	if len(named) == 0 {
		return nil
	}
	var missing bool
	if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM unnest($1::uuid[]) AS n(id) WHERE NOT EXISTS (SELECT 1 FROM process WHERE id = n.id))`,
		named).Scan(&missing); err != nil {
		return err
	}
	if missing {
		return ErrSubprocessGone
	}
	return nil
}

func writeGraph(ctx context.Context, q db.DBTX, version uuid.UUID, g Graph) error {
	n := len(g.Nodes)
	keys, kinds, names, descriptions := make([]string, n), make([]string, n), make([]string, n), make([]string, n)
	xs, ys, subs := make([]float64, n), make([]float64, n), make([]*uuid.UUID, n)
	for i, node := range g.Nodes {
		keys[i], kinds[i], names[i], descriptions[i] = node.Key, string(node.Kind), node.Name, node.Description
		xs[i], ys[i], subs[i] = node.X, node.Y, node.SubprocessID
	}
	if _, err := q.Exec(ctx, `INSERT INTO process_node (version_id, key, kind, name, description, x, y, subprocess_id)
		SELECT $1, k, kind::process_node_kind, name, description, x, y, sub
		FROM unnest($2::text[], $3::text[], $4::text[], $5::text[], $6::float8[], $7::float8[], $8::uuid[])
			AS t(k, kind, name, description, x, y, sub)`,
		version, keys, kinds, names, descriptions, xs, ys, subs); err != nil {
		return processError(err)
	}
	var nodeKeys, raci []string
	var roles, positions, units []*uuid.UUID
	for _, node := range g.Nodes {
		for _, r := range node.Responsibilities {
			nodeKeys, raci = append(nodeKeys, node.Key), append(raci, string(r.Kind))
			roles, positions, units = append(roles, r.RoleID), append(positions, r.PositionID), append(units, r.UnitID)
		}
	}
	if len(nodeKeys) > 0 {
		if _, err := q.Exec(ctx, `INSERT INTO process_responsibility (version_id, node_key, kind, role_id, position_id, unit_id)
			SELECT $1, k, r::raci, role, pos, unit FROM unnest($2::text[], $3::text[], $4::uuid[], $5::uuid[], $6::uuid[])
				AS t(k, r, role, pos, unit)`, version, nodeKeys, raci, roles, positions, units); err != nil {
			return processError(err)
		}
	}
	m := len(g.Arrows)
	arrowKeys, froms, tos, labels, routes := make([]string, m), make([]string, m), make([]string, m), make([]string, m), make([]string, m)
	for i, a := range g.Arrows {
		arrowKeys[i], froms[i], tos[i], labels[i], routes[i] = a.Key, a.From, a.To, a.Label, arrayLiteral(a.Route)
	}
	// Routes are arrays of their own lengths, which unnest would flatten; each goes as its array literal.
	_, err := q.Exec(ctx, `INSERT INTO process_arrow (version_id, key, from_key, to_key, label, route)
		SELECT $1, k, f, t, l, r::float8[] FROM unnest($2::text[], $3::text[], $4::text[], $5::text[], $6::text[]) AS a(k, f, t, l, r)`,
		version, arrowKeys, froms, tos, labels, routes)
	return err
}

func arrayLiteral(values []float64) string {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = strconv.FormatFloat(v, 'g', -1, 64)
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// Publish makes the draft the process's next version, as an editor, when nothing keeps it from it. The draft goes on
// as a copy of what was published.
func (s *Service) Publish(ctx context.Context, id uuid.UUID, note string) (Version, db.LSN, error) {
	if utf8.RuneCountInString(note) > 1000 {
		return Version{}, 0, Invalid{"note", "Keep it to 1000 characters."}
	}
	var out Version
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		draft, revision, err := lockDraft(ctx, q, id)
		if err != nil {
			return err
		}
		var p Process
		if err := process(ctx, q, id, &p); err != nil {
			return err
		}
		if problems := Check(id, p.Draft.Graph); Blocking(problems) {
			return Unpublishable{problems}
		}
		if !p.Changed {
			return ErrNothingChanged
		}
		next := 1
		if p.Latest != nil {
			next = *p.Latest + 1
		}
		// The clock, not the transaction's start: one transaction may publish, save and publish again, as the demo does.
		if _, err := q.Exec(ctx, `UPDATE process_version SET number = $2, published_at = clock_timestamp(),
			published_by = current_actor_id(), note = $3 WHERE id = $1`, draft, next, note); err != nil {
			return err
		}
		var copied uuid.UUID
		if err := q.QueryRow(ctx, `INSERT INTO process_version (process_id, revision, saved_at)
			SELECT $1, $2, published_at FROM process_version WHERE id = $3 RETURNING id`, id, revision+1, draft).Scan(&copied); err != nil {
			return err
		}
		if err := writeGraph(ctx, q, copied, p.Draft.Graph); err != nil {
			return err
		}
		return q.QueryRow(ctx, `SELECT v.number, v.published_at, v.published_by, a.name, v.note
			FROM process_version v LEFT JOIN account a ON a.id = v.published_by WHERE v.id = $1`, draft).
			Scan(&out.Number, &out.PublishedAt, &out.PublishedBy, &out.PublisherName, &out.Note)
	})
	return out, lsn, err
}

func processError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	switch pgErr.ConstraintName {
	case "process_key_key":
		return ErrProcessKeyTaken
	case "process_owner_id_fkey":
		return ErrOwnerNotThere
	case "process_responsibility_role_id_fkey", "process_responsibility_position_id_fkey", "process_responsibility_unit_id_fkey":
		return ErrPartyGone
	}
	return err
}
