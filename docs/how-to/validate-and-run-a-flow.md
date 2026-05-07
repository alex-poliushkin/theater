# Validate And Run A Flow

Use this when you already have a Theater DSL or YAML file and want the smallest
preflight before a real run.

Validate the Theater DSL file first:

<!-- theater-doc: command id=howto-validate-flow-thtr cwd=../.. expect-stdout=valid reject-stdout=hint -->
```sh
go run ./cmd/theater validate docs/examples/first-stage/stage.thtr
```

Run it after validation passes:

<!-- theater-doc: command id=howto-run-flow-thtr cwd=../.. expect-stdout=passed -->
```sh
go run ./cmd/theater run docs/examples/first-stage/stage.thtr --live off
```

Use the same two-step shape for YAML:

<!-- theater-doc: command id=howto-validate-flow-yaml cwd=../.. expect-stdout=valid reject-stdout=hint -->
```sh
go run ./cmd/theater validate docs/examples/first-stage/stage.yaml
```

<!-- theater-doc: command id=howto-run-flow-yaml cwd=../.. expect-stdout=passed -->
```sh
go run ./cmd/theater run docs/examples/first-stage/stage.yaml --live off
```

`validate` checks the file without executing actions. `run` validates again,
executes the selected calls, and prints the final result.

For the model behind this split, open
[Validate, Run, Report](../concepts/validate-run-report.md). For the guided
first pass, open [First Run](../tutorial/01-first-run.md).
