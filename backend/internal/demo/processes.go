package demo

import "github.com/praetorianer777/bearing/backend/internal/org"

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
	sub string
}

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
		step("check", "Check the order"),
		decision("stock", "In stock?"),
		sub("produce", "Production run", "production-run"),
		step("pack", "Pick and pack"),
		step("ship", "Ship and invoice"),
		end("delivered", "Delivered"),
	}
	orderWays = join(line("received", "check", "stock"),
		[]demoArrow{{"stock", "pack", "In stock"}, {"stock", "produce", "Not in stock"}},
		line("produce", "pack", "ship", "delivered"))

	onboardingNodes = []demoNode{
		start("signed", "Contract signed"),
		step("workplace", "Prepare the workplace"),
		step("accounts", "Create the accounts"),
		step("welcome", "Welcome on the first day"),
		step("training", "Agree the training plan"),
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
				step("plan", "Plan the run"),
				step("assemble", "Assemble the sensors"),
				step("test", "Test the batch"),
				decision("tolerance", "Within tolerance?"),
				step("rework", "Rework"),
				step("release", "Release to stock"),
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
					decision("credit", "Within the credit limit?"), step("prepay", "Ask for prepayment")),
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
				step("match", "Match it to the order"),
				decision("matches", "Does it match?"),
				step("clarify", "Clarify with the supplier"),
				decision("limit", "Over 10 000 EUR?"),
				step("approve", "Approve the invoice"),
				step("book", "Book the invoice"),
				step("pay", "Pay on the due date"),
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
				step("record", "Record the complaint"),
				decision("fault", "Is the product at fault?"),
				step("cause", "Find the root cause"),
				step("correct", "Correct and prevent"),
				step("answer", "Answer the customer"),
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
			nodes:  append(append([]demoNode(nil), onboardingNodes...), step("buddy", "Assign a buddy")),
			arrows: line("signed", "workplace", "accounts", "buddy", "welcome", "training", "settled"),
		},
	},
	{
		key: "product-release", name: "Product release", area: "Engineering", owner: "Head of Platform",
		description: "How a release of the sensor firmware and apps ships. Still a draft: what happens when the tests fail is open.",
		draft: &flow{
			nodes: []demoNode{
				start("planned", "Release planned"),
				step("freeze", "Freeze the release"),
				step("tests", "Run the test suite"),
				decision("green", "All tests green?"),
				step("ship", "Ship the release"),
				end("shipped", "Shipped"),
			},
			arrows: join(line("planned", "freeze", "tests", "green"), []demoArrow{{"green", "ship", "Yes"}}, line("ship", "shipped")),
		},
	},
}
