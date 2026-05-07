# Use Persistent State

Use persistent state when a flow must remember a checked value across acts or
coordinate with a local fixture store.

The checked example uses the built-in file backend under `/tmp/theater-doc-state`.
The docs fixture wrapper resets that directory before and after each run.

Run the Theater DSL state example:

<!-- theater-doc: command id=howto-state-run-thtr cwd=../.. expect-stdout=passed expect-stdout-2="eventually: converged_acts=1" -->
```sh
go run ./docs/examples/http-state/fixture -- go run ./cmd/theater run docs/examples/http-state/profile-state.thtr --live off
```

Run the YAML state example:

<!-- theater-doc: command id=howto-state-run-yaml cwd=../.. expect-stdout=passed expect-stdout-2="eventually: converged_acts=1" -->
```sh
go run ./docs/examples/http-state/fixture -- go run ./cmd/theater run docs/examples/http-state/profile-state.yaml --live off
```

`passed` proves the flow read and updated the local state record. The
`eventually: converged_acts=1` line proves the status act needed at least one
retry before the final expectation passed.

The pattern is:

- read the record to get its current version
- write with that version so the update is compare-and-set
- export values that later acts need

For the value-flow model, open [Dataflow](../concepts/dataflow.md). For the
guided explanation, open [Wait For Result](../tutorial/07-wait-for-result.md).
