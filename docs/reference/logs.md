# Scenario Logs

Scenario-authored logs are act-local observations declared by the scenario
author. They are useful for recording selected runtime values in live output and
in the durable run report without turning those values into exports,
expectations, or graph transitions.

Source of truth:

- [YAML reference](yaml/index.md#scenarios-and-acts)
- [Theater DSL reference](theater-dsl/index.md#act-order)
- [Report reference](reports.md#log-fields)
- [Output format reference](outputs/index.md)
- `spec.LogSpec` and `spec.LogValueSpec`
- `theater validate`

## Checked Commands

<!-- theater-doc: command id=reference-logs-json-thtr cwd=../.. expect-stdout="\"logs\":" expect-stdout-2="\"id\": \"response\"" expect-stdout-3="\"id\": \"audit\"" expect-stdout-4="\"log_summary\"" expect-stdout-5="\"records\": 2" -->
```sh
go run ./cmd/theater run docs/examples/reference/logs.thtr --live off --format json
```

<!-- theater-doc: command id=reference-logs-json-yaml cwd=../.. expect-stdout="\"logs\":" expect-stdout-2="\"capture\": \"summary\"" expect-stdout-3="\"sensitivity\": \"internal\"" expect-stdout-4="\"id\": \"audit\"" -->
```sh
go run ./cmd/theater run docs/examples/reference/logs.yaml --live off --format json
```

<!-- theater-doc: command id=reference-logs-live-stderr cwd=../.. expect-stdout=passed expect-stderr="log response" expect-stderr-2="log audit" reject-stdout="log response" reject-stdout-2="log audit" -->
```sh
go run ./cmd/theater run docs/examples/reference/logs.thtr --live auto
```

## Theater DSL Form

The Theater DSL log form is compact sugar over canonical YAML `LogSpec`:

| Form | Meaning |
| --- | --- |
| `log response = field(values)` | Record one selected action output |
| `log response = field(body) \| decode(json) \| path("/data")` | Record a selected decoded value |
| `log response = object { status: field(values) \| path("/status_code") }` | Record a structured object |
| `log items = list [ field(values) \| path("/first"), $second ]` | Record a structured list |

Allowed log value roots are:

| Root | Meaning |
| --- | --- |
| `field(<action-output>)` | Select a current action output field |
| `$<ref>` | Select a scenario-scope value |
| `object { ... }` | Build a dynamic object from nested log values |
| `list [ ... ]` | Build a dynamic list from nested log values |

Theater DSL logs lower with `capture: summary` and
`sensitivity: internal`. The DSL form does not expose `message`, `fields`,
`format`, `capture`, `sensitivity`, or `required`; use YAML when those knobs are
needed. Scalar-only logs such as `log note = "text"` are not part of the DSL
surface.

Checked Theater DSL example:

<!-- theater-doc: source id=reference-logs-dsl-source kind=thtr path=../examples/reference/logs.thtr checks=fmt,lower,validate,run -->
```thtr
stage reference-logs

scenario inspect(request_id: string!)
  act read
    do action.generate
      outputs:
        status_code: 200
        correlation_id: "req-123"
    log response = object {
      status: field(values) | path("/status_code"),
      correlation_id: field(values) | path("/correlation_id")
    }
    log audit = object {
      request_id: $request_id,
      status: field(values) | path("/status_code")
    }

call run = inspect(request_id: "req-123")
```

## YAML Form

YAML exposes the full `acts[].logs[]` contract.

| Field | Required | Values | Meaning |
| --- | --- | --- | --- |
| `id` | yes | string | Act-local log id |
| `value` | conditional | `LogValueSpec` | Dynamic value to record |
| `message` | conditional | string | Static message to record |
| `fields` | no | map of `LogValueSpec` | Dynamic fields paired with `message` |
| `format` | no | `text`, `json` | Requested author-facing format |
| `capture` | no | `omit`, `summary` | Whether retained reports may include a preview |
| `sensitivity` | no | `internal`, `personal`, `secret` | Sensitivity metadata for projection and redaction |
| `required` | no | boolean | Treat log evaluation failure as an act failure |

A log must provide either `value` or `message`. `fields` requires `message`.
`value` cannot be combined with `message` or `fields`.

`LogValueSpec` supports these selectors and containers:

| Field | Meaning |
| --- | --- |
| `field` | Select a current action output field |
| `ref` | Select a scenario-scope value |
| `object` | Build an object from nested log values |
| `list` | Build a list from nested log values |
| `decode` | Decode a selected `field` or `ref` before traversal |
| `path` | Traverse a selected value with an RFC 6901 JSON Pointer |
| `through` | Apply selector steps such as regexp extraction or pure transforms |

Checked YAML example:

<!-- theater-doc: source id=reference-logs-yaml-source kind=yaml path=../examples/reference/logs.yaml checks=validate,run -->
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

## Runtime And Report Contract

| Contract | Behavior |
| --- | --- |
| Evaluation order | Logs evaluate after successful action execution and before expectations, exports, and transitions |
| Dataflow | Logs do not write to scope and cannot be used by expectations, exports, or transitions |
| JSON report | Retained records appear under `result.report.logs` in CLI JSON output; inside the nested report object the field is `logs` |
| Log summary | Counts and limits appear under `result.report.log_summary` in CLI JSON output; inside the nested report object the field is `log_summary` |
| Text output | Passing text summaries do not dump all logs by default |
| Live output | Live log preview lines go to stderr when live output is enabled |
| Default failure behavior | Ordinary log evaluation errors produce error log records and do not fail the act |
| `required: true` | Runtime log evaluation errors fail the act before expectations run |
| Cancellation and deadline errors | Run cancellation or deadline expiry still terminates the act independently of `required` |
| Per-log preview limit | `4096` bytes |
| Per-act retained limit | `32` log records |
| Per-run retained limit | `1024` log records |

`capture` and `sensitivity` control report-safe projection. `capture: summary`
may retain selected plaintext preview data. `capture: omit` suppresses previews.
`sensitivity: secret` redacts secret values even when summary capture is
requested.

For a task-oriented walkthrough, use
[Log Runtime Values](../how-to/log-runtime-values.md). For the JSON record
shape, use [Reports](reports.md#log-fields). For stdout and stderr behavior,
use [Output Formats](outputs/index.md).
