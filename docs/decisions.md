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
