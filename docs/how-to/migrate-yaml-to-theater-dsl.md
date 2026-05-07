# Migrate YAML To Theater DSL

Use `migrate from-yaml` when you want to convert an existing YAML stage into
formatter-clean Theater DSL without hand-rewriting the file.

Preview the Theater DSL output:

<!-- theater-doc: command id=howto-migrate-yaml cwd=../.. expect-stdout="stage check-values" expect-stdout-2="expect profile-id" -->
```sh
go run ./cmd/theater migrate from-yaml --file docs/examples/check-values/profile.yaml
```

The command writes `.thtr` source to stdout. After you save that output in your
own flow file, run `fmt --check` and `validate` on the saved `.thtr` file.
The preview starts with `stage check-values` and includes the `profile-id`
expectation from the YAML source.

Validate the original YAML while you are comparing behavior:

<!-- theater-doc: command id=howto-migrate-source-yaml-validate cwd=../.. expect-stdout=valid reject-stdout=hint -->
```sh
go run ./cmd/theater validate docs/examples/check-values/profile.yaml
```

The migration is one-way authoring help. YAML remains a first-class format, so
you do not need to migrate a working YAML flow unless Theater DSL is easier to
read for that file. Use [Inspect YAML From Theater DSL](inspect-yaml-from-theater-dsl.md)
when you want to compare the generated `.thtr` back to canonical YAML.
