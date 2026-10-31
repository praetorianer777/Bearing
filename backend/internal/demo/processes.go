package demo

import (
	"slices"

	"github.com/praetorianer777/bearing/backend/internal/org"
)

// demoProcess is one of Kestrel Works' processes: the versions published in turn, then a draft left unpublished, if
// any. The owner is named by the position's title.
type demoProcess struct {
	key, name, area, owner, description string
	versions                            []flow
	draft                               *flow
}

type flow struct {
	note   string
	nodes  []demoNode
	arrows []demoArrow
}

type demoNode struct {
	key  string
	kind org.NodeKind
	name string
	// sub is the key of the process a subprocess node stands for.
	sub  string
	raci []demoRACI
}

// demoRACI names its party as "role:<key>", "unit:<name>" or a position's title.
type demoRACI struct {
	kind  org.RACI
	party string
}

func (n demoNode) with(kind org.RACI, parties []string) demoNode {
	raci := slices.Clone(n.raci)
	for _, p := range parties {
		raci = append(raci, demoRACI{kind, p})
	}
	n.raci = raci
	return n
}

func (n demoNode) r(parties ...string) demoNode { return n.with(org.Responsible, parties) }
func (n demoNode) a(parties ...string) demoNode { return n.with(org.Accountable, parties) }
func (n demoNode) c(parties ...string) demoNode { return n.with(org.Consulted, parties) }
func (n demoNode) i(parties ...string) demoNode { return n.with(org.Informed, parties) }

type demoArrow struct{ from, to, label string }

func start(key, name string) demoNode    { return demoNode{key: key, kind: org.Start, name: name} }
func end(key, name string) demoNode      { return demoNode{key: key, kind: org.End, name: name} }
func step(key, name string) demoNode     { return demoNode{key: key, kind: org.Step, name: name} }
func decision(key, name string) demoNode { return demoNode{key: key, kind: org.Decision, name: name} }
func sub(key, name, process string) demoNode {
	return demoNode{key: key, kind: org.Subprocess, name: name, sub: process}
}

func line(keys ...string) []demoArrow {
	out := make([]demoArrow, len(keys)-1)
	for i := range out {
		out[i] = demoArrow{from: keys[i], to: keys[i+1]}
	}
	return out
}

func join(groups ...[]demoArrow) []demoArrow {
	var out []demoArrow
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

var (
	orderNodes = []demoNode{
		start("received", "Order received"),
		step("check", "Check the order").r("Sales representative").a("Inside sales lead"),
		decision("stock", "In stock?"),
		sub("produce", "Production run", "production-run").i("unit:Sales"),
		step("pack", "Pick and pack").r("Assembly technician").a("Assembly lead"),
		step("ship", "Ship and invoice").r("Accountant").a("Head of Sales").c("Account manager").i("unit:Sales"),
		end("delivered", "Delivered"),
	}
	orderWays = join(line("received", "check", "stock"),
		[]demoArrow{{"stock", "pack", "In stock"}, {"stock", "produce", "Not in stock"}},
		line("produce", "pack", "ship", "delivered"))

	onboardingNodes = []demoNode{
		start("signed", "Contract signed"),
		step("workplace", "Prepare the workplace").r("Executive assistant").a("People partner"),
		step("accounts", "Create the accounts").r("Site reliability engineer").a("Infrastructure lead").c("role:data-protection-officer"),
		step("welcome", "Welcome on the first day").r("People partner").a("Head of Finance and People"),
		step("training", "Agree the training plan").r("People partner").a("Head of Finance and People").i("unit:Finance and People"),
		end("settled", "Settled in"),
	}
)

var processes = []demoProcess{
	{
		key: "production-run", name: "Production run", area: "Operations", owner: "Head of Manufacturing",
		description: "From a production order to tested sensors in stock.",
		versions: []flow{{
			note: "First version.",
			nodes: []demoNode{
				start("order", "Production order"),
				step("plan", "Plan the run").r("Production planner").a("Head of Manufacturing").i("unit:Assembly"),
				step("assemble", "Assemble the sensors").r("Assembly technician").a("Assembly lead").c("Production planner"),
				step("test", "Test the batch").r("Quality engineer").a("Test lab lead").i("Assembly lead"),
				decision("tolerance", "Within tolerance?").a("Test lab lead"),
				step("rework", "Rework").r("Assembly technician").a("Assembly lead").c("Quality engineer"),
				step("release", "Release to stock").r("Production planner").a("Head of Manufacturing").i("unit:Sales"),
				end("stocked", "In stock"),
			},
			arrows: join(line("order", "plan", "assemble", "test", "tolerance"),
				[]demoArrow{{"tolerance", "release", "Yes"}, {"tolerance", "rework", "No"}, {"rework", "test", ""}},
				line("release", "stocked")),
		}},
	},
	{
		key: "order-to-delivery", name: "Order to delivery", area: "Sales", owner: "Head of Commercial",
		description: "How a customer's order becomes a delivered and invoiced shipment.",
		versions: []flow{
			{note: "First version.", nodes: orderNodes, arrows: orderWays},
			{
				note: "Check the customer's credit before anything is picked or made.",
				nodes: append(append([]demoNode(nil), orderNodes...),
					decision("credit", "Within the credit limit?").a("Inside sales lead").c("Accountant"),
					step("prepay", "Ask for prepayment").r("Accountant").a("Head of Finance and People").i("Sales representative")),
				arrows: join(line("received", "check", "credit"),
					[]demoArrow{{"credit", "stock", "Within the limit"}, {"credit", "prepay", "Over the limit"}, {"prepay", "stock", ""}},
					[]demoArrow{{"stock", "pack", "In stock"}, {"stock", "produce", "Not in stock"}},
					line("produce", "pack", "ship", "delivered")),
			},
		},
	},
	{
		key: "supplier-invoice", name: "Supplier invoice", area: "Finance", owner: "Head of Finance and People",
		description: "From a supplier's invoice to its payment, with approval above the limit.",
		versions: []flow{{
			note: "First version.",
			nodes: []demoNode{
				start("arrives", "Invoice arrives"),
				step("match", "Match it to the order").r("Accountant").a("Head of Finance and People"),
				decision("matches", "Does it match?"),
				step("clarify", "Clarify with the supplier").r("Accountant").a("Head of Finance and People").c("unit:Operations"),
				decision("limit", "Over 10 000 EUR?"),
				step("approve", "Approve the invoice").r("role:invoice-approver").a("Head of Finance and People"),
				step("book", "Book the invoice").r("Accountant").a("Head of Finance and People"),
				step("pay", "Pay on the due date").r("Accountant").a("Head of Finance and People").i("role:invoice-approver"),
				end("paid", "Paid"),
			},
			arrows: join(line("arrives", "match", "matches"),
				[]demoArrow{{"matches", "limit", "Yes"}, {"matches", "clarify", "No"}, {"clarify", "match", ""}},
				[]demoArrow{{"limit", "approve", "Yes"}, {"limit", "book", "No"}},
				line("approve", "book", "pay", "paid")),
		}},
	},
	{
		key: "complaint-handling", name: "Customer complaint", area: "Quality", owner: "Head of Quality",
		description: "What happens when a customer complains about a sensor.",
		versions: []flow{{
			note: "First version.",
			nodes: []demoNode{
				start("received", "Complaint received"),
				step("record", "Record the complaint").r("Account manager").a("Head of Sales").i("unit:Quality"),
				decision("fault", "Is the product at fault?").a("Head of Quality").c("Test lab lead"),
				step("cause", "Find the root cause").r("Quality engineer").a("Head of Quality").c("Assembly lead"),
				step("correct", "Correct and prevent").r("unit:Manufacturing").a("Head of Quality").i("Head of Operations"),
				step("answer", "Answer the customer").r("Account manager").a("Head of Sales").c("Head of Quality"),
				end("closed", "Closed"),
			},
			arrows: join(line("received", "record", "fault"),
				[]demoArrow{{"fault", "cause", "Yes"}, {"fault", "answer", "No"}},
				line("cause", "correct", "answer", "closed")),
		}},
	},
	{
		key: "onboarding", name: "Onboarding", area: "People", owner: "People partner",
		description: "From a signed contract to a new colleague settled in.",
		versions:    []flow{{note: "First version.", nodes: onboardingNodes, arrows: line("signed", "workplace", "accounts", "welcome", "training", "settled")}},
		draft: &flow{
			nodes:  append(append([]demoNode(nil), onboardingNodes...), step("buddy", "Assign a buddy").r("People partner").a("Head of Finance and People")),
			arrows: line("signed", "workplace", "accounts", "buddy", "welcome", "training", "settled"),
		},
	},
	{
		key: "product-release", name: "Product release", area: "Engineering", owner: "Head of Platform",
		description: "How a release of the sensor firmware and apps ships. Still a draft: what happens when the tests fail is open.",
		draft: &flow{
			nodes: []demoNode{
				start("planned", "Release planned"),
				step("freeze", "Freeze the release").r("role:release-manager").a("Head of Platform"),
				step("tests", "Run the test suite").r("Quality engineer").a("Test lab lead"),
				decision("green", "All tests green?"),
				step("ship", "Ship the release").r("Site reliability engineer"),
				end("shipped", "Shipped"),
			},
			arrows: join(line("planned", "freeze", "tests", "green"), []demoArrow{{"green", "ship", "Yes"}}, line("ship", "shipped")),
		},
	},
}
