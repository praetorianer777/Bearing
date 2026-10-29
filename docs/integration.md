# Integration capabilities

Proposed. None of these names is in Dynamo's `docs/compatibility.md` yet;
each is added there by pull request before Bearing reports it.

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
