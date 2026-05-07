# Theater DSL Core Syntax

This page is a compact syntax lookup for the core `.thtr` forms. It uses the
same source-of-truth contracts as the main
[Theater DSL reference](index.md).

Equivalent YAML lookup:

- [YAML reference](../yaml/index.md)
- [YAML stage schema](../yaml/stage-schema.md)

Checked Theater DSL example:

- [Theater DSL reference checked example](index.md#checked-example)

## Stage

| Form | Meaning |
| --- | --- |
| `stage <id>` | Required stage id |
| `name "..."` | Optional display name; single-line string only |

## Scenario

| Form | Meaning |
| --- | --- |
| `scenario <id>` | Reusable scenario without inputs |
| `scenario <id>(name: string!)` | Reusable scenario with required input |
| `scenario <id>(name: string)` | Reusable scenario with optional input |

Input type shorthand supports the existing value-kind surface: `string`,
`number`, `bool`, `object`, `list`, `bytes`, `null`, and `any`.

## Act

| Form | Meaning |
| --- | --- |
| `act <id>` | Executable step inside a scenario |
| `eventually 3s every 100ms` | Whole-act polling window |
| `prop <name> = inventory.env(...)` | Current-act property |
| `do action.command(...)` | Required action call |
| `do repeatable action.http(...)` | Action call that may be retried by `eventually` |
| `log response = object { status: field(status_code) }` | Scenario-authored report log |
| `on pass -> next` | Transition to another act |

Supported transition events are `pass`, `fail`, `timeout`, and `cancel`.

## Values

| Form | Meaning |
| --- | --- |
| `$name` | Reference a value in scope |
| `object { id: $id }` | Dynamic object binding |
| `list [ "a", $b ]` | Dynamic list binding |
| `"prefix-${id}"` | String interpolation |
| `generate.uuid()` | Generator binding |

`generate.<name>(...)` lowers to canonical YAML `kind: generate` with the
`generate.` prefix removed from the generator ref.

## Logs

Scenario-authored logs are act-local observations. The right-hand side is a log
value, not the full data-expression surface: supported roots are `field(...)`,
`$ref`, `object { ... }`, and `list [ ... ]`. Theater DSL log syntax lowers to
YAML `acts[].logs` with `capture: summary` and `sensitivity: internal`. Use
YAML `capture` and `sensitivity` fields when a log may contain sensitive values.
Use [Scenario Logs](../logs.md) for the full field table, report behavior, and
checked examples:

<!-- theater-doc: source id=reference-dsl-logs kind=thtr path=../../examples/reference/logs.thtr checks=fmt,lower,validate,run -->
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

Logs evaluate after the action succeeds and before expectations, exports, and
transitions. They do not write to stdout and cannot be used as dataflow or
control-flow inputs.

## Selectors

| Form | Meaning |
| --- | --- |
| `field(body)` | Current action output field |
| `field(body) | decode(json)` | Decode selected value as JSON |
| `path("/data/id")` | RFC 6901 JSON Pointer traversal |
| `regexp(pattern: r"...", group: 1)` | Regex extraction |

Selectors are exact. Missing roots, missing paths, decode failures, and
wrong-type traversals are selector failures rather than matcher mismatches.

## Expectations

Advanced checked example:

<!-- theater-doc: source id=reference-dsl-advanced kind=thtr path=../../examples/reference/advanced-expectations.thtr pair=reference-advanced checks=fmt,lower,validate,run -->
```thtr
stage reference-advanced

scenario inspect
  act read
    do action.generate
      outputs:
        users: list [ object { id: "user-123", email: "demo@example.test" } ]
        code: "A123"
    expect user: field(values) | path("/users") has item where path("/id") == "user-123"
    export code = field(values) | path("/code") matches r"^[A-Z][0-9]{3}$"

call run = inspect()
```

| Form | Meaning |
| --- | --- |
| `expect ok: field(status_code) == 200` | Equality matcher |
| `expect id: field(body) | decode(json) | path("/id") matches r"^[0-9]+$"` | Regex matcher |
| `expect data: field(body) | decode(json) has key("data")` | Object key matcher |
| `expect item: field(body) | decode(json) has item where path("/id") == $id` | Collection matcher |
| `expect custom: field(body) assert matcher.vendor.capability(expected: "ok")` | Plugin matcher escape hatch |

Use [Expectations](../expectations.md) for the matcher table.

## Exports

| Form | Meaning |
| --- | --- |
| `export id = field(body) | decode(json) | path("/id")` | Export selected action output |
| `export id = field(body) | decode(json) | path("/id") matches r"^[0-9]+$"` | Add a same-id expectation before export |
| `call run = scenario() ... export shared = $id` | Export a scenario result to stage scope |

An assertion-backed act export creates a canonical expectation with the export
name as its id. Do not reuse that id for another expectation in the same act.
