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
