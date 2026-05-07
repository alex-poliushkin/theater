# First Stage

This page explains the tiny stage you ran in [First Run](01-first-run.md). The
goal is not to learn every field yet. The goal is to recognize the smallest
shape: one stage, one scenario, one act, one expectation, and one call.

Think of a stage as a small checklist. It contains the steps Theater can run and
the checks Theater must make before it says the run passed.

## Theater DSL

Theater DSL keeps the checklist compact:

<!-- theater-doc: source id=tutorial-first-stage-thtr kind=thtr path=../examples/first-stage/stage.thtr pair=tutorial-first-stage checks=fmt,lower,validate,run -->
```thtr
stage docs-first

scenario hello
  act say-hello
    do action.generate
      outputs:
        message: "hello"
    expect message: field(values) | path("/message") == "hello"

call run = hello()
```

Read it from top to bottom:

- `stage docs-first` names the file-level stage Theater loads.
- `scenario hello` names the reusable check.
- `act say-hello` is the single step inside the scenario.
- `do action.generate` creates a value without calling an external service.
- `expect message` checks the generated value.
- `call run = hello()` tells Theater which scenario to execute.

That is enough model for this first file.

## YAML

YAML describes the same stage with explicit fields. It is more verbose, but it
is an equal authoring form and the canonical interchange form.

<!-- theater-doc: source id=tutorial-first-stage-yaml kind=yaml path=../examples/first-stage/stage.yaml pair=tutorial-first-stage checks=validate,run -->
```yaml
id: docs-first
scenarios:
  - id: hello
    acts:
      - id: say-hello
        action:
          use: action.generate
          with:
            outputs:
              kind: object
              object:
                message: hello
        expectations:
          - id: message
            subject:
              field: values
              path: /message
            assert:
              ref: expectation.equal
              args:
                expected: hello
scenario_calls:
  - id: run
    scenario_id: hello
```

The names line up with the Theater DSL file: `id: docs-first` is the stage,
`id: hello` is the scenario, `id: say-hello` is the act, and `scenario_calls`
starts the scenario.

Read the YAML by the same landmarks:

- `action.use` is the action the act runs.
- `object.message` is the generated value.
- `expectations` contains the check.
- `subject.field` and `subject.path` choose the value to check.
- `args.expected` is the value Theater compares against.

Use Theater DSL when you want the smallest file to read. Use YAML when you want
the same model written as explicit fields.

## Change One Value

To experiment without changing setup, change the generated value and the
expected value to the same new word.

In Theater DSL, the generated value is in the `outputs` block and the expected
value is on the `expect` line. Then rerun the Theater DSL file:

<!-- theater-doc: command id=first-stage-rerun-thtr cwd=../.. expect-stdout=passed -->
```sh
go run ./cmd/theater run docs/examples/first-stage/stage.thtr --live off
```

In YAML, the generated value is `object.message` and the expected value is
`args.expected`. Then rerun the YAML file:

<!-- theater-doc: command id=first-stage-rerun-yaml cwd=../.. expect-stdout=passed -->
```sh
go run ./cmd/theater run docs/examples/first-stage/stage.yaml --live off
```

Next: open [Edit And Fix](03-edit-and-fix.md) to make one controlled mistake and
see how Theater points at a failed check.
