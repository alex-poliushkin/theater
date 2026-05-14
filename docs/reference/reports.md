# Reports

The run report is the serializable record of one stage run. Text, JUnit, and
Markdown output are renderings; `run --format json` exposes the public run
document.

Source of truth:

- `report/run_document.go`
- `report/report.go`
- `report/outcome.go`
- `report/observations.go`
- `internal/theatercli/renderers.go`
- `internal/theatercli/report_command.go`
- [Output Formats](outputs/index.md)

## Checked Report Output

<!-- theater-doc: command id=reference-report-json cwd=../.. expect-stdout="\"schema_version\": \"v1alpha1\"" expect-stdout-2="\"report\"" expect-stdout-3="\"summary\"" -->
```sh
go run ./cmd/theater run docs/examples/first-stage/stage.thtr --live off --format json
```

<!-- theater-doc: command id=reference-report-text cwd=../.. expect-stdout="docs/examples/first-stage/stage.thtr: passed" expect-stdout-2="passed=1 failed=0" -->
```sh
go run ./cmd/theater run docs/examples/first-stage/stage.thtr --live off
```

<!-- theater-doc: command id=reference-report-logs-json cwd=../.. expect-stdout="\"logs\":" expect-stdout-2="\"id\": \"response\"" expect-stdout-3="\"id\": \"audit\"" expect-stdout-4="\"log_summary\"" expect-stdout-5="\"records\": 2" expect-stdout-6="\"preview_limit_bytes\": 4096" expect-stdout-7="\"per_act_limit\": 32" expect-stdout-8="\"per_run_limit\": 1024" -->
```sh
go run ./cmd/theater run docs/examples/reference/logs.thtr --live off --format json
```

<!-- theater-doc: command id=reference-report-render-markdown cwd=../.. expect-stdout="# Theater Run Report" expect-stdout-2="- Status: `passed`" expect-stdout-3="- Expectation `status` passed" -->
```sh
go run ./cmd/theater report render --input docs/examples/reference/saved-run.json --format markdown
```

## Run Document

`run --format json` wraps the run document with the file path:

| CLI field | Meaning |
| --- | --- |
| `file` | Stage file path passed to the command |
| `result` | Public run document |

`theater report render --input <run.json>` reads this same wrapper. The render
command exits `0` when artifact generation succeeds, even when the saved run
document describes a failed Theater run. Failed outcomes are represented inside
the rendered JUnit or Markdown artifact.

The public run document has:

| Field | Meaning |
| --- | --- |
| `schema_version` | Current value: `v1alpha1` |
| `diagnostics` | Optional diagnostics emitted before or during run preparation |
| `report` | Final materialized report |

## Report Fields

| Field | Meaning |
| --- | --- |
| `stage_id` | Stage id |
| `stage_path` | Stable stage runtime path |
| `status` | Final stage status |
| `failure` | Stage-level terminal failure when present |
| `started_at`, `ended_at`, `duration_ms` | Timing metadata |
| `generation` | Deterministic generator seed and base time |
| `nodes` | Terminal node snapshots for scenarios, acts, actions, and expectations |
| `logs` | Scenario-authored log records emitted by acts |
| `log_summary` | Scenario-authored log limit settings and accounting |
| `failures` | Index of failed nodes |
| `summary` | Scenario outcome counts |

## Log Fields

`logs` contains bounded, projected scenario-authored records. Logs are
observations: they are not `nodes`, do not affect scenario counts, and never
carry unbounded inline raw payloads. A retained `capture: summary` preview may
include selected plaintext unless `capture: omit`, `sensitivity: secret`, or a
secret-wrapped value suppresses or redacts it. JSON run output includes retained
log records for passing and failing runs. Text run output keeps stdout focused
on the selected command output; live-enabled text runs mirror bounded log
preview lines to stderr.

| Field | Meaning |
| --- | --- |
| `id` | Act-local log id |
| `path` | Stable runtime path for the log record |
| `stage_id`, `scenario_id`, `scenario_call_id`, `scenario_path`, `act_id` | Runtime identity |
| `attempt`, `scenario_seq` | Retry and scenario sequence identity |
| `status` | `emitted`, `omitted`, or `error` |
| `format` | Requested author-facing format, when set |
| `source_span` | Source file, line, and column when available |
| `address` | Structured address with `kind: log` and phase `log.evaluate` |
| `preview`, `payload` | Bounded preview and payload metadata |
| `failure` | Log evaluation failure when `status` is `error` |
| `truncated` | Retained preview was shortened to the report limit |

Log report limits are fixed defaults in this release:

| Limit | Value |
| --- | --- |
| Per-log preview | `4096` bytes |
| Per-act retained records | `32` |
| Per-run retained records | `1024` |

## Log Summary Fields

`log_summary` is present when a report has retained or dropped log records.
Dropped records are counted in `log_summary.dropped_records`; `report.logs`
contains retained records only. Runtime event streams may contain compact dropped
log events so replay can reproduce the same dropped count without carrying a
payload preview. Zero counters are omitted from JSON.

| Field | Meaning |
| --- | --- |
| `records` | Retained log record count |
| `dropped_records` | Log records omitted from `logs` because a limit was exceeded |
| `truncated_records` | Retained log records with truncated previews |
| `preview_limit_bytes` | Effective per-log preview limit |
| `per_act_limit` | Effective per-act retained record limit |
| `per_run_limit` | Effective per-run retained record limit |

## Node Fields

| Field | Meaning |
| --- | --- |
| `kind` | `scenario`, `act`, `action`, or `expectation` |
| `path` | Stable runtime path |
| `scenario_id`, `scenario_call_id`, `scenario_path` | Scenario identity |
| `attempt`, `scenario_seq` | Retry and scenario sequence identity |
| `status` | Terminal node status |
| `skip_reason` | Why a skipped node was skipped |
| `failure` | Node failure when present |
| `address` | Structured node address for tools |
| `source_span` | Source file, line, and column when available |
| `preview`, `artifacts`, `contrast`, `observations`, `payload` | Report-safe observed data |
| `eventually` | Retry summary for an act with `eventually` |

## Failure Fields

Failures carry the user-facing classification and message fields used by text,
JUnit, JSON, and Markdown renderers.

| Field | Meaning |
| --- | --- |
| `kind` | Failure category |
| `phase` | Compile, validate, or run phase |
| `at` | Runtime path where the failure was reported |
| `summary` | Stable short failure summary |

## Enum Values

| Enum | Values |
| --- | --- |
| `status` | `pending`, `running`, `passed`, `failed`, `canceled`, `skipped` |
| `failure.kind` | `definition`, `setup`, `observation`, `action`, `expectation`, `timeout`, `internal` |
| `failure.phase` | `compile`, `validate`, `run` |
| `node.kind` | `scenario`, `act`, `action`, `expectation` |
| `address.kind` | `scenario`, `act`, `action`, `expectation`, `log` |
| `log.status` | `emitted`, `omitted`, `error` |
| `skip_reason` | `explicit`, `stage_aborted` |
| `eventually.termination_reason` | `converged`, `deadline_exceeded`, `terminal_failure`, `parent_cancelled` |
| `sensitivity` | `none`, `internal`, `personal`, `secret` |
| `capture` | `omit`, `summary`, `artifact_ref` |

For a reader path through reports, use
[Validate, Run, Report](../concepts/validate-run-report.md). For procedures,
use [Read Run Output](../how-to/read-run-output.md) and
[Validate And Run A Flow](../how-to/validate-and-run-a-flow.md). For machine
output formats and `report render`, use [Output Formats](outputs/index.md).
