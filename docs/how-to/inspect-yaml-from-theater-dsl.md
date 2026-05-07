# Inspect YAML From Theater DSL

Use `lower` when you want to see the canonical YAML model behind a compact
`.thtr` file.

Lower the Theater DSL file:

<!-- theater-doc: command id=howto-lower-thtr cwd=../.. expect-stdout=scenario_calls expect-stdout-2=messages/make -->
```sh
go run ./cmd/theater lower docs/examples/reusable-scenario/theater/flows/reuse-message.thtr
```

The command prints YAML to stdout. It does not change the `.thtr` file.
The output includes `scenario_calls` and the reusable `messages/make` call.

Validate the YAML version when you want to compare the same flow as YAML:

<!-- theater-doc: command id=howto-lower-yaml-validate cwd=../.. expect-stdout=valid reject-stdout=hint -->
```sh
go run ./cmd/theater validate docs/examples/reusable-scenario/theater/flows/reuse-message.yaml
```

Use this command for inspection and debugging. Keep authoring in the format that
is clearer for the flow you are editing. Use
[Stage, Scenario, Act](../concepts/stage-scenario-act.md) for the shared model
and [Selectors](../reference/selectors.md) when you are comparing selected
values.
