# Dataflow

Theater dataflow is explicit: actions produce values, and later checks select
the value they need.

Think of each act as putting labeled items into small boxes. An expectation does
not guess which item to inspect. It names the box and the path inside it.

In the first tutorial example, `action.generate` creates an output named
`values`.

That output contains a `message` value. The expectation opens `values`, follows
`/message`, and compares the value it finds with the expected word.

That explicit path is why a failure can say what was actually seen and what was
expected. The data moved through a named action output, not hidden global state.

This matters more as files grow:

- reusable scenarios need clear inputs and outputs
- later acts should not depend on invisible side effects
- failure reports should point to the value that was checked

Start with the concrete selector in [First Stage](../tutorial/02-first-stage.md).
Use [reference](../reference/index.md) when you need exact selector syntax.
