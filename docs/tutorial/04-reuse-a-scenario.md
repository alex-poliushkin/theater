# Reuse A Scenario

You already ran one small file. This page keeps that success and adds one new
idea: put a scenario in a small library, then call it from a flow.

Think of the library scenario as a reusable checklist card. The flow decides
which card to use and what values to write on it for this run.

## Repository Shape

The example uses this ordinary directory shape:

- `theater/flows/reuse-message.thtr`
- `theater/flows/reuse-message.yaml`
- `theater/lib/messages/make.thtr`
- `theater/lib/messages/make.yaml`

`theater/flows/` holds runnable entry files. `theater/lib/` holds reusable
scenario definitions. The flow calls a scenario by its scenario id, such as
`messages/make`; that id is not a file import.

## Theater DSL Library

The library scenario takes one input, creates a message, checks it, and exports
the message for the caller.

<!-- theater-doc: source id=reuse-library-thtr kind=thtr path=../examples/reusable-scenario/theater/lib/messages/make.thtr pair=reuse-library checks=fmt,lower,validate -->
```thtr
stage message-library

scenario messages/make(text: string!)
  act create
    do action.generate
      outputs:
        message: $text
    expect message: field(values) | path("/message") == $text
    export message = field(values) | path("/message")
```

The important parts are small:

- `text: string!` says the scenario needs a text input.
- `$text` reads the value passed by the caller.
- `export message` makes the checked value available after the scenario passes.

## Theater DSL Flow

The flow calls the library scenario, promotes its export to `shared_message`,
then passes that value into a second local scenario.

<!-- theater-doc: source id=reuse-flow-thtr kind=thtr path=../examples/reusable-scenario/theater/flows/reuse-message.thtr pair=reuse-flow checks=fmt,lower,validate,run -->
```thtr
stage reusable-message-flow

scenario verify-message(expected: string!, actual: string!)
  act check
    do action.generate
      outputs:
        actual: $actual
    expect message: field(values) | path("/actual") == $expected

call make-message = messages/make(text: "hello from Theater")
  export shared_message = $message

call check-message = verify-message(expected: "hello from Theater", actual: $shared_message)
  dependency make-message
```

Read the flow as two cards on a table:

- `make-message` runs first and exports `shared_message`.
- `check-message` depends on `make-message`, then reads `$shared_message`.
- the dependency is explicit; Theater does not guess ordering from the `$ref`.

Validate the Theater DSL flow:

<!-- theater-doc: command id=reuse-validate-thtr cwd=../.. expect-stdout=valid reject-stdout=hint -->
```sh
go run ./cmd/theater validate docs/examples/reusable-scenario/theater/flows/reuse-message.thtr
```

Run it:

<!-- theater-doc: command id=reuse-run-thtr cwd=../.. expect-stdout=passed -->
```sh
go run ./cmd/theater run docs/examples/reusable-scenario/theater/flows/reuse-message.thtr --live off
```

A passing result here proves that Theater loaded the flow from
`theater/flows/`, resolved `messages/make` from `theater/lib/`, exported
`shared_message`, and used that value in the dependent check.

## YAML Library

YAML describes the same reusable scenario with explicit fields.

<!-- theater-doc: source id=reuse-library-yaml kind=yaml path=../examples/reusable-scenario/theater/lib/messages/make.yaml pair=reuse-library checks=validate -->
```yaml
id: message-library
scenarios:
  - id: messages/make
    inputs:
      text:
        type: string
        required: true
    acts:
      - id: create
        action:
          use: action.generate
          with:
            outputs:
              kind: object
              object:
                message:
                  kind: ref
                  ref:
                    name: text
        expectations:
          - id: message
            subject:
              field: values
              path: /message
            assert:
              ref: expectation.equal
              args:
                expected:
                  kind: ref
                  ref:
                    name: text
        exports:
          - as: message
            field: values
            path: /message
```

The same landmarks are present: `inputs.text`, `ref.name: text`, and
`exports.as: message`.

## YAML Flow

The YAML flow has the same two calls.

<!-- theater-doc: source id=reuse-flow-yaml kind=yaml path=../examples/reusable-scenario/theater/flows/reuse-message.yaml pair=reuse-flow checks=validate,run -->
```yaml
id: reusable-message-flow
scenarios:
  - id: verify-message
    inputs:
      expected:
        type: string
        required: true
      actual:
        type: string
        required: true
    acts:
      - id: check
        action:
          use: action.generate
          with:
            outputs:
              kind: object
              object:
                actual:
                  kind: ref
                  ref:
                    name: actual
        expectations:
          - id: message
            subject:
              field: values
              path: /actual
            assert:
              ref: expectation.equal
              args:
                expected:
                  kind: ref
                  ref:
                    name: expected
scenario_calls:
  - id: make-message
    scenario_id: messages/make
    bindings:
      text: hello from Theater
    exports:
      - ref:
          name: message
        as: shared_message
  - id: check-message
    scenario_id: verify-message
    bindings:
      expected: hello from Theater
      actual:
        kind: ref
        ref:
          name: shared_message
    dependencies:
      - call_id: make-message
        when: success
```

Validate the YAML flow:

<!-- theater-doc: command id=reuse-validate-yaml cwd=../.. expect-stdout=valid reject-stdout=hint -->
```sh
go run ./cmd/theater validate docs/examples/reusable-scenario/theater/flows/reuse-message.yaml
```

Run it:

<!-- theater-doc: command id=reuse-run-yaml cwd=../.. expect-stdout=passed -->
```sh
go run ./cmd/theater run docs/examples/reusable-scenario/theater/flows/reuse-message.yaml --live off
```

You now have a runnable flow that reuses a library scenario in both Theater DSL
and YAML. Open [Reusable Scenarios](../concepts/reusable-scenarios.md) for the
mental model behind the same example.
