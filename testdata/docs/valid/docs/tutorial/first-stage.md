# First Stage

<!-- theater-doc: source id=first-stage-thtr kind=thtr path=../examples/first-stage/stage.thtr pair=first-stage checks=fmt,lower,validate,run -->
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

<!-- theater-doc: source id=first-stage-yaml kind=yaml path=../examples/first-stage/stage.yaml pair=first-stage checks=validate,run -->
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
                message:
                  kind: literal
                  value: hello
        expectations:
          - id: message
            subject:
              field: values
              path: /message
            assert:
              ref: expectation.equal
              args:
                expected:
                  kind: literal
                  value: hello
scenario_calls:
  - id: run
    scenario_id: hello
```

<!-- theater-doc: command id=validate-first-stage expect-stdout=valid -->
```sh
theater validate ../examples/first-stage/stage.thtr
```
