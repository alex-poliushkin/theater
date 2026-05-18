# Output Formats

This page records stdout shapes for CLI commands that return user-facing or
machine-readable output.

Source of truth:

- `go run ./cmd/theater help formats`
- `internal/theatercli/renderers.go`
- `internal/theatercli/report_command.go`
- `internal/theatercli/report_markdown_renderer.go`
- `internal/theatercli/report_summary_markdown_renderer.go`
- `internal/theatercli/debug_path_renderer.go`
- `internal/theatercli/list_command.go`
- [Reports](../reports.md)

## Checked Output Commands

<!-- theater-doc: command id=reference-output-formats cwd=../../.. expect-stdout="Output formats:" expect-stdout-2="json  machine-readable stdout" expect-stdout-3="markdown  detailed human-readable CI report" expect-stdout-4="summary-md  compact Markdown summary" -->
```sh
go run ./cmd/theater explain formats
```

<!-- theater-doc: command id=reference-output-validate-json cwd=../../.. expect-stdout="\"valid\": true" expect-stdout-2="\"diagnostics\": null" -->
```sh
go run ./cmd/theater validate docs/examples/first-stage/stage.thtr --format json
```

<!-- theater-doc: command id=reference-output-run-text cwd=../../.. expect-stdout=passed expect-stdout-2="passed=1 failed=0" -->
```sh
go run ./cmd/theater run docs/examples/first-stage/stage.thtr --live off
```

<!-- theater-doc: command id=reference-output-run-json cwd=../../.. expect-stdout="\"report_schema_version\": \"v1alpha1\"" expect-stdout-2="\"status\": \"passed\"" expect-stdout-3="\"nodes\"" -->
```sh
go run ./cmd/theater run docs/examples/first-stage/stage.thtr --live off --format json
```

<!-- theater-doc: command id=reference-output-run-junit cwd=../../.. expect-stdout="<testsuites>" expect-stdout-2="<testsuite name=\"docs-first\"" expect-stdout-3="<testcase classname=\"hello\" name=\"run\"" -->
```sh
go run ./cmd/theater run docs/examples/first-stage/stage.thtr --live off --format junit
```

<!-- theater-doc: command id=reference-output-report-markdown cwd=../../.. expect-stdout="# Theater Run Report" expect-stdout-2="### Scenario `run`" expect-stdout-3="- Act `check` passed" -->
```sh
go run ./cmd/theater report render --input docs/examples/reference/saved-run.json --format markdown
```

<!-- theater-doc: command id=reference-output-report-summary cwd=../../.. expect-stdout="# Theater Run Summary" expect-stdout-2="- Status: `passed`" expect-stdout-3="- Run: `" reject-stdout="### Scenario" reject-stdout-2="log response" reject-stdout-3="HTTP body:" -->
```sh
go run ./cmd/theater report render --input docs/examples/reference/saved-run.json --format summary-md
```

<!-- theater-doc: command id=reference-output-live-log-stderr cwd=../../.. expect-stdout=passed expect-stderr="log response" expect-stderr-2="log audit" reject-stdout="log response" reject-stdout-2="log audit" -->
```sh
go run ./cmd/theater run docs/examples/reference/logs.thtr --live auto
```

## Format Matrix

| Format | Commands | Contract |
| --- | --- | --- |
| `text` | `validate`, `run`, `validate --debug-paths`, `list scenarios`, `plugins inspect` | Human-readable stdout; text output may use ANSI styling when color policy allows it |
| `json` | `validate`, `run`, `validate --debug-paths`, `list scenarios`, `plugins inspect` | Machine-readable stdout; no ANSI styling |
| `junit` | `run`, `report render` | Compact scenario-call JUnit XML stdout for CI test-report ingestion |
| `markdown` | `report render`, `run` sidecars | Detailed human-readable run report for CI artifacts |
| `summary-md` | `report render`, `run` sidecars | Compact Markdown run summary for CI job summaries |

Live progress, scenario-authored live log lines, debug prompts, and interactive
pause cards use stderr so redirected stdout remains safe for JSON, JUnit, or
text summary capture. Passing text summaries do not print all scenario-authored
report logs by default; use `run --format json` to read retained log records
from `result.report.logs`. Use `run --json-output`, `--junit-output`,
`--markdown-output`, and `--summary-output` when one execution should produce
multiple artifacts. Use `report render` when a saved run JSON file should
become compact JUnit, detailed Markdown, or compact summary Markdown without
executing the stage again.

The compact summary Markdown projection is a separate v0.5.0 report projection
over the same run document. It is bounded for CI job summaries and must not
include raw logs, raw HTTP bodies, secrets, unbounded payloads, or
renderer-only diagnostics.

## Run Sidecar Outputs

`theater run` keeps `--format` as the stdout renderer. Sidecar flags add file
outputs from the same run document:

| Flag | File output |
| --- | --- |
| `--json-output <path>` | JSON run wrapper |
| `--junit-output <path>` | JUnit XML |
| `--markdown-output <path>` | Markdown report |
| `--summary-output <path>` | Compact Markdown summary |
| `--overwrite` | Allow replacing existing sidecar files |

Sidecar flags do not accept `-`. Existing files require `--overwrite`, and
unsafe targets such as directories, symlinks, non-regular files, duplicate
paths, missing parents, symlinked parent directories, and parent traversal are
rejected.

<!-- theater-doc: command id=reference-output-run-sidecars cwd=../../.. expect-stdout=passed expect-stdout-2="passed=1 failed=0" -->
```sh
go run ./cmd/theater run docs/examples/first-stage/stage.thtr --live off --format text --json-output /tmp/theater-first-stage.run.json --junit-output /tmp/theater-first-stage.junit.xml --markdown-output /tmp/theater-first-stage.md --summary-output /tmp/theater-first-stage.summary.md --overwrite
```

Exit precedence:

| Result | Exit code |
| --- | --- |
| Run passed and all requested outputs were written | `0` |
| Run failed or was canceled and all requested outputs were written | `1` |
| Sidecar path preflight, render, or write failed | `2` |

## JSON Wrappers

| Command | Top-level JSON shape |
| --- | --- |
| `validate --format json` | `{ "file": "...", "valid": true|false, "diagnostics": [...]|null }` |
| `run --format json` | `{ "file": "...", "result": <run document> }` |
| `validate --debug-paths --format json` | `{ "file": "...", "paths": [...] }` |
| `list scenarios --format json` | `{ "repo_root": "...", "library_root": "...", "scenarios": [...] }` |
| `libraries inspect --format json` | `{ "file": "...", "repo_root": ".", "selected_library_files": [...], "scenario_call_edges": [...] }` |
| `plugins inspect --format json` | `{ "config_path": "...", "lock_path": "...", "plugins": [...] }` |

`libraries inspect --format json` reports repo-relative paths. Its `repo_root`
field is `.` by design so the inspection report can be shared without exposing
the local workspace path.

Open [Reports](../reports.md) for the run document inside `run --format json`.
Open [CLI Reference](../cli/index.md) for command flags.
For procedures, use [Validate And Run A Flow](../../how-to/validate-and-run-a-flow.md)
and [Read Run Output](../../how-to/read-run-output.md).
