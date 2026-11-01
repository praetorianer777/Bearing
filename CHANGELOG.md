# Changelog

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
