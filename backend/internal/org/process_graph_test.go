package org

import (
	"slices"
	"testing"

	"github.com/google/uuid"
)

func codes(problems []Problem) []string {
	out := make([]string, len(problems))
	for i, p := range problems {
		out[i] = p.Code
		if p.Node != "" {
			out[i] += "@" + p.Node
		}
		if p.Arrow != "" {
			out[i] += "@" + p.Arrow
		}
	}
	slices.Sort(out)
	return out
}

func approval() Graph {
	return Graph{
		Nodes: []Node{
			{Key: "in", Kind: Start}, {Key: "check", Kind: Step, Name: "Check"}, {Key: "big", Kind: Decision, Name: "Big?"},
			{Key: "approve", Kind: Step, Name: "Approve"}, {Key: "out", Kind: End},
		},
		Arrows: []Arrow{
			{Key: "a1", From: "in", To: "check"}, {Key: "a2", From: "check", To: "big"},
			{Key: "a3", From: "big", To: "approve", Label: "Yes"}, {Key: "a4", From: "big", To: "out", Label: "No"},
			{Key: "a5", From: "approve", To: "out"},
		},
	}
}

func TestAWholeFlowHasNoProblems(t *testing.T) {
	if got := Check(uuid.New(), approval()); len(got) != 0 {
		t.Errorf("a whole flow has %v", codes(got))
	}
	if got := Check(uuid.New(), Graph{Nodes: []Node{{Key: "s", Kind: Start}, {Key: "e", Kind: End}},
		Arrows: []Arrow{{Key: "a", From: "s", To: "e"}}}); len(got) != 0 {
		t.Errorf("a start joined to an end has %v", codes(got))
	}
}

func TestAFlowNeedsOneStartAndAnEnd(t *testing.T) {
	if got := codes(Check(uuid.New(), Graph{})); !slices.Equal(got, []string{"no_end", "no_start"}) {
		t.Errorf("an empty graph has %v", got)
	}
	g := approval()
	g.Nodes = append(g.Nodes, Node{Key: "again", Kind: Start})
	g.Arrows = append(g.Arrows, Arrow{Key: "a6", From: "again", To: "check"}, Arrow{Key: "a7", From: "out", To: "in"})
	got := codes(Check(uuid.New(), g))
	if !slices.Equal(got, []string{"into_start@in", "many_starts@again", "out_of_end@out"}) {
		t.Errorf("two starts and arrows round the ends have %v", got)
	}
}

func TestEveryNodeLiesOnAWayFromTheStartToAnEnd(t *testing.T) {
	g := approval()
	g.Nodes = append(g.Nodes, Node{Key: "lost", Kind: Step, Name: "Lost"}, Node{Key: "stuck", Kind: Step, Name: "Stuck"})
	g.Arrows = append(g.Arrows, Arrow{Key: "a6", From: "check", To: "stuck"})
	got := codes(Check(uuid.New(), g))
	if !slices.Equal(got, []string{"dead_end@stuck", "unreachable@lost"}) {
		t.Errorf("a lost and a stuck step have %v", got)
	}
	g = approval()
	g.Arrows = append(g.Arrows, Arrow{Key: "back", From: "approve", To: "check"})
	if got := Check(uuid.New(), g); len(got) != 0 {
		t.Errorf("a loop back that still leads on has %v", codes(got))
	}
}

func TestDecisionsAndSubprocessesSayEnough(t *testing.T) {
	self := uuid.New()
	g := approval()
	g.Arrows[3].Label = ""
	g.Nodes[1].Name = ""
	g.Nodes = append(g.Nodes, Node{Key: "one-way", Kind: Decision, Name: "Only one way?"},
		Node{Key: "sub", Kind: Subprocess, Name: "Elsewhere"}, Node{Key: "me", Kind: Subprocess, Name: "Me", SubprocessID: &self})
	g.Arrows = append(g.Arrows, Arrow{Key: "a6", From: "approve", To: "one-way"}, Arrow{Key: "a7", From: "one-way", To: "sub", Label: "Yes"},
		Arrow{Key: "a8", From: "sub", To: "me"}, Arrow{Key: "a9", From: "me", To: "out"})
	got := codes(Check(self, g))
	want := []string{"decision_ways@one-way", "no_subprocess@sub", "own_subprocess@me", "unlabelled_way@a4", "unnamed@check"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestTheStoreHoldsOnlyAWellFormedGraph(t *testing.T) {
	sub := uuid.New()
	cases := map[string]func(g *Graph){
		"bad key":         func(g *Graph) { g.Nodes[0].Key = "with space" },
		"key twice":       func(g *Graph) { g.Nodes[1].Key = "in" },
		"unknown kind":    func(g *Graph) { g.Nodes[1].Kind = "gateway" },
		"long name":       func(g *Graph) { g.Nodes[1].Name = string(make([]rune, 201)) },
		"sub on a step":   func(g *Graph) { g.Nodes[1].SubprocessID = &sub },
		"arrow key twice": func(g *Graph) { g.Arrows[1].Key = "a1" },
		"to nowhere":      func(g *Graph) { g.Arrows[0].To = "nowhere" },
		"into itself":     func(g *Graph) { g.Arrows[0].To = "in" },
		"same way twice":  func(g *Graph) { g.Arrows = append(g.Arrows, Arrow{Key: "a6", From: "in", To: "check"}) },
		"bad arrow key":   func(g *Graph) { g.Arrows[0].Key = "" },
	}
	for name, spoil := range cases {
		g := approval()
		spoil(&g)
		if checkShape(g) == nil {
			t.Errorf("%s was taken", name)
		}
	}
	if err := checkShape(approval()); err != nil {
		t.Errorf("a whole flow was refused: %v", err)
	}
	if checkShape(Graph{Nodes: make([]Node, maxNodes+1)}) == nil || checkShape(Graph{Arrows: make([]Arrow, maxArrows+1)}) == nil {
		t.Error("a graph past the bounds was taken")
	}
}

func TestAProcessIsMadeWithAKeyAndAName(t *testing.T) {
	key, name, long := "order-to-cash", "Order to cash", string(make([]rune, 4001))
	if err := (&ProcessInput{Key: &key, Name: &name}).check(true); err != nil {
		t.Errorf("a key and a name were refused: %v", err)
	}
	bad := "Order To Cash"
	for label, in := range map[string]ProcessInput{
		"no key": {Name: &name}, "no name": {Key: &key}, "bad key": {Key: &bad, Name: &name},
		"long description": {Key: &key, Name: &name, Description: &long},
	} {
		if (&in).check(true) == nil {
			t.Errorf("%s was taken", label)
		}
	}
	if (&ProcessInput{}).check(false) != nil {
		t.Error("a change of nothing was refused")
	}
}
