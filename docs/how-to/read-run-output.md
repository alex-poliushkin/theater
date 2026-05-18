# Read Run Output

Use JSON output when another tool needs the run result or when the text summary
is not enough.

Run the Theater DSL example as JSON:

<!-- theater-doc: command id=howto-read-output-thtr cwd=../.. expect-stdout=check-values expect-stdout-2=passed -->
```sh
go run ./cmd/theater run docs/examples/check-values/profile.thtr --format json --live off
```

Run the YAML example as JSON:

<!-- theater-doc: command id=howto-read-output-yaml cwd=../.. expect-stdout=check-values expect-stdout-2=passed -->
```sh
go run ./cmd/theater run docs/examples/check-values/profile.yaml --format json --live off
```

Look for `result.report.status` first. When it is `passed`, all selected scenario
calls and expectations passed. When it is not `passed`, inspect `result.report`
nodes and the failure fields near the failed act or expectation.

Use `result.report_schema_version`, `result.theater_version`, and
`result.run_id` to identify the run document that produced derived artifacts
such as JUnit, Markdown, or CI summaries. These fields describe the emitted run
document; they are not inputs for selecting scenarios or comparing historical
runs.

The checked commands above assert that the JSON contains the `check-values`
stage and a `passed` status.

When CI needs JSON, JUnit, detailed Markdown, and compact summary artifacts
from one execution, write sidecar outputs from `theater run`:

<!-- theater-doc: command id=howto-read-output-sidecars cwd=../.. expect-stdout=passed expect-stdout-2="passed=1 failed=0" -->
```sh
go run ./cmd/theater run docs/examples/check-values/profile.thtr --live off --format text --json-output /tmp/theater-profile.run.json --junit-output /tmp/theater-profile.junit.xml --markdown-output /tmp/theater-profile.md --summary-output /tmp/theater-profile.summary.md --overwrite
```

For a repository-local CI job, create the output directory first and use paths
owned by the job workspace. `--markdown-output` writes the detailed report
artifact; `--summary-output` writes the bounded job summary:

```
mkdir -p build
theater run docs/examples/check-values/profile.thtr --live off --format text --json-output build/profile.run.json --junit-output build/profile.junit.xml --markdown-output build/profile.md --summary-output build/profile.summary.md
```

For GitHub Actions, preserve the Theater exit code while still appending the
compact summary for failed runs:

```
mkdir -p build
set +e
theater run docs/examples/check-values/profile.thtr --live off --format text --json-output build/profile.run.json --junit-output build/profile.junit.xml --markdown-output build/profile.md --summary-output build/profile.summary.md
status=$?
if [ -f build/profile.summary.md ]; then
  cat build/profile.summary.md >> "$GITHUB_STEP_SUMMARY"
fi
exit "$status"
```

Sidecar paths are always explicit. Theater does not derive filenames from the
stage, scenario, source path, or report data. Existing sidecar files are rejected
unless `--overwrite` is present, and `-` is not accepted for sidecar flags.

Exit precedence for `theater run` with sidecars:

| Outcome | Artifacts | Exit code |
| --- | --- | --- |
| Run passes and all requested outputs are written | Requested sidecars plus stdout renderer | `0` |
| Run fails or is canceled and all requested outputs are written | Requested sidecars plus stdout renderer | `1` |
| Authoring diagnostics produce a failed run document and sidecars are written | Requested sidecars plus stdout renderer | `1` |
| Sidecar render or write fails after a run document exists | No stdout rendering after the sidecar failure | `2` |
| Sidecar path preflight fails before execution | No run, no sidecars | `2` |

`report render` remains available when a saved run JSON already exists. It is a
converter and exits successfully when the artifact is written, even if the saved
run document describes a failed Theater run.

Use `report render --format summary-md` when a CI step already has saved run
JSON and only needs the compact Markdown summary:

```
theater report render --input build/profile.run.json --format summary-md > build/profile.summary.md
```

Use [Selectors](../reference/selectors.md) and
[Expectations](../reference/expectations.md) when the failure is about a checked
value. Use [Validate, Run, Report](../concepts/validate-run-report.md) for the
mental model behind the report.
