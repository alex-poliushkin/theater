# List Reusable Scenarios

Use `list scenarios` when you want to see the public scenarios available under
`theater/lib` before calling one from a flow.

The docs example contains both Theater DSL and YAML versions of the same library
scenario. Choose the syntax you want to call.

List Theater DSL scenarios and print a call skeleton:

<!-- theater-doc: command id=howto-list-scenarios-thtr cwd=../.. expect-stdout=messages/make expect-stdout-2="call run-messages-make" -->
```sh
go run ./cmd/theater list scenarios --root docs/examples/reusable-scenario --syntax thtr --call-skeleton
```

List YAML scenarios and print a YAML call skeleton:

<!-- theater-doc: command id=howto-list-scenarios-yaml cwd=../.. expect-stdout=messages/make expect-stdout-2=scenario_calls -->
```sh
go run ./cmd/theater list scenarios --root docs/examples/reusable-scenario --syntax yaml --call-skeleton
```

Use the listed `SCENARIO` value as the `scenario_id` in YAML or as the called
scenario name in Theater DSL.

The successful output includes the `messages/make` scenario and a call skeleton
for the requested syntax.

For the concept, open [Reusable Scenarios](../concepts/reusable-scenarios.md).
For the guided version, open [Reuse A Scenario](../tutorial/04-reuse-a-scenario.md).
