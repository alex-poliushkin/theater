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

Selected library files can also carry slot-backed `http.auth` entries for the
scenarios they provide. When a library file is selected, every auth declaration
in that file must be slot-backed and non-colliding; only auth names referenced
by selected scenarios are copied into the assembled flow. Unselected libraries
do not contribute auth entries. Static bearer tokens, basic credentials, and
static API key values are rejected from selected library auth declarations; keep
those values in the runnable flow or external runtime configuration.

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
| `auth_bindings` | no | Scenario-start HTTP auth slot bindings |
| `acts` | no | Ordered act definitions |

`auth_bindings` initializes named HTTP auth slots before the first act in one
scenario execution. The key under `auth_bindings` must match a top-level
`http.auth` entry after repo-aware composition, and each `slots.<name>` binding
must target a slot declared by that auth entry, such as a bearer `token_slot`.
Slot values are stored as secret-sensitive auth state and are never report
outputs.

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

Scenario-level cleanup hooks are a [ratified future contract](../cleanup-hooks.md).
They are not available in the current YAML schema or runtime.

Scenario-level preflight guardrails are a
[ratified future contract](../preflight-guardrails.md). They are not available
in the current YAML schema or runtime.

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

Bindings appear in `properties.<name>.value`, `action.with`, `inventory.with`,
`scenario_calls.bindings`, and `assert.args`.

| Binding form | Purpose |
| --- | --- |
| Plain YAML scalar/list/object | Literal value |
| `kind: literal` | Literal object whose keys would otherwise look like a binding |
| `kind: ref` | Reference a value by name, with optional `decode`, `path`, and `through` |
| `kind: object` | Object whose nested values are bindings |
| `kind: list` | List whose items are bindings |
| `kind: string` | Ordered string composition through `parts` |
| `kind: generate` | Generator binding |
| `kind: coalesce` | First concrete value from ordered `candidates` |
| `kind: env` | Named host environment variable source |

Do not wrap ordinary scalars, lists, or simple objects in `kind: literal`.
`theater validate` keeps the stage valid but reports a non-fatal hint for
redundant literal wrappers.

`kind: coalesce` evaluates `candidates` left to right and skips only typed
missing values such as omitted optional scenario inputs or unset `kind: env`
sources. Empty string, zero, `false`, `null`, empty object, and empty list are
concrete values. Selector errors, generator errors, validation failures,
cancellation, and arbitrary runtime errors are not fallback signals.
When `coalesce` candidates have different sensitivity, Theater uses the most
conservative candidate visibility for the resolved value.

`kind: env` reads one named host environment variable. `name` is required and
must be a non-empty literal string. Validation does not read the environment.
At runtime an unset variable resolves to typed missing, so `coalesce` can select
a later candidate. A set-but-empty variable resolves to `""` and does not fall
back. Env values are treated as secret for diagnostics, debug snapshots, and
report previews because the variable name alone does not prove the value is
safe to show.

Checked coalesce example:

<!-- theater-doc: source id=reference-yaml-coalesce kind=yaml path=../../examples/reference/coalesce.yaml checks=validate,run -->
```yaml
id: reference-coalesce
scenarios:
  - id: greet
    inputs:
      nickname:
        type: string
    acts:
      - id: choose-name
        action:
          use: action.generate
          with:
            outputs:
              kind: object
              object:
                name:
                  kind: coalesce
                  candidates:
                    - kind: ref
                      ref:
                        name: nickname
                    - guest
        expectations:
          - id: uses-fallback
            subject:
              field: values
              path: /name
            assert:
              ref: expectation.equal
              args:
                expected: guest
scenario_calls:
  - id: run
    scenario_id: greet
```

## Properties

Act properties are resolved before the action and become available in the
current act scope. A property must define exactly one value source:

| Property field | Meaning |
| --- | --- |
| `value` | General binding value resolved before the action |
| `inventory` | Inventory capability call retained for resource-oriented inputs |
| `decorators` | Optional transforms applied after `value` or `inventory` resolves |

Use `value` for local runtime configuration, literals, refs, objects, lists,
strings, generators, and `coalesce`. Use `inventory` when a capability must
acquire a resource or external value through Theater's inventory boundary.
Decorators apply only to the selected property value.

Checked runtime configuration example:

<!-- theater-doc: source id=reference-yaml-runtime-config kind=yaml path=../../examples/reference/runtime-config.yaml pair=reference-runtime-config checks=validate,run unset-env=THEATER_DOC_REFERENCE_RUNTIME_CONFIG_EMAIL_UNSET unset-env-2=THEATER_DOC_REFERENCE_RUNTIME_CONFIG_GENERATED_EMAIL_UNSET -->
```yaml
id: reference-runtime-config
scenarios:
  - id: configure-runtime
    inputs:
      email:
        type: string
    acts:
      - id: configure
        properties:
          email_literal:
            value:
              kind: coalesce
              candidates:
                - kind: env
                  name: THEATER_DOC_REFERENCE_RUNTIME_CONFIG_EMAIL_UNSET
                - guest@example.test
          email_generated:
            value:
              kind: coalesce
              candidates:
                - kind: env
                  name: THEATER_DOC_REFERENCE_RUNTIME_CONFIG_GENERATED_EMAIL_UNSET
                - kind: generate
                  generator: email
                  domain: example.test
          email_input:
            value:
              kind: coalesce
              candidates:
                - kind: ref
                  ref:
                    name: email
                - kind: generate
                  generator: email
                  domain: example.test
        action:
          use: action.generate
          with:
            outputs:
              kind: object
              object:
                email_literal:
                  kind: ref
                  ref:
                    name: email_literal
                email_generated:
                  kind: ref
                  ref:
                    name: email_generated
                email_input:
                  kind: ref
                  ref:
                    name: email_input
        expectations:
          - id: literal-fallback
            subject:
              field: values
              path: /email_literal
            assert:
              ref: expectation.equal
              args:
                expected: guest@example.test
          - id: generated-fallback
            subject:
              field: values
              path: /email_generated
            assert:
              ref: expectation.matches
              args:
                pattern: ^[^@]+@example\.test$
          - id: input-fallback
            subject:
              field: values
              path: /email_input
            assert:
              ref: expectation.matches
              args:
                pattern: ^[^@]+@example\.test$
scenario_calls:
  - id: run
    scenario_id: configure-runtime
```

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
                items:
                  kind: list
                  list:
                    - kind: object
                      object:
                        id: item-123
                        kind: sample
                        status: ready
                        label: Primary
                    - kind: object
                      object:
                        id: item-456
                        kind: sample
                        status: blocked
                        label: Secondary
        expectations:
          - id: item-present
            subject:
              field: values
              path: /items
            assert:
              has_item:
                - subject:
                    path: /id
                  assert:
                    eq: item-123
                - subject:
                    path: /kind
                  assert:
                    eq: sample
        exports:
          - as: selected_status
            field: values
            path: /items
            through:
              - pick:
                  where:
                    - subject:
                        path: /id
                      assert:
                        ref: expectation.equal
                        args:
                          expected: item-123
                    - subject:
                        path: /kind
                      assert:
                        ref: expectation.equal
                        args:
                          expected: sample
              - path: /status
scenario_calls:
  - id: run
    scenario_id: inspect
```

Use [Selectors](../selectors.md) for `field`, `ref`, `decode`, `path`,
`through`, and selector transform rules.

Use [Expectations](../expectations.md) for canonical matcher refs and YAML
matcher sugar.

YAML expectation subject defaults to current action output through `field`.
Property-targeted subjects use `from: property` with `ref`.

## Exports

Act exports commit values to scenario scope only after the act passes.
`field` selects from the current action output map, while `ref` selects an
already-available value from the current act scope, including resolved act
properties and scenario inputs or previous exports. Both forms support
`decode`, `path`, and `through` selector steps.

<!-- theater-doc: source id=reference-yaml-act-ref-export kind=yaml path=../../examples/reference/act-ref-export.yaml checks=validate -->
```yaml
id: reference-act-ref-export
scenarios:
  - id: runtime
    acts:
      - id: load-runtime
        properties:
          runtime_path:
            inventory:
              use: inventory.env
              with:
                name: PATH
        action:
          use: action.generate
          with:
            outputs:
              loaded: true
        exports:
          - as: runtime_path
            ref:
              name: runtime_path
scenario_calls:
  - id: run
    scenario_id: runtime
```

Scenario-call exports commit values to stage scope after the called scenario
passes. They use `ref` to select a value from the completed scenario scope.

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

Plugin-provided transforms can decorate inventory properties through
`decorators[].use` and can run inside selector pipelines through
`through[].transform.use`.
