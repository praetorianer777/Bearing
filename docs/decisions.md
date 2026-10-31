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
