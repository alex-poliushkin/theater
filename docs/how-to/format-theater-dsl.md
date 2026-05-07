# Format Theater DSL

Use `fmt` when a `.thtr` file should be normalized before review or before
lowering it to YAML.

Check whether the file already uses canonical formatting:

<!-- theater-doc: command id=howto-format-thtr-check cwd=../.. -->
```sh
go run ./cmd/theater fmt --check docs/examples/reusable-scenario/theater/flows/reuse-message.thtr
```

No output means the file is already formatted.

Print the formatted form without changing the file:

<!-- theater-doc: command id=howto-format-thtr-print cwd=../.. expect-stdout="stage reusable-message-flow" -->
```sh
go run ./cmd/theater fmt docs/examples/reusable-scenario/theater/flows/reuse-message.thtr
```

The printed source starts with `stage reusable-message-flow`.

When you want to rewrite the file, add `--write` to the same command. YAML does
not go through Theater DSL formatting; validate the YAML companion instead:

<!-- theater-doc: command id=howto-format-yaml-validate cwd=../.. expect-stdout=valid reject-stdout=hint -->
```sh
go run ./cmd/theater validate docs/examples/reusable-scenario/theater/flows/reuse-message.yaml
```

Use [Inspect YAML From Theater DSL](inspect-yaml-from-theater-dsl.md) when you
want to see the canonical YAML produced from a `.thtr` file. Use
[Stage, Scenario, Act](../concepts/stage-scenario-act.md) when you need the
shared model behind both authoring formats.
