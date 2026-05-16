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

### Failure Diagnostics Contract

The v0.4 report contract emits HTTP failure diagnostics as node-scoped report
data. They are not `action.http` output fields and cannot be selected with
`field(...)`.

When emitted, an HTTP diagnostic can describe a failed transport or request
assembly on the failed action node, or a failed expectation that inspected a
completed HTTP response on the failed expectation node. The diagnostic includes
the HTTP action address and uses report-safe data: method, redacted resolved
URL, status and duration when available, allowlisted response headers, and a
bounded response preview. Request bodies, authorization headers, cookies, typed
auth material, session state, and raw response bodies are not diagnostic
carriers.

URL path segment values and query values are redacted by default, userinfo is
hidden, and URL fragments are omitted. Response header projection is
allowlist-based and excludes authorization, proxy authorization, cookie,
set-cookie, unknown header values, and credential-like values even under
allowlisted header names. Body previews use report `Preview` semantics. Content
type or valid UTF-8 alone is not enough to expose response text; unclassified
textual bodies, binary bodies, and unknown bodies use metadata-only previews by
default.

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
  auth service_api = http.auth
    attach: list [
      object { bearer: object { token_slot: "session_token" } }
    ]

scenario service/sample-ready(session_token: string!)
  bind auth service_api
    session_token: $session_token
  act get-sample-resource
    do action.http
      method: "GET"
      url: "https://api.example.test/sample-resource"
      session: "none"
      auth: "service_api"

call run-sample = service/sample-ready(session_token: env("THEATER_DOC_SERVICE_SESSION_TOKEN"))
```

Checked YAML example using the same auth slot:

<!-- theater-doc: source id=reference-dynamic-http-auth-yaml kind=yaml path=../../examples/reference/dynamic-http-auth.yaml pair=reference-dynamic-http-auth checks=validate -->
```yaml
id: reference-dynamic-http-auth
http:
  auth:
    service_api:
      attach:
        - bearer:
            token_slot: session_token
scenarios:
  - id: service/sample-ready
    inputs:
      session_token:
        type: string
        required: true
        sensitivity: secret
        capture: omit
    auth_bindings:
      service_api:
        slots:
          session_token:
            kind: ref
            ref: session_token
    acts:
      - id: get-sample-resource
        action:
          use: action.http
          with:
            method: GET
            url: https://api.example.test/sample-resource
            session: none
            auth: service_api
scenario_calls:
  - id: run-sample
    scenario_id: service/sample-ready
    bindings:
      session_token:
        kind: env
        name: THEATER_DOC_SERVICE_SESSION_TOKEN
```

## Repo-Aware Reusable Auth

Repo-aware flow loading composes selected library auth entries into the
assembled flow before validation. This is authoring/load behavior only. It is
not runtime fallback, does not add new Go API surface, and is not a generic
import/include mechanism.

Only library files selected by the flow's scenario calls can contribute auth
entries. Unselected library files do not contribute registries and cannot cause
auth-name collisions. When a library file is selected, every auth declaration
in that file must be slot-backed and non-colliding; only auth names referenced
by selected scenarios are copied into the assembled flow.

Selected library auth entries are composable only when every attachment is
slot-backed. Static bearer tokens, basic credentials, and static API key values
declared in a selected library are rejected before run. Duplicate auth names
across the flow and selected libraries, or across selected libraries, are hard
validation errors even when definitions match textually.

Checked Theater DSL library file:

<!-- theater-doc: source id=reusable-auth-library-thtr kind=thtr path=../../examples/reusable-auth/theater/lib/service/sample-ready.thtr pair=reusable-auth-library checks=fmt,lower,validate -->
```thtr
stage reusable-auth-service-lib

http
  auth service_api = http.auth
    attach: list [
      object { bearer: object { token_slot: "session_token" } }
    ]

scenario service/sample-ready(session_token: string!)
  bind auth service_api
    session_token: $session_token
  act get-sample
    do action.http
      method: "GET"
      url: "https://api.example.test/sample"
      session: "none"
      auth: "service_api"
```

Checked Theater DSL flow file. The flow calls the library scenario and supplies
the runtime slot value, but it does not repeat `http.auth.service_api`:

<!-- theater-doc: source id=reusable-auth-flow-thtr kind=thtr path=../../examples/reusable-auth/theater/flows/sample-ready.thtr pair=reusable-auth-flow checks=fmt,lower,validate -->
```thtr
stage reusable-auth-sample

call run-sample = service/sample-ready(session_token: env("THEATER_DOC_SERVICE_SESSION_TOKEN"))
```

Checked YAML library file:

<!-- theater-doc: source id=reusable-auth-library-yaml kind=yaml path=../../examples/reusable-auth/theater/lib/service/sample-ready.yaml pair=reusable-auth-library checks=validate -->
```yaml
id: reusable-auth-service-lib
http:
  auth:
    service_api:
      attach:
        - bearer:
            token_slot: session_token
scenarios:
  - id: service/sample-ready
    inputs:
      session_token:
        type: string
        required: true
        sensitivity: secret
        capture: omit
    auth_bindings:
      service_api:
        slots:
          session_token:
            kind: ref
            ref: session_token
    acts:
      - id: get-sample
        action:
          use: action.http
          with:
            method: GET
            url: https://api.example.test/sample
            session: none
            auth: service_api
```

Checked YAML flow file:

<!-- theater-doc: source id=reusable-auth-flow-yaml kind=yaml path=../../examples/reusable-auth/theater/flows/sample-ready.yaml pair=reusable-auth-flow checks=validate -->
```yaml
id: reusable-auth-sample
scenarios: []
scenario_calls:
  - id: run-sample
    scenario_id: service/sample-ready
    bindings:
      session_token:
        kind: env
        name: THEATER_DOC_SERVICE_SESSION_TOKEN
```

For a task recipe, use [HTTP Sessions](../../how-to/use-http-sessions.md).
