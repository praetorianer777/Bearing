# Bearing

**Organisation modeling**, self-hosted, on its own or beside
[Armature](https://github.com/Cloudster1/Armature),
[Exciter](https://github.com/Cloudster1/Exciter),
[Stator](https://github.com/praetorianer777/Stator) and
[Rotor](https://github.com/Cloudster1/Rotor). Model the organisation as a
graph of units, positions, roles and people with their reporting lines, draw
its processes as flow graphs whose steps say who is responsible, accountable,
consulted and informed, and search it both ways: what a person answers for,
and who answers for a task. Same look, same themes, same single sign-on as
its siblings.

An installation serves one organisation. It runs alone with local accounts,
or signs in through the suite's Keycloak realm `dynamo` and offers what it
knows to the other tools over its API, its webhooks and MCP.

Pre-1.0 and in planning. What is planned and why:

- [`docs/plan.md`](docs/plan.md): the domain model, the architecture, the
  milestones and what the proof of concept holds.
- [`docs/decisions.md`](docs/decisions.md): the decisions taken so far and
  their reasons.
- [`docs/integration.md`](docs/integration.md): the capabilities Bearing
  proposes to offer the suite.
- The [milestones](https://github.com/praetorianer777/Bearing/milestones) and
  [issues](https://github.com/praetorianer777/Bearing/issues).

## Stack

Go (chi, pgx, goose), PostgreSQL with CloudNativePG, Valkey, React with
Tailwind and React Flow, OIDC single sign-on and local accounts. Every
toolchain runs in a container.

## License

[Apache License 2.0](LICENSE).
