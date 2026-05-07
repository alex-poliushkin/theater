# HTTP Capabilities

HTTP capability refs cover request actions, HTTP inventory fetches, and the YAML
registries for sessions, auth bundles, and identities.

Source of truth:

- `go run ./cmd/theater explain action http`
- `go run ./cmd/theater explain inventory http.get`
- [YAML stage schema](../yaml/stage-schema.md)

## Checked HTTP Descriptors

<!-- theater-doc: command id=reference-http-action cwd=../../.. expect-stdout="Capability: action.http" expect-stdout-2="url       string; required" expect-stdout-3="status_code  number" -->
```sh
go run ./cmd/theater explain action http
```

<!-- theater-doc: command id=reference-http-inventory cwd=../../.. expect-stdout="Capability: inventory.http.get" expect-stdout-2="url       string; required" expect-stdout-3="Produces:" -->
```sh
go run ./cmd/theater explain inventory http.get
```

## `action.http`

| Input | Notes |
| --- | --- |
| `url` | Required request URL |
| `method` | HTTP method |
| `headers` | Header object |
| `body` | Raw body; incompatible with `json` and `form` |
| `json` | JSON body encoded from any runtime value; incompatible with `body` and `form` |
| `form` | Form body; incompatible with `body` and `json` |
| `timeout` | Request timeout text |
| `session` | Named HTTP session or `none` |
| `identity` | Named identity that can provide default session/auth |
| `auth` | Named auth config or `none` |

When `headers.User-Agent` is not supplied, Theater HTTP requests send
`theater-client/1.0`. An explicit `User-Agent` header overrides that default.

### Response Output Fields

These are the `field(...)` names available after `do action.http`.

| Field | Kind | Meaning |
| --- | --- | --- |
| `status_code` | number | Numeric HTTP response code, for example `200`. |
| `status` | string | Full HTTP status text, for example `200 OK`. |
| `headers` | object | Response headers as an object whose keys are header names and whose values are lists of strings. |
| `body` | string | Response body converted to a string. Use `decode(json)` before `path(...)` when the body is JSON. |

Examples: `field(status_code) == 200`,
`field(body) | decode(json) | path("/data/id")`,
and `field(headers) | path("/Content-Type/0")`.

## `inventory.http.get`

`inventory.http.get` fetches a remote resource body as `bytes`. It accepts
`url`, `headers`, `form`, `timeout`, `session`, `identity`, and `auth`.
It is an inventory value, not an action response, so use the property ref that
received it instead of `field(...)`.

## YAML Registries

| YAML path | Purpose |
| --- | --- |
| `http.sessions.<name>` | Named managed cookie session |
| `http.auth.<name>.attach[]` | Reusable auth attachment list |
| `http.identities.<name>.session` | Session bundled into an identity |
| `http.identities.<name>.auth` | Auth bundle selected by an identity |

For a task recipe, use [HTTP Sessions](../../how-to/use-http-sessions.md).
