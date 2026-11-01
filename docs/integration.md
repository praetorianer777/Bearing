# Integration capabilities

Proposed. None of these names is in Dynamo's `docs/compatibility.md` yet;
each is added there by pull request before Bearing reports it. Bearing serves
`theme-export@1`, `oauth.partner-tokens@1` and `oauth.token-exchange@1`
already, in the shape the siblings serve them, and lists them at `/meta`.

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
  "capabilities": ["mcp@1", "oauth.partner-tokens@1", "oauth.token-exchange@1", "theme-export@1"]
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
| `org-embed@1` | Person, role and unit cards for chips, found by the words typed so far. | Stator, Armature |
| `process-remote-links@1` | Another tool puts links to its objects on a process step, keyed by url, a link with the same url replacing the one there. | Stator, Exciter, Armature, Rotor |
