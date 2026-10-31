package org

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"unicode/utf8"

	"github.com/google/uuid"
)

// NodeKind is what a node of a process stands for.
type NodeKind string

const (
	Start      NodeKind = "start"
	End        NodeKind = "end"
	Step       NodeKind = "step"
	Decision   NodeKind = "decision"
	Subprocess NodeKind = "subprocess"
)

// NodeKinds are the kinds a node may be, in the order an editor offers them.
var NodeKinds = []NodeKind{Start, Step, Decision, Subprocess, End}

// RACI is how a party stands to a node: responsible does the work, accountable answers for it, consulted is asked
// before and informed is told after.
type RACI string

const (
	Responsible RACI = "responsible"
	Accountable RACI = "accountable"
	Consulted   RACI = "consulted"
	Informed    RACI = "informed"
)

// RACIs are the kinds in the order a matrix shows them.
var RACIs = []RACI{Responsible, Accountable, Consulted, Informed}

// Responsibility is a party's part in a node: a role, a position or a unit, exactly one of them.
type Responsibility struct {
	Kind       RACI
	RoleID     *uuid.UUID
	PositionID *uuid.UUID
	UnitID     *uuid.UUID
}

// party is the one of role, position and unit a responsibility names, as a comparable key.
func (r Responsibility) party() string {
	switch {
	case r.RoleID != nil:
		return "role:" + r.RoleID.String()
	case r.PositionID != nil:
		return "position:" + r.PositionID.String()
	case r.UnitID != nil:
		return "unit:" + r.UnitID.String()
	}
	return ""
}

// Node is one node of a process graph.
type Node struct {
	// Key names the node within its version; the editor chooses it.
	Key         string
	Kind        NodeKind
	Name        string
	Description string
	// X and Y are where it was left on the canvas.
	X, Y float64
	// SubprocessID is the process a subprocess node stands for.
	SubprocessID *uuid.UUID
	// Responsibilities say who does, answers for, is asked about and is told of what the node stands for.
	Responsibilities []Responsibility
}

// Arrow leads from one node to the next; out of a decision, Label is the condition it is taken on.
type Arrow struct {
	Key   string
	From  string
	To    string
	Label string
	// Route is the way the layout routed it, x and y of each point in turn; empty when it has none.
	Route []float64
}

// Graph is what a version of a process draws: its nodes and the arrows between them.
type Graph struct {
	Nodes  []Node
	Arrows []Arrow
}

// Problem is something keeping a draft from being published, on a node or an arrow when it is about one. A warning
// does not keep it from being published; it is worth a look first.
type Problem struct {
	Code    string
	Message string
	Node    string
	Arrow   string
	Warning bool
}

// Blocking says whether any of the problems keeps a draft from being published.
func Blocking(problems []Problem) bool {
	for _, p := range problems {
		if !p.Warning {
			return true
		}
	}
	return false
}

const (
	maxNodes            = 500
	maxArrows           = 1000
	maxResponsibilities = 50
	maxRoutePoints      = 100
)

var graphKey = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// checkShape refuses a graph the store cannot hold: unknown kinds, keys used twice, arrows to nowhere. What a draft
// may still lack on the way to being published is a Problem, not this.
func checkShape(g Graph) error {
	if len(g.Nodes) > maxNodes {
		return Invalid{"nodes", fmt.Sprintf("Keep a process to %d nodes.", maxNodes)}
	}
	if len(g.Arrows) > maxArrows {
		return Invalid{"arrows", fmt.Sprintf("Keep a process to %d arrows.", maxArrows)}
	}
	nodes := make(map[string]bool, len(g.Nodes))
	for _, n := range g.Nodes {
		switch {
		case !graphKey.MatchString(n.Key):
			return Invalid{"nodes", "Name each node by letters, digits, dashes and underscores, up to 64."}
		case nodes[n.Key]:
			return Invalid{"nodes", "Two nodes have the key " + n.Key + "."}
		case !validKind(n.Kind):
			return Invalid{"nodes", "A node is a start, a step, a decision, a subprocess or an end."}
		case utf8.RuneCountInString(n.Name) > 200:
			return Invalid{"nodes", "Keep a node's name to 200 characters."}
		case utf8.RuneCountInString(n.Description) > 4000:
			return Invalid{"nodes", "Keep a node's description to 4000 characters."}
		case n.SubprocessID != nil && n.Kind != Subprocess:
			return Invalid{"nodes", "Only a subprocess node names another process."}
		}
		if err := checkResponsibilities(n); err != nil {
			return err
		}
		nodes[n.Key] = true
	}
	arrows := make(map[string]bool, len(g.Arrows))
	pairs := make(map[[2]string]bool, len(g.Arrows))
	for _, a := range g.Arrows {
		switch {
		case !graphKey.MatchString(a.Key):
			return Invalid{"arrows", "Name each arrow by letters, digits, dashes and underscores, up to 64."}
		case arrows[a.Key]:
			return Invalid{"arrows", "Two arrows have the key " + a.Key + "."}
		case !nodes[a.From] || !nodes[a.To]:
			return Invalid{"arrows", "The arrow " + a.Key + " must lead from a node to a node."}
		case a.From == a.To:
			return Invalid{"arrows", "The arrow " + a.Key + " leads back into its own node."}
		case pairs[[2]string{a.From, a.To}]:
			return Invalid{"arrows", "Two arrows lead from " + a.From + " to " + a.To + "."}
		case utf8.RuneCountInString(a.Label) > 200:
			return Invalid{"arrows", "Keep an arrow's condition to 200 characters."}
		case len(a.Route)%2 != 0 || len(a.Route) > 2*maxRoutePoints || (len(a.Route) > 0 && len(a.Route) < 4):
			return Invalid{"arrows", fmt.Sprintf("The route of the arrow %s is pairs of x and y, from 2 to %d points.", a.Key, maxRoutePoints)}
		}
		for _, v := range a.Route {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return Invalid{"arrows", "The route of the arrow " + a.Key + " has a point that is not a number."}
			}
		}
		arrows[a.Key] = true
		pairs[[2]string{a.From, a.To}] = true
	}
	return nil
}

func checkResponsibilities(n Node) error {
	if len(n.Responsibilities) == 0 {
		return nil
	}
	switch {
	case n.Kind == Start || n.Kind == End:
		return Invalid{"nodes", "A start or an end has no responsibilities."}
	case len(n.Responsibilities) > maxResponsibilities:
		return Invalid{"nodes", fmt.Sprintf("Keep a node to %d responsibilities.", maxResponsibilities)}
	}
	seen := map[string]bool{}
	for _, r := range n.Responsibilities {
		party := r.party()
		named := 0
		for _, id := range []*uuid.UUID{r.RoleID, r.PositionID, r.UnitID} {
			if id != nil {
				named++
			}
		}
		switch {
		case !slices.Contains(RACIs, r.Kind):
			return Invalid{"nodes", "A responsibility is responsible, accountable, consulted or informed."}
		case named != 1:
			return Invalid{"nodes", "A responsibility names one role, position or unit."}
		case seen[string(r.Kind)+party]:
			return Invalid{"nodes", "The node " + n.Key + " names the same responsibility twice."}
		}
		seen[string(r.Kind)+party] = true
	}
	return nil
}

func validKind(k NodeKind) bool {
	for _, known := range NodeKinds {
		if k == known {
			return true
		}
	}
	return false
}

// Check finds what keeps a graph from being published as a version of the process self: it needs one start, every
// node reachable from it and an end reachable from every node, names on what is not a start or an end, two ways or
// more out of a decision, each with its condition, and another process behind each subprocess node. It warns of a
// step without exactly one accountable or without anyone responsible.
func Check(self uuid.UUID, g Graph) []Problem {
	var problems []Problem
	add := func(code, node, arrow, format string, args ...any) {
		problems = append(problems, Problem{Code: code, Message: fmt.Sprintf(format, args...), Node: node, Arrow: arrow})
	}
	warn := func(code, node, message string) {
		problems = append(problems, Problem{Code: code, Message: message, Node: node, Warning: true})
	}
	outOf := map[string][]Arrow{}
	into := map[string][]Arrow{}
	for _, a := range g.Arrows {
		outOf[a.From] = append(outOf[a.From], a)
		into[a.To] = append(into[a.To], a)
	}
	var starts []string
	ends := 0
	for _, n := range g.Nodes {
		switch n.Kind {
		case Start:
			starts = append(starts, n.Key)
			if len(into[n.Key]) > 0 {
				add("into_start", n.Key, "", "No arrow leads into the start.")
			}
		case End:
			ends++
			if len(outOf[n.Key]) > 0 {
				add("out_of_end", n.Key, "", "No arrow leads out of an end.")
			}
		default:
			if n.Name == "" {
				add("unnamed", n.Key, "", "Every %s needs a name.", n.Kind)
			}
		}
		switch n.Kind {
		case Step:
			accountable, responsible := 0, 0
			for _, r := range n.Responsibilities {
				switch r.Kind {
				case Accountable:
					accountable++
				case Responsible:
					responsible++
				}
			}
			switch {
			case accountable == 0:
				warn("no_accountable", n.Key, "Nobody is accountable for this step.")
			case accountable > 1:
				warn("many_accountable", n.Key, "One party only is accountable for a step.")
			}
			if responsible == 0 {
				warn("no_responsible", n.Key, "Nobody is responsible for this step.")
			}
		case Decision:
			if len(outOf[n.Key]) < 2 {
				add("decision_ways", n.Key, "", "A decision needs two ways out or more.")
			}
			for _, a := range outOf[n.Key] {
				if a.Label == "" {
					add("unlabelled_way", "", a.Key, "Every way out of a decision needs its condition.")
				}
			}
		case Subprocess:
			switch {
			case n.SubprocessID == nil:
				add("no_subprocess", n.Key, "", "A subprocess node names the process it stands for.")
			case *n.SubprocessID == self:
				add("own_subprocess", n.Key, "", "A process is not a subprocess of itself.")
			}
		}
	}
	switch {
	case len(starts) == 0:
		add("no_start", "", "", "A process needs a start.")
	case len(starts) > 1:
		for _, s := range starts[1:] {
			add("many_starts", s, "", "A process has one start only.")
		}
	}
	if ends == 0 {
		add("no_end", "", "", "A process needs an end.")
	}
	if len(starts) == 0 || ends == 0 {
		return problems
	}

	reached := walkFrom([]string{starts[0]}, func(k string) []string { return heads(outOf[k]) })
	var endKeys []string
	for _, n := range g.Nodes {
		if n.Kind == End {
			endKeys = append(endKeys, n.Key)
		}
	}
	finishing := walkFrom(endKeys, func(k string) []string { return tails(into[k]) })
	for _, n := range g.Nodes {
		if n.Kind == Start && n.Key != starts[0] {
			continue
		}
		if !reached[n.Key] {
			add("unreachable", n.Key, "", "Nothing leads from the start to here.")
		} else if !finishing[n.Key] {
			add("dead_end", n.Key, "", "No way leads from here to an end.")
		}
	}
	return problems
}

func walkFrom(from []string, next func(string) []string) map[string]bool {
	seen := map[string]bool{}
	queue := append([]string(nil), from...)
	for _, k := range from {
		seen[k] = true
	}
	for len(queue) > 0 {
		k := queue[0]
		queue = queue[1:]
		for _, n := range next(k) {
			if !seen[n] {
				seen[n] = true
				queue = append(queue, n)
			}
		}
	}
	return seen
}

func heads(arrows []Arrow) []string {
	out := make([]string, len(arrows))
	for i, a := range arrows {
		out[i] = a.To
	}
	return out
}

func tails(arrows []Arrow) []string {
	out := make([]string, len(arrows))
	for i, a := range arrows {
		out[i] = a.From
	}
	return out
}
