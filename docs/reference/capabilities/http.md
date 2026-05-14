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
| `scenarios[].auth_bindings.<name>.slots.<slot>` | Scenario-start auth slot binding |

Auth attachments are typed. Bearer auth accepts exactly one of:

- `token`: a static bearer token stored in the stage file.
- `token_slot`: a scenario-local auth slot. Bind the slot from scenario inputs
  with `auth_bindings` or populate it with `capture_auth` before a request uses
  the auth bundle.

Dynamic bearer slot values are stored as secret-sensitive auth state. Missing,
empty, non-string, or typed-missing bearer slot values fail before the HTTP
request is sent. `auth: none` remains the explicit request-level opt-out, and
manual `headers.Authorization` remains incompatible with typed bearer/basic
auth.

Checked Theater DSL example using a scenario-start bearer slot:

<!-- theater-doc: source id=reference-dynamic-http-auth-thtr kind=thtr path=../../examples/reference/dynamic-http-auth.thtr pair=reference-dynamic-http-auth checks=fmt,lower,validate -->
```thtr
stage reference-dynamic-http-auth

http
  auth mobile_api = http.auth
    attach: list [
      object { bearer: object { token_slot: "access_token" } }
    ]

scenario mobile/dashboard-ready(access_token: string!)
  bind auth mobile_api
    access_token: $access_token
  act wait-customer
    do action.http
      method: "GET"
      url: "https://gateway.example.test/customer"
      session: "none"
      auth: "mobile_api"

call run-dashboard = mobile/dashboard-ready(access_token: "issued-token")
```

Checked YAML example using the same auth slot:

<!-- theater-doc: source id=reference-dynamic-http-auth-yaml kind=yaml path=../../examples/reference/dynamic-http-auth.yaml pair=reference-dynamic-http-auth checks=validate -->
```yaml
id: reference-dynamic-http-auth
http:
  auth:
    mobile_api:
      attach:
        - bearer:
            token_slot: access_token
scenarios:
  - id: mobile/dashboard-ready
    inputs:
      access_token:
        type: string
        required: true
        sensitivity: secret
        capture: omit
    auth_bindings:
      mobile_api:
        slots:
          access_token:
            kind: ref
            ref: access_token
    acts:
      - id: wait-customer
        action:
          use: action.http
          with:
            method: GET
            url: https://gateway.example.test/customer
            session: none
            auth: mobile_api
scenario_calls:
  - id: run-dashboard
    scenario_id: mobile/dashboard-ready
    bindings:
      access_token: issued-token
```

For a task recipe, use [HTTP Sessions](../../how-to/use-http-sessions.md).
