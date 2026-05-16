# Cleanup Hooks

Cleanup hooks are a ratified future contract. They are not available in the
current YAML schema, `.thtr` syntax, CLI validation, runtime, report JSON, or
editor tooling.

The future model is scenario-level cleanup: a scenario may declare cleanup acts
that run after the main scenario path reaches a terminal outcome. Cleanup acts
are ordinary action acts in a dedicated post-scenario cleanup section. They stay
visible in reports and use the same action registry, auth handling, redaction,
and source-map diagnostics as main acts.

## Shape

The planned YAML shape is:

```
scenarios:
  - id: check-sample-resource
    inputs:
      sample_resource_id:
        type: string
    acts:
      - id: verify-resource
        action:
          use: action.http
          with: {}
    cleanup:
      - id: delete-sample-resource
        when:
          exists:
            ref: sample_resource_id
        required_identifiers:
          - ref: sample_resource_id
        action:
          use: action.http
          with:
            method: DELETE
            url:
              kind: string
              parts:
                - https://api.example.test/resources/
                - ref: sample_resource_id
```

The exact `.thtr` syntax will be finalized with the implementation, but it must
lower into the same scenario-level semantic model as YAML.

## Semantics

Cleanup is scheduled only after `Prepare` has succeeded and scenario execution
has entered the `Run` phase. It is not scheduled after static validation or
preparation failure.

When implemented, cleanup will run after:

- main path pass
- main path failure
- graceful cancellation after scenario start
- soft timeout when the runner still controls execution
- recoverable internal failure at the scenario boundary

Timeout uses the existing report model: final status remains `failed` with a
timeout failure kind unless a later report-schema migration explicitly changes
the status vocabulary.

Cleanup is best effort under a finite cleanup budget. Theater will not promise
cleanup after process kill, host termination, unrecoverable crash, or a hard
runner stop.

Cleanup acts run sequentially in declaration order and continue after individual
cleanup failures unless cleanup itself is interrupted.

The first cleanup act field subset is intentionally narrow:

- `id`
- `when.exists`
- `required_identifiers`
- `action`
- `logs`
- `expectations`

`properties`, `transitions`, `eventually`, `capture_auth`, and
scenario-visible cleanup exports are not part of the first cleanup slice.
Future cleanup-local properties or exports need a separate contract and must
not read inventories or ambient process state unless that behavior is explicitly
ratified.

## Safety

Cleanup may read only values already available in scenario scope: scenario
inputs, exports committed before the main path ended, and cleanup-local values
if a later implementation explicitly supports them.

Cleanup must not read raw action outputs, report contents, ambient process
state, provider inventories, sibling scenario internals, or parent scenario
internals.

Before dispatching a cleanup action, Theater must fail closed for missing,
null, empty, out-of-scope, or unresolved required identifiers. A skipped cleanup
act must say why it was skipped and whether the action was dispatched.

`when.exists.ref` is an applicability guard. It is not enough to prove that a
destructive request target is safe. Cleanup acts that can delete or mutate a
remote object must declare the target refs in `required_identifiers`. Theater
must check those refs before building action arguments, because a composed URL
or request body can still be syntactically valid after an identifier is blank.

Skipped cleanup records use cleanup-result reasons. These reasons are part of
the future cleanup record, not the current `NodeReport.skip_reason` enum.

| Reason | Meaning |
| --- | --- |
| `guard_false` | Cleanup was not applicable |
| `guard_missing_value` | The applicability guard referenced a value that was not available |
| `missing_value` | A required identifier was not produced before cleanup |
| `null_value` | A required identifier resolved to null |
| `empty_identifier` | A required identifier was empty or blank |
| `cleanup_budget_exhausted` | Cleanup could not dispatch before its bounded budget expired |

For all skipped cases above, the report must record `action_dispatched=false`.
If the cleanup budget expires after a cleanup action has been dispatched, the
cleanup act is `interrupted` and the report must preserve that dispatch was
attempted.

## Reporting

Cleanup reporting keeps main verification and cleanup separate:

- the main outcome remains visible
- cleanup has its own aggregate outcome
- cleanup records are a report cleanup section, not a new shipped
  `failure.phase` enum; cleanup failures should keep the existing run-phase
  failure model unless a future report-schema migration explicitly changes it
- cleanup failures after a failed main path are secondary
- cleanup failures after a passed main path fail the scenario with a
  cleanup-specific reason
- text, Markdown, and JUnit projections are built from the same canonical report
  data

The first implementation is expected to keep one JUnit testcase per scenario
call. Cleanup details should be projected into that testcase instead of adding
synthetic cleanup testcases.

## Tooling

The implementation must update YAML support, `.thtr` parsing and lowering,
source maps, LSP analysis, and native JetBrains fixtures before claiming cleanup
runtime support. Source maps must cover the cleanup block, cleanup act ids,
guards, `required_identifiers` refs, ref-bearing arguments, and action
declarations.

## Non-Goals

The first cleanup implementation will not include act-level `finally`,
stage-level cleanup, hidden global finalizers, automatic resource discovery,
provider-specific cleanup managers, report scraping, parallel cleanup, broad
policy predicates, hidden retries, or cleanup guarantees after hard process
termination.
