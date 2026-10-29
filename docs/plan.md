# Implementation plan

Bearing models an organisation: who belongs where, who reports to whom, and
who is responsible for which step of which process. This file says what it
holds, how it is built, and in which order it is built. Why each choice was
made is in [`decisions.md`](decisions.md).

## What it answers

- Who is this person's manager, and the chain up to the top?
- Who reports to this person, directly and all the way down?
- Which units, positions and roles does this person hold, and since when?
- What is this person, position or role responsible for, across every process?
- Who is responsible for this task, accountable for it, to be consulted on it
  or informed of it, today or on a given date?
- Who stands in while a position is vacant?

## Domain model

| Thing | What it is |
|---|---|
| Unit | A part of the organisation (division, department, team), of a type an administrator defines, in one tree by `part_of`. |
| Position | A slot in a unit, such as "Head of Quality". It reports to another position, by a solid (disciplinary) or dotted (functional) line, and may be vacant. |
| Role | A function that does not follow the tree, such as "Invoice approver", held through positions or by people directly. |
| Person | Somebody with a local account, somebody the realm signs in (keyed by issuer and `sub`), or an external person without an account. |
| Holding | A person holds a position or role from a date, until a date, at a share of their time. |
| Deputy | A position stands in for another. |
| Process | A flow graph: start, end, step, decision and subprocess nodes joined by arrows, an arrow optionally labelled with its condition. Drafts change; published versions do not. |
| Responsibility | A step's RACI (responsible, accountable, consulted, informed) to a role, a position or a unit, resolved to people through the holdings valid on the date asked for. |

Reporting lines run between positions, not people, so vacancies, deputies,
shared positions and people moving do not break the graph. Every relation is
dated, so each view can be asked "as of" a date, and every change is audited.

## Architecture

The same as the siblings, so code, tooling and operations carry over.

- **Backend**: Go with chi, pgx and goose, an `api` and a `worker` binary
  (webhook deliveries, imports, search indexing).
- **Database**: PostgreSQL 18. Under the chart, CloudNativePG runs the
  cluster. Every write, migration and admin task goes to the primary (`-rw`);
  reads go to the replicas (`-ro`) when there is more than one instance, each
  read checking that its replica has caught up with the person's last write
  and otherwise going to the primary, so a person always sees their own
  changes. The development stack runs a primary and a replica to keep both
  paths tested. Hierarchy queries are recursive CTEs; search is Postgres full
  text search.
- **Cache and queues**: Valkey.
- **Web**: React with Tailwind, React Flow (`@xyflow/react`) for the org chart,
  the graph explorer and the process canvas, i18n from the start.
- **Identity**: local accounts with passwords, and OIDC single sign-on with
  PKCE through the realm `dynamo`, the suite's realm, with a client `bearing`.
- **Themes**: the suite's `armature-theme/1` files, imported and exported.
- **API**: OpenAPI document checked in; `GET /api/v1/meta` lists the
  capabilities, as every sibling's does.
- **Tooling**: everything in containers behind a `Makefile`, ports derived
  from the checkout's path, `run-tests.sh` as the gate, a Helm chart for
  Kubernetes.

## Standalone first

Bearing needs none of its siblings. Local accounts work without Keycloak, and
every feature that reaches a partner asks the partner's `/api/v1/meta` first
and is left out, with a sentence saying which partner to update, when the
capability is missing.

## Milestones

The proof of concept is M0 to M4, labelled `mvp`; its end is tagged v0.1.0.
It runs on its own with the org model, its views, search, processes and an
MCP endpoint. M5 onwards is `backlog`.

| Milestone | What it brings |
|---|---|
| M0 Foundation | Repository tooling, the stack, backend and web skeletons, identity, roles, themes, the test gate |
| M1 Organisation model | Units, positions, people, holdings, roles, deputies, hierarchy queries, dates and audit, a demo organisation |
| M2 Org views and search | Org chart, person, unit and position pages, search, graph explorer, editing, CSV import and export |
| M3 Processes | Flow graph processes on a canvas, RACI on steps, resolution to people, "who does it" and "what does X answer for", swimlanes, version diff |
| M4 MCP and partner access | OAuth for partner apps, token exchange, personal access tokens, the MCP endpoint, the PoC walkthrough |
| M5 Directory sync | Realm users imported, SCIM, realm groups mapped to units and roles |
| M6 Suite integration | `org-directory@1`, `responsibility@1`, `org-embed@1`, `webhooks@1`, `process-remote-links@1`, a suite profile |
| M7 Deployment and release | Helm chart with CloudNativePG, release tooling and images, backup and upgrade docs |
| Backlog | Group write-back to the realm, deputies from absences, BPMN exchange, planned reorganisations, privacy controls |
