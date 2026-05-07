# First Run

Use this page when you have a Theater checkout and want the fastest visible
success. You will validate and run one tiny example. You do not need to write a
file, export environment variables, or read the reference first.

Run the commands from the repository root. You need Go available because the
commands use `go run ./cmd/theater`.

The tiny directory is `docs/examples/first-stage/`. It contains a compact
Theater DSL file, `stage.thtr`, and the equivalent YAML file, `stage.yaml`.

## 1. Validate The Theater DSL File

Validate first. This checks that Theater can read the file before it tries to
run anything.

<!-- theater-doc: command id=first-run-validate-thtr cwd=../.. expect-stdout=valid reject-stdout=hint -->
```sh
go run ./cmd/theater validate docs/examples/first-stage/stage.thtr
```

The command should print `docs/examples/first-stage/stage.thtr: valid`.

## 2. Run The Theater DSL File

Now run the same file with live progress turned off so the result is a single
stable line.

<!-- theater-doc: command id=first-run-run-thtr cwd=../.. expect-stdout=passed -->
```sh
go run ./cmd/theater run docs/examples/first-stage/stage.thtr --live off
```

The command should print a line containing `passed`.

## 3. Validate And Run The YAML Form

The YAML file describes the same check. Validate it the same way:

<!-- theater-doc: command id=first-run-validate-yaml cwd=../.. expect-stdout=valid reject-stdout=hint -->
```sh
go run ./cmd/theater validate docs/examples/first-stage/stage.yaml
```

The command should print `docs/examples/first-stage/stage.yaml: valid`.

Then run it:

<!-- theater-doc: command id=first-run-run-yaml cwd=../.. expect-stdout=passed -->
```sh
go run ./cmd/theater run docs/examples/first-stage/stage.yaml --live off
```

The command should print a line containing `passed`.

You now have a working Theater run in both authoring forms. Next, open
[First Stage](02-first-stage.md) to inspect the file you just ran.
