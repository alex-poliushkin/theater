# Action Capabilities

Action capabilities execute one act step and publish action outputs.

Source of truth:

- `go run ./cmd/theater explain action`
- `go run ./cmd/theater explain action <action-ref>`
- [YAML stage schema](../yaml/stage-schema.md)

## Checked Action Catalog

<!-- theater-doc: command id=reference-actions-family cwd=../../.. expect-stdout="Capabilities (9):" expect-stdout-2="action.http" expect-stdout-3="action.state.update" -->
```sh
go run ./cmd/theater explain action
```

<!-- theater-doc: command id=reference-actions-command cwd=../../.. expect-stdout="Capability: action.command" expect-stdout-2="executable   string; required" expect-stdout-3="stdout     string" -->
```sh
go run ./cmd/theater explain action command
```

<!-- theater-doc: command id=reference-actions-generate cwd=../../.. expect-stdout="Capability: action.generate" expect-stdout-2="outputs  object; required" expect-stdout-3="values  object" -->
```sh
go run ./cmd/theater explain action generate
```

## Built-In Actions

| Ref | Purpose | Main outputs |
| --- | --- | --- |
| `action.command` | Direct process execution without a shell | `exit_code`, `stdout`, `stderr` |
| `action.generate` | Materialize generated values into action output | `values` |
| `action.http` | HTTP request | `status_code`, `status`, `headers`, `body` |
| `action.state.read` | Read a persistent state record | `value`, `version` |
| `action.state.update` | Compare-and-set update of a persistent record | `value`, `version` |
| `action.state.claim` | Claim a pool item | `item`, `claim` |
| `action.state.renew` | Renew a pool claim lease | `claim` |
| `action.state.release` | Release a pool claim | none |
| `action.state.consume` | Consume a pool claim | none |

## Action Output Fields

Use these names with `field(...)` in the same act after the action runs. In a
later act, first export the selected value and then use the exported `$ref`.

`field(...)` selects an action output field, not a nested member. When an output
is an object, use `path(...)` after `field(...)` to select a member.

### `action.command`

| Field | Kind | Meaning |
| --- | --- | --- |
| `exit_code` | number | Process exit code. Non-zero exits are captured as normal completed command outputs. |
| `stdout` | string | Captured standard output text. |
| `stderr` | string | Captured standard error text. |

Examples: `field(exit_code)`, `field(stdout)`, `field(stderr)`.

### `action.generate`

| Field | Kind | Meaning |
| --- | --- | --- |
| `values` | object | Object containing the resolved entries from the action `outputs` input. |

Example: if `outputs.profile_id` is authored, select it as
`field(values) | path("/profile_id")`. `field(profile_id)` is not a valid
selector for an `action.generate` value unless a different action explicitly
publishes a top-level `profile_id` output field.

### `action.http`

| Field | Kind | Meaning |
| --- | --- | --- |
| `status_code` | number | Numeric HTTP response code, for example `200`. |
| `status` | string | Full HTTP status text, for example `200 OK`. |
| `headers` | object | Response headers as an object whose keys are header names and whose values are lists of strings. |
| `body` | string | Response body converted to a string. Use `decode(json)` before `path(...)` when the body is JSON. |

Examples: `field(status_code)`, `field(status)`,
`field(headers) | path("/Set-Cookie/0")`, and
`field(body) | decode(json) | path("/data/id")`.

### State Actions

| Action | Field | Kind | Meaning |
| --- | --- | --- | --- |
| `action.state.read` | `value` | object | Current record value. |
| `action.state.read` | `version` | string | Current record version returned by the backend. |
| `action.state.update` | `value` | object | Stored record value after the compare-and-set update succeeds. |
| `action.state.update` | `version` | string | New record version returned by the backend. |
| `action.state.claim` | `item` | object | Claimed pool item. |
| `action.state.claim` | `claim` | any | Opaque claim handle for `renew`, `release`, or `consume`. Export it when later acts need the claim. |
| `action.state.renew` | `claim` | any | Renewed opaque claim handle for later state actions. |
| `action.state.release` | none | - | This action publishes no output fields. |
| `action.state.consume` | none | - | This action publishes no output fields. |

`action.command` is direct process execution. It does not run through a shell.

For HTTP inputs, open [HTTP Capabilities](http.md). For persistent state inputs,
open [State Capabilities](state.md).

For procedures, use [Validate And Run A Flow](../../how-to/validate-and-run-a-flow.md),
[Read Run Output](../../how-to/read-run-output.md), and
[Persistent State](../../how-to/use-persistent-state.md).
