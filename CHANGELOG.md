# Changelog

## Unreleased

- **Nobody there:** with Armature's days away, the planning page, a
  role's page and a process's page show editors the working days a
  position, a role or a step's responsible or accountable party has
  neither its holder nor a deputy there; no person is named and no
  reason given. `GET /api/v1/absences`, `GET /api/v1/roles/{id}/absences`,
  `GET /api/v1/processes/{id}/absences` (#93).

- **Demand from Armature:** with Armature connected, the planning page
  shows a unit's capacity against what the work there asks of its
  people, week by week: allocations, load, working hours and days away,
  each person at their share in the unit, with who is given out beyond
  their week. Signing in through the realm trades the sign-in for a
  grant at Armature, so Bearing reads Armature as the person. Without
  Armature, or with one too old for `resource-load@1`, the page says
  what is missing. `GET/PUT/DELETE /api/v1/armature/connection`,
  `GET/DELETE /api/v1/armature/me`, `GET /api/v1/units/{id}/demand`
  (#92).

- **Cards for chips (`org-embed@1`):** partners show a person, position,
  unit or role as a chip with a line beneath its name and a link back to
  Bearing, by id or a person by the realm's subject, and find them by
  the words typed so far. `GET /api/v1/cards/{kind}/{id}`,
  `GET /api/v1/cards/person/by-subject`, `GET /api/v1/cards/suggest`
  (#41).

- **Webhook topics for resource planning:** `holding.planned`,
  `holding.started` and `holding.ended`, the last two also sent by the
  worker on their day when nobody writes then; `person.left`; and
  `capacity.gap_changed`, when a gap over the next thirteen weeks opens,
  closes or changes size for a unit, position or role (#94).

- **Webhooks (`webhooks@1`):** administrators name addresses under
  Settings, Webhooks, and Bearing posts changes there, signed and in the
  suite's envelope, as Armature and Stator do: a position's holder
  changes, a position is left empty, a reporting line or a unit changes,
  or who is responsible changes. A failed post is tried again for a
  day, an endpoint failing that long is turned off, and its log shows
  every attempt, which can be sent again; Test posts a ping. Secrets are
  shown once and sealed with the new `BEARING_SECRET_KEY`;
  `BEARING_OUTBOUND_ALLOW` lets posts reach hosts inside the server's
  own network. `GET/POST /api/v1/webhooks`,
  `PATCH/DELETE /api/v1/webhooks/{id}`,
  `GET /api/v1/webhooks/{id}/deliveries`,
  `POST /api/v1/webhooks/{id}/deliveries/{deliveryId}/redeliver`,
  `POST /api/v1/webhooks/{id}/test`,
  `POST /api/v1/webhooks/{id}/rotate-secret` (#42).

- **`org-directory@1` and `responsibility@1`:** partners look a person
  up by the realm's issuer and subject, with their positions, units and
  roles, and walk the chain up and the reports down from them; read a
  unit's tree; ask who a role comes down to on a day, deputies standing
  in for vacant positions; and ask for a person's managers a given
  number of levels up. Every person in an answer carries the realm's
  issuer and subject. `GET /api/v1/people/by-subject`,
  `GET /api/v1/people/{id}/entry`, `GET /api/v1/people/by-subject/chain`,
  `GET /api/v1/people/by-subject/reports`, `GET /api/v1/units/{id}/tree`,
  `GET /api/v1/roles/{id}/people`, `GET /api/v1/people/{id}/managers`,
  `GET /api/v1/people/by-subject/managers` (#40).

- **Themes of one's own:** anybody makes a theme in an editor, starting
  blank or from Deep-Tech or Constellation: colours by group for light
  and dark, type, corners and shadows, cursors, icons, a backdrop picture
  and a moving picture (a constellation or confetti), files and extra
  CSS, previewed live on the page. A theme is its maker's until shared;
  an administrator may tidy a shared one and name the one everybody sees.
  The choice is kept on the server and follows the person between
  devices; the browser paints the theme it showed last before the page
  loads. The theme list has My and Shared views with a menu on each row;
  the sidebar steps through light, dark and the system's.
  `POST /api/v1/themes`, `GET/PATCH /api/v1/themes/{id}`,
  `GET/PUT /api/v1/themes/active`, `GET /api/v1/themes/default`,
  `GET /api/v1/themes/examples`, `POST /api/v1/themes/{id}/assets`,
  `DELETE /api/v1/themes/{id}/assets/{assetId}` (#84).

- **Graph explorer:** any unit, position, person or role and what is
  around it, opened node by node, following reporting lines, holdings,
  roles, deputies, units and responsibilities as switched on, as of a
  day; reached from each page, the relations also listed as sentences
  (#22).
- **CSV import and export:** units, positions, people and holdings out to
  four CSV files as of a day, and back in: a preview lists what each row
  would add or change and the rows that cannot be imported, by line; the
  worker runs the import in one transaction, all or nothing.
  `GET /api/v1/export/{kind}`, `POST /api/v1/imports`,
  `POST /api/v1/imports/{id}/run` (#24).
- **Editing the organisation:** editors add, change, move and archive
  units and positions, give positions and end or remove holdings, change
  people's entries and mark them gone, and add, change and delete roles,
  each from a day, on the pages where they read them; a new roles page
  lists every role. On the chart a card dropped onto another proposes
  that it reports there, or that its unit moves there, from a day (#23).
- **Capacity:** full-time equivalents of a person, a position, a unit
  with or without its sub-units, and a role, per day, week or month over
  up to two years, with a position's and a unit's seats filled and
  vacant; planned holdings and leavers count on their days.
  `GET /api/v1/{units,positions,roles,people}/{id}/capacity` (#87).
- **Targets:** what a unit, a position or a role should have, dated and
  audited; editors set one for a span on the subject's page, which shows
  it against what the subject has on the day. Capacity answers carry the
  target. `GET/POST /api/v1/targets`, `DELETE /api/v1/targets/{id}` (#88).
- **Gaps:** seats nobody fills, capacity under or over target, and roles
  nobody or only one person holds, each a run of days, on a new planning
  page for the whole organisation and on unit pages. `GET /api/v1/gaps`
  (#89).
- **Planning view:** units over weeks or months from the root or a unit,
  opening into sub-units, positions and their holders, each cell what is
  held against the target with gaps marked; editors plan a hire or a
  handover into a position where a gap shows; the grid downloads as CSV
  (#90).
- **`capacity@1`:** offered to partners at `/meta`, with people also named
  by the realm's issuer and `sub`. A person's capacity is for the person,
  their managers and editors; `docs/privacy.md` says who reads what (#91).
- **Search:** one box along the top finds people, positions, units, roles,
  processes and their steps as they are typed, ignoring case and accents
  and forgiving a slip; Enter opens every result on the search page, as of
  a day. `GET /api/v1/search` and `/search/quick`, and the MCP `search`
  tool answers from them (#21).

## 0.1.0 (2026-11-01)

The proof of concept: Bearing on its own, from setting it up to an assistant
asking it who is responsible. [`docs/poc.md`](docs/poc.md) walks through it.

- **Foundation:** the stack in containers with ports from the checkout's
  path, Postgres with a streaming replica and read-your-writes, local accounts
  with invitations and password resets, single sign-on through the suite's
  realm with join requests and group grants, the roles administrator, editor,
  member and reader held in the API and in the database, themes in
  `armature-theme/1`, the OpenAPI document, and one test gate for CI and the
  push hook.
- **Organisation model:** units and unit types, positions with solid and
  dotted reporting lines kept free of cycles, people local, from the realm or
  external, holdings with dates and shares, roles and deputies; every
  relation dated, so every read can be as of a day; an audit trail; and the
  example organisation Kestrel Works.
- **Views:** the organisation chart, as of a day and focused on a person,
  and pages for people, positions, units and roles.
- **Processes:** flow graphs drawn on a canvas, drafts and versions that
  never change once published, RACI on every step down to the people on a
  day with deputies standing in, who does what both ways, arrows routed
  around the nodes, swimlanes, and comparing versions.
- **Partners and assistants:** an OAuth authorization server with a
  consent page (`oauth.partner-tokens@1`), token exchange for the suite's
  partners (`oauth.token-exchange@1`), the MCP endpoint with ten reading
  tools (`mcp@1`), and personal access tokens.

Not in it yet, though planned for the proof of concept: search across the
organisation (#21), the graph explorer (#22), editing the organisation in
the web client (#23), and CSV import and export (#24).
