# Preflight Guardrails

Preflight guardrails are a ratified future contract. They are not available in
the current YAML schema, `.thtr` syntax, CLI validation, runtime, report JSON,
or editor tooling.

The future model is scenario-level preflight: a scenario may declare checks over
resolved scenario inputs before any act can run. Preflight is for rejecting
unsafe runtime values before live side effects, not for action-output
assertions, adapter-local validation, or general policy evaluation.

## Shape

The planned YAML shape is:

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

The exact `.thtr` syntax will be finalized with the implementation, but it must
lower into the same scenario-level semantic model as YAML.

## Semantics

Preflight runs after scenario-call bindings resolve and before act properties,
inventories, actions, logs, expectations, exports, transitions, cleanup,
scenario auth slot initialization, or auth capture can run.

A rejected preflight prevents later scenario action side effects. Value-source
side effects that happen before scenario input binding resolution are outside
the first preflight guarantee unless a later contract explicitly covers them.

The first slice checks scenario inputs only. Preflight must not read action
outputs, report contents, inventories, cleanup values, parent scenario
internals, sibling scenario state, or adapter-local state.

## Matchers

Preflight checks use Theater's matcher descriptor registry for validation and
execution. The first slice supports only descriptor-backed string allow-list
checks.

Regex allow-lists must be full-string checks. If the implementation uses the
built-in `expectation.matches` matcher, it must reject unanchored patterns or
provide an equivalent full-string wrapper. Partial substring matching is not a
valid preflight allow-list check.

## Overrides

An override is authored on one preflight check and scoped to that check. There
is no global CLI bypass and no hidden safe-mode switch.

The first override shape is a boolean scenario input:

```
override:
  ref: allow_non_test_recipient
```

Absent, null, non-bool, or selector-error override values do not bypass the
check. An override that bypasses a failed check must be visible in the report.

## Reporting

A preflight rejection is a runtime setup failure:

- final scenario status is `failed`
- failure kind is `setup`
- failure phase is `run`
- JUnit projection treats the scenario as an error, not an expectation failure
- no actions, inventories, logs, expectations, exports, transitions, cleanup, or
  auth capture run after rejection

Preflight diagnostics include guard id, scenario input path, reason code,
override presence and usage, and source spans when available. They must not
include the rejected value, raw input binding value, secret or personal
material, sensitive matcher args, or raw host-provided values.

## Tooling

The implementation must update YAML support, `.thtr` parsing and lowering,
source maps, LSP analysis, and native JetBrains fixtures before claiming
preflight runtime support. Source maps must cover the preflight block, guard
ids, guarded input refs, matcher refs, matcher args, override refs, and
scenario-call input binding spans when available.

## Non-Goals

The first preflight implementation will not include a general policy language,
arbitrary predicates over scenario scope, action-output checks, inventory reads,
provider-specific validators, global CLI bypasses, hidden safe mode,
side-effect rollback, cleanup after preflight rejection, or automatic redaction
based on variable names alone.
