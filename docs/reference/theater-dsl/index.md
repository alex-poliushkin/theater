# Theater DSL Reference

Theater DSL is the compact `.thtr` authoring format for Theater stages. It
lowers to the same stage model that YAML uses; it is not a separate runtime.

Source of truth:

- [Theater DSL core syntax](core-syntax.md)
- [YAML reference](../yaml/index.md)
- `theater fmt`, `theater lower`, and `theater validate`

Equivalent YAML lookup:

- [YAML reference](../yaml/index.md)
- [YAML stage schema](../yaml/stage-schema.md)

## Checked Example

<!-- theater-doc: source id=reference-dsl-first-stage kind=thtr path=../../examples/first-stage/stage.thtr pair=reference-first-stage checks=fmt,lower,validate -->
```thtr
stage docs-first

scenario hello
  act say-hello
    do action.generate
      outputs:
        message: "hello"
    expect message: field(values) | path("/message") == "hello"

call run = hello()
```

## File And Loading Rules

| Contract | Current value |
| --- | --- |
| Extension | `.thtr` |
| Public name | Theater DSL (`.thtr`) |
| Runtime model | Lowers into `theater.StageSpec` |
| Canonical interchange form | YAML emitted by `theater lower` |
| Repo-aware flow path | `theater/flows/*.thtr` |
| Repo-aware library path | `theater/lib/**/*.thtr` |

Repo-aware library files contribute reusable scenarios. They must not declare
`scenario_calls`.

## Top-Level Order

The top-level order is part of the authoring contract:

| Position | Form | Required |
| --- | --- | --- |
| 1 | `stage <id>` | yes |
| 2 | `name "Display name"` | no |
| 3 | `http` block | no |
| 4 | `state` block | no |
| 5 | `scenario <id> ...` blocks | no |
| 6 | `call <id> = <scenario-id>(...)` blocks | no |

## Act Order

Scenario body order is fixed:

| Position | Form | Required |
| --- | --- | --- |
| 1 | `name "Display name"` | no |
| 2 | `bind auth <auth-id>` | no |
| 3 | `act <id>` | yes |

Act body order is fixed so the file reads in execution order:

| Position | Form | Required |
| --- | --- | --- |
| 1 | `name "Display name"` | no |
| 2 | `eventually <timeout> every <interval>` | no |
| 3 | `prop <name> = ...` | no |
| 4 | `do [repeatable] <action-call>` | yes |
| 5 | `capture_auth <auth-id>` | no |
| 6 | `log <id> = <log-value>` | no |
| 7 | `expect <id>: ...` | no |
| 8 | `export <name> = ...` | no |
| 9 | `on <event> -> <act-id>` | no |

`eventually` can retry the whole act only when the action call is marked
`repeatable`.

## Identifiers

Core identifiers start with an ASCII letter and then may contain ASCII letters,
digits, `_`, and `-`.

Additional forms:

| Surface | Example |
| --- | --- |
| Capability ref | `action.http` |
| Generator call introducer | `generate.uuid()` |
| Scenario namespace | `identity/login` |
| Data object key | `object { "Content-Type": "application/json" }` |

Quoted keys are valid only inside data containers such as `object {}`. Core
Theater ids are not quoted.

## Scalars

| Form | Example | Notes |
| --- | --- | --- |
| bool | `true` | also `false` |
| null | `null` | literal null value |
| integer | `42` | signed integer accepted |
| decimal | `3.14` | signed decimal accepted |
| string | `"hello"` | supports escapes and interpolation |
| raw string | `r"^[0-9]+$"` | no escape processing |
| multiline string | `"""..."""` | trims delimiter-only blank edge lines, then parser-owned dedent |
| duration | `500ms`, `3s`, `5m` | only where duration text is expected |

Duration syntax follows Go `time.ParseDuration`. A bare number is not a duration
in `eventually`.

## Calls And Data

| Purpose | Theater DSL form | YAML equivalent |
| --- | --- | --- |
| Property value | `prop email = coalesce(env("EMAIL"), "guest@example.test")` | `properties.<name>.value` |
| Inventory property | `prop token = inventory.env(name: "TOKEN")` | `properties.<name>.inventory` |
| Action | `do action.http(method: "GET", url: $url)` | `action.use` plus `action.with` |
| Repeatable action | `do repeatable action.http(...)` | `action.repeatable: true` |
| Scenario-authored log | `log response = object { status: field(status_code) }` | `logs[]` with `capture: summary` |
| Act export | `export token = $issued_token` | `acts[].exports[]` |
| Scenario call | `call run = hello()` | `scenario_calls[]` |
| Scenario call dependency | `dependency setup when success` | `dependencies[].when: success` |
| Scenario call export | `export token = $session_token` | `scenario_calls[].exports[]` |

Omitting `when` on a dependency lowers to `success`.

## Selectors And Expectations

Selectors start from action output with `field(...)`, from a ref with `$name`, or
from an explicit property target in YAML. Common Theater DSL pipeline steps are
`decode(json)`, `path("/...")`, `pick where ...`, `regexp(...)`, and plugin
transform calls such as `transform.jwt.claims()`.

Act exports can start from `field(...)` or from an available `$name` in the
current act scope. Scenario-call exports stay narrower: they must be direct
`$name` exports from the completed scenario scope.

For exact selector rules, use [Selectors](../selectors.md).
For matcher names and expectation forms, use [Expectations](../expectations.md).

Common expectation sugar:

| Theater DSL form | Canonical matcher |
| --- | --- |
| `S == V` | `expectation.equal` |
| `S != V` | `expectation.not` wrapping `expectation.equal` |
| `S matches P` | `expectation.matches` |
| `S contains V` | `expectation.contains` |
| `S is present` | `expectation.present` |
| `S is null` | `expectation.null` |
| `S is not null` | `expectation.not_null` |
| `S has key(K)` | `expectation.has_key` |
| `S has no key(K)` | `expectation.lacks_key` |
| `S lacks key(K)` | `expectation.lacks_key` |

Plugin-provided matchers use the canonical escape hatch:
`assert matcher.vendor.capability(...)`.

## State Sugar

Theater DSL state aliases are authoring sugar over the same persistent-state
contract as YAML.

| Theater DSL form | Canonical YAML |
| --- | --- |
| `record profile_cache = state.record ...` | `inventory.state.record` hidden handle |
| `pool otp_pool = state.pool ...` | `inventory.state.pool` hidden handle |
| `state.read(record: profile_cache)` | `action.state.read` |
| `state.update(... if_version: ...)` | `action.state.update` with `expected_version` |
| `state.claim(pool: otp_pool, fields: ...)` | `action.state.claim` |
| `state.renew(claim: ..., ttl: ...)` | `action.state.renew` |
| `state.release(claim: ...)` | `action.state.release` |
| `state.consume(claim: ..., tombstone: ...)` | `action.state.consume` |

`state.cas` and `state.claim where:` are not accepted spellings.

## Commands

| Task | Command |
| --- | --- |
| Format check | `theater fmt --check <file.thtr>` |
| Print formatted source | `theater fmt <file.thtr>` |
| Lower to YAML | `theater lower <file.thtr>` |
| Validate | `theater validate <file.thtr>` |
| Run | `theater run <file.thtr> --live off` |

For task recipes, use [Format Theater DSL](../../how-to/format-theater-dsl.md),
[Inspect YAML From Theater DSL](../../how-to/inspect-yaml-from-theater-dsl.md),
and [Migrate YAML To Theater DSL](../../how-to/migrate-yaml-to-theater-dsl.md).
