# Preflight Guardrails

Preflight guardrails are scenario-level checks over resolved scenario inputs.
They run before any act can start so a reusable scenario can reject unsafe
runtime values before live side effects.

Preflight is not an action-output assertion, adapter-local validator, inventory
probe, or general policy engine. The first runtime slice checks scenario inputs
and static matcher arguments only.

## Shape

YAML uses `scenarios[].preflight`:

```
scenarios:
  - id: send-sample-message
    inputs:
      recipient_email:
        type: string
        required: true
      allow_non_test_recipient:
        type: bool
    preflight:
      - id: recipient-test-domain
        input:
          ref: recipient_email
        assert:
          ref: expectation.matches
          args:
            pattern: '^[^@]+@example\.test$'
        override:
          ref: allow_non_test_recipient
    acts:
      - id: submit-message
        action:
          use: action.http
          with: {}
```

The YAML `input` and `override` objects use the same ref shape as other
references, but preflight accepts only the named `ref`. `decode`, `path`, and
`through` are reserved for future use and rejected by validation in this
contract.

Theater DSL lowers to the same semantic model:

```
scenario send-sample-message(recipient_email: string!, allow_non_test_recipient: bool)
  preflight recipient-test-domain: $recipient_email matches r"^[^@]+@example\.test$" override $allow_non_test_recipient
  act submit-message
    do action.http
      method: "POST"
      url: "https://api.example.test/messages"
```

## Semantics

Preflight runs after scenario-call bindings resolve and before act properties,
inventories, actions, logs, expectations, exports, transitions, cleanup,
scenario auth slot initialization, or auth capture can run.

A rejected preflight prevents later scenario action side effects. Value-source
side effects that happen before scenario input binding resolution are outside
the preflight boundary.

Stage preparation can initialize plugin host sessions and run plugin
`theater.prepare` hooks before scenario execution starts. Preflight gates
scenario execution after input binding resolution; it is not a sandbox for
plugin startup or descriptor preparation side effects.

Preflight checks scenario inputs only. It does not read action outputs, report
contents, inventories, cleanup values, parent scenario internals, sibling
scenario state, adapter-local state, environment variables, files, network
services, or plugin host state.

## Matchers

Preflight checks use Theater's matcher descriptor registry for validation and
execution. A matcher must opt in through its descriptor preflight policy before
it can be used as a preflight assertion. The shipped slice supports
descriptor-backed `expectation.matches` string allow-list checks.

Regex allow-lists must be full-string checks. `expectation.matches` preflight
patterns must begin with `^` and end with `$`; substring matching is rejected
during validation.

## Checked Examples

Checked Theater DSL source:

<!-- theater-doc: source id=reference-preflight-thtr kind=thtr path=../examples/reference/preflight.thtr checks=fmt,lower,validate,run pair=reference-preflight -->
```thtr
stage reference-preflight

scenario send-sample-message(recipient_email: string!, allow_non_test_recipient: bool)
  preflight recipient-test-domain: $recipient_email matches r"^[^@]+@example\.test$" override $allow_non_test_recipient
  act submit-message
    do action.generate
      outputs:
        accepted: true

call send-test = send-sample-message(
  recipient_email: "person@example.test",
  allow_non_test_recipient: false
)
```

Checked YAML source:

<!-- theater-doc: source id=reference-preflight-yaml kind=yaml path=../examples/reference/preflight.yaml checks=validate,run pair=reference-preflight -->
```yaml
id: reference-preflight
scenarios:
  - id: send-sample-message
    inputs:
      recipient_email:
        type: string
        required: true
      allow_non_test_recipient:
        type: bool
    preflight:
      - id: recipient-test-domain
        input:
          ref: recipient_email
        assert:
          matches: '^[^@]+@example\.test$'
        override:
          ref: allow_non_test_recipient
    acts:
      - id: submit-message
        action:
          use: action.generate
          with:
            outputs:
              kind: object
              object:
                accepted: true
scenario_calls:
  - id: send-test
    scenario_id: send-sample-message
    bindings:
      recipient_email: person@example.test
      allow_non_test_recipient: false
```

Checked rejection command:

<!-- theater-doc: command id=reference-preflight-reject cwd=../.. expect-exit=1 expect-stdout=failed expect-stdout-2=preflight expect-stdout-3=matcher_mismatch reject-stdout=person@example.com reject-stderr=person@example.com -->
```sh
go run ./cmd/theater run docs/examples/reference/preflight-rejected.thtr --live off
```

## Overrides

An override is authored on one preflight check and scoped to that check. There
is no global CLI bypass and no hidden safe-mode switch.

The first override shape is a boolean scenario input:

```
override:
  ref: allow_non_test_recipient
```

Absent, null, or missing override values do not bypass the check. Non-bool
override inputs are rejected during validation. An override that bypasses a
check is visible in the report through `override_present` and `override_used`.

## Reporting

A preflight rejection is a runtime setup failure:

- final scenario status is `failed`
- failure kind is `setup`
- failure phase is `run`
- JUnit projection treats the scenario as an error, not an expectation failure
- no actions, inventories, logs, expectations, exports, transitions, cleanup, or
  auth capture run after rejection

Preflight diagnostics include guard id, scenario input path, reason code,
assert matcher ref, override presence and usage, and source spans when
available. They must not include the rejected value, raw input binding value,
secret or personal material, sensitive matcher args, or raw host-provided
values.

The JSON diagnostic shape is node-scoped:

```
{
  "kind": "preflight",
  "preflight": {
    "guard_id": "recipient-test-domain",
    "input_ref": "recipient_email",
    "input_path": "stage.main/call.send-prod/binding.recipient_email",
    "assert_ref": "expectation.matches",
    "reason_code": "matcher_mismatch",
    "override_ref": "allow_non_test_recipient",
    "override_present": true,
    "override_used": false
  }
}
```

Text, Markdown, and JUnit render the same report-safe metadata without raw
values.

## Tooling

YAML loading, `.thtr` parsing, formatting, lowering, source maps, LSP analysis,
and the native JetBrains plugin recognize the shipped preflight syntax.
Validation diagnostics are source-mapped to the preflight declaration, matcher
arguments, and scenario-call input binding spans when available.

## Non-Goals

Preflight does not include a general policy language, arbitrary predicates over
scenario scope, action-output checks, inventory reads, provider-specific
validators, global CLI bypasses, hidden safe mode, side-effect rollback, cleanup
after preflight rejection, or automatic redaction based on variable names alone.
