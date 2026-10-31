package demo

// The demo organisation, Kestrel Works, a fictional maker of industrial sensors: four levels of units, about sixty
// people, a matrix of dotted lines, two vacancies, deputies and a few roles. The suite realm's alice, bob, carol and
// dave are in it by their addresses, so their first sign-in links them.

type unitSpec struct {
	key, name, short, kind, parent string
}

type positionSpec struct {
	key, unit, title string
	head             bool
	seats            int
	// reportsTo is the position it reports to by a solid line; left empty, a head reports to the head of the unit
	// above, anybody else to the head of their own unit.
	reportsTo string
	dottedTo  []string
}

type personSpec struct {
	key, name, email, location string
}

type holdingSpec struct {
	person, position, role string
	share                  int
	// from and to are days written 2006-01-02; from left empty is the day the organisation begins.
	from, to string
}

type roleSpec struct {
	key, name, description string
	carriedBy              []string
}

const since = "2025-01-01"

var kinds = []struct {
	name, icon, colour string
	rank               int
}{
	{"Organisation", "organisation", "#1f3a5f", 0},
	{"Division", "division", "#2f5fd0", 1},
	{"Department", "department", "#3b8c6e", 2},
	{"Team", "team", "#c2701f", 3},
}

var units = []unitSpec{
	{"kestrel", "Kestrel Works", "KW", "Organisation", ""},
	{"eng", "Engineering", "ENG", "Division", "kestrel"},
	{"platform", "Platform", "PLT", "Department", "eng"},
	{"backend", "Backend", "BE", "Team", "platform"},
	{"infra", "Infrastructure", "INF", "Team", "platform"},
	{"product", "Product", "PRD", "Department", "eng"},
	{"web", "Web", "WEB", "Team", "product"},
	{"mobile", "Mobile", "MOB", "Team", "product"},
	{"ops", "Operations", "OPS", "Division", "kestrel"},
	{"manufacturing", "Manufacturing", "MFG", "Department", "ops"},
	{"assembly", "Assembly", "ASM", "Team", "manufacturing"},
	{"quality", "Quality", "QA", "Department", "ops"},
	{"lab", "Test lab", "LAB", "Team", "quality"},
	{"commercial", "Commercial", "COM", "Division", "kestrel"},
	{"sales", "Sales", "SAL", "Department", "commercial"},
	{"inside", "Inside sales", "IS", "Team", "sales"},
	{"field", "Field sales", "FS", "Team", "sales"},
	{"marketing", "Marketing", "MKT", "Department", "commercial"},
	{"finance", "Finance and People", "FIN", "Department", "kestrel"},
}

var positions = []positionSpec{
	{key: "ceo", unit: "kestrel", title: "Chief executive", head: true},
	{key: "assistant", unit: "kestrel", title: "Executive assistant"},
	{key: "head-eng", unit: "eng", title: "Head of Engineering", head: true},
	{key: "head-platform", unit: "platform", title: "Head of Platform", head: true},
	{key: "lead-backend", unit: "backend", title: "Backend lead", head: true},
	{key: "backend-dev", unit: "backend", title: "Backend developer", seats: 4},
	{key: "lead-infra", unit: "infra", title: "Infrastructure lead", head: true},
	{key: "sre", unit: "infra", title: "Site reliability engineer", seats: 2},
	{key: "security", unit: "infra", title: "Security engineer"},
	{key: "security-consultant", unit: "infra", title: "Security consultant", dottedTo: []string{"head-quality"}},
	{key: "head-product", unit: "product", title: "Head of Product", head: true},
	{key: "pm", unit: "product", title: "Product manager", seats: 2, dottedTo: []string{"head-commercial"}},
	{key: "designer", unit: "product", title: "Product designer"},
	{key: "lead-web", unit: "web", title: "Web lead", head: true},
	{key: "web-dev", unit: "web", title: "Web developer", seats: 4},
	{key: "lead-mobile", unit: "mobile", title: "Mobile lead", head: true},
	{key: "mobile-dev", unit: "mobile", title: "Mobile developer", seats: 2},
	{key: "head-ops", unit: "ops", title: "Head of Operations", head: true},
	{key: "head-manufacturing", unit: "manufacturing", title: "Head of Manufacturing", head: true},
	{key: "planner", unit: "manufacturing", title: "Production planner"},
	{key: "lead-assembly", unit: "assembly", title: "Assembly lead", head: true},
	{key: "technician", unit: "assembly", title: "Assembly technician", seats: 6},
	{key: "head-quality", unit: "quality", title: "Head of Quality", head: true},
	{key: "lead-lab", unit: "lab", title: "Test lab lead", head: true},
	{key: "quality-engineer", unit: "lab", title: "Quality engineer", seats: 2, dottedTo: []string{"head-platform"}},
	{key: "head-commercial", unit: "commercial", title: "Head of Commercial", head: true},
	{key: "head-sales", unit: "sales", title: "Head of Sales", head: true},
	{key: "lead-inside", unit: "inside", title: "Inside sales lead", head: true},
	{key: "sales-rep", unit: "inside", title: "Sales representative", seats: 3},
	{key: "lead-field", unit: "field", title: "Field sales lead", head: true},
	{key: "account-manager", unit: "field", title: "Account manager", seats: 4},
	{key: "head-marketing", unit: "marketing", title: "Head of Marketing", head: true},
	{key: "marketer", unit: "marketing", title: "Marketing specialist", seats: 2},
	{key: "writer", unit: "marketing", title: "Content writer"},
	{key: "head-finance", unit: "finance", title: "Head of Finance and People", head: true},
	{key: "accountant", unit: "finance", title: "Accountant", seats: 2},
	{key: "payroll", unit: "finance", title: "Payroll specialist"},
	{key: "people-partner", unit: "finance", title: "People partner", dottedTo: []string{"head-ops"}},
}

var people = []personSpec{
	{"maren", "Maren Holt", "maren.holt@kestrel.example", "Hamburg"},
	{"tomas", "Tomás Ruiz", "tomas.ruiz@kestrel.example", "Hamburg"},
	{"alice", "Alice Admin", "alice@suite.test", "Hamburg"},
	{"priya", "Priya Natarajan", "priya.natarajan@kestrel.example", "Lisbon"},
	{"jonas", "Jonas Weber", "jonas.weber@kestrel.example", "Hamburg"},
	{"bob", "Bob Builder", "bob@suite.test", "Hamburg"},
	{"aiko", "Aiko Tanaka", "aiko.tanaka@kestrel.example", "Lisbon"},
	{"lukas", "Lukas Brandt", "lukas.brandt@kestrel.example", "Remote"},
	{"fatima", "Fatima Zahra", "fatima.zahra@kestrel.example", "Lisbon"},
	{"sven", "Sven Larsen", "sven.larsen@kestrel.example", "Remote"},
	{"chen", "Chen Wei", "chen.wei@kestrel.example", "Lisbon"},
	{"olu", "Olu Adeyemi", "olu.adeyemi@kestrel.example", "Remote"},
	{"dave", "Dave Visitor", "dave@suite.test", "Remote"},
	{"nadia", "Nadia Petrova", "nadia.petrova@kestrel.example", "Hamburg"},
	{"elif", "Elif Kaya", "elif.kaya@kestrel.example", "Hamburg"},
	{"grace", "Grace Okafor", "grace.okafor@kestrel.example", "Lisbon"},
	{"mateo", "Mateo Rossi", "mateo.rossi@kestrel.example", "Hamburg"},
	{"hana", "Hana Novak", "hana.novak@kestrel.example", "Remote"},
	{"leon", "Leon Fischer", "leon.fischer@kestrel.example", "Hamburg"},
	{"sara", "Sara Lind", "sara.lind@kestrel.example", "Lisbon"},
	{"yusuf", "Yusuf Demir", "yusuf.demir@kestrel.example", "Hamburg"},
	{"clara", "Clara Moreau", "clara.moreau@kestrel.example", "Remote"},
	{"arjun", "Arjun Mehta", "arjun.mehta@kestrel.example", "Lisbon"},
	{"ravi", "Ravi Kumar", "ravi.kumar@kestrel.example", "Lisbon"},
	{"ines", "Ines Costa", "ines.costa@kestrel.example", "Lisbon"},
	{"gabriel", "Gabriel Santos", "gabriel.santos@kestrel.example", "Hamburg"},
	{"ewa", "Ewa Kowalska", "ewa.kowalska@kestrel.example", "Hamburg"},
	{"daniel", "Daniel Mensah", "daniel.mensah@kestrel.example", "Hamburg"},
	{"marco", "Marco Bianchi", "marco.bianchi@kestrel.example", "Hamburg"},
	{"lena", "Lena Hoffmann", "lena.hoffmann@kestrel.example", "Hamburg"},
	{"pavel", "Pavel Novak", "pavel.novak@kestrel.example", "Hamburg"},
	{"amara", "Amara Diallo", "amara.diallo@kestrel.example", "Hamburg"},
	{"kofi", "Kofi Boateng", "kofi.boateng@kestrel.example", "Hamburg"},
	{"julia", "Julia Svensson", "julia.svensson@kestrel.example", "Hamburg"},
	{"tobias", "Tobias Klein", "tobias.klein@kestrel.example", "Hamburg"},
	{"carol", "Carol Checker", "carol@suite.test", "Hamburg"},
	{"tariq", "Tariq Hassan", "tariq.hassan@kestrel.example", "Hamburg"},
	{"mei", "Mei Lin", "mei.lin@kestrel.example", "Lisbon"},
	{"sofia", "Sofia Andersson", "sofia.andersson@kestrel.example", "Hamburg"},
	{"oliver", "Oliver Grant", "oliver.grant@kestrel.example", "Remote"},
	{"zoe", "Zoe Martin", "zoe.martin@kestrel.example", "Hamburg"},
	{"ahmed", "Ahmed Karimi", "ahmed.karimi@kestrel.example", "Hamburg"},
	{"lucia", "Lucía Fernández", "lucia.fernandez@kestrel.example", "Lisbon"},
	{"noah", "Noah Becker", "noah.becker@kestrel.example", "Hamburg"},
	{"ingrid", "Ingrid Berg", "ingrid.berg@kestrel.example", "Remote"},
	{"samuel", "Samuel Osei", "samuel.osei@kestrel.example", "Remote"},
	{"elena", "Elena Popescu", "elena.popescu@kestrel.example", "Remote"},
	{"victor", "Victor Dubois", "victor.dubois@kestrel.example", "Remote"},
	{"mia", "Mia Johansson", "mia.johansson@kestrel.example", "Remote"},
	{"aylin", "Aylin Yıldız", "aylin.yildiz@kestrel.example", "Hamburg"},
	{"paul", "Paul Meyer", "paul.meyer@kestrel.example", "Hamburg"},
	{"rosa", "Rosa Jiménez", "rosa.jimenez@kestrel.example", "Lisbon"},
	{"kenji", "Kenji Mori", "kenji.mori@kestrel.example", "Remote"},
	{"henrik", "Henrik Dahl", "henrik.dahl@kestrel.example", "Hamburg"},
	{"lea", "Lea Schmid", "lea.schmid@kestrel.example", "Hamburg"},
	{"omar", "Omar Haddad", "omar.haddad@kestrel.example", "Hamburg"},
	{"felix", "Felix Wagner", "felix.wagner@kestrel.example", "Hamburg"},
	{"nina", "Nina Kraus", "nina.kraus@kestrel.example", "Hamburg"},
}

// The Head of Product changes hands in spring 2026, when Nadia leaves; the security engineer's chair and the Head
// of Quality's stay empty, and the test lab lead stands in for the latter.
const handover = "2026-04-01"

var holdings = []holdingSpec{
	{person: "maren", position: "ceo"},
	{person: "tomas", position: "assistant"},
	{person: "alice", position: "head-eng"},
	{person: "priya", position: "head-platform"},
	{person: "jonas", position: "lead-backend"},
	{person: "bob", position: "backend-dev"},
	{person: "aiko", position: "backend-dev"},
	{person: "lukas", position: "backend-dev", share: 80},
	{person: "fatima", position: "backend-dev"},
	{person: "sven", position: "lead-infra"},
	{person: "chen", position: "sre"},
	{person: "olu", position: "sre"},
	{person: "dave", position: "security-consultant", share: 20},
	{person: "nadia", position: "head-product", to: handover},
	{person: "elif", position: "head-product", from: handover},
	{person: "grace", position: "pm"},
	{person: "mateo", position: "pm"},
	{person: "hana", position: "designer"},
	{person: "leon", position: "lead-web"},
	{person: "sara", position: "web-dev"},
	{person: "yusuf", position: "web-dev"},
	{person: "clara", position: "web-dev"},
	{person: "arjun", position: "web-dev", from: "2025-09-01"},
	{person: "ravi", position: "lead-mobile"},
	{person: "ines", position: "mobile-dev"},
	{person: "gabriel", position: "head-ops"},
	{person: "ewa", position: "head-manufacturing"},
	{person: "daniel", position: "planner"},
	{person: "marco", position: "lead-assembly"},
	{person: "lena", position: "technician"},
	{person: "pavel", position: "technician"},
	{person: "amara", position: "technician"},
	{person: "kofi", position: "technician"},
	{person: "julia", position: "technician"},
	{person: "tobias", position: "technician", share: 50},
	{person: "carol", position: "lead-lab"},
	{person: "tariq", position: "quality-engineer"},
	{person: "mei", position: "quality-engineer"},
	{person: "sofia", position: "head-commercial"},
	{person: "oliver", position: "head-sales"},
	{person: "zoe", position: "lead-inside"},
	{person: "ahmed", position: "sales-rep"},
	{person: "lucia", position: "sales-rep"},
	{person: "noah", position: "sales-rep"},
	{person: "ingrid", position: "lead-field"},
	{person: "samuel", position: "account-manager"},
	{person: "elena", position: "account-manager"},
	{person: "victor", position: "account-manager"},
	{person: "mia", position: "account-manager"},
	{person: "aylin", position: "head-marketing"},
	{person: "paul", position: "marketer"},
	{person: "rosa", position: "marketer"},
	{person: "kenji", position: "writer", share: 50},
	{person: "henrik", position: "head-finance"},
	{person: "lea", position: "accountant"},
	{person: "omar", position: "accountant"},
	{person: "felix", position: "payroll"},
	{person: "nina", position: "people-partner"},
	{person: "leon", role: "release-manager", share: 10},
	{person: "lena", role: "first-aider", share: 5},
	{person: "daniel", role: "first-aider", share: 5},
	{person: "zoe", role: "first-aider", share: 5},
	{person: "nina", role: "data-protection-officer", share: 20},
}

// leavers leave on the day given.
var leavers = map[string]string{"nadia": handover}

var roles = []roleSpec{
	{"invoice-approver", "Invoice approver", "Approves invoices for their part of the organisation.",
		[]string{"ceo", "head-eng", "head-ops", "head-commercial", "head-finance"}},
	{"release-manager", "Release manager", "Decides when a product release ships.", []string{"head-platform"}},
	{"first-aider", "First aider", "Gives first aid on site until help arrives.", nil},
	{"data-protection-officer", "Data protection officer", "Advises on and watches over the handling of personal data.", nil},
}

var deputies = map[string][]string{
	"ceo":          {"head-eng", "head-ops"},
	"head-eng":     {"head-platform", "head-product"},
	"head-quality": {"lead-lab"},
	"head-sales":   {"lead-inside", "lead-field"},
}
