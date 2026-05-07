# Stage, Scenario, Act

Theater uses a small hierarchy so a run can point to the exact thing that
passed or failed.

Think of a stage as the table for one checklist. The stage holds the scenarios
and says which scenario calls should run.

A scenario is a reusable checklist card. It gives a name to a flow that can be
called from the stage.

An act is one step on that card. An act runs one action and then checks what
that action produced.

An expectation is the check attached to an act. If the check fails, Theater can
report the scenario, act, and expectation address instead of only saying that
the whole run failed.

The smallest tutorial example has:

- one stage: `docs-first`
- one scenario: `hello`
- one act: `say-hello`
- one expectation: `message`
- one call: `run = hello()`

The concrete file is explained in [First Stage](../tutorial/02-first-stage.md).
The failure address is shown in [Edit And Fix](../tutorial/03-edit-and-fix.md).

Use [reference](../reference/index.md) when you need exact field names.
