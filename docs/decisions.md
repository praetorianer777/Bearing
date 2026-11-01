# Decisions

## One installation is one organisation

Bearing models a whole organisation, and its siblings serve one organisation
each, so the two map one to one. Teams are units inside it.

## Reporting lines run between positions

A person holds a position; the position reports to another. A vacancy, a
deputy, two people sharing a position, or somebody moving on changes a
holding and leaves the graph standing. Lines are solid (disciplinary) or
dotted (functional), which a matrix organisation needs.

## Every relation is dated

Holdings, reporting lines and responsibilities have a start and an end, so
"who was responsible in March" has an answer and a reorganisation can be
entered before it takes effect.

## PostgreSQL, not a graph database

The graph is small (thousands of nodes) and its questions are trees and
short paths, which recursive CTEs answer. One database keeps the operations,
backups, CloudNativePG cluster and read/write split the siblings already
have.

## People are keyed by the realm

A person signed in through the realm is known by issuer and `sub`, the key
every sibling maps its own users from, so partners need no mapping table.
Local and external people have Bearing's own id.

## Processes describe, they do not run

A process says who does what in which order. Running work belongs to Rotor's
workflows and Armature's issue workflows; a step links to them instead.

## Processes are flow graphs

Steps, decisions and subprocesses joined by arrows, drawn on a canvas, rather
than plain lists (too little for real processes) or full BPMN (too much for a
first release; an exchange can come later).

## Standalone first

Bearing runs without its siblings. Every partner feature is gated by the
partner's `/api/v1/meta` capabilities and degrades with a sentence saying
what is missing.

## Roles follow realm groups while they are held

An account holds the highest of the role granted to it and the roles granted
to the realm groups the provider last named for it. A group's role lasts as
long as the person is in the group, so access follows the realm without a
second list to keep. Somebody let in through a group gets reader as their own
role. The database decides with `current_actor_role()` and the policies, and
the API's operation table says the same thing first, with a clearer refusal.

## Themes are the installation's; the look is the browser's

Administrators import, export and remove themes and name the one everybody
starts from. Each person chooses light, dark or the system's, and a theme,
in their account menu, and the browser keeps that choice. Theme files and
their pictures are stored in PostgreSQL, so Bearing needs no object store.
That is why files are smaller here (4 MB, 32 per theme) than in the
siblings.

## Where a unit sits is dated; what it is called is not

A unit's place in the tree is a series of placements, each from a day until a
day, so a move can be entered before it takes effect and the tree can be read
as of any day. A unit's name, type and description are not dated; their
history belongs to the audit log. Archiving ends a unit's placements and keeps
the days before it. A move that would put a unit inside itself is refused if
the cycle would form on any day the new placement covers. The database checks
this itself, under a lock, so two moves at once cannot together form a cycle.

## A position stays in its unit

A position belongs to one unit for as long as it exists. A reorganisation
that moves the work makes a new position in the new unit and archives the
old one, so the holdings of each keep their meaning. Reporting lines are
dated like placements. A position has at most one solid line on any day, and
solid lines may not form a circle. Dotted lines may cross the tree, but a
position never has two dotted lines to the same position at once.

## A sign-in links an account to its person

A person may be entered before they ever sign in, as external, by name and
address. When an account signs in, Bearing links it to its entry: first the
entry the realm knows by one of the account's identities, then an unlinked
entry with the account's address, which then takes on the realm's issuer and
`sub`. An address names one person, so the match is unambiguous. A failed
link never keeps anybody out; the next sign-in tries again. Somebody signed
in keeps the fields of their own entry that administrators allow, by default
phone, location and photo. The database enforces this as well.

## Holdings end with the person and the position

A holding is a person holding a position from a day, until a day or for
good, at a share of their week. A position has seats, one unless it says
otherwise; the database refuses more holders on any day than there are
seats, under a lock, so two holdings given at once cannot together take a
seat too many. A position nobody holds on a day is vacant then. Shares are
not capped across a person's holdings: an overbooked week is something to
show, not to refuse. When a person leaves or a position is archived, its
holdings end that day and those planned later go; taking the leaving or the
archiving back does not bring them back, since the seat may have been given
to somebody else meanwhile. A share that changes from a day is the holding
ended there and a new one.

## Roles come through positions or directly; deputies are a standing order

A role is a function that does not follow the tree, named by a key other
tools can rely on. A position carries roles for a time, dated like every
relation, so whoever holds the position has them; a person may also hold a
role directly, as a holding of the role itself, without seats. A role's
holders on a day are both kinds together, each saying how they have it. A
role nobody ever held and no position ever carried may be removed; any
other stays for the history. A position's deputies are other positions in
the order they stand in. The order is not dated: it is a standing
arrangement, changed as a whole, and its history belongs to the audit log.
A deputy archived later drops out of the order from that day.

## Hierarchy questions are answered by walks the database does in one query

Who someone reports to, who reports to them and who is in a unit are
recursive queries on the read path, each for one day. A walk over solid
lines alone needs no guard against circles, since solid lines form a tree;
one over dotted lines as well keeps the path it went and lists each
position once, where it is nearest. Holders come back as a row each and the
answer is sorted in Go: both are far cheaper for thousands of rows than
arrays per position and sorting under the database's collation. The
model's row-level policies ask who acts once per query, through a
sub-select the planner runs once, not once per row. A 5000-person
organisation answers every question within 50 ms; `make test-timing`
holds it to that, without the race detector, which slows the code it
watches too much for times to mean anything.

## The audit log is written by the database, read by editors

Every insert, update and delete on the organisation model's tables gets an
entry in the same transaction: who it was made for, when, and the row
before and after, without timestamps or a photo's bytes. A trigger writes
it, so no code path can change the model without its entry, and nobody may
change the log afterwards. Each entry names the units, positions, people,
roles and holdings it concerns, so each has its own view of the log,
including the relations naming it. The log tells who changed what, which
is for editors and administrators, not for every member. Dated relations
answer what held on a day; the log answers who changed it and when.
Accounts and settings are not part of the model and are not in this log.

## The demo organisation goes only into an empty installation, once

The demo organisation is code, loaded through the model's own service in one
transaction, so it obeys every rule real changes obey and is audited as the
administrator who loaded it. It goes only into an installation without
units, since it brings its own root, and only once: the installation notes
when it was loaded, so neither the seed run again nor an administrator's
second try adds it twice, even after its units were changed. The realm's
demo people are entered by address, not by `sub`: Keycloak gives users their
ids when it imports the realm, and the realm file is Dynamo's, so the ids
cannot be known ahead. The first sign-in links each one, as it would anybody
entered ahead of signing in.

The demo's processes are a second part, loaded the same way and noted
apart: into an installation without processes whose organisation is the
demo's, since they are owned by its positions. An installation that had
the demo organisation before there were processes gets them on asking.

## The org chart is laid out in the browser, and moved cards are each viewer's own

The chart draws positions, not people, as cards, with their holders on
them, from the units, positions and people of one day. ELK lays out the
tree of solid lines alone, so it stays a tree; dotted lines and deputies
are drawn across it afterwards. A card someone drags stays where they put
it, in that browser only, until they reset the layout: an arrangement is
a way of looking, not a fact about the organisation, so it is not shared
or audited. Folding is by unit, into the unit's head. The day and the
focus live in the address, so a chart can be linked to as it was seen.

## A process is a draft and numbered versions, stored as rows

A process is a flow graph of nodes (start, step, decision, subprocess,
end) and arrows, kept as rows rather than one document, so later steps
can name roles, positions and units by key and searches can reach every
step. The draft is the version without a number. Saving replaces the
draft's graph whole and names the revision it was made from, so two
editors do not overwrite each other unseen. Publishing numbers the draft
and copies it into a new one; a trigger keeps published versions from
changing, so whatever later hangs off a node is copied along and kept
as it was. The draft takes anything the store can hold, and what keeps
it from being published (one start, every node on a way from the start
to an end, decisions with two or more labelled ways, subprocesses
naming another process) comes back with it as problems, so the editor
can show them while drawing. The log keeps processes and each
publishing, not every save of a draft.

## The process editor keeps its own history and saves when asked

The canvas edits a copy of the draft in the browser: every change is an
edit to that copy, so undo and redo walk back and forth through it, a run
of typing or one drag counting as one step. Saving sends the whole graph
with the revision it was read at, and the problems that come back are
those of what was saved, so the editor says when they are older than the
canvas. Saving is a step the editor takes, not something done on each
keystroke: a draft half drawn is no one else's business until it is
saved, and publishing takes the saved draft only. Where a node was left
is part of the draft, so everyone sees the flow as it was arranged;
Arrange lays it out anew on request.

## Responsibilities name roles, positions or units, and warn rather than refuse

A step's responsibilities name a role, a position or a unit with its RACI
kind, never a person: people come and go, and who answers for a step on a
day is worked out from who holds the party then. They belong to the
version like its nodes, so a published version keeps who answered for
what when it was published. A step without exactly one accountable or
without anyone responsible is a warning, not a refusal: a process is often
published before every party is agreed, and refusing it would push
editors to name somebody for the sake of the rule. The editor and the
RACI matrix mark such steps so they are not forgotten.

## A responsibility comes down to people on a day, deputies once

Who does a step is worked out when asked, for a day, never stored: a
position comes down to whoever holds it then; a role to whoever holds it
directly or holds a position carrying it; a unit to its head. A vacant
position comes down to the holders of its first deputy position that has
any, who are marked as standing in, so nobody mistakes a deputy for the
holder. Deputies stand in once: a deputy's own deputies do not, since a
chain of stand-ins nobody chose is no answer to who does it. A party
nobody holds, even by deputy, says so rather than going quiet.

## What somebody answers for is read from the newest published versions

"What is X responsible for" and "who does it" are asked of the newest
published version of each process, never of drafts: a draft is not yet
how the organisation works. A person answers for whatever comes down to
them on the day, by position, role, headship or standing in as a deputy;
a position for what names it, a role it carries, the unit it heads or a
vacant position it stands in for; a role for what names it; a unit for
what names it or one of its positions. Both answers are worked out from
the same resolution as a step's people, so the pages never disagree.

## Arrows keep the routes the layout gave them

A process drawn by hand soon has arrows crossing nodes and each other, and
a published version cannot be rearranged. Arrange therefore asks ELK for
the routes as well as the places, and each arrow keeps its route in the
draft and in every version published from it. Moving a node drops the
routes of its own arrows only; the others keep theirs. An arrow without a
route is drawn between the sides the canvas chooses: back up the flow
round the side, down and well aside from the side facing its target.
Pointing at or picking a node brings its arrows forward and fades the
rest, and a version nobody may change can be laid out for reading
without changing it. The demo routes its own arrows so that none crosses
a node, as its unit test holds it to.

## Swimlanes are derived, not drawn

A process seen in lanes is laid out from its graph and responsibilities
each time it is shown, never stored and never edited there. A step goes
in the lane of its first responsible party, or of its first accountable
one where nobody is responsible, or in a lane for nobody, last, so the
gap shows. A start, an end or a decision without a party follows the
node before it in the flow. The layout is Bearing's own rather than
ELK's, because ELK cannot hold nodes to lanes: rows run down the flow,
loops not pushing anything further down, and every arrow travels only
in the gaps between rows and the gutters between columns, where no node
stands, so none crosses a node.

## Versions are compared in the browser, by key

Both versions of a comparison are read through the API that already
serves them, and the difference is worked out in the browser: nothing
new to store or serve. Nodes and arrows are the same across versions by
key, as every draft is copied from the version before it. Places and
routes are left out, as they change how a process is drawn, not what it
says. The canvas shows the later version with what was removed put back
where it stood, each node marked added, removed or changed, and the
changes are listed beside it. A draft set against a version cannot be
edited until the comparison is put away, as removed nodes stand on the
canvas among its own.

## Bearing is the siblings' kind of authorization server

Partner apps connect to Bearing as Stator and Armature let them: an app
registers itself, an administrator allows it by connecting it once, and
people connect it on a consent page by authorization code with PKCE. The
schema follows Stator's, so the suite's apps meet one model: a grant is a
row of the person's `api_token`, the refresh token rotates with the access
token, and a replayed refresh token or code ends the grant. A grant made on
the consent page acts at the MCP endpoint alone, so an assistant cannot
reach further into the API than its tools go; a token holding read only
may only read anywhere. Bearer tokens are looked up on the primary, as a
token is used the moment it is issued. The token endpoint and the consent
page act for nobody and run as `bearing_admin`; a person reads and revokes
their own grants under row level security, never their hashes. Who
allowed or connected what is logged rather than written to the
organisation's audit trail, which records changes to the model.

## Partners trade realm tokens for grants that reach the whole API

A partner app such as Rotor already signed the person in through the
realm, so asking them again on a consent page would be a second sign-in
for nothing. An administrator registers the partner with its realm
client, and the partner trades the person's realm access token for a
grant (RFC 8693), as with Stator. The token must name Bearing among its
audiences and have been issued to that client, so a token meant for
another service cannot be replayed here. The grant reaches the whole
API, not only MCP, because partners read the organisation through the
API's own operations; the person's role and the grant's scopes still
hold. The person's account is found or made as single sign-on would, so
token exchange is no back door past admission. A narrower token traded
from an app's own grant lets the app hand a run less than it holds. The
stack's realm carries `bearing-partner`, a client of Bearing's own that
stands in for a partner in the tests; real partners need an audience
mapper for `bearing` in Dynamo's realm when Bearing joins the suite.

## MCP tools read through the API's own operations

Each MCP tool runs the API's own GET operations in the process, as the
person the assistant acts for, rather than querying the model itself.
What a tool answers is then exactly what the API answers that person:
their role, the database's policies and the "as of" day all hold without
being written again for MCP, and a new rule in the API reaches the tools
at once. A token given on the consent page acts at the MCP endpoint only,
yet reaches these reads, as they are what its tools are made of. Every
tool reads; changing the organisation stays with the web client and the
API. `search` answers what the API's own search answers that person.
Answers are
cut at 64 KiB so a large organisation cannot flood an assistant's
context.

## Personal tokens are the person's own rows, made only while signed in

A personal access token is an `api_token` row of its person's without an
app, so one lookup authenticates every bearer token and one list shows
what a person has handed out. The person makes it as themselves, under row
level security that lets nobody make a token for somebody else. Every
token ends, a year at the latest, and a person holds twenty at most.
Making a token and connecting an app need a session, not a token: a token
that leaks then cannot be turned into a lasting one, and revoking it ends
what it could do. Read only is the default, as most scripts only read.

## Search is Postgres full text, with trigrams for slips

Search runs in Postgres, as the siblings' does, with no search engine
beside it. Each kind keeps a generated search vector: its name weighted
A, its other words B, read with a configuration that ignores case and
accents and stems nothing, so a name is found as written in any language.
Descriptions are read once more with the English and the German stemmer,
weighted D, so "approves" finds "approve" without ranking a sentence
above a name. Every word typed matches as the start of a word, which
lets one query serve the box being typed in and the search page, and a
name typed with a slip is found by trigram likeness. A name that is what
was typed comes first, then one that starts so, then the words' weights.
One statement searches every kind, so the "as of" day, the newest
published version of a process and the database's policies hold for
each the same way.

## Capacity is counted on the days something changes

Capacity is the sum of the holdings' shares, read from the same dated
relations as every other "as of" answer, so it needs no table of its own
that could fall out of step. Over a range it is counted only on the
range's first day and the days a holding, placement, carried role,
position, unit or leaver starts or ends, and every day between takes the
count before it: a two-year range of a unit and its sub-units costs a
few dozen days of work rather than seven hundred. A unit's sub-units are
those placed under it on each day, so a team counts where it sat then.
A position's filled seats are at most its seats, so one held twice over
does not hide a vacancy elsewhere in the unit. Weeks start on Monday,
and a week or a month is the mean of its working days, so a weekend
nobody works does not halve it.

## A target set for some days replaces what was set for them

A target is dated like every relation, and a subject has one on any day.
Setting one for a span paints over what was there: a target inside the
span goes, one reaching into it is cut at the span's edge, and one
reaching over it is split around it. An editor who wants six from
January then only says so, rather than ending the old target first, and
a planned change for a quarter leaves the target after it standing. A
unit's target is for the unit with everything inside it, as a head of
department plans for the whole department, so the page compares it with
the unit's capacity counted with its sub-units. Capacity answers carry
the target day by day, a period's being the mean of its days that have
one.

## Gaps are runs of days of one size

A gap is a subject missing people, or having more than its target, over
a run of days on which the shortfall keeps one size: a seat half filled
for three weeks and then empty for two is two gaps, each saying exactly
what is missing when. Bearing finds four kinds from its own data: seats
nobody fills, capacity under or over a target, roles nobody holds, and
roles only one person holds, which have nobody to stand in. A holding
ending without a successor needs no kind of its own: the vacancy after
it is a gap from its first day, and a planned holding closes it from
its first day. Positions are read together in one statement; targets
and roles reuse the capacity count, which keeps every gap equal to what
the capacity answers say.

## The planning grid reads one row at a time

The planning page draws the unit tree as rows and the weeks or months of
a range as columns, and each row asks the capacity of its own subject:
a unit with everything inside it, a position, or a person holding it in
the range. Only the rows shown are asked for, so opening a unit costs as
many small reads as it has rows, and a large organisation never loads
whole. A hire or a handover is planned where the gap shows, as a holding
planned ahead of somebody in the organisation or of somebody new added
as external; it needs nothing a holding does not already have, and the
grid and the gaps close from its first day. The CSV is what is shown,
one line per row and period, so a spreadsheet can sum and chart it.

## A person's capacity is for them, their managers and editors

Holdings and their shares are the organisation's structure, readable by
everybody signed in, as the chart shows them; so are sums over a unit, a
position or a role. A person's own capacity over time is narrower: it is
where partners will add their load and the days they are away, and those
are for the person, those above them in the management chain today, and
editors. A database function says who that is, and the capacity query
asks it, so the rule holds for every client and every token alike. Who
is above whom is read on today, not on the days asked about, as it is
today's manager who plans.
