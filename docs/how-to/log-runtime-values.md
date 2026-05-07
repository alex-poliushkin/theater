# Log Runtime Values

Use scenario-authored logs when a run should keep selected runtime values for
live inspection and JSON reports without changing assertions, exports, or
stdout output.

Start from an act that already has an action. In Theater DSL, add `log` entries
after `do` and before `expect` or `export`:

```
log response = object {
  status: field(values) | path("/status_code"),
  correlation_id: field(values) | path("/correlation_id")
}
```

Run the checked Theater DSL example as JSON:

<!-- theater-doc: command id=howto-log-runtime-values-thtr-json cwd=../.. expect-stdout="\"logs\":" expect-stdout-2="\"id\": \"response\"" expect-stdout-3="\"id\": \"audit\"" expect-stdout-4="\"log_summary\"" expect-stdout-5="\"records\": 2" expect-stdout-6="\"preview_limit_bytes\": 4096" expect-stdout-7="\"per_act_limit\": 32" expect-stdout-8="\"per_run_limit\": 1024" -->
```sh
go run ./cmd/theater run docs/examples/reference/logs.thtr --format json --live off
```

Run the YAML form when you need explicit `capture`, `sensitivity`, `format`, or
`required` fields:

<!-- theater-doc: command id=howto-log-runtime-values-yaml-json cwd=../.. expect-stdout="\"logs\":" expect-stdout-2="\"id\": \"response\"" expect-stdout-3="\"id\": \"audit\"" expect-stdout-4="\"capture\": \"summary\"" -->
```sh
go run ./cmd/theater run docs/examples/reference/logs.yaml --format json --live off
```

Check the human text summary stays quiet with live output disabled:

<!-- theater-doc: command id=howto-log-runtime-values-text-quiet cwd=../.. expect-stdout=passed reject-stdout="log response" reject-stdout-2="log audit" reject-stderr="log response" reject-stderr-2="log audit" -->
```sh
go run ./cmd/theater run docs/examples/reference/logs.thtr --live off
```

Enable live output when you want the same bounded log preview on stderr
during the run:

<!-- theater-doc: command id=howto-log-runtime-values-live-stderr cwd=../.. expect-stdout=passed expect-stderr="log response" expect-stderr-2="log audit" reject-stdout="log response" reject-stdout-2="log audit" -->
```sh
go run ./cmd/theater run docs/examples/reference/logs.thtr --live auto
```

Read retained records from `result.report.logs`. Each record carries the log id,
runtime path, scenario call, act, attempt, status, source span, bounded preview,
payload metadata, and address. `capture: summary` may include selected plaintext
in JSON reports and live stderr; use YAML `capture: omit` to suppress previews
and `sensitivity: secret` for secret values. Limit accounting lives in
`result.report.log_summary`.

Use [Scenario Logs](../reference/logs.md) for the full authoring and runtime
contract, [Theater DSL](../reference/theater-dsl/index.md) for compact syntax,
[Reports](../reference/reports.md) for JSON fields, and
[Output formats](../reference/outputs/index.md) for stdout and stderr behavior.
