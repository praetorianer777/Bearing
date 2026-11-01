# Integration capabilities

Proposed. None of these names is in Dynamo's `docs/compatibility.md` yet;
each is added there by pull request before Bearing reports it. Bearing serves
`theme-export@1`, `oauth.partner-tokens@1`, `oauth.token-exchange@1`,
`mcp@1`, `capacity@1`, `org-directory@1` and `responsibility@1` already, in the shape the siblings serve them where
they share one, and lists them at `/meta`.

## MCP

`POST /api/v1/mcp` speaks MCP over Streamable HTTP, protocol revisions
2025-11-25 and 2025-06-18, one JSON-RPC request at a time; it answers with
JSON and opens no stream. Without a session or a token it answers 401 with
`WWW-Authenticate` naming the protected resource metadata, where a client
learns how to connect. Every tool reads, as the person, within their role,
through the API's own operations; each takes `asOf` for another day than
today:

| Tool | Answers |
|---|---|
| `whoami` | The account the assistant acts for, and the person it is linked to. |
| `search` | People, positions, units, roles, processes and steps whose name holds a word, with their ids. |
| `get_person`, `get_unit`, `get_position` | One of them as it stands on a day; a unit with its members. |
| `management_chain` | Whom a person or position reports to, nearest first. |
| `reports` | Who reports to a person or position, directly or all below. |
| `responsibilities_of` | What a person, position, role or unit answers for, by RACI kind. |
| `who_is_responsible` | The published steps a word names, and who each comes down to. |
| `get_process` | A process's newest or a numbered version, with who each step comes down to. |

## Connecting an app

An app reads every address from
`/.well-known/oauth-authorization-server`, and an MCP client finds that
document through `/.well-known/oauth-protected-resource/api/v1/mcp`, which
a 401 at the MCP endpoint names. The app registers itself at
`POST /api/v1/oauth/register` and waits, pending, until an administrator
allows it by connecting it once on the consent page; an app nobody allows
within a day is removed. Then anybody signed in connects it there, by
authorization code with PKCE S256, for `read`, or `read write` to change
what their role lets them. The grant acts at the MCP endpoint only: its
access token lives an hour, its refresh token 90 days from its last use,
and both rotate at each refresh. A refresh token or a code presented a
second time ends the grant. People disconnect their apps under Connected
apps; administrators remove an app for everybody there too. Tokens carry
their kind in their prefix: `bearing_oat_` access, `bearing_ort_`
refresh, `bearing_ocs_` client secret.

## Personal access tokens

A script or an assistant that cannot connect on a consent page uses a
personal access token, which its person makes under Access tokens in the
account menu: `bearing_pat_`, read only or acting as far as the person may,
for 7 days up to a year, shown once. It is sent as
`Authorization: Bearer <token>` to the API and to the MCP endpoint alike,
and stops working the moment it is revoked or its day passes. A token can
neither make another token nor connect an app; that needs the person
signed in.

## Capacity

`capacity@1` is what the organisation has, in full-time equivalents, against
what it should have, over a range of days. Every answer is read as the person
the call acts for.

```
GET /api/v1/units/{id}/capacity?from=&to=&by=day|week|month&subUnits=true
GET /api/v1/positions/{id}/capacity?from=&to=&by=
GET /api/v1/roles/{id}/capacity?from=&to=&by=
GET /api/v1/people/{id}/capacity?from=&to=&by=
GET /api/v1/people/by-subject/capacity?issuer=&subject=&from=&to=&by=
GET /api/v1/targets?unitId=|positionId=|roleId=&asOf=
GET /api/v1/gaps?from=&to=&unitId=&subUnits=true
```

- A range runs from `from` to `to`, both counted, today and thirteen weeks
  on by default, at most 731 days.
- A period is a day, a week from Monday or a month; a week or a month is
  the mean of its working days in the range, and the first and last are
  cut by it.
- Each period has `fte`, what the holdings' shares add up to; `people`, the
  most holding on one day; `target`, the mean target of its days that have
  one; and for positions and units `seats`, `filled` and `vacant`.
- A unit counts its positions, with `subUnits` those of the units placed
  under it on each day; its target is for the unit with everything inside.
- A role counts its holders directly and through the positions carrying it.
- A gap is a run of days of one size: `vacant` seats, `under_target` or
  `over_target`, a role `unheld` or with a `single_holder`.
- A person's capacity is for the person, those above them in the management
  chain today, and editors; anybody else is refused with 403. Sums over a
  unit, a position or a role are for everybody signed in. Partners name a
  person by the realm's issuer and `sub` with `by-subject`.

## Directory and responsibility

`org-directory@1` is who the organisation has and how it hangs together;
`responsibility@1` is who answers for something. Both read as the person
the call acts for, on the day `asOf` names, today when left out. Partners
name people by the realm's issuer and `sub`; every person in an answer
carries `issuer` and `subject`, null for somebody the realm does not know.

```
GET /api/v1/people/by-subject?issuer=&subject=&asOf=
GET /api/v1/people/{id}/entry?asOf=
GET /api/v1/people/by-subject/chain?issuer=&subject=&dotted=
GET /api/v1/people/by-subject/reports?issuer=&subject=&all=&dotted=
GET /api/v1/people/{id}/chain, /people/{id}/reports
GET /api/v1/units/{id}/tree?asOf=
GET /api/v1/search?q=
GET /api/v1/roles/{id}/people?asOf=
GET /api/v1/processes/{id}/steps/{key}/people?asOf=&version=
GET /api/v1/people/{id}/managers?levels=&asOf=
GET /api/v1/people/by-subject/managers?issuer=&subject=&levels=&asOf=
```

- An entry is the person with the positions they hold, heads of units
  first, each with its unit and share, and the roles they have, directly
  or `through` a position.
- A unit's tree is the unit and every unit inside it, each after the one
  it is part of, with its `depth` below the first.
- A role comes down to those holding it directly, then for each position
  carrying it its holders, or when it is vacant the holders of its first
  deputy with any, marked `deputyFor`. A step comes down to people the
  same way, a unit to its head.
- `managers` are the positions exactly `levels` solid lines up, one when
  left out, with who they come down to; a person holding two positions
  has a manager in each chain, and none where the chain ends sooner. This
  is what an approval routed to "the manager two levels up" asks.
- An unknown subject answers 404; a call without both issuer and subject
  answers 422.

## Trading a realm token

A partner app of the suite that signs people in through the realm is
registered by an administrator under Connected apps, with the realm client
it signs them in through; its secret is shown once. It trades a person's
realm access token at the token endpoint (RFC 8693):

```
POST /api/v1/oauth/token
Authorization: Basic <client id>:<secret>

grant_type=urn:ietf:params:oauth:grant-type:token-exchange
&subject_token=<the realm access token>
&subject_token_type=urn:ietf:params:oauth:token-type:access_token
&scope=read write
```

The realm token must be signed by the realm, live, issued to the partner's
client (`azp`) and name `bearing` among its audiences (`aud`), which an
audience mapper on the partner's client adds. The person needs a Bearing
account, found or made as their sign-in would; somebody who has none asks
to join and is refused until an administrator lets them in. The answer is
a grant of theirs, with a refresh token, that acts on the whole API within
their role and the scopes.

Any app may also trade an access token of its own grant for a narrower
one, for one run: `subject_token` is then the `bearing_oat_` token,
`scope` may narrow to `read`, and `bearing_expires_in` may shorten it
below the grant's remaining hour. A narrower token has no refresh token,
ends with its grant, and is revoked like any other.

`GET /api/v1/meta` lists, under `capabilities`, the integration features this
build offers, as stable names with a version, in the scheme the suite uses. A
partner decides from these names whether a feature is there, not from the
version number of the build. `/meta` answers anyone; the names promise
features, not data.

```json
{
  "product": "bearing",
  "build": { "version": "0.1.0" },
  "apiVersion": "1",
  "capabilities": ["capacity@1", "mcp@1", "oauth.partner-tokens@1", "oauth.token-exchange@1", "org-directory@1",
    "responsibility@1", "theme-export@1"]
}
```

Every call is made as the person it acts for, with a session, a personal
access token or a partner token; no capability needs a shared service
account. People are named by the realm's issuer and `sub` where they have
one.

## Shared with the siblings

| Capability | Promises |
|---|---|
| `mcp@1` | An assistant works with the organisation over MCP as the person whose token it holds: `POST /mcp`. |
| `oauth.partner-tokens@1` | Authorization code flow with PKCE, refresh tokens and revocation, apps registered by RFC 7591 and allowed by an administrator. |
| `oauth.token-exchange@1` | A realm access token traded for a Bearing token within the person's rights (RFC 8693). |
| `theme-export@1` | A theme exported and imported as one `armature-theme/1` file. |
| `webhooks@1` | Endpoints subscribed to topics with the suite's signed, versioned envelope. Topics: `position.holder_changed`, `position.vacated`, `reporting_line.changed`, `unit.changed`, `responsibility.changed`. |

## Bearing's own

| Capability | Promises | For |
|---|---|---|
| `org-directory@1` | A person by `sub` with their positions, roles and units; search; a unit's tree; the management chain up from a person and their reports down. | Rotor, Armature, Exciter, Stator |
| `responsibility@1` | Who holds a role or answers for a process step on a date, deputies included, and the managers of a person a given number of levels up. | Rotor approvals, Armature routing, Exciter ownership |
| `capacity@1` | Full-time equivalents of units, positions, roles and people over a range, against dated targets, and the gaps where people are missing. | Armature, Stator, Rotor |
| `org-embed@1` | Person, role and unit cards for chips, found by the words typed so far. | Stator, Armature |
| `process-remote-links@1` | Another tool puts links to its objects on a process step, keyed by url, a link with the same url replacing the one there. | Stator, Exciter, Armature, Rotor |
