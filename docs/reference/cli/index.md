# CLI Reference

The `theater` CLI is the validate-first interface for authoring, checking,
running, and inspecting Theater stages.

Source of truth:

- `go run ./cmd/theater help`
- `go run ./cmd/theater help <command>`
- [Output Formats](../outputs/index.md)
- [Capabilities](../capabilities/index.md)

## Checked Help Surface

<!-- theater-doc: command id=reference-cli-root-help cwd=../../.. expect-stdout="Usage:" expect-stdout-2="validate, check" expect-stdout-3="THEATER_PLUGINS_CONFIG" -->
```sh
go run ./cmd/theater help
```

<!-- theater-doc: command id=reference-cli-validate-help cwd=../../.. expect-stdout="theater validate <stage.{yaml|yml|thtr}>" expect-stdout-2="--plugins-readiness" expect-stdout-3="--debug-paths" -->
```sh
go run ./cmd/theater help validate
```

<!-- theater-doc: command id=reference-cli-run-help cwd=../../.. expect-stdout="--format text|json|junit" expect-stdout-2="--live auto|off" expect-stdout-3="--json-output" expect-stdout-4="--junit-output" expect-stdout-5="--markdown-output" expect-stdout-6="--plugin-exporter" -->
```sh
go run ./cmd/theater help run
```

<!-- theater-doc: command id=reference-cli-plugins-help cwd=../../.. expect-stdout="theater plugins <command>" expect-stdout-2="digest" expect-stdout-3="inspect, ls" -->
```sh
go run ./cmd/theater help plugins
```

<!-- theater-doc: command id=reference-cli-exit-codes-help cwd=../../.. expect-stdout="0  success" expect-stdout-2="1  validation diagnostics" expect-stdout-3="2  command usage error" -->
```sh
go run ./cmd/theater help exit-codes
```

<!-- theater-doc: command id=reference-cli-list-scenarios-help cwd=../../.. expect-stdout="--syntax all|yaml|thtr" expect-stdout-2="--call-skeleton" -->
```sh
go run ./cmd/theater help list scenarios
```

<!-- theater-doc: command id=reference-cli-migrate-from-yaml-help cwd=../../.. expect-stdout="--file <stage.yaml|stage.yml>" expect-stdout-2="--plugins-config <path.json>" -->
```sh
go run ./cmd/theater help migrate from-yaml
```

<!-- theater-doc: command id=reference-cli-report-render-help cwd=../../.. expect-stdout="theater report render --input <run.json>" expect-stdout-2="--format <string>" expect-stdout-3="Markdown summary" -->
```sh
go run ./cmd/theater help report render
```

## Command Groups

| Group | Commands | Purpose |
| --- | --- | --- |
| Start Here | `init`, `validate`, `check`, `run` | Create, validate, and run stages |
| Authoring | `fmt`, `lower`, `migrate` | Format `.thtr`, inspect canonical YAML, and convert YAML to `.thtr` |
| Discover | `explain`, `doctor`, `list`, `report` | Inspect capabilities, environment readiness, repo-aware resources, and saved run reports |
| Plugins | `plugins digest`, `plugins inspect`, `plugins lock`, `plugins doctor` | Inspect and lock plugin registries |
| Environment | `help`, `version`, `completion` | Inspect CLI contracts and shell integration |

## Command Lookup

| Command | Usage | Stable notes |
| --- | --- | --- |
| `theater init` | `theater init [theater/flows/.../starter.{yaml|thtr}] [--syntax yaml|thtr]` | Creates one starter stage under `theater/flows/` and prepares `theater/lib/`; never overwrites an existing target |
| `theater validate` | `theater validate <stage.{yaml|yml|thtr}> [--format text|json] [--plugins-readiness runtime\|descriptor] [--debug-paths]` | Alias: `check`; validates without live execution |
| `theater run` | `theater run <stage.{yaml|yml|thtr}> [--format text|json|junit] [--json-output <path>] [--junit-output <path>] [--markdown-output <path>] [--overwrite] [--live auto|off] [--debug off|dump|interactive]` | Validates first, executes once, renders stdout, and can write explicit sidecar artifacts |
| `theater fmt` | `theater fmt <path.thtr> [--write] [--check] [--diff]` | Prints formatted `.thtr` by default; `--write` mutates the file |
| `theater lower` | `theater lower <path.thtr> [--map <path.json>]` | Writes canonical YAML to stdout; `--map` writes a source-map sidecar |
| `theater migrate from-yaml` | `theater migrate from-yaml --file <stage.yaml\|stage.yml> [--plugins-config <path.json> --plugins-lock <path.lock.json>]` | Emits formatter-clean `.thtr` to stdout |
| `theater explain` | `theater explain [family\|topic\|query] [ref]` | Lists families/topics, inspects `family ref`, or lists non-topic query matches |
| `theater doctor` | `theater doctor [--plugins-config <path> --plugins-lock <path>] [--write-path <path>...]` | Checks common local workflow preconditions |
| `theater list scenarios` | `theater list scenarios [--root <path>] [--format text\|json] [--syntax all\|yaml\|thtr] [--call-skeleton]` | Lists reusable public scenario ids from `theater/lib` |
| `theater report render` | `theater report render --input <run.json> [--format junit\|markdown]` | Converts `run --format json` output into compact JUnit or detailed Markdown without rerunning the stage |
| `theater plugins inspect` | `theater plugins inspect --plugins-config <path> [--plugins-lock <path>] [--format text\|json]` | Alias: `ls`; resolves the plugin set |
| `theater plugins digest` | `theater plugins digest --manifest <path> [--write]` | Prints or updates a plugin manifest descriptor digest |
| `theater plugins lock` | `theater plugins lock --plugins-config <path> --plugins-lock <path>` | Writes plugin checksum lock data |
| `theater plugins doctor` | `theater plugins doctor --plugins-config <path> [--plugins-lock <path>] [--plugins-readiness runtime\|descriptor]` | Diagnoses plugin registry readiness and optional lock drift |
| `theater version` | `theater version` | Prints one line: `theater <version>` |
| `theater completion` | `theater completion <bash\|zsh\|fish\|powershell>` | Generates shell completion scripts |

## Shared Resolution

Command flags override environment variables. Environment variables override
built-in defaults. The CLI does not read a mutable config file.

| Variable | Meaning |
| --- | --- |
| `THEATER_PLUGINS_CONFIG` | Default plugin registry file when `--plugins-config` is omitted |
| `THEATER_PLUGINS_LOCK` | Default plugin lock file when `--plugins-lock` is omitted |
| `THEATER_COLOR` | Color policy for human-oriented text output: `auto`, `always`, or `never` |
| `NO_COLOR` | Disable ANSI styling when `THEATER_COLOR` is unset |
| `CLICOLOR` | Set to `0` to disable ANSI styling when `THEATER_COLOR` is unset |
| `CLICOLOR_FORCE` | Set to a non-zero value to force ANSI styling when `THEATER_COLOR` is unset |

For task recipes, use [Validate And Run A Flow](../../how-to/validate-and-run-a-flow.md),
[Format Theater DSL](../../how-to/format-theater-dsl.md), and
[Inspect Plugin Registry](../../how-to/inspect-plugin-registry.md).
