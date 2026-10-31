// Package demo loads a demo organisation into an installation that has none yet, to look at and to test against.
package demo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/db"
	"github.com/praetorianer777/bearing/backend/internal/org"
)

var (
	ErrLoaded   = errors.New("the demo is loaded already")
	ErrNotEmpty = errors.New("the installation has an organisation or processes of its own; the demo goes only where there are none")
)

// Load adds what of the demo may go in, as an administrator, all of it in one transaction or nothing: the
// organisation into an installation without units, and its processes into one without processes whose organisation
// is the demo's. Each part goes in once, and not again even after it is changed or removed.
func Load(ctx context.Context, s *org.Service) (db.LSN, error) {
	return s.Together(ctx, func(ctx context.Context, s *org.Service) error {
		return load(ctx, s)
	})
}

func load(ctx context.Context, s *org.Service) error {
	state, err := s.DemoState(ctx)
	if err != nil {
		return err
	}
	organisation := !state.Organisation.Loaded && state.Organisation.Empty
	flows := !state.Processes.Loaded && state.Processes.Empty && (state.Organisation.Loaded || organisation)
	switch {
	case !organisation && !flows && state.Organisation.Loaded && state.Processes.Loaded:
		return ErrLoaded
	case !organisation && !flows:
		return ErrNotEmpty
	}
	if organisation {
		if err := loadOrganisation(ctx, s); err != nil {
			return err
		}
	}
	if flows {
		return loadProcesses(ctx, s)
	}
	return nil
}

func loadOrganisation(ctx context.Context, s *org.Service) error {
	start := mustDay(since)
	kindIDs, err := unitTypes(ctx, s)
	if err != nil {
		return err
	}

	unitIDs := map[string]uuid.UUID{}
	for _, u := range units {
		var parent *uuid.UUID
		if u.parent != "" {
			id := unitIDs[u.parent]
			parent = &id
		}
		kind := kindIDs[u.kind]
		made, _, err := s.CreateUnit(ctx, org.UnitInput{TypeID: &kind, Name: ptr(u.name), ShortName: ptr(u.short)}, parent, start)
		if err != nil {
			return fmt.Errorf("unit %s: %w", u.key, err)
		}
		unitIDs[u.key] = made.ID
	}

	positionIDs := map[string]uuid.UUID{}
	heads := map[string]string{}
	for _, p := range positions {
		in := org.PositionInput{Title: ptr(p.title), HeadOfUnit: ptr(p.head)}
		if p.seats > 0 {
			in.Seats = ptr(p.seats)
		}
		made, _, err := s.CreatePosition(ctx, unitIDs[p.unit], in, start)
		if err != nil {
			return fmt.Errorf("position %s: %w", p.key, err)
		}
		positionIDs[p.key] = made.ID
		if p.head {
			heads[p.unit] = p.key
		}
	}
	parents := map[string]string{}
	for _, u := range units {
		parents[u.key] = u.parent
	}
	for _, p := range positions {
		to := p.reportsTo
		switch {
		case to != "":
		case p.head && parents[p.unit] != "":
			to = heads[parents[p.unit]]
		case !p.head:
			to = heads[p.unit]
		}
		if to != "" {
			target := positionIDs[to]
			if _, _, err := s.SetReportsTo(ctx, positionIDs[p.key], &target, start); err != nil {
				return fmt.Errorf("line from %s to %s: %w", p.key, to, err)
			}
		}
		for _, d := range p.dottedTo {
			if _, _, err := s.AddDottedLine(ctx, positionIDs[p.key], positionIDs[d], start); err != nil {
				return fmt.Errorf("dotted line from %s to %s: %w", p.key, d, err)
			}
		}
	}

	personIDs := map[string]uuid.UUID{}
	for _, p := range people {
		made, _, err := s.CreatePerson(ctx, org.PersonInput{Name: ptr(p.name), Email: ptr(p.email), Location: ptr(p.location)})
		if err != nil {
			return fmt.Errorf("person %s: %w", p.key, err)
		}
		personIDs[p.key] = made.ID
	}

	roleIDs := map[string]uuid.UUID{}
	for _, r := range roles {
		made, _, err := s.CreateRole(ctx, org.RoleInput{Key: ptr(r.key), Name: ptr(r.name), Description: ptr(r.description)})
		if err != nil {
			return fmt.Errorf("role %s: %w", r.key, err)
		}
		roleIDs[r.key] = made.ID
		for _, p := range r.carriedBy {
			if _, _, err := s.CarryRole(ctx, positionIDs[p], made.ID, start); err != nil {
				return fmt.Errorf("role %s on %s: %w", r.key, p, err)
			}
		}
	}

	for _, h := range holdings {
		in := org.HoldingInput{PersonID: personIDs[h.person], From: start}
		if h.position != "" {
			id := positionIDs[h.position]
			in.PositionID = &id
		} else {
			id := roleIDs[h.role]
			in.RoleID = &id
		}
		if h.share > 0 {
			in.Share = ptr(h.share)
		}
		if h.from != "" {
			in.From = mustDay(h.from)
		}
		if h.to != "" {
			to := mustDay(h.to)
			in.To = &to
		}
		if _, _, err := s.GiveHolding(ctx, in); err != nil {
			return fmt.Errorf("holding of %s%s by %s: %w", h.position, h.role, h.person, err)
		}
	}
	for person, on := range leavers {
		if _, _, err := s.Leave(ctx, personIDs[person], mustDay(on)); err != nil {
			return fmt.Errorf("%s leaving: %w", person, err)
		}
	}
	for position, stand := range deputies {
		ids := make([]uuid.UUID, len(stand))
		for i, d := range stand {
			ids[i] = positionIDs[d]
		}
		if _, _, err := s.SetDeputies(ctx, positionIDs[position], ids); err != nil {
			return fmt.Errorf("deputies of %s: %w", position, err)
		}
	}
	return s.MarkDemoLoaded(ctx)
}

// unitTypes finds the demo's kinds of unit by name, adding those the installation lacks.
func unitTypes(ctx context.Context, s *org.Service) (map[string]uuid.UUID, error) {
	have, err := s.UnitTypes(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]uuid.UUID{}
	for _, k := range kinds {
		for _, t := range have {
			if strings.EqualFold(t.Name, k.name) {
				out[k.name] = t.ID
			}
		}
		if _, ok := out[k.name]; ok {
			continue
		}
		made, _, err := s.CreateUnitType(ctx, org.UnitTypeInput{Name: ptr(k.name), Icon: ptr(k.icon), Colour: ptr(k.colour), Rank: ptr(k.rank)})
		if err != nil {
			return nil, fmt.Errorf("unit type %s: %w", k.name, err)
		}
		out[k.name] = made.ID
	}
	return out, nil
}

// loadProcesses adds the demo's processes, publishing their versions in turn, each owned by the position of its
// owner's title today.
func loadProcesses(ctx context.Context, s *org.Service) error {
	today := org.DayOf(time.Now())
	positions, err := s.Positions(ctx, today, nil, false)
	if err != nil {
		return err
	}
	units, err := s.Units(ctx, today)
	if err != nil {
		return err
	}
	roles, err := s.Roles(ctx)
	if err != nil {
		return err
	}
	known := parties{positions: map[string]uuid.UUID{}, units: map[string]uuid.UUID{}, roles: map[string]uuid.UUID{}}
	for _, p := range positions {
		known.positions[p.Title] = p.ID
	}
	for _, u := range units {
		known.units[u.Name] = u.ID
	}
	for _, r := range roles {
		known.roles[r.Key] = r.ID
	}
	owners := known.positions
	ids := map[string]uuid.UUID{}
	for _, p := range processes {
		in := org.ProcessInput{Key: ptr(p.key), Name: ptr(p.name), Area: ptr(p.area), Description: ptr(p.description)}
		if id, ok := owners[p.owner]; ok {
			in.OwnerID = &id
		}
		made, _, err := s.CreateProcess(ctx, in)
		if err != nil {
			return fmt.Errorf("process %s: %w", p.key, err)
		}
		ids[p.key] = made.ID
	}
	for _, p := range processes {
		id := ids[p.key]
		save := func(f flow) error {
			current, err := s.Process(ctx, id)
			if err != nil {
				return err
			}
			g, err := f.graph(ids, known)
			if err != nil {
				return err
			}
			_, _, err = s.SaveDraft(ctx, id, current.Draft.Revision, g)
			return err
		}
		for i, f := range p.versions {
			if err := save(f); err != nil {
				return fmt.Errorf("version %d of %s: %w", i+1, p.key, err)
			}
			if _, _, err := s.Publish(ctx, id, f.note); err != nil {
				return fmt.Errorf("publishing version %d of %s: %w", i+1, p.key, err)
			}
		}
		if p.draft != nil {
			if err := save(*p.draft); err != nil {
				return fmt.Errorf("draft of %s: %w", p.key, err)
			}
		}
	}
	return s.MarkDemoProcessesLoaded(ctx)
}

// widths and heights are those the web client draws each kind of node at, so a row's nodes share a centre line and
// the routes meet their nodes' edges.
var (
	widths  = map[org.NodeKind]float64{org.Start: 160, org.End: 160, org.Step: 180, org.Subprocess: 180, org.Decision: 180}
	heights = map[org.NodeKind]float64{org.Start: 44, org.End: 44, org.Step: 56, org.Subprocess: 60, org.Decision: 90}
)

const (
	rowHeight = 150
	halfSpan  = 150
	// elbow is how far above its target a route turns across, below the row before.
	elbow = 30
	lane  = 40
)

// parties are the positions by title, the units by name and the roles by key a demo responsibility may name.
type parties struct{ positions, units, roles map[string]uuid.UUID }

func (p parties) responsibility(r demoRACI) (org.Responsibility, error) {
	out := org.Responsibility{Kind: r.kind}
	kind, name, scoped := strings.Cut(r.party, ":")
	var id uuid.UUID
	var ok bool
	switch {
	case scoped && kind == "role":
		id, ok = p.roles[name]
		out.RoleID = &id
	case scoped && kind == "unit":
		id, ok = p.units[name]
		out.UnitID = &id
	default:
		id, ok = p.positions[r.party]
		out.PositionID = &id
	}
	if !ok {
		return out, fmt.Errorf("no such party as %s", r.party)
	}
	return out, nil
}

// rows lays a flow out in rows: a walk from the start finds the arrows leading back up, and each node goes a row
// below the farthest node leading to it by the others, so a loop back never runs down through the flow. The nodes
// come in the order the walk reaches them, which puts each row's nodes side by side in the order their ways leave.
func (f flow) rows() (map[string]int, []demoNode) {
	next := map[string][]string{}
	for _, a := range f.arrows {
		next[a.from] = append(next[a.from], a.to)
	}
	const (
		unseen = iota
		open
		done
	)
	state := map[string]int{}
	back := map[[2]string]bool{}
	var reached, finished []string
	var walk func(string)
	walk = func(k string) {
		state[k] = open
		reached = append(reached, k)
		for _, to := range next[k] {
			switch state[to] {
			case open:
				back[[2]string{k, to}] = true
			case unseen:
				walk(to)
			}
		}
		state[k] = done
		finished = append(finished, k)
	}
	for _, n := range f.nodes {
		if n.kind == org.Start {
			walk(n.key)
		}
	}
	row := map[string]int{}
	for i := len(finished) - 1; i >= 0; i-- {
		k := finished[i]
		for _, to := range next[k] {
			if !back[[2]string{k, to}] && row[to] < row[k]+1 {
				row[to] = row[k] + 1
			}
		}
	}
	byKey := map[string]demoNode{}
	for _, n := range f.nodes {
		byKey[n.key] = n
	}
	order := make([]demoNode, 0, len(f.nodes))
	for _, k := range reached {
		order = append(order, byKey[k])
	}
	for _, n := range f.nodes {
		if state[n.key] == unseen {
			order = append(order, n)
		}
	}
	return row, order
}

// graph lays a flow out top to bottom, row by row, the nodes of a row side by side.
func (f flow) graph(ids map[string]uuid.UUID, known parties) (org.Graph, error) {
	row, order := f.rows()
	across := map[int]int{}
	for _, n := range f.nodes {
		across[row[n.key]]++
	}
	placed := map[int]int{}
	var g org.Graph
	for _, n := range order {
		r := row[n.key]
		centre := float64(placed[r]*2-across[r]+1) * halfSpan
		node := org.Node{Key: n.key, Kind: n.kind, Name: n.name, X: centre - widths[n.kind]/2, Y: float64(r) * rowHeight}
		placed[r]++
		if n.sub != "" {
			id := ids[n.sub]
			node.SubprocessID = &id
		}
		for _, r := range n.raci {
			resp, err := known.responsibility(r)
			if err != nil {
				return g, fmt.Errorf("%s: %w", n.key, err)
			}
			node.Responsibilities = append(node.Responsibilities, resp)
		}
		g.Nodes = append(g.Nodes, node)
	}
	byKey := map[string]org.Node{}
	right := 0.0
	for _, n := range g.Nodes {
		byKey[n.Key] = n
		right = max(right, n.X+widths[n.Kind])
	}
	loops := 0
	for _, a := range f.arrows {
		s, t := byKey[a.from], byKey[a.to]
		g.Arrows = append(g.Arrows, org.Arrow{Key: a.from + "-" + a.to, From: a.from, To: a.to, Label: a.label,
			Route: route(s, t, g.Nodes, row[a.to] <= row[a.from], right+lane*float64(1+loops))})
		if row[a.to] <= row[a.from] {
			loops++
		}
	}
	return g, nil
}

// route is the way an arrow takes between two placed nodes. Down the flow it leaves the bottom, turns across just
// above its target and enters the top; when a node stands in that way, it turns aside just below its source and runs
// down the nearest clear column instead, the lane right of every node at the last. A loop back leaves the right side,
// runs up its lane and enters the target's right side. Turns lie between the rows, so no route crosses a node.
func route(s, t org.Node, nodes []org.Node, back bool, laneX float64) []float64 {
	sx, tx := s.X+widths[s.Kind]/2, t.X+widths[t.Kind]/2
	bottom := s.Y + heights[s.Kind]
	if back {
		sy, ty := s.Y+heights[s.Kind]/2, t.Y+heights[t.Kind]/2
		return []float64{s.X + widths[s.Kind], sy, laneX, sy, laneX, ty, t.X + widths[t.Kind], ty}
	}
	straight := []float64{sx, bottom, sx, t.Y - elbow, tx, t.Y - elbow, tx, t.Y}
	if sx == tx {
		straight = []float64{sx, bottom, tx, t.Y}
	}
	if clear(straight, nodes, s, t) {
		return straight
	}
	for step := 1.0; step <= 4; step++ {
		for _, x := range []float64{sx + step*halfSpan/2, sx - step*halfSpan/2} {
			aside := []float64{sx, bottom, sx, bottom + elbow, x, bottom + elbow, x, t.Y - elbow, tx, t.Y - elbow, tx, t.Y}
			if clear(aside, nodes, s, t) {
				return aside
			}
		}
	}
	return []float64{sx, bottom, sx, bottom + elbow, laneX, bottom + elbow, laneX, t.Y - elbow, tx, t.Y - elbow, tx, t.Y}
}

// clear says whether a route keeps off every node but the two it joins, with a margin.
func clear(route []float64, nodes []org.Node, s, t org.Node) bool {
	const margin = 12
	for j := 0; j+3 < len(route); j += 2 {
		x1, y1, x2, y2 := route[j], route[j+1], route[j+2], route[j+3]
		for _, n := range nodes {
			if n.Key == s.Key || n.Key == t.Key {
				continue
			}
			left, top, right, bottom := n.X-margin, n.Y-margin, n.X+widths[n.Kind]+margin, n.Y+heights[n.Kind]+margin
			if min(x1, x2) < right && max(x1, x2) > left && min(y1, y2) < bottom && max(y1, y2) > top {
				return false
			}
		}
	}
	return true
}

func mustDay(s string) org.Day {
	d, err := org.ParseDay(s)
	if err != nil {
		panic(err)
	}
	return d
}

func ptr[T any](v T) *T { return &v }
