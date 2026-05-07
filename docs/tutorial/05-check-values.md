# Check Values

You have already seen a passing flow and a reusable scenario. This page focuses
on one smaller skill: choosing a value from an action result and checking it.

The example prints one JSON profile with `action.command`. That keeps the run
deterministic while showing the same selector shape used for JSON response
bodies.

Read the file in two passes. First look at `read-profile`: it checks fields from
one JSON value. Then look at `reuse-profile-id`: it uses the exported value in
the next act.

## Theater DSL

Theater DSL keeps the value path close to the check:

<!-- theater-doc: source id=check-values-thtr kind=thtr path=../examples/check-values/profile.thtr pair=check-values checks=fmt,lower,validate,run -->
```thtr
stage check-values

scenario check-profile
  act read-profile
    do action.command
      executable: "printf"
      args: list [
        "%s",
        """
        {"data":{"id":"user-123","status":"active","email":"demo@example.test"}}
        """
      ]
      timeout: "5s"
    log profile = object {
      id: field(stdout) | decode(json) | path("/data/id"),
      status: field(stdout) | decode(json) | path("/data/status"),
      email: field(stdout) | decode(json) | path("/data/email")
    }
    expect command-ok: field(exit_code) == 0
    expect profile-id: field(stdout) | decode(json) | path("/data/id") == "user-123"
    expect profile-status: field(stdout) | decode(json) | path("/data/status") == "active"
    export profile_id = field(stdout) | decode(json) | path("/data/id")
    on pass -> reuse-profile-id

  act reuse-profile-id
    do action.generate
      outputs:
        seen_id: $profile_id
    expect same-id: field(values) | path("/seen_id") == "user-123"

call run = check-profile()
```

Read one selector from left to right:

- `field(stdout)` chooses the action output field.
- `decode(json)` parses that output as JSON.
- `path("/data/id")` selects one JSON Pointer path.
- `== "user-123"` is compact Theater DSL sugar for an equality assertion.

The `log profile` block records selected fields for report inspection. It does
not assert anything and does not create a value that later acts can read.

The export line uses the same selector. The next act reads `$profile_id`, so the
page also shows dataflow without adding another service or setup step.

Validate the Theater DSL file:

<!-- theater-doc: command id=check-values-validate-thtr cwd=../.. expect-stdout=valid reject-stdout=hint -->
```sh
go run ./cmd/theater validate docs/examples/check-values/profile.thtr
```

Run it:

<!-- theater-doc: command id=check-values-run-thtr cwd=../.. expect-stdout=passed -->
```sh
go run ./cmd/theater run docs/examples/check-values/profile.thtr --live off
```

A passing result confirms three things: the JSON text was decoded, `/data/id`
and `/data/status` matched the expected values, and the exported `profile_id`
was reused by the next act.

Print the JSON report when you want to inspect retained log records:

<!-- theater-doc: command id=check-values-run-thtr-json-logs cwd=../.. expect-stdout="\"logs\":" expect-stdout-2="\"id\": \"profile\"" expect-stdout-3="\"log_summary\"" -->
```sh
go run ./cmd/theater run docs/examples/check-values/profile.thtr --format json --live off
```

The log appears under `result.report.logs`. It gives you a durable debug record
without changing expectations, exports, transitions, or the normal text-mode
run summary.

## YAML

YAML uses the same model with explicit `subject` and `assert` fields.

<!-- theater-doc: source id=check-values-yaml kind=yaml path=../examples/check-values/profile.yaml pair=check-values checks=validate,run -->
```yaml
id: check-values
scenarios:
  - id: check-profile
    acts:
      - id: read-profile
        action:
          use: action.command
          with:
            executable: printf
            args:
              kind: list
              list:
                - '%s'
                - '{"data":{"id":"user-123","status":"active","email":"demo@example.test"}}'
            timeout: 5s
        logs:
          - id: profile
            value:
              object:
                id:
                  field: stdout
                  decode: json
                  path: /data/id
                status:
                  field: stdout
                  decode: json
                  path: /data/status
                email:
                  field: stdout
                  decode: json
                  path: /data/email
            capture: summary
            sensitivity: internal
        expectations:
          - id: command-ok
            subject:
              field: exit_code
            assert:
              ref: expectation.equal
              args:
                expected: 0
          - id: profile-id
            subject:
              field: stdout
              decode: json
              path: /data/id
            assert:
              ref: expectation.equal
              args:
                expected: user-123
          - id: profile-status
            subject:
              field: stdout
              decode: json
              path: /data/status
            assert:
              ref: expectation.equal
              args:
                expected: active
        exports:
          - as: profile_id
            field: stdout
            decode: json
            path: /data/id
        transitions:
          - on: on_pass
            to: reuse-profile-id
      - id: reuse-profile-id
        action:
          use: action.generate
          with:
            outputs:
              kind: object
              object:
                seen_id:
                  kind: ref
                  ref:
                    name: profile_id
        expectations:
          - id: same-id
            subject:
              field: values
              path: /seen_id
            assert:
              ref: expectation.equal
              args:
                expected: user-123
scenario_calls:
  - id: run
    scenario_id: check-profile
```

In YAML, the Theater DSL line
`field(stdout) | decode(json) | path("/data/id") == "user-123"` becomes:

- `subject.field: stdout`
- `subject.decode: json`
- `subject.path: /data/id`
- `assert.ref: expectation.equal`
- `assert.args.expected: user-123`

The YAML example declares the same report log under `logs`. For the full logging
schema and output behavior, use
[Log Runtime Values](../how-to/log-runtime-values.md).

Validate the YAML file:

<!-- theater-doc: command id=check-values-validate-yaml cwd=../.. expect-stdout=valid reject-stdout=hint -->
```sh
go run ./cmd/theater validate docs/examples/check-values/profile.yaml
```

Run it:

<!-- theater-doc: command id=check-values-run-yaml cwd=../.. expect-stdout=passed -->
```sh
go run ./cmd/theater run docs/examples/check-values/profile.yaml --live off
```

You now have the core checking pattern: choose a field, optionally decode it,
select a path, and assert the expected value. For task form, open
[Check JSON Response Fields](../how-to/check-json-response-fields.md). For exact
lookup, open [Selectors](../reference/selectors.md) and
[Expectations](../reference/expectations.md).
