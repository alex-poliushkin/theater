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

The checked commands above assert that the JSON contains the `check-values`
stage and a `passed` status.

When CI needs JSON, JUnit, and Markdown artifacts from one execution, write
sidecar outputs from `theater run`:

<!-- theater-doc: command id=howto-read-output-sidecars cwd=../.. expect-stdout=passed expect-stdout-2="passed=1 failed=0" -->
```sh
go run ./cmd/theater run docs/examples/check-values/profile.thtr --live off --format text --json-output /tmp/theater-profile.run.json --junit-output /tmp/theater-profile.junit.xml --markdown-output /tmp/theater-profile.md --overwrite
```

For a repository-local CI job, create the output directory first and use paths
owned by the job workspace:

```
mkdir -p build
theater run docs/examples/check-values/profile.thtr --live off --format text --json-output build/profile.run.json --junit-output build/profile.junit.xml --markdown-output build/profile.md
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

Use [Selectors](../reference/selectors.md) and
[Expectations](../reference/expectations.md) when the failure is about a checked
value. Use [Validate, Run, Report](../concepts/validate-run-report.md) for the
mental model behind the report.
