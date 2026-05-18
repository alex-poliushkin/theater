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
- `internal/theatercli/report_summary_markdown_renderer.go`
- [Output Formats](outputs/index.md)

## Checked Report Output

<!-- theater-doc: command id=reference-report-json cwd=../.. expect-stdout="\"report_schema_version\": \"v1alpha1\"" expect-stdout-2="\"theater_version\"" expect-stdout-3="\"run_id\"" expect-stdout-4="\"report\"" expect-stdout-5="\"summary\"" -->
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

<!-- theater-doc: command id=reference-report-render-summary cwd=../.. expect-stdout="# Theater Run Summary" expect-stdout-2="- Status: `passed`" expect-stdout-3="- Run: `" reject-stdout="### Scenario" reject-stdout-2="log response" reject-stdout-3="HTTP body:" -->
```sh
go run ./cmd/theater report render --input docs/examples/reference/saved-run.json --format summary-md
```

<!-- theater-doc: command id=reference-report-http-diagnostics-markdown cwd=../.. expect-stdout="HTTP request" expect-stdout-2="api.example.test/redacted?token=redacted" expect-stdout-3="HTTP body:" expect-stdout-4="retry later" reject-stdout=credential-secret -->
```sh
go run ./cmd/theater report render --input docs/examples/reference/failed-http-run.json --format markdown
```

<!-- theater-doc: command id=reference-report-http-diagnostics-junit cwd=../.. expect-stdout=http.request expect-stdout-2="retry later" expect-stdout-3="[redacted]" reject-stdout=credential-secret -->
```sh
go run ./cmd/theater report render --input docs/examples/reference/failed-http-run.json --format junit
```

## Run Document

`run --format json` wraps the run document with the file path:

| CLI field | Meaning |
| --- | --- |
| `file` | Stage file path passed to the command |
| `result` | Public run document |

`theater run` can also write the same run document and derived CI artifacts to
sidecar files with `--json-output`, `--junit-output`, `--markdown-output`, and
`--summary-output`. Sidecars are rendered from the same in-memory run document
as stdout output and do not execute the stage again.

`theater report render --input <run.json>` reads this same wrapper. The render
command exits `0` when artifact generation succeeds, even when the saved run
document describes a failed Theater run. Failed outcomes are represented inside
the rendered JUnit, Markdown, or summary artifact.

## Run Sidecar Outputs

| Flag | Artifact |
| --- | --- |
| `--json-output <path>` | JSON wrapper with `file` and `result`, matching `run --format json` stdout |
| `--junit-output <path>` | JUnit XML rendered from the run document |
| `--markdown-output <path>` | Markdown report rendered from the run document |
| `--summary-output <path>` | Compact Markdown summary rendered from the run document |
| `--overwrite` | Replace existing sidecar files; without this flag existing files are rejected |

Sidecar paths must be explicit file paths. Theater rejects `-`, parent
traversal, missing or non-directory parents, directories, symlinks, symlinked
parent directories, non-regular existing files, and duplicate sidecar paths. New
files are created with owner read/write permissions.

Sidecars are written after execution and before stdout rendering. When sidecar
rendering or writing fails, Theater exits with command failure status and prints
a concise stderr diagnostic without printing report contents.

The public run document has:

| Field | Meaning |
| --- | --- |
| `report_schema_version` | Current run document and report schema version. Current value: `v1alpha1` |
| `theater_version` | Theater build or release version that produced the document |
| `run_id` | Opaque identifier for this materialized run document |
| `diagnostics` | Optional diagnostics emitted before or during run preparation |
| `report` | Final materialized report |

Report JSON is the canonical machine-readable surface. Markdown, JUnit, and
summary outputs are projections over the run document and must not invent their
own runtime truth. The v0.5.0 identity contract is intentionally small: it does
not promise `theater report diff`, cross-run trend identifiers, artifact index
metadata, hosted dashboard upload fields, telemetry upload, or broad backward
compatibility beyond the documented current schema.

`run_id` is stable inside one emitted run document and its derived artifacts. It
is not a cross-run comparison key. `theater_version` is `dev` for local builds
unless a release build embeds a tag. Node `id` values are stable inside the run
document and are suitable for CI projections. They are report identities, not
authoring references.

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
| `id` | Stable node identity inside the current run document |
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
| `diagnostics` | Node-level collection of typed report-safe diagnostics |

## Summary Projection Contract

The compact summary projection is available through `report render --format
summary-md` and `run --summary-output`. It is separate from the detailed
Markdown report.
The existing `markdown` renderer may show scenario calls, acts, expectations,
logs, diagnostics, and report-safe observed values. The compact summary
projection is for CI job summaries and must stay short.

A compact summary may show:

- run status and scenario counts
- failed scenario calls and failed nodes
- source locations when the run document has them
- bounded failure summaries from report-safe data
- run identity fields needed to correlate the summary with its JSON document

A compact summary must not show raw scenario-authored logs, raw HTTP request or
response bodies, secrets, unbounded payloads, or renderer-only diagnostics.

## HTTP Failure Diagnostics Contract

The v0.4 report contract emits node-scoped HTTP diagnostics for failed
`action.http` exchanges and for expectations that fail after inspecting a
completed HTTP response. Run documents without HTTP diagnostic evidence simply
omit the diagnostic fields.

HTTP diagnostics are report data. They are not top-level
`RunDocument.Diagnostics`, not renderer-only text, and not `action.http`
outputs. Scenario authors cannot read them with `field(...)`, selectors, or
exports.

When emitted, an HTTP diagnostic is attached to the failed node that owns the
runtime failure. Transport and request-assembly failures attach to the failed
action node and carry request metadata only. Expectation failures after a
completed HTTP response attach to the failed expectation node and include the
address of the HTTP action that produced the exchange. Parent act, scenario,
text, Markdown, and JUnit views may summarize that diagnostic, but they are not
the canonical storage owner.

The typed diagnostic records:

| Field | Meaning |
| --- | --- |
| kind | HTTP diagnostic kind |
| action address | Node address or path for the HTTP action that produced the exchange |
| method | Request method |
| URL | Resolved URL with userinfo hidden, path segment values redacted by default, fragments omitted, and query values redacted |
| status | Response status code and status text when a response exists |
| duration | Measured exchange duration |
| response headers | Allowlisted response headers such as content type and correlation ids |
| response preview | Bounded `Preview` for classified and redacted textual bodies, or metadata-only preview for binary, unknown, or unclassified textual bodies |

Response header projection is allowlist-based. Authorization, proxy
authorization, cookie, and set-cookie values are excluded by default, and
unknown headers are omitted rather than guessed safe. Credential-like values are
excluded even when they appear under an allowlisted header name.

Body previews use the same `Preview` semantics as the rest of the report:
content type, size hint, truncation, redacted state, omitted state, and bounded
text. Content type or valid UTF-8 alone is not enough to expose response text;
the body must pass pre-preview classification and redaction first. Unclassified
textual bodies use metadata-only or omitted previews by default. These preview
bounds are a reporting and privacy contract. They do not promise a transport
read limit unless a future implementation explicitly adds bounded response
reads.

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
