# YAML Stage Schema

This page is a compact field lookup for the current YAML stage shape. It uses
the same source-of-truth contracts as the main [YAML reference](index.md).

Equivalent Theater DSL lookup:

- [Theater DSL reference](../theater-dsl/index.md)
- [Theater DSL core syntax](../theater-dsl/core-syntax.md)
- [Theater DSL checked example](../theater-dsl/index.md#checked-example)

## HTTP Registry

| Path | Meaning |
| --- | --- |
| `http.sessions.<name>` | Named managed cookie session |
| `http.auth.<name>.attach[]` | Reusable auth attachment list |
| `http.identities.<name>.session` | Session bundled into an identity |
| `http.identities.<name>.auth` | Auth bundle selected by an identity |

`session: none` is reserved and must not be declared as a session name.

Auth attachment kinds are `bearer`, `basic`, `api_key`, `header_slot`,
`query_slot`, and `form_slot`.

## State Registry

| Path | Meaning |
| --- | --- |
| `state.backends.<name>.use` | Backend capability ref |
| `state.backends.<name>.with` | Backend config |

The built-in backend is `state.backend.file`. Its current guarantee is
`local-atomic`.

## Action HTTP Inputs

| Input | Notes |
| --- | --- |
| `method` | HTTP method |
| `url` | Request URL |
| `headers` | Header map |
| `body` | Raw body; mutually exclusive with `json` and `form` |
| `form` | Form body; mutually exclusive with `body` and `json` |
| `json` | JSON body; mutually exclusive with `body` and `form` |
| `timeout` | Request timeout |
| `session` | Named session or `none` |
| `identity` | Named identity |
| `auth` | Named auth or `none` |

`json` sets `Content-Type: application/json` when the header is absent.
Conflicting explicit `Content-Type` with `json` is invalid.
When `headers.User-Agent` is absent, Theater HTTP requests send
`theater-client/1.0`. An explicit `User-Agent` header overrides that default.

## Command Action Inputs

| Input | Notes |
| --- | --- |
| `executable` | Required executable name or path |
| `args` | Argument list |
| `env` | Environment map |
| `working_dir` | Working directory |
| `stdin` | Input text |
| `timeout` | Process timeout |

`action.command` is direct process execution, not a shell.

## State Action Inputs

| Action | Required handle | Notes |
| --- | --- | --- |
| `action.state.read` | `record` | returns `value` and `version` |
| `action.state.update` | `record` | requires `expected_version`; blind write is not supported |
| `action.state.claim` | `pool` | requires `lease.ttl`; `lease.on_expiry` is `stale` or `reclaim` |
| `action.state.renew` | `claim` | requires `ttl` |
| `action.state.release` | `claim` | no outputs |
| `action.state.consume` | `claim` | optional `tombstone` |

Pool selectors support exact top-level field matches in v1.

## Capture Auth

`capture_auth` is valid only with `action.http`.

| Source | Meaning |
| --- | --- |
| `response_header` | Capture value from response header |
| `response_cookie` | Capture value from response cookie |
| `json_pointer` | Capture value from JSON response body |
| `form_field` | Capture value from form response body |

Captured slots update the named auth bundle for later HTTP calls.

## Act Logs

`acts[].logs` declares scenario-authored report observations. Logs evaluate
after successful action execution and appear in JSON reports under
`report.logs`. Text live runs mirror bounded log preview lines to stderr when
live output is enabled; stdout still carries only the selected command output format.
Theater DSL supports compact `log <id> = <log-value>` syntax over the same
`LogSpec` model. Report output retains up to 32 log records per act and 1024
per run. Summary previews are capped at 4096 bytes per log record, with dropped
and truncated counts under `report.log_summary`. One act may declare at most 32
logs; repeated attempts after the retained limit emit compact dropped events
rather than evaluating additional log values.

| Path | Meaning |
| --- | --- |
| `acts[].logs[].id` | Act-local log id |
| `acts[].logs[].value` | Dynamic value expression |
| `acts[].logs[].message` | Static text message |
| `acts[].logs[].fields.<name>` | Dynamic field paired with `message` |
| `acts[].logs[].format` | `text` or `json` |
| `acts[].logs[].capture` | `omit` or `summary` |
| `acts[].logs[].sensitivity` | `internal`, `personal`, or `secret` |
| `acts[].logs[].required` | Treat runtime log evaluation errors as act failures |

Log entries use either `value` or `message`. `fields` requires `message`.
Runtime log evaluation errors are non-fatal by default. `required: true` turns a
runtime log evaluation error into an act failure before expectations run.
`capture` and `sensitivity` control report preview and payload metadata in JSON
output. The default capture mode omits the value preview. `capture: summary`
stores a bounded preview and may include selected plaintext; use `capture: omit`
to suppress previews and `sensitivity: secret` for secret values.

Log value expressions support:

| Field | Meaning |
| --- | --- |
| `field` | Select a current action output |
| `ref` | Select a scenario-scope value |
| `object` | Build an object from nested log values |
| `list` | Build a list from nested log values |
| `decode` | Decode selected value, currently `json` |
| `path` | RFC 6901 path over selected value |
| `through` | Selector pipeline, including path, pick, regexp, and transform steps |

## Act Exports

`acts[].exports` publishes selected values to scenario scope after successful
act completion. A failed act or failed eventual attempt does not commit its
exports.

| Path | Meaning |
| --- | --- |
| `acts[].exports[].as` | Exported name; defaults to `field` or `ref.name` |
| `acts[].exports[].field` | Select a current action output |
| `acts[].exports[].ref.name` | Select an available value from current act scope |
| `acts[].exports[].decode` | Decode selected value, currently `json` |
| `acts[].exports[].path` | RFC 6901 path over selected value |
| `acts[].exports[].through` | Selector pipeline, including path, pick, regexp, and transform steps |

Use exactly one of `field` or `ref` on each act export. `field` is for action
outputs such as `status_code`, `body`, or `values`. `ref` is for already
available scope values such as scenario inputs, prior exports, and current-act
properties.
