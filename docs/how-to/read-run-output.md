# Read Run Output

Use JSON output when another tool needs the run result or when the text summary
is not enough.

Run the Theater DSL example as JSON:

<!-- theater-doc: command id=howto-read-output-thtr cwd=../.. expect-stdout=check-values expect-stdout-2=passed -->
```sh
go run ./cmd/theater run docs/examples/check-values/profile.thtr --format json --live off
```

Run the YAML example as JSON:

<!-- theater-doc: command id=howto-read-output-yaml cwd=../.. expect-stdout=check-values expect-stdout-2=passed -->
```sh
go run ./cmd/theater run docs/examples/check-values/profile.yaml --format json --live off
```

Look for `result.report.status` first. When it is `passed`, all selected scenario
calls and expectations passed. When it is not `passed`, inspect `result.report`
nodes and the failure fields near the failed act or expectation.

The checked commands above assert that the JSON contains the `check-values`
stage and a `passed` status.

Use [Selectors](../reference/selectors.md) and
[Expectations](../reference/expectations.md) when the failure is about a checked
value. Use [Validate, Run, Report](../concepts/validate-run-report.md) for the
mental model behind the report.
