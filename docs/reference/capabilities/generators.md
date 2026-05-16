# Generators

Generators produce deterministic values during one stage run. They are used in
Theater DSL with `generate.<name>(...)` and in YAML with `kind: generate`.

Source of truth:

- `go run ./cmd/theater explain generator`
- `go run ./cmd/theater explain generator <generator-name>`
- [YAML reference](../yaml/index.md)

## Checked Generator Catalog

<!-- theater-doc: command id=reference-generator-family cwd=../../.. expect-stdout="Capabilities (9):" expect-stdout-2="uuid" expect-stdout-3="timestamp" expect-stdout-4="date" -->
```sh
go run ./cmd/theater explain generator
```

<!-- theater-doc: command id=reference-generator-date cwd=../../.. expect-stdout="Capability: date" expect-stdout-2="UTC date string" expect-stdout-3="format" expect-stdout-4="Produces:" -->
```sh
go run ./cmd/theater explain generator date
```

<!-- theater-doc: command id=reference-generator-uuid cwd=../../.. expect-stdout="Capability: uuid" expect-stdout-2="version  string" expect-stdout-3="Produces:" -->
```sh
go run ./cmd/theater explain generator uuid
```

## Built-In Generators

| Ref | Produces |
| --- | --- |
| `date` | UTC date string with `iso` or `basic` format and optional offset |
| `digits` | deterministic pseudo-random digit string |
| `email` | unique-looking email per binding and scenario invocation |
| `phone` | deterministic phone-like string with finite suffix space and optional shuffled suffix order |
| `sequence` | deterministic per-binding stage-run sequence number |
| `slug` | slug with deterministic run token and sequence suffix |
| `string` | deterministic pseudo-random string |
| `timestamp` | run-base timestamp with optional offset |
| `uuid` | deterministic UUID string |

## Usage Examples

Use generators as value bindings. In Theater DSL, generator bindings use
`generate.<name>(...)`. In YAML, use `kind: generate` with the canonical
generator ref in `generator`; every other key is a generator argument.

`action.generate` publishes one action output field named `values`. Every
generated entry is nested under that object, so select generated data with
`field(values) | path("/<entry-name>")`.

Checked Theater DSL example using every built-in generator:

<!-- theater-doc: source id=reference-generators-thtr kind=thtr path=../../examples/reference/generators.thtr checks=fmt,lower,validate,run -->
```thtr
stage reference-generators

scenario inspect
  act generate-values
    do action.generate
      outputs:
        sequence_value: generate.sequence(start: 1000, step: 1)
        uuid_value: generate.uuid(version: "v7")
        uuid_default: generate.uuid()
        timestamp_value: generate.timestamp(format: "rfc3339", offset: "5m")
        timestamp_default: generate.timestamp()
        date_value: generate.date(format: "basic", offset: "15h")
        date_default: generate.date()
        string_value: generate.string(length: 12, alphabet: "abcdef0123456789")
        digits_value: generate.digits(length: 6)
        email_value: generate.email(prefix: "demo", domain: "example.test")
        email_stem: generate.email(stem: "customer", domain: "example.test")
        phone_value: generate.phone(prefix: "+1555", digits: 4, random: true)
        slug_value: generate.slug(prefix: "profile", max_length: 24)
    expect generated: field(values) has key("email_value")

call run = inspect()
```

Checked YAML example using the same generator refs:

<!-- theater-doc: source id=reference-generators-yaml kind=yaml path=../../examples/reference/generators.yaml checks=validate,run -->
```yaml
id: reference-generators
scenarios:
  - id: inspect
    acts:
      - id: generate-values
        action:
          use: action.generate
          with:
            outputs:
              kind: object
              object:
                sequence_value:
                  kind: generate
                  generator: sequence
                  start: 1000
                  step: 1
                uuid_value:
                  kind: generate
                  generator: uuid
                  version: v7
                uuid_default:
                  kind: generate
                  generator: uuid
                timestamp_value:
                  kind: generate
                  generator: timestamp
                  format: rfc3339
                  offset: 5m
                timestamp_default:
                  kind: generate
                  generator: timestamp
                date_value:
                  kind: generate
                  generator: date
                  format: basic
                  offset: 15h
                date_default:
                  kind: generate
                  generator: date
                string_value:
                  kind: generate
                  generator: string
                  length: 12
                  alphabet: abcdef0123456789
                digits_value:
                  kind: generate
                  generator: digits
                  length: 6
                email_value:
                  kind: generate
                  generator: email
                  prefix: demo
                  domain: example.test
                email_stem:
                  kind: generate
                  generator: email
                  stem: customer
                  domain: example.test
                phone_value:
                  kind: generate
                  generator: phone
                  prefix: "+1555"
                  digits: 4
                  random: true
                slug_value:
                  kind: generate
                  generator: slug
                  prefix: profile
                  max_length: 24
        expectations:
          - id: generated
            subject:
              field: values
            assert:
              ref: expectation.has_key
              args:
                key: email_value
scenario_calls:
  - id: run
    scenario_id: inspect
```

### `date`

Theater DSL binding: `start_date: generate.date(format: "basic", offset: "15h")`

Use `start_date: generate.date()` for the run base date in the default `iso`
format. The date is calculated from the stage run base time plus the optional
duration offset, converted to UTC, and rendered as a string.

Supported formats:

| Format | Output shape |
| --- | --- |
| `iso` | `YYYY-MM-DD` |
| `basic` | `YYYYMMDD` |

The `format` argument is a closed enum. It does not accept Go layouts, PHP date
tokens, Java/ICU patterns, or arbitrary pattern strings. Use `timestamp` when a
full RFC3339 instant is required.

YAML binding:

    start_date:
      kind: generate
      generator: date
      format: basic
      offset: 15h

### `digits`

Theater DSL binding: `otp: generate.digits(length: 6)`

YAML binding:

    otp:
      kind: generate
      generator: digits
      length: 6

### `email`

Theater DSL binding: `email: generate.email(prefix: "demo", domain: "example.test")`

Use `stem` instead of `prefix` when the local-part base should be named
directly: `email: generate.email(stem: "customer", domain: "example.test")`.

YAML binding:

    email:
      kind: generate
      generator: email
      prefix: demo
      domain: example.test

### `phone`

Theater DSL binding: `phone: generate.phone(prefix: "+1555", digits: 4, random: true)`

YAML binding:

    phone:
      kind: generate
      generator: phone
      prefix: "+1555"
      digits: 4
      random: true

### `sequence`

Theater DSL binding: `order_number: generate.sequence(start: 1000, step: 1)`

YAML binding:

    order_number:
      kind: generate
      generator: sequence
      start: 1000
      step: 1

### `slug`

Theater DSL binding: `slug: generate.slug(prefix: "profile", max_length: 24)`

YAML binding:

    slug:
      kind: generate
      generator: slug
      prefix: profile
      max_length: 24

### `string`

Theater DSL binding: `token: generate.string(length: 12, alphabet: "abcdef0123456789")`

YAML binding:

    token:
      kind: generate
      generator: string
      length: 12
      alphabet: abcdef0123456789

### `timestamp`

Theater DSL binding: `created_at: generate.timestamp(format: "rfc3339", offset: "5m")`

Use `created_at: generate.timestamp()` for the run base time in the default
`rfc3339` format.

YAML binding:

    created_at:
      kind: generate
      generator: timestamp
      format: rfc3339
      offset: 5m

### `uuid`

Theater DSL binding: `request_id: generate.uuid(version: "v7")`

Use `request_id: generate.uuid()` for the default UUID version.

YAML binding:

    request_id:
      kind: generate
      generator: uuid
      version: v7

The final run report records generation metadata under
`report.generation.seed` and `report.generation.base_time`.

For a guided first use of generated output, start with
[First Stage](../../tutorial/02-first-stage.md).
