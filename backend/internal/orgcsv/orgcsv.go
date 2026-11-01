// Package orgcsv reads and writes the organisation as four CSV files, as organisations keep it in spreadsheets:
// units, positions, people and holdings. Rows name each other by ref, which is the id on export and any name the
// sheet likes on import; people are matched by address as well. An import is planned first, every row checked
// against the organisation as it stands, and applied in one transaction, all of it or nothing.
package orgcsv

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/org"
)

// Kinds are the files, in the order an import applies them.
var Kinds = []string{"units", "positions", "people", "holdings"}

// Headers are each file's columns, as export writes them and import reads them, in any order.
var Headers = map[string][]string{
	"units":     {"ref", "name", "short_name", "type", "parent_ref", "description"},
	"positions": {"ref", "unit_ref", "title", "description", "seats", "head_of_unit", "reports_to_ref", "from"},
	"people":    {"ref", "name", "email", "phone", "location"},
	"holdings":  {"person_ref", "position_ref", "share", "from", "to"},
}

// MaxRows bounds each file, so one import stays one transaction a person can wait for.
const MaxRows = 5000

// Files are the CSV texts of an import; a file left empty changes nothing of its kind.
type Files struct {
	Units     string `json:"units,omitempty"`
	Positions string `json:"positions,omitempty"`
	People    string `json:"people,omitempty"`
	Holdings  string `json:"holdings,omitempty"`
}

func (f Files) of(kind string) string {
	return map[string]string{"units": f.Units, "positions": f.Positions, "people": f.People, "holdings": f.Holdings}[kind]
}

// Change is what an import does with one row.
type Change struct {
	File   string `json:"file"`
	Row    int    `json:"row"`
	Action string `json:"action"`
	Name   string `json:"name"`
}

// RowError is why a row cannot be imported; Row 0 is the file as a whole.
type RowError struct {
	File    string `json:"file"`
	Row     int    `json:"row"`
	Message string `json:"message"`
}

// Plan is what an import would do, row by row, and what stops it.
type Plan struct {
	Changes []Change   `json:"changes"`
	Errors  []RowError `json:"errors"`
}

// Counts are how many rows of each action the plan has.
func (p Plan) Counts() map[string]int {
	out := map[string]int{"create": 0, "update": 0, "unchanged": 0}
	for _, c := range p.Changes {
		out[c.Action]++
	}
	return out
}

func write(rows [][]string) []byte {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.WriteAll(rows)
	return buf.Bytes()
}

func optional(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// Export writes one file of the organisation as it stands on a day. Units come parents first; holdings are those of
// the positions exported that have not ended by then, planned ones too.
func Export(ctx context.Context, s *org.Service, kind string, asOf org.Day) ([]byte, error) {
	rows := [][]string{Headers[kind]}
	switch kind {
	case "units":
		units, err := s.Units(ctx, asOf)
		if err != nil {
			return nil, err
		}
		types, err := s.UnitTypes(ctx)
		if err != nil {
			return nil, err
		}
		typeNames := map[uuid.UUID]string{}
		for _, t := range types {
			typeNames[t.ID] = t.Name
		}
		for _, u := range parentsFirst(units) {
			parent := ""
			if u.ParentID != nil {
				parent = u.ParentID.String()
			}
			rows = append(rows, []string{u.ID.String(), u.Name, optional(u.ShortName), typeNames[u.TypeID], parent, u.Description})
		}
	case "positions":
		positions, err := s.Positions(ctx, asOf, nil, false)
		if err != nil {
			return nil, err
		}
		for _, p := range positions {
			reports := ""
			if p.ReportsTo != nil {
				reports = p.ReportsTo.String()
			}
			rows = append(rows, []string{p.ID.String(), p.UnitID.String(), p.Title, p.Description, strconv.Itoa(p.Seats),
				strconv.FormatBool(p.HeadOfUnit), reports, p.ExistsFrom.String()})
		}
	case "people":
		people, err := s.People(ctx, asOf, false, "")
		if err != nil {
			return nil, err
		}
		for _, p := range people {
			rows = append(rows, []string{p.ID.String(), p.Name, optional(p.Email), optional(p.Phone), optional(p.Location)})
		}
	case "holdings":
		positions, err := s.Positions(ctx, asOf, nil, false)
		if err != nil {
			return nil, err
		}
		exported := map[uuid.UUID]bool{}
		for _, p := range positions {
			exported[p.ID] = true
		}
		holdings, err := s.Holdings(ctx, org.HoldingFilter{})
		if err != nil {
			return nil, err
		}
		for _, h := range holdings {
			if h.PositionID == nil || !exported[*h.PositionID] || h.To != nil && !asOf.Before(*h.To) {
				continue
			}
			to := ""
			if h.To != nil {
				to = h.To.String()
			}
			rows = append(rows, []string{h.PersonID.String(), h.PositionID.String(), strconv.Itoa(h.Share), h.From.String(), to})
		}
	default:
		return nil, fmt.Errorf("no file is called %q", kind)
	}
	return write(rows), nil
}

func parentsFirst(units []org.Unit) []org.Unit {
	children := map[uuid.UUID][]org.Unit{}
	var out, roots []org.Unit
	known := map[uuid.UUID]bool{}
	for _, u := range units {
		known[u.ID] = true
	}
	for _, u := range units {
		if u.ParentID == nil || !known[*u.ParentID] {
			roots = append(roots, u)
		} else {
			children[*u.ParentID] = append(children[*u.ParentID], u)
		}
	}
	for queue := roots; len(queue) > 0; queue = queue[1:] {
		out = append(out, queue[0])
		queue = append(queue, children[queue[0].ID]...)
	}
	return out
}

// sheet is one file read: its rows as maps by column, and the line each has in the file, the header being 1.
type sheet struct {
	rows []map[string]string
	nums []int
}

func read(kind, text string) (sheet, []RowError) {
	var out sheet
	if strings.TrimSpace(text) == "" {
		return out, nil
	}
	r := csv.NewReader(strings.NewReader(strings.TrimPrefix(text, "\uFEFF")))
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	header, err := r.Read()
	if err != nil {
		return out, []RowError{{kind, 0, "The file cannot be read as CSV."}}
	}
	for i := range header {
		header[i] = strings.ToLower(strings.TrimSpace(header[i]))
	}
	var errs []RowError
	for _, column := range header {
		if !slices.Contains(Headers[kind], column) {
			errs = append(errs, RowError{kind, 1, fmt.Sprintf("There is no column %q; the columns are %s.", column, strings.Join(Headers[kind], ", "))})
		}
	}
	for {
		record, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		var parse *csv.ParseError
		if errors.As(err, &parse) {
			errs = append(errs, RowError{kind, parse.StartLine, "This row cannot be read as CSV."})
			break
		}
		if err != nil {
			return out, append(errs, RowError{kind, 0, "The file cannot be read as CSV."})
		}
		// Rows are numbered as the sheet shows them, blank lines counted, the header being 1.
		n, _ := r.FieldPos(0)
		if len(out.rows) == MaxRows {
			errs = append(errs, RowError{kind, n, fmt.Sprintf("Keep a file to %d rows.", MaxRows)})
			break
		}
		row := map[string]string{}
		empty := true
		for i, value := range record {
			if i < len(header) {
				row[header[i]] = strings.TrimSpace(value)
				empty = empty && row[header[i]] == ""
			}
		}
		if empty {
			continue
		}
		out.rows = append(out.rows, row)
		out.nums = append(out.nums, n)
	}
	return out, errs
}

// slot is what a ref names: a row that exists, with its id, or one the import makes, whose id its step fills in when
// it runs, so the steps after it read it.
type slot struct{ id uuid.UUID }

// state is the organisation an import is planned against, read once.
type state struct {
	units     []org.Unit
	types     []org.UnitType
	positions []org.Position
	people    []org.Person
	holdings  []org.Holding
}

func load(ctx context.Context, s *org.Service, from org.Day) (state, error) {
	var st state
	var err error
	if st.units, err = s.Units(ctx, from); err != nil {
		return st, err
	}
	if st.types, err = s.UnitTypes(ctx); err != nil {
		return st, err
	}
	if st.positions, err = s.Positions(ctx, from, nil, false); err != nil {
		return st, err
	}
	if st.people, err = s.People(ctx, from, false, ""); err != nil {
		return st, err
	}
	st.holdings, err = s.Holdings(ctx, org.HoldingFilter{})
	return st, err
}

// plan holds what an import reads and decides; its steps carry it out with the service they are given.
type plan struct {
	Plan
	state  state
	from   org.Day
	sheets map[string]sheet
	steps  []func(ctx context.Context, s *org.Service) error
	units  map[string]*slot
	posts  map[string]*slot
	people map[string]*slot
}

func (p *plan) fail(file string, row int, format string, args ...any) {
	p.Errors = append(p.Errors, RowError{file, row, fmt.Sprintf(format, args...)})
}

func (p *plan) change(file string, row int, action, name string) {
	p.Changes = append(p.Changes, Change{file, row, action, name})
}

func (p *plan) step(file string, row int, fn func(ctx context.Context, s *org.Service) error) {
	p.steps = append(p.steps, func(ctx context.Context, s *org.Service) error {
		if err := fn(ctx, s); err != nil {
			return &ApplyError{RowError{file, row, sentence(err.Error())}, err}
		}
		return nil
	})
}

// fresh are the refs a file makes, read before any of its rows, so a row may name one further down; a ref twice is
// an error on the second row.
func (p *plan) fresh(file string, refs map[string]*slot) {
	sh := p.sheets[file]
	seen := map[string]bool{}
	for i, row := range sh.rows {
		ref := row["ref"]
		if ref == "" {
			continue
		}
		if seen[ref] {
			p.fail(file, sh.nums[i], "Another row has the ref %q.", ref)
			continue
		}
		seen[ref] = true
		if refs[ref] == nil {
			refs[ref] = &slot{}
		}
	}
}

// day reads a day a row gives, or the import's when the row leaves it empty.
func day(raw string, fallback org.Day) (org.Day, error) {
	if raw == "" {
		return fallback, nil
	}
	return org.ParseDay(raw)
}

func idOf(sl *slot) *uuid.UUID {
	if sl == nil {
		return nil
	}
	return &sl.id
}

// Prepare reads the files and checks every row against the organisation as it stands on from, the day changes take
// effect: it is the preview, and what Apply carries out.
func Prepare(ctx context.Context, s *org.Service, files Files, from org.Day) (Plan, error) {
	p, err := prepare(ctx, s, files, from)
	if err != nil {
		return Plan{}, err
	}
	return p.Plan, nil
}

// Apply prepares the files again, against the organisation as it is now, and makes every change in one transaction;
// a row that no longer goes, or a refusal from the model, makes none. A refusal comes back as an ApplyError.
func Apply(ctx context.Context, s *org.Service, files Files, from org.Day) (Plan, error) {
	var out Plan
	_, err := s.Together(ctx, func(ctx context.Context, tx *org.Service) error {
		p, err := prepare(ctx, tx, files, from)
		if err != nil {
			return err
		}
		out = p.Plan
		if len(p.Errors) > 0 {
			return errRows
		}
		for _, step := range p.steps {
			if err := step(ctx, tx); err != nil {
				return err
			}
		}
		return nil
	})
	if errors.Is(err, errRows) {
		return out, nil
	}
	return out, err
}

var errRows = errors.New("rows refused")

func prepare(ctx context.Context, s *org.Service, files Files, from org.Day) (*plan, error) {
	st, err := load(ctx, s, from)
	if err != nil {
		return nil, err
	}
	return planned(st, files, from), nil
}

// planned decides what the files do to the organisation as st has it, row by row.
func planned(st state, files Files, from org.Day) *plan {
	p := &plan{Plan: Plan{Changes: []Change{}, Errors: []RowError{}}, state: st, from: from, sheets: map[string]sheet{},
		units: map[string]*slot{}, posts: map[string]*slot{}, people: map[string]*slot{}}
	for _, kind := range Kinds {
		sh, errs := read(kind, files.of(kind))
		p.sheets[kind] = sh
		p.Errors = append(p.Errors, errs...)
	}
	p.planUnits()
	p.planPositions()
	p.planPeople()
	p.planHoldings()
	return p
}

func (p *plan) planUnits() {
	existing, types := p.state.units, p.state.types
	byID := map[string]org.Unit{}
	hasRoot := false
	for _, u := range existing {
		byID[u.ID.String()] = u
		p.units[u.ID.String()] = &slot{u.ID}
		hasRoot = hasRoot || u.ParentID == nil
	}
	typeByName := map[string]uuid.UUID{}
	for _, t := range types {
		typeByName[strings.ToLower(t.Name)] = t.ID
	}
	p.fresh("units", p.units)
	sh := p.sheets["units"]
	for i, row := range sh.rows {
		n, name := sh.nums[i], row["name"]
		if name == "" {
			p.fail("units", n, "Name the unit.")
			continue
		}
		typeID, typed := typeByName[strings.ToLower(row["type"])]
		if row["type"] != "" && !typed {
			p.fail("units", n, "There is no unit type %q.", row["type"])
			continue
		}
		var parent *slot
		if ref := row["parent_ref"]; ref != "" {
			if parent = p.units[ref]; parent == nil {
				p.fail("units", n, "No unit has the ref %q.", ref)
				continue
			}
		}
		if u, ok := byID[row["ref"]]; ok {
			in := org.UnitInput{}
			if u.Name != name {
				in.Name = &name
			}
			if short := row["short_name"]; short != optional(u.ShortName) {
				in.ShortName = &short
			}
			if desc := row["description"]; desc != u.Description {
				in.Description = &desc
			}
			if typed && typeID != u.TypeID {
				in.TypeID = &typeID
			}
			moved := parent != nil && (u.ParentID == nil || parent.id != *u.ParentID)
			if in == (org.UnitInput{}) && !moved {
				p.change("units", n, "unchanged", name)
				continue
			}
			p.change("units", n, "update", name)
			if in != (org.UnitInput{}) {
				p.step("units", n, func(ctx context.Context, s *org.Service) error {
					_, _, err := s.UpdateUnit(ctx, u.ID, in, p.from)
					return err
				})
			}
			if moved {
				p.step("units", n, func(ctx context.Context, s *org.Service) error {
					_, _, err := s.MoveUnit(ctx, u.ID, idOf(parent), p.from)
					return err
				})
			}
			continue
		}
		if !typed {
			p.fail("units", n, "Give the new unit a type.")
			continue
		}
		if parent == nil {
			if hasRoot {
				p.fail("units", n, "The organisation has a root already; give the unit a parent_ref.")
				continue
			}
			hasRoot = true
		}
		p.change("units", n, "create", name)
		in := org.UnitInput{TypeID: &typeID, Name: &name}
		if short := row["short_name"]; short != "" {
			in.ShortName = &short
		}
		if desc := row["description"]; desc != "" {
			in.Description = &desc
		}
		own := p.units[row["ref"]]
		if own == nil {
			own = &slot{}
		}
		p.step("units", n, func(ctx context.Context, s *org.Service) error {
			u, _, err := s.CreateUnit(ctx, in, idOf(parent), p.from)
			own.id = u.ID
			return err
		})
	}
}

func (p *plan) planPositions() {
	existing := p.state.positions
	byID := map[string]org.Position{}
	for _, pos := range existing {
		byID[pos.ID.String()] = pos
		p.posts[pos.ID.String()] = &slot{pos.ID}
	}
	p.fresh("positions", p.posts)
	sh := p.sheets["positions"]
	type line struct {
		n            int
		self, target *slot
	}
	var lines []line
	for i, row := range sh.rows {
		n, title := sh.nums[i], row["title"]
		if title == "" {
			p.fail("positions", n, "Give the position a title.")
			continue
		}
		in := org.PositionInput{}
		if raw := row["seats"]; raw != "" {
			seats, err := strconv.Atoi(raw)
			if err != nil || seats < 1 {
				p.fail("positions", n, "Give the seats as a whole number from 1.")
				continue
			}
			in.Seats = &seats
		}
		if raw := row["head_of_unit"]; raw != "" {
			head, err := strconv.ParseBool(raw)
			if err != nil {
				p.fail("positions", n, "Write head_of_unit as true or false.")
				continue
			}
			in.HeadOfUnit = &head
		}
		var target *slot
		if ref := row["reports_to_ref"]; ref != "" {
			if target = p.posts[ref]; target == nil {
				p.fail("positions", n, "No position has the ref %q.", ref)
				continue
			}
		}
		if pos, ok := byID[row["ref"]]; ok {
			change := org.PositionInput{}
			if pos.Title != title {
				change.Title = &title
			}
			if desc := row["description"]; desc != pos.Description {
				change.Description = &desc
			}
			if in.Seats != nil && *in.Seats != pos.Seats {
				change.Seats = in.Seats
			}
			if in.HeadOfUnit != nil && *in.HeadOfUnit != pos.HeadOfUnit {
				change.HeadOfUnit = in.HeadOfUnit
			}
			relined := target != nil && (pos.ReportsTo == nil || target.id != *pos.ReportsTo)
			if change == (org.PositionInput{}) && !relined {
				p.change("positions", n, "unchanged", title)
				continue
			}
			p.change("positions", n, "update", title)
			if change != (org.PositionInput{}) {
				p.step("positions", n, func(ctx context.Context, s *org.Service) error {
					_, _, err := s.UpdatePosition(ctx, pos.ID, change, p.from)
					return err
				})
			}
			if relined {
				lines = append(lines, line{n: n, self: p.posts[row["ref"]], target: target})
			}
			continue
		}
		unit := p.units[row["unit_ref"]]
		if unit == nil {
			p.fail("positions", n, "No unit has the ref %q.", row["unit_ref"])
			continue
		}
		start, err := day(row["from"], p.from)
		if err != nil {
			p.fail("positions", n, "Write from as a day, such as 2026-01-31.")
			continue
		}
		p.change("positions", n, "create", title)
		in.Title = &title
		if desc := row["description"]; desc != "" {
			in.Description = &desc
		}
		own := p.posts[row["ref"]]
		if own == nil {
			own = &slot{}
		}
		p.step("positions", n, func(ctx context.Context, s *org.Service) error {
			pos, _, err := s.CreatePosition(ctx, unit.id, in, start)
			own.id = pos.ID
			return err
		})
		if target != nil {
			lines = append(lines, line{n: n, self: own, target: target})
		}
	}
	// Lines come after every position exists, so a position may report to one made further down.
	for _, l := range lines {
		p.step("positions", l.n, func(ctx context.Context, s *org.Service) error {
			_, _, err := s.SetReportsTo(ctx, l.self.id, &l.target.id, p.from)
			return err
		})
	}
}

func (p *plan) planPeople() {
	existing := p.state.people
	byID := map[string]org.Person{}
	byEmail := map[string]org.Person{}
	for _, person := range existing {
		byID[person.ID.String()] = person
		p.people[person.ID.String()] = &slot{person.ID}
		if person.Email != nil {
			byEmail[strings.ToLower(*person.Email)] = person
			p.people[strings.ToLower(*person.Email)] = &slot{person.ID}
		}
	}
	p.fresh("people", p.people)
	sh := p.sheets["people"]
	for i, row := range sh.rows {
		n, name := sh.nums[i], row["name"]
		if name == "" {
			p.fail("people", n, "Name the person.")
			continue
		}
		person, ok := byID[row["ref"]]
		if !ok && row["email"] != "" {
			person, ok = byEmail[strings.ToLower(row["email"])]
		}
		fields := map[string]string{"email": row["email"], "phone": row["phone"], "location": row["location"]}
		if ok {
			in := org.PersonInput{}
			if person.Name != name {
				in.Name = &name
			}
			for field, current := range map[string]*string{"email": person.Email, "phone": person.Phone, "location": person.Location} {
				if v := fields[field]; v != "" && v != optional(current) {
					set := v
					switch field {
					case "email":
						in.Email = &set
					case "phone":
						in.Phone = &set
					default:
						in.Location = &set
					}
				}
			}
			if row["ref"] != "" && row["ref"] != person.ID.String() {
				p.people[row["ref"]] = &slot{person.ID}
			}
			if in == (org.PersonInput{}) {
				p.change("people", n, "unchanged", name)
				continue
			}
			p.change("people", n, "update", name)
			p.step("people", n, func(ctx context.Context, s *org.Service) error {
				_, _, err := s.UpdatePerson(ctx, person.ID, in)
				return err
			})
			continue
		}
		p.change("people", n, "create", name)
		in := org.PersonInput{Name: &name}
		for field, v := range fields {
			if v == "" {
				continue
			}
			set := v
			switch field {
			case "email":
				in.Email = &set
			case "phone":
				in.Phone = &set
			default:
				in.Location = &set
			}
		}
		own := p.people[row["ref"]]
		if own == nil {
			own = &slot{}
		}
		if row["email"] != "" {
			p.people[strings.ToLower(row["email"])] = own
		}
		p.step("people", n, func(ctx context.Context, s *org.Service) error {
			made, _, err := s.CreatePerson(ctx, in)
			own.id = made.ID
			return err
		})
	}
}

func (p *plan) planHoldings() {
	existing := p.state.holdings
	type key struct {
		person, position uuid.UUID
		from             string
	}
	held := map[key]org.Holding{}
	for _, h := range existing {
		if h.PositionID != nil {
			held[key{h.PersonID, *h.PositionID, h.From.String()}] = h
		}
	}
	sh := p.sheets["holdings"]
	for i, row := range sh.rows {
		n := sh.nums[i]
		person := p.people[row["person_ref"]]
		if person == nil {
			person = p.people[strings.ToLower(row["person_ref"])]
		}
		if person == nil {
			p.fail("holdings", n, "No person has the ref or address %q.", row["person_ref"])
			continue
		}
		position := p.posts[row["position_ref"]]
		if position == nil {
			p.fail("holdings", n, "No position has the ref %q.", row["position_ref"])
			continue
		}
		start, err := org.ParseDay(row["from"])
		if err != nil {
			p.fail("holdings", n, "Write from as a day, such as 2026-01-31.")
			continue
		}
		var end *org.Day
		if row["to"] != "" {
			to, err := org.ParseDay(row["to"])
			if err != nil {
				p.fail("holdings", n, "Write to as a day, such as 2026-01-31, or leave it empty.")
				continue
			}
			end = &to
		}
		share := 100
		if raw := row["share"]; raw != "" {
			if share, err = strconv.Atoi(raw); err != nil || share < 1 || share > 100 {
				p.fail("holdings", n, "Give the share as a whole number from 1 to 100.")
				continue
			}
		}
		label := row["person_ref"] + " / " + row["position_ref"]
		if person.id != uuid.Nil && position.id != uuid.Nil {
			if _, ok := held[key{person.id, position.id, start.String()}]; ok {
				p.change("holdings", n, "unchanged", label)
				continue
			}
		}
		p.change("holdings", n, "create", label)
		p.step("holdings", n, func(ctx context.Context, s *org.Service) error {
			_, _, err := s.GiveHolding(ctx, org.HoldingInput{PersonID: person.id, PositionID: &position.id, Share: &share, From: start, To: end})
			return err
		})
	}
}

// ApplyError is a refusal the model gave while an import ran, with the row that met it.
type ApplyError struct {
	RowError
	Err error
}

func (e *ApplyError) Error() string { return fmt.Sprintf("%s row %d: %s", e.File, e.Row, e.Message) }
func (e *ApplyError) Unwrap() error { return e.Err }

func sentence(s string) string {
	if s == "" {
		return s
	}
	s = strings.ToUpper(s[:1]) + s[1:]
	if !strings.HasSuffix(s, ".") {
		s += "."
	}
	return s
}
