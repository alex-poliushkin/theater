# YAML Reference

YAML is a first-class Theater authoring format. It uses the canonical stage
shape directly and stays equal to Theater DSL in the public docs.

Source of truth:

- [YAML stage schema](stage-schema.md)
- [Theater DSL reference](../theater-dsl/index.md)
- `theater validate`

Equivalent Theater DSL lookup:

- [Theater DSL reference](../theater-dsl/index.md)
- [Theater DSL checked example](../theater-dsl/index.md#checked-example)
- [Theater DSL core syntax](../theater-dsl/core-syntax.md)

## Checked Example

<!-- theater-doc: source id=reference-yaml-first-stage kind=yaml path=../../examples/first-stage/stage.yaml pair=reference-first-stage checks=validate -->
```yaml
id: docs-first
scenarios:
  - id: hello
    acts:
      - id: say-hello
        action:
          use: action.generate
          with:
            outputs:
              kind: object
              object:
                message: hello
        expectations:
          - id: message
            subject:
              field: values
              path: /message
            assert:
              ref: expectation.equal
              args:
                expected: hello
scenario_calls:
  - id: run
    scenario_id: hello
```

## Top-Level Fields

| Field | Required | Meaning |
| --- | --- | --- |
| `id` | yes | Stage id |
| `name` | no | Optional display name |
| `http` | no | Shared HTTP session, auth, and identity registries |
| `state` | no | Shared persistent-state backend registry |
| `scenarios` | no | Reusable scenario definitions |
| `scenario_calls` | no | Concrete stage-level invocations |

Repo-aware YAML flow files can live under `theater/flows/`. Reusable library
files live under `theater/lib/`, contribute reusable scenarios, and must not
declare `scenario_calls`.

## Scenario Calls

| Field | Required | Meaning |
| --- | --- | --- |
| `id` | yes | Scenario call id |
| `scenario_id` | yes | Target scenario id |
| `bindings` | no | Input bindings for the target scenario |
| `exports` | no | Values committed to stage scope after success |
| `dependencies` | no | Explicit upstream gating |

Dependency `when` values are `success`, `failure`, and `done`.

## Scenarios And Acts

| Scenario field | Required | Meaning |
| --- | --- | --- |
| `id` | yes | Scenario id |
| `inputs` | no | Input contracts keyed by input name |
| `acts` | no | Ordered act definitions |

| Act field | Required | Meaning |
| --- | --- | --- |
| `id` | yes | Act id |
| `eventually` | no | Whole-act polling window |
| `properties` | no | Current-act values resolved before action |
| `action` | yes | Action call |
| `capture_auth` | no | HTTP auth-state capture |
| `logs` | no | Scenario-authored log declarations |
| `expectations` | no | Assertions over action output or properties |
| `exports` | no | Values exported after successful act completion |
| `transitions` | no | Explicit act graph edges |

Transition `on` values are `on_pass`, `on_fail`, `on_timeout`, and `on_cancel`.

`logs` declares report-oriented observations. Logs evaluate after a successful
action and before expectations, exports, and transitions. `run --format json`
includes emitted, omitted, and error records under `result.report.logs`; inside
the nested report object the field is `logs`. Text live runs mirror bounded log
preview lines to stderr when live output is enabled; stdout still carries only
the selected command output format. Theater DSL supports compact
`log <id> = <log-value>` syntax over the same `LogSpec` model. Report output
retains up to 32 log records per act and 1024 per run. Summary previews are
capped at 4096 bytes per log record, with dropped and truncated counts under
`result.report.log_summary` in CLI JSON output. One act may declare at most 32
logs; repeated attempts after the retained limit emit compact dropped events
rather than evaluating additional log values.

Use [Scenario Logs](../logs.md) for the full cross-syntax log contract, checked
commands, and report behavior.

Log entries support:

| Log field | Meaning |
| --- | --- |
| `id` | Act-local log id |
| `value` | Dynamic value expression |
| `message` | Static text message |
| `fields` | Dynamic fields paired with `message` |
| `format` | `text` or `json` |
| `capture` | `omit` or `summary` |
| `sensitivity` | `internal`, `personal`, or `secret` |
| `required` | Treat runtime log evaluation errors as act failures |

Use either `value` or `message`. `fields` requires `message`. Ordinary runtime
log evaluation errors are non-fatal by default. `required: true` turns a runtime
log evaluation error into an act failure before expectations run. Run
cancellation or deadline expiry still terminates the act independently of
`required`.
`capture` and `sensitivity` control report preview and payload metadata in JSON
output. The default capture mode omits the value preview. `capture: summary`
stores a bounded preview and may include selected plaintext; use `capture: omit`
to suppress previews and `sensitivity: secret` for secret values.

Log value expressions use `field` for current action outputs, `ref` for scenario
scope, `object` and `list` for containers, and `decode`, `path`, and `through`
for field/ref selectors.

Checked log declaration example:

<!-- theater-doc: source id=reference-yaml-logs kind=yaml path=../../examples/reference/logs.yaml checks=validate,run -->
```yaml
id: reference-logs
scenarios:
  - id: inspect
    inputs:
      request_id:
        type: string
        required: true
    acts:
      - id: read
        action:
          use: action.generate
          with:
            outputs:
              kind: object
              object:
                status_code: 200
                correlation_id: req-123
        logs:
          - id: response
            value:
              object:
                status:
                  field: values
                  path: /status_code
                correlation_id:
                  field: values
                  path: /correlation_id
            capture: summary
            sensitivity: internal
          - id: audit
            message: response received
            fields:
              request_id:
                ref: request_id
              status:
                field: values
                path: /status_code
scenario_calls:
  - id: run
    scenario_id: inspect
    bindings:
      request_id: req-123
```

## Bindings

Bindings appear in `action.with`, `inventory.with`, `scenario_calls.bindings`,
and `assert.args`.

| Binding form | Purpose |
| --- | --- |
| Plain YAML scalar/list/object | Literal value |
| `kind: literal` | Literal object whose keys would otherwise look like a binding |
| `kind: ref` | Reference a value by name, with optional `decode`, `path`, and `through` |
| `kind: object` | Object whose nested values are bindings |
| `kind: list` | List whose items are bindings |
| `kind: string` | Ordered string composition through `parts` |
| `kind: generate` | Generator binding |

Do not wrap ordinary scalars, lists, or simple objects in `kind: literal`.
`theater validate` keeps the stage valid but reports a non-fatal hint for
redundant literal wrappers.

## Selectors And Expectations

Advanced checked example:

<!-- theater-doc: source id=reference-yaml-advanced kind=yaml path=../../examples/reference/advanced-expectations.yaml pair=reference-advanced checks=validate,run -->
```yaml
id: reference-advanced
scenarios:
  - id: inspect
    acts:
      - id: read
        action:
          use: action.generate
          with:
            outputs:
              kind: object
              object:
                users:
                  kind: list
                  list:
                    - kind: object
                      object:
                        id: user-123
                        email: demo@example.test
                code: A123
        expectations:
          - id: user
            subject:
              field: values
              path: /users
            assert:
              has_item:
                - subject:
                    path: /id
                  assert:
                    eq: user-123
          - id: code
            subject:
              field: values
              path: /code
            assert:
              matches: '^[A-Z][0-9]{3}$'
        exports:
          - as: code
            field: values
            path: /code
scenario_calls:
  - id: run
    scenario_id: inspect
```

Use [Selectors](../selectors.md) for `field`, `ref`, `decode`, `path`, and
`through` rules.

Use [Expectations](../expectations.md) for canonical matcher refs and YAML
matcher sugar.

YAML expectation subject defaults to current action output through `field`.
Property-targeted subjects use `from: property` with `ref`.

## Built-In Action Refs

| Ref | Purpose | Outputs |
| --- | --- | --- |
| `action.http` | HTTP request | `status_code`, `status`, `headers`, `body` |
| `action.command` | Direct process execution | `exit_code`, `stdout`, `stderr` |
| `action.generate` | Batch generated values | `values` |
| `action.state.read` | Read persistent record | `value`, `version` |
| `action.state.update` | Compare-and-set record update | `value`, `version` |
| `action.state.claim` | Claim a pool item | `item`, `claim` |
| `action.state.renew` | Renew a claim lease | `claim` |
| `action.state.release` | Release a claim | none |
| `action.state.consume` | Consume a claim | none |

Mutating state actions are invalid inside an `eventually` act.

## Built-In Inventory Refs

| Ref | Produces |
| --- | --- |
| `inventory.env` | string |
| `inventory.file` | bytes |
| `inventory.http.get` | bytes |
| `inventory.state.record` | opaque record handle |
| `inventory.state.pool` | opaque pool handle |

State inventory inputs `backend`, `record` or `pool`, and `min_guarantee` are
literal-only in v1.

## Built-In Decorators

| Ref | Input | Output |
| --- | --- | --- |
| `json.decode` | string or bytes | structured JSON value |
| `csv.decode` | string or bytes | list of row objects keyed by header |

Plugin-provided transforms use the same `decorators[].use` chain surface.
