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
| `bind auth <auth-id>` | Initialize a named HTTP auth slot bundle before the first act |

Input type shorthand supports the existing value-kind surface: `string`,
`number`, `bool`, `object`, `list`, `bytes`, `null`, and `any`.

`bind auth` entries appear after an optional scenario `name` and before `act`
entries. Each indented slot line is a binding expression:

    scenario service/sample-ready(session_token: string!)
      bind auth service_api
        session_token: $session_token
      act get-sample-resource
        do action.http(url: "https://api.example.test/sample-resource", auth: "service_api")

The target auth id must be declared in the assembled stage `http` block. In a
standalone file that means the same stage. In a repo-aware flow, a selected
library file may contribute a slot-backed auth declaration. A slot such as
`session_token` must be declared by that auth entry, for example with bearer
`token_slot`.

Scenario-level cleanup hooks are a [ratified future contract](../cleanup-hooks.md).
They are not available in the current `.thtr` parser, formatter, lowerer, or
runtime.

## Act

| Form | Meaning |
| --- | --- |
| `act <id>` | Executable step inside a scenario |
| `eventually 3s every 100ms` | Whole-act polling window |
| `prop <name> = coalesce(env("NAME"), "fallback")` | Current-act property value |
| `prop <name> = inventory.env(...)` | Current-act property from inventory |
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
| `env("NAME")` | Named host environment variable source |
| `coalesce($name, "fallback")` | First concrete value from ordered candidates |

`generate.<name>(...)` lowers to canonical YAML `kind: generate` with the
`generate.` prefix removed from the generator ref.

`env("NAME")` is explicit and environment-agnostic at validation time. At
runtime an unset variable is typed missing, so `coalesce` may select a later
candidate. A set-but-empty variable is the concrete empty string. `coalesce`
does not skip `""`, `0`, `false`, `null`, empty objects, empty lists, selector
errors, generator errors, validation failures, cancellation, or arbitrary
runtime errors. When candidates have different sensitivity, Theater uses the
most conservative candidate visibility for the resolved value. Env values are
treated as secret for diagnostics, debug snapshots, and report previews
because the variable name alone does not prove the value is safe to show.

Checked runtime configuration example:

<!-- theater-doc: source id=reference-dsl-runtime-config kind=thtr path=../../examples/reference/runtime-config.thtr pair=reference-runtime-config checks=fmt,lower,validate,run unset-env=THEATER_DOC_REFERENCE_RUNTIME_CONFIG_EMAIL_UNSET unset-env-2=THEATER_DOC_REFERENCE_RUNTIME_CONFIG_GENERATED_EMAIL_UNSET -->
```thtr
stage reference-runtime-config

scenario configure-runtime(email: string)
  act configure
    prop email_literal = coalesce(env("THEATER_DOC_REFERENCE_RUNTIME_CONFIG_EMAIL_UNSET"), "guest@example.test")
    prop email_generated = coalesce(env("THEATER_DOC_REFERENCE_RUNTIME_CONFIG_GENERATED_EMAIL_UNSET"), generate.email(domain: "example.test"))
    prop email_input = coalesce($email, generate.email(domain: "example.test"))
    do action.generate
      outputs:
        email_literal: $email_literal
        email_generated: $email_generated
        email_input: $email_input
    expect literal-fallback: field(values) | path("/email_literal") == "guest@example.test"
    expect generated-fallback: field(values) | path("/email_generated") matches r"^[^@]+@example\.test$"
    expect input-fallback: field(values) | path("/email_input") matches r"^[^@]+@example\.test$"

call run = configure-runtime()
```

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
| `transform.jwt.claims() | path("/uid")` | Apply a plugin transform in a selector |

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
        items: list [
          object { id: "item-123", kind: "sample", status: "ready", label: "Primary" },
          object { id: "item-456", kind: "sample", status: "blocked", label: "Secondary" }
        ]
    expect item-present: field(values) | path("/items") has item where (
      path("/id") == "item-123",
      path("/kind") == "sample"
    )
    export selected_status = (
      field(values)
      | path("/items")
      | pick where (
        path("/id") == "item-123",
        path("/kind") == "sample"
      )
      | path("/status")
    )

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
| `export id = $profile | path("/id")` | Export selected value from current act scope |
| `export id = field(body) | decode(json) | path("/id") matches r"^[0-9]+$"` | Add a same-id expectation before export |
| `call run = scenario() ... export shared = $id` | Export a scenario result to stage scope |

An assertion-backed act export creates a canonical expectation with the export
name as its id and must start from `field(...)`. Do not reuse that id for
another expectation in the same act. Scenario-call exports must be direct
`$ref` exports without selector steps or assertion tails.
