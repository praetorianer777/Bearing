package org

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/praetorianer777/bearing/backend/internal/db"
)

// What a search may be asked for, and how many hits it gives.
const (
	MinSearchLength    = 2
	MaxSearchLength    = 200
	DefaultSearchLimit = 10
	MaxSearchLimit     = 50
	DefaultQuickLimit  = 8
	MaxQuickLimit      = 20
	maxSearchWords     = 10
	SearchKindPerson   = "person"
	SearchKindPosition = "position"
	SearchKindUnit     = "unit"
	SearchKindRole     = "role"
	SearchKindProcess  = "process"
	SearchKindStep     = "step"
)

// SearchKinds are the kinds a search finds, in the order the groups come.
var SearchKinds = []string{SearchKindPerson, SearchKindPosition, SearchKindUnit, SearchKindRole, SearchKindProcess, SearchKindStep}

// SearchHit is one thing a search found. ID is the person, position, unit, role or process; a step is found in its
// process, so its ID is the process's, StepKey names it there and Version is the version it stands in, nil for a
// draft not published yet. Detail tells two of a name apart: a person's position, a position's unit, a unit's type, a
// role's description, a process's area, a step's process.
type SearchHit struct {
	Kind    string
	ID      uuid.UUID
	Name    string
	Detail  string
	StepKey string
	Version *int
	Rank    float64
}

// SearchWords are the words of what was typed, the runs of letters and digits, so nothing typed reaches the tsquery
// syntax.
func SearchWords(typed string) []string {
	words := strings.FieldsFunc(typed, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	if len(words) > maxSearchWords {
		words = words[:maxSearchWords]
	}
	return words
}

// prefixQuery asks for every word as the start of a word, so a search finds while it is typed.
func prefixQuery(words []string) string {
	terms := make([]string, len(words))
	for i, w := range words {
		terms[i] = "'" + strings.ToLower(w) + "':*"
	}
	return strings.Join(terms, " & ")
}

// searchSQL finds every kind in one statement. A hit matches a word's start, a stemmed word of a description, or,
// with a slip, a name's trigrams. It ranks a name that is what was typed first, then one that starts so, then by the
// words' weights and the name's likeness; the rows come with their rank for the caller to group or cut.
const searchSQL = `
WITH q AS (
    SELECT to_tsquery('bearing_search', $1) AS prefix, plainto_tsquery('bearing_english', $2) AS en,
           plainto_tsquery('bearing_german', $2) AS de, search_fold($2) AS folded
), candidate AS (
    SELECT 'person' AS kind, p.id, p.name,
           COALESCE((SELECT pos.title FROM holding h JOIN position pos ON pos.id = h.position_id
                     WHERE h.person_id = p.id AND h.valid_from <= $3 AND (h.valid_to IS NULL OR h.valid_to > $3)
                     ORDER BY h.share DESC, lower(pos.title) LIMIT 1), p.email::text, '') AS detail,
           '' AS step_key, NULL::integer AS version, p.search_vector AS words, search_fold(p.name) AS folded_name
    FROM person p WHERE p.left_on IS NULL OR p.left_on > $3
    UNION ALL
    SELECT 'position', pos.id, pos.title, u.name, '', NULL, pos.search_vector, search_fold(pos.title)
    FROM position pos JOIN unit u ON u.id = pos.unit_id
    WHERE pos.exists_from <= $3 AND (pos.archived_on IS NULL OR pos.archived_on > $3)
    UNION ALL
    SELECT 'unit', u.id, u.name, t.name, '', NULL, u.search_vector, search_fold(u.name)
    FROM unit u JOIN unit_type t ON t.id = u.type_id
    WHERE (u.archived_on IS NULL OR u.archived_on > $3)
      AND EXISTS (SELECT 1 FROM unit_placement pl WHERE pl.unit_id = u.id AND daterange(pl.valid_from, pl.valid_to) @> $3::date)
    UNION ALL
    SELECT 'role', r.id, r.name, left(r.description, 120), '', NULL, r.search_vector, search_fold(r.name)
    FROM role r
    UNION ALL
    SELECT 'process', pr.id, pr.name, pr.area, '', NULL, pr.search_vector, search_fold(pr.name)
    FROM process pr
    UNION ALL
    SELECT 'step', pr.id, n.name, pr.name, n.key, v.number, n.search_vector, search_fold(n.name)
    FROM process pr
    JOIN LATERAL (SELECT id, number FROM process_version WHERE process_id = pr.id ORDER BY number DESC NULLS LAST LIMIT 1) v ON true
    JOIN process_node n ON n.version_id = v.id
    WHERE n.kind IN ('step', 'decision', 'subprocess') AND n.name <> ''
), hit AS (
    SELECT c.kind, c.id, c.name, c.detail, c.step_key, c.version,
           CASE WHEN c.folded_name = q.folded THEN 2 WHEN starts_with(c.folded_name, q.folded) THEN 1 ELSE 0 END
             + ts_rank(c.words, q.prefix || q.en || q.de) + word_similarity(q.folded, c.folded_name) AS rank
    FROM candidate c, q
    WHERE c.words @@ q.prefix OR c.words @@ q.en OR c.words @@ q.de OR q.folded <% c.folded_name
)`

// Search finds what holds the words typed on a day, at most limit of each kind, best first.
func (s *Service) Search(ctx context.Context, typed string, asOf Day, limit int) ([]SearchHit, error) {
	return s.search(ctx, typed, asOf, limit, `
		SELECT kind, id, name, detail, step_key, version, rank FROM (
		    SELECT hit.*, row_number() OVER (PARTITION BY kind ORDER BY rank DESC, lower(name), id, step_key) AS place FROM hit
		) ranked WHERE place <= $4 ORDER BY kind, place`)
}

// Quick finds the best few of any kind for a search box being typed in.
func (s *Service) Quick(ctx context.Context, typed string, asOf Day, limit int) ([]SearchHit, error) {
	return s.search(ctx, typed, asOf, limit, `
		SELECT kind, id, name, detail, step_key, version, rank FROM hit ORDER BY rank DESC, lower(name), id, step_key LIMIT $4`)
}

func (s *Service) search(ctx context.Context, typed string, asOf Day, limit int, tail string) ([]SearchHit, error) {
	typed = strings.TrimSpace(typed)
	words := SearchWords(typed)
	out := []SearchHit{}
	if len(words) == 0 || utf8.RuneCountInString(typed) < MinSearchLength {
		return out, nil
	}
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		rows, err := q.Query(ctx, searchSQL+tail, prefixQuery(words), strings.Join(words, " "), asOf.Time(), limit)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (SearchHit, error) {
			var h SearchHit
			err := row.Scan(&h.Kind, &h.ID, &h.Name, &h.Detail, &h.StepKey, &h.Version, &h.Rank)
			return h, err
		})
		return err
	})
	return out, err
}
