# Test Local Mocks Against Client Contracts

Use this practice when a downstream repository runs Theater flows against a
local substitute for an external service. The goal is to keep the local mock
aligned with the client contract consumed by the system under test, while
leaving Theater flow execution as the final composed-system check.

This does not add a Theater command, schema field, plugin capability, or mock
orchestration DSL.

## Put The Test At The Client Boundary

Test the mock through the same HTTP or RPC shape that the system's external
client consumes. Name the test after the external contract slice, not after the
mock's internal handler.

For example:

```
TestExternalClientSampleResourceContract
TestExternalClientSearchContractMissingConsumedField
```

The test should start the local mock in process, point the external client at
that server, call the client method, and assert the client-visible result. Avoid
testing the mock database, broker, queue, scheduler, or provider-private
implementation details.

Make request-shape failures explicit. Either configure the mock route narrowly
so an unexpected method, path, query value, or request body shape returns a test
failure, or record the observed request and assert it after the client call. A
permissive catch-all handler can let a drifted client pass against the mock.

## Cover The Consumed Slice

For each mock endpoint added to support local Theater flows, cover the smallest
contract slice the client actually consumes:

| Area | Assert |
| --- | --- |
| Request method | The client sends the expected method |
| Request path | The mock accepts the expected path shape |
| Query parameters | Only parameters the client depends on |
| Request body | Only fields needed by the client contract |
| Response status | Success status and any client-defined non-success status |
| Response fields | Only fields read by the client model |

Do not expand the mock contract test into a full fake provider suite. Add fields
only when the client reads them or the local Theater flow needs them through the
public system behavior.

## Include A Failure Case

Add at least one non-success or malformed-response case when the client defines
that behavior. Good examples are:

- the mock returns a known non-success status and the client maps it to the
  expected error
- a consumed response field is missing and the client rejects the response
- a consumed response field has the wrong type and the client reports a decode
  or contract error

Do not add negative cases for provider behavior the client does not model.

## Keep Tests Deterministic

Use per-test server instances and isolated fixtures.

Avoid:

- fixed external ports
- test-order dependency
- unsynchronized shared handler state
- sleeps for readiness
- shared mutable fixtures across parallel tests
- real network calls to the external provider
- database, broker, queue, or scheduler dependencies

If the mock stores state, keep it in a per-test in-memory store and protect it
with ordinary synchronization. Reset it by constructing a new server for each
test rather than by relying on global cleanup.

## Run Theater After Mock Contract Tests

Focused mock contract tests prove that the local substitute still speaks the
client contract. They do not prove the full public workflow.

After the mock contract tests pass, run the relevant Theater local flow against
the composed local system. That flow remains the final guardrail for routing,
auth, dataflow, timing, read-model behavior, and report output.

The typical order in a downstream repository is:

1. Run focused mock contract tests.
2. Start the local composed system with the mock enabled.
3. Run the Theater flow with live output disabled or bounded for CI.
4. Inspect the Theater report when the composed workflow fails.

Keep the two layers separate: mock contract tests protect local substitutes,
and Theater protects the end-to-end behavior visible through public system
interfaces.
