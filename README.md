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

## Working on it

Docker, make, `jq` and `python3` are all the host needs; every toolchain runs
in a container.

```sh
make up          # Postgres with a streaming replica, Valkey, Mailpit, Keycloak, the api, the worker and the web client
make ports       # this checkout's compose project and its twenty ports
make down        # stop (make clean also drops the volumes)
make realm-check # compare the realm with Dynamo's canonical copy
make check-go    # the backend's formatting, vet, unit tests and API document drift
make openapi     # regenerate api/openapi.json from the route table, and the web client's types
make test-integration  # the integration suite against the running stack
make check-web   # the web client's lint, type check and unit tests
make web-dev     # the web client with hot reload, against the running stack's api
make test-e2e    # the browser suite against the running stack (ONLY=<grep> for some of it)
make help        # every target
./run-tests.sh   # the whole gate, as CI and the push hook run it (--list names its layers)
```

The stack's seed sets the installation up as "Demo organisation" with the
administrator `admin@bearing.test` / `bearing admin password`; mail goes to
Mailpit. Without `BEARING_BOOTSTRAP_ADMIN_EMAIL` nothing is seeded, and whoever
opens the web client first sets the installation up.

The seed also loads the demo organisation, Kestrel Works: about sixty people
in four levels of units, with dotted lines, two vacancies, deputies, a few
roles and a Head of Product who changed in April 2026, to read as of a day.
`alice`, `bob`, `carol` and `dave` are in it by address and are linked to their
entries when they first sign in. Its six processes come along: from an order
to its delivery to a new colleague's onboarding, with decisions, a
subprocess, two versions of one process and a draft not yet publishable.
`make seed` runs the seed again and changes nothing; `BEARING_DEMO=false make
up` starts empty. An administrator loads what of it may still go in under
Settings, Example data, or from the empty organisation chart.

"Continue with single sign-on" signs in through the stack's Keycloak. The seed
lets `alice` in as an administrator and `bob` as a member ahead of time
(`BEARING_BOOTSTRAP_MEMBERS`); `carol` and `dave` ask to join, and an
administrator lets them in under Settings, Accounts, one by one or by their
realm groups. Groups follow people at every sign-in. Without
`BEARING_OIDC_ISSUER` Bearing offers passwords only.

Every port is bound to `127.0.0.1` and derived from the checkout's path, in
61000 to 65499, clear of the siblings' ranges, so worktrees and sibling stacks
run side by side; the ports are also written to `.cache/stack.env`.
Keycloak's realm `dynamo` (admin console `admin` / `admin`) is the suite's
realm with a client `bearing` (secret `bearing-dev-secret`, PKCE) and a group
`bearing-administrators` added; alice, bob, carol and dave sign in with
`<name> password`. The secrets and passwords are for development only.

Every change hangs off an issue and a branch named `<type>/<issue>-<slug>`
and lands by pull request; `.claude/skills/gh/SKILL.md` says how, and
`.claude/hooks/branch-guard.sh` holds an agent to it, running the quick gate
before a push (`BEARING_GATE=full` for all of it). Coverage may not fall below
the floors in `coverage/`, each the measured total rounded down.

## License

[Apache License 2.0](LICENSE). Contains code adapted from Exciter and
Rotor, under the same license; see [NOTICE](NOTICE).
