package theatercli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/internal/testkit"
)

var chdirTestMu sync.Mutex

func TestMain(m *testing.M) {
	for _, name := range []string{envTheaterColor, envNoColor, envCLIColor, envCLIColorForce} {
		_ = os.Unsetenv(name)
	}
	os.Exit(m.Run())
}

func TestRunValidateText(t *testing.T) {
	t.Parallel()

	path := writeStageYAML(t, `
id: main
scenarios:
  - id: probe
    acts:
      - id: request
        properties:
          url:
            inventory:
              use: inventory.env
              with:
                name: THEATER_STAGE_URL
        action:
          use: action.http
          with:
            url:
              kind: ref
              ref: url
scenario_calls:
  - id: probe-server
    scenario_id: probe
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"validate", "-file", path}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}

	if got, want := strings.TrimSpace(stdout.String()), path+": valid"; got != want {
		t.Fatalf("stdout mismatch: got %q want %q", got, want)
	}
}

func TestRunValidateJSONWithDiagnostics(t *testing.T) {
	t.Parallel()

	path := writeStageYAML(t, `
id: main
scenarios:
  - id: probe
    acts:
      - id: request
        properties:
          url:
            inventory:
              use: inventory.env
              with:
                name: THEATER_STAGE_URL
        action:
          use: action.http
          with:
            url:
              kind: ref
              ref: url
        transitions:
          - on: on_pass
            to: missing
scenario_calls:
  - id: probe-server
    scenario_id: probe
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"validate", "-file", path, "-format", "json"}, &stdout, &stderr)
	if got, want := code, 1; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}

	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}

	if !strings.Contains(stdout.String(), `"valid": false`) {
		t.Fatalf("json output must mark invalid spec: %q", stdout.String())
	}

	if !strings.Contains(stdout.String(), `"code": "missing_transition_target"`) {
		t.Fatalf("json output must include diagnostic code: %q", stdout.String())
	}
}

func TestRunValidateYAMLLiteralWrapperHintsAreNonFatalInText(t *testing.T) {
	t.Parallel()

	path := writeStageYAML(t, `
id: main
scenarios:
  - id: probe
    acts:
      - id: request
        action:
          use: action.http
          with:
            method:
              kind: literal
              value: GET
            url: /health
scenario_calls:
  - id: probe-server
    scenario_id: probe
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"validate", "-file", path}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}

	output := normalizeRunOutput(stdout.String(), map[string]string{
		path: "<stage-file>",
	})
	if !strings.Contains(output, "<stage-file>: valid with 1 hint(s)") {
		t.Fatalf("output must mark hint-only validation as valid: %q", output)
	}
	if !strings.Contains(output, "[redundant_literal_wrapper]") {
		t.Fatalf("output must include literal wrapper hint code: %q", output)
	}
	if !strings.Contains(output, "source: <stage-file>:11:15") {
		t.Fatalf("output must include literal wrapper source span: %q", output)
	}
}

func TestRunValidateYAMLLiteralWrapperHintsAreNonFatalInJSON(t *testing.T) {
	t.Parallel()

	path := writeStageYAML(t, `
id: main
scenarios:
  - id: probe
    acts:
      - id: request
        action:
          use: action.http
          with:
            method:
              kind: literal
              value: GET
            url: /health
scenario_calls:
  - id: probe-server
    scenario_id: probe
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"validate", "-file", path, "-format", "json"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}

	var response struct {
		File        string               `json:"file"`
		Valid       bool                 `json:"valid"`
		Diagnostics []theater.Diagnostic `json:"diagnostics"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &response); err != nil {
		t.Fatalf("decode validate json failed: %v\n%s", err, stdout.String())
	}
	if !response.Valid {
		t.Fatalf("hint-only response must stay valid: %#v", response)
	}
	if got, want := len(response.Diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d: %#v", got, want, response.Diagnostics)
	}
	diagnostic := response.Diagnostics[0]
	if got, want := diagnostic.Code, "redundant_literal_wrapper"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Severity, theater.SeverityHint; got != want {
		t.Fatalf("diagnostic severity mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Path, "stage.main/scenario.probe/act.request/action/binding.method"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Span.File, path; got != want {
		t.Fatalf("diagnostic source file mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Span.Line, 11; got != want {
		t.Fatalf("diagnostic source line mismatch: got %d want %d", got, want)
	}
}

func TestRunValidateReportsDateGeneratorDiagnosticsWithArgumentSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		fileName   string
		source     string
		wantPath   string
		wantError  string
		wantSource string
	}{
		{
			name:     "thtr unsupported format",
			fileName: "stage.thtr",
			source: `stage invalid-date
scenario inspect
  act generate-values
    do action.generate
      outputs:
        start_date: generate.date(format: "rfc3339")
call run = inspect()
`,
			wantPath:   "binding.start_date/binding.format",
			wantError:  `format "rfc3339" is not supported`,
			wantSource: "source: <stage-file>:6:35",
		},
		{
			name:     "thtr invalid offset",
			fileName: "stage.thtr",
			source: `stage invalid-date
scenario inspect
  act generate-values
    do action.generate
      outputs:
        start_date: generate.date(offset: "soon")
call run = inspect()
`,
			wantPath:   "binding.start_date/binding.offset",
			wantError:  `offset "soon" is invalid`,
			wantSource: "source: <stage-file>:6:35",
		},
		{
			name:     "yaml unsupported format",
			fileName: "stage.yaml",
			source: `id: invalid-date
scenarios:
  - id: inspect
    acts:
      - id: generate-values
        action:
          use: action.generate
          with:
            outputs:
              kind: object
              object:
                start_date:
                  kind: generate
                  generator: date
                  format: rfc3339
scenario_calls:
  - id: run
    scenario_id: inspect
`,
			wantPath:   "binding.start_date/binding.format",
			wantError:  `format "rfc3339" is not supported`,
			wantSource: "source: <stage-file>:15:27",
		},
		{
			name:     "yaml invalid offset",
			fileName: "stage.yaml",
			source: `id: invalid-date
scenarios:
  - id: inspect
    acts:
      - id: generate-values
        action:
          use: action.generate
          with:
            outputs:
              kind: object
              object:
                start_date:
                  kind: generate
                  generator: date
                  offset: soon
scenario_calls:
  - id: run
    scenario_id: inspect
`,
			wantPath:   "binding.start_date/binding.offset",
			wantError:  `offset "soon" is invalid`,
			wantSource: "source: <stage-file>:15:27",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			path := writeStageFile(t, test.fileName, test.source)
			var stdout strings.Builder
			var stderr strings.Builder

			code := run([]string{"validate", "-file", path}, &stdout, &stderr)
			if got, want := code, 1; got != want {
				t.Fatalf("exit code mismatch: got %d want %d, stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
			}
			if got := strings.TrimSpace(stderr.String()); got != "" {
				t.Fatalf("stderr mismatch: got %q want empty", got)
			}

			output := normalizeRunOutput(stdout.String(), map[string]string{
				path: "<stage-file>",
			})
			for _, want := range []string{test.wantPath, test.wantError, test.wantSource} {
				if !strings.Contains(output, want) {
					t.Fatalf("output missing %q: %q", want, output)
				}
			}
		})
	}
}

func TestValidationExitCodeTreatsOnlyHintsAsNonFatalDiagnostics(t *testing.T) {
	t.Parallel()

	if got, want := validationExitCode([]theater.Diagnostic{{Severity: theater.SeverityHint}}), 0; got != want {
		t.Fatalf("hint exit code mismatch: got %d want %d", got, want)
	}
	if got, want := validationExitCode([]theater.Diagnostic{{Severity: theater.DiagnosticSeverity("warning")}}), 1; got != want {
		t.Fatalf("unknown severity exit code mismatch: got %d want %d", got, want)
	}
	if got, want := validationExitCode([]theater.Diagnostic{{}}), 1; got != want {
		t.Fatalf("empty severity exit code mismatch: got %d want %d", got, want)
	}
}

func TestRunValidateTHTRParseDiagnostics(t *testing.T) {
	t.Parallel()

	path := writeStageFile(t, "stage.thtr", "stage main\nscenario\n")

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"validate", "-file", path}, &stdout, &stderr)
	if got, want := code, 1; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}

	output := normalizeRunOutput(stdout.String(), map[string]string{
		path: "<stage-file>",
	})

	if !strings.Contains(output, "[thtr_parse_error]") {
		t.Fatalf("output must include parse diagnostic code: %q", output)
	}
	if !strings.Contains(output, "source: <stage-file>:2:9") {
		t.Fatalf("output must include parse diagnostic source span: %q", output)
	}
}

func TestRunValidateTHTRValidationDiagnosticsShowSourceAndBreadcrumb(t *testing.T) {
	t.Parallel()

	path := writeStageFile(t, "stage.thtr", `stage main
scenario login
  act submit
    eventually 1s every 1s
    do repeatable action.http(method: "GET", url: "/health")
    expect status: field(status_code) == 200
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"validate", "-file", path}, &stdout, &stderr)
	if got, want := code, 1; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}

	output := normalizeRunOutput(stdout.String(), map[string]string{
		path: "<stage-file>",
	})

	if !strings.Contains(output, "[invalid_eventually_interval]") {
		t.Fatalf("output must include validation diagnostic code: %q", output)
	}
	if !strings.Contains(output, "source: <stage-file>:4:5") {
		t.Fatalf("output must include validation diagnostic source span: %q", output)
	}
	if !strings.Contains(output, "breadcrumb: scenario login -> act submit -> eventually -> interval") {
		t.Fatalf("output must include validation breadcrumb: %q", output)
	}
}

func TestRunLowerTHTRWritesCanonicalYAMLAndSourceMap(t *testing.T) {
	t.Parallel()

	path := writeStageFile(t, "stage.thtr", `stage smoke

scenario ping
  act get-health
    do action.http(method: "GET", url: "/health")
`)
	mapPath := filepath.Join(t.TempDir(), "stage.map.json")

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"lower", "-file", path, "-map", mapPath}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}
	if !strings.Contains(stdout.String(), "id: smoke") {
		t.Fatalf("lower output must include canonical yaml stage id: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "scenario_calls: []") {
		t.Fatalf("lower output must include canonical yaml scenario_calls: %q", stdout.String())
	}

	mapData, err := os.ReadFile(mapPath)
	if err != nil {
		t.Fatalf("read source map failed: %v", err)
	}
	if !strings.Contains(string(mapData), `"version": "v1alpha1"`) {
		t.Fatalf("source map must include version marker: %q", string(mapData))
	}
	if !strings.Contains(string(mapData), `"spec_path": "stage.smoke"`) {
		t.Fatalf("source map must include stage spec path: %q", string(mapData))
	}
}

func TestRunLowerTHTRAcceptsPositionalPath(t *testing.T) {
	t.Parallel()

	path := writeStageFile(t, "stage.thtr", `stage smoke

scenario ping
  act get-health
    do action.http(method: "GET", url: "/health")
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"lower", path}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}
	if !strings.Contains(stdout.String(), "id: smoke") {
		t.Fatalf("lower output must include canonical yaml stage id: %q", stdout.String())
	}
}

func TestRunLowerTHTRParseDiagnostics(t *testing.T) {
	t.Parallel()

	path := writeStageFile(t, "stage.thtr", "stage main\nscenario\n")

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"lower", "-file", path}, &stdout, &stderr)
	if got, want := code, 1; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}
	if !strings.Contains(stdout.String(), "[thtr_parse_error]") {
		t.Fatalf("lower output must include parse diagnostic code: %q", stdout.String())
	}
}

func TestRunFmtTHTRWritesFormattedSourceToStdout(t *testing.T) {
	t.Parallel()

	path := writeStageFile(t, "stage.thtr", `stage smoke
scenario ping
  act get-health
    do action.http
      method: "GET"
      url: "/health"
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"fmt", "-file", path}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, `do action.http(method: "GET", url: "/health")`) {
		t.Fatalf("formatted output must normalize simple block call into inline form: %q", got)
	}
}

func TestRunFmtTHTRAcceptsPositionalPath(t *testing.T) {
	t.Parallel()

	path := writeStageFile(t, "stage.thtr", `stage smoke
scenario ping
  act get-health
    do action.http
      method: "GET"
      url: "/health"
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"fmt", path}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, `do action.http(method: "GET", url: "/health")`) {
		t.Fatalf("formatted output must normalize simple block call into inline form: %q", got)
	}
}

func TestRunFmtTHTRWriteRewritesFile(t *testing.T) {
	t.Parallel()

	path := writeStageFile(t, "stage.thtr", `stage smoke
scenario ping
  act get-health
    do action.http
      method: "GET"
      url: "/health"
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"fmt", "-file", path, "-write"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "" {
		t.Fatalf("stdout mismatch: got %q want empty", got)
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read formatted file failed: %v", err)
	}
	if got := string(data); !strings.Contains(got, `do action.http(method: "GET", url: "/health")`) {
		t.Fatalf("formatted file must normalize simple block call into inline form: %q", got)
	}
}

func TestRunFmtTHTRCheckReportsCleanFile(t *testing.T) {
	t.Parallel()

	path := writeStageFile(t, "stage.thtr", `stage smoke

scenario ping
  act get-health
    do action.http(method: "GET", url: "/health")
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"fmt", "--check", path}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "" {
		t.Fatalf("stdout mismatch: got %q want empty", got)
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}
}

func TestRunFmtTHTRCheckReportsDirtyFile(t *testing.T) {
	t.Parallel()

	source := `stage smoke
scenario ping
  act get-health
    do action.http
      method: "GET"
      url: "/health"
`
	path := writeStageFile(t, "stage.thtr", source)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"fmt", "--check", path}, &stdout, &stderr)
	if got, want := code, 1; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "" {
		t.Fatalf("stdout mismatch: got %q want empty", got)
	}
	for _, want := range []string{
		path + " is not formatted",
		"theater fmt --write",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr missing %q: %q", want, stderr.String())
		}
	}
	if got := readFileString(t, path); got != source {
		t.Fatalf("fmt --check must not rewrite source:\n--- got ---\n%s\n--- want ---\n%s", got, source)
	}
}

func TestRunFmtTHTRCheckReportsWidthAwareObjectFormatting(t *testing.T) {
	t.Parallel()

	source := `stage smoke
scenario ping
  act get-user
    do action.http(method: "GET", url: "/users/123")
    expect profile: field(body) | decode(json) == object { id: "usr_1234567890", email: "demo@example.test", display_name: "Demo User", timezone: "Europe/Vilnius" }
`
	path := writeStageFile(t, "stage.thtr", source)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"fmt", "--check", path}, &stdout, &stderr)
	if got, want := code, 1; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "" {
		t.Fatalf("stdout mismatch: got %q want empty", got)
	}
	if !strings.Contains(stderr.String(), path+" is not formatted") {
		t.Fatalf("stderr missing formatting message: %q", stderr.String())
	}
	if got := readFileString(t, path); got != source {
		t.Fatalf("fmt --check must not rewrite source:\n--- got ---\n%s\n--- want ---\n%s", got, source)
	}
}

func TestRunFmtTHTRDiffReportsDirtyFile(t *testing.T) {
	t.Parallel()

	source := `stage smoke
scenario ping
  act get-health
    do action.http
      method: "GET"
      url: "/health"
`
	path := writeStageFile(t, "stage.thtr", source)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"fmt", "--diff", path}, &stdout, &stderr)
	if got, want := code, 1; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}
	for _, want := range []string{
		"--- " + path,
		"+++ " + path + " (formatted)",
		"@@ -2,5 +2,4 @@",
		`-    do action.http`,
		`+    do action.http(method: "GET", url: "/health")`,
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("diff output missing %q: %q", want, stdout.String())
		}
	}
	if got := readFileString(t, path); got != source {
		t.Fatalf("fmt --diff must not rewrite source:\n--- got ---\n%s\n--- want ---\n%s", got, source)
	}
}

func TestRunFmtTHTRCheckDiffReportsDirtyFile(t *testing.T) {
	t.Parallel()

	source := `stage smoke
scenario ping
  act get-health
    do action.http
      method: "GET"
      url: "/health"
`
	path := writeStageFile(t, "stage.thtr", source)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"fmt", "--check", "--diff", path}, &stdout, &stderr)
	if got, want := code, 1; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}
	if !strings.Contains(stdout.String(), "+++ "+path+" (formatted)") {
		t.Fatalf("check+diff output missing formatted header: %q", stdout.String())
	}
	if got := readFileString(t, path); got != source {
		t.Fatalf("fmt --check --diff must not rewrite source:\n--- got ---\n%s\n--- want ---\n%s", got, source)
	}
}

func TestRunFmtTHTRDiffReportsCleanFile(t *testing.T) {
	t.Parallel()

	path := writeStageFile(t, "stage.thtr", `stage smoke

scenario ping
  act get-health
    do action.http(method: "GET", url: "/health")
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"fmt", "--diff", path}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "" {
		t.Fatalf("stdout mismatch: got %q want empty", got)
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}
}

func TestRunFmtTHTRWritePreservesFileMode(t *testing.T) {
	t.Parallel()

	path := writeStageFile(t, "stage.thtr", `stage smoke
scenario ping
  act get-health
    do action.http
      method: "GET"
      url: "/health"
`)
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("chmod test file failed: %v", err)
	}

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"fmt", "-file", path, "-write"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat formatted file failed: %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o644); got != want {
		t.Fatalf("file mode mismatch: got %o want %o", got, want)
	}
}

func TestRunFmtTHTRParseDiagnostics(t *testing.T) {
	t.Parallel()

	path := writeStageFile(t, "stage.thtr", "stage main\nscenario\n")

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"fmt", "-file", path}, &stdout, &stderr)
	if got, want := code, 1; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}
	if !strings.Contains(stdout.String(), "[thtr_parse_error]") {
		t.Fatalf("fmt output must include parse diagnostic code: %q", stdout.String())
	}
}

func TestRunStageTHTRAuthoringDiagnosticsRenderInRunOutput(t *testing.T) {
	t.Parallel()

	path := writeStageFile(t, "stage.thtr", "stage main\nscenario\n")

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"run", "-file", path, "-live", "off"}, &stdout, &stderr)
	if got, want := code, 1; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}

	output := normalizeRunOutput(stdout.String(), map[string]string{
		path: "<stage-file>",
	})

	if !strings.Contains(output, "<stage-file>: failed") {
		t.Fatalf("run output must render a failed authoring document: %q", output)
	}
	if !strings.Contains(output, "diagnostics:") {
		t.Fatalf("run output must include diagnostics block: %q", output)
	}
	if !strings.Contains(output, "[thtr_parse_error]") {
		t.Fatalf("run output must include authoring diagnostic code: %q", output)
	}
}

func TestRunStageWritesSidecarsForTHTRAuthoringDiagnostics(t *testing.T) {
	outputDir := t.TempDir()
	jsonPath := filepath.Join(outputDir, "authoring.json")
	junitPath := filepath.Join(outputDir, "authoring.junit.xml")
	markdownPath := filepath.Join(outputDir, "authoring.md")
	path := writeStageFile(t, "stage.thtr", "stage main\nscenario\n")

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{
		"run",
		"-file", path,
		"-live", "off",
		"--json-output", jsonPath,
		"--junit-output", junitPath,
		"--markdown-output", markdownPath,
	}, &stdout, &stderr)
	if got, want := code, 1; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}
	if !strings.Contains(readFileString(t, jsonPath), `"code": "thtr_parse_error"`) {
		t.Fatalf("json sidecar missing authoring diagnostic: %q", readFileString(t, jsonPath))
	}
	if !strings.Contains(readFileString(t, junitPath), `<error`) {
		t.Fatalf("junit sidecar missing authoring failure: %q", readFileString(t, junitPath))
	}
	if !strings.Contains(readFileString(t, markdownPath), "thtr_parse_error") {
		t.Fatalf("markdown sidecar missing authoring diagnostic: %q", readFileString(t, markdownPath))
	}
}

func TestRunValidateRejectsUnknownSubcommand(t *testing.T) {
	t.Parallel()

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"unknown"}, &stdout, &stderr)
	if got, want := code, exitCodeCommandError; got != want {
		t.Fatalf("exit code mismatch: got %d want %d", got, want)
	}

	if !strings.Contains(stderr.String(), `unknown subcommand "unknown"`) {
		t.Fatalf("stderr mismatch: %q", stderr.String())
	}
}

func TestRunStageText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte("scenario-log-live-body"))
	}))
	defer server.Close()

	t.Setenv("THEATER_STAGE_URL", server.URL)

	path := writeStageYAML(t, `
id: main
scenarios:
  - id: probe
    acts:
      - id: request
        properties:
          url:
            inventory:
              use: inventory.env
              with:
                name:
                  kind: literal
                  value: THEATER_STAGE_URL
        action:
          use: action.http
          with:
            url:
              kind: ref
              ref: url
        logs:
          - id: response
            value:
              field: body
            capture: summary
            sensitivity: internal
scenario_calls:
  - id: probe-server
    scenario_id: probe
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"run", "-file", path}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}

	if !strings.Contains(stdout.String(), path+": passed") {
		t.Fatalf("stdout mismatch: %q", stdout.String())
	}
	if strings.Contains(stdout.String(), "scenario-log-live-body") || strings.Contains(stdout.String(), "log response") {
		t.Fatalf("successful text stdout must not print scenario logs: %q", stdout.String())
	}

	if !strings.Contains(stderr.String(), "[live] stage.running") {
		t.Fatalf("stderr must include live stage event: %q", stderr.String())
	}

	if !strings.Contains(stderr.String(), "[live] action.running") {
		t.Fatalf("stderr must include live action event: %q", stderr.String())
	}

	if !strings.Contains(stderr.String(), "[log] log response: scenario-log-live-body") {
		t.Fatalf("stderr must include live scenario log: %q", stderr.String())
	}
}

func TestRunStageTextLiveOffKeepsStderrQuiet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte("ok"))
	}))
	defer server.Close()

	t.Setenv("THEATER_STAGE_URL", server.URL)

	path := writeStageYAML(t, `
id: main
scenarios:
  - id: probe
    acts:
      - id: request
        properties:
          url:
            inventory:
              use: inventory.env
              with:
                name:
                  kind: literal
                  value: THEATER_STAGE_URL
        action:
          use: action.http
          with:
            url:
              kind: ref
              ref: url
scenario_calls:
  - id: probe-server
    scenario_id: probe
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"run", "-file", path, "-live", "off"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}

	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}
}

func TestRunStageTextRendersFailureCardWithSourceSpan(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	t.Setenv("THEATER_STAGE_URL", server.URL)

	path := writeStageYAML(t, `
id: main
scenarios:
  - id: probe
    acts:
      - id: request
        properties:
          url:
            inventory:
              use: inventory.env
              with:
                name:
                  kind: literal
                  value: THEATER_STAGE_URL
        action:
          use: action.http
          with:
            url:
              kind: ref
              ref: url
        expectations:
          - id: status
            subject: status_code
            assert:
              eq: 201
scenario_calls:
  - id: probe-server
    scenario_id: probe
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"run", "-file", path}, &stdout, &stderr)
	if got, want := code, 1; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}

	output := normalizeRunOutput(stdout.String(), map[string]string{
		path: "<stage-file>",
	})

	if !strings.Contains(output, `<stage-file>: failed`) {
		t.Fatalf("output must include failed stage summary: %q", output)
	}
	if !strings.Contains(output, `scenario probe-server [failed]`) {
		t.Fatalf("output must include scenario failure card: %q", output)
	}
	if !strings.Contains(output, `  scenario: probe`) {
		t.Fatalf("output must include scenario id: %q", output)
	}
	if !strings.Contains(output, `  act: request`) {
		t.Fatalf("output must include failing act id: %q", output)
	}
	if !strings.Contains(output, `summary: expectation failed: actual 200 does not equal expected 201`) {
		t.Fatalf("output must include expectation failure summary: %q", output)
	}
	if !regexp.MustCompile(`source: <stage-file>:\d+:\d+`).MatchString(output) {
		t.Fatalf("output must include source span: %q", output)
	}
}

func TestRunStageTextRendersDiagnosticsFromRunDocument(t *testing.T) {
	t.Parallel()

	path := writeStageYAML(t, `
id: main
scenarios:
  - id: probe
    acts:
      - id: request
        action:
          use: action.command
          with:
            executable:
              kind: literal
              value: echo
        transitions:
          - on: on_pass
            to: missing
scenario_calls:
  - id: probe-server
    scenario_id: probe
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"run", "-file", path}, &stdout, &stderr)
	if got, want := code, 1; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}

	output := normalizeRunOutput(stdout.String(), map[string]string{
		path: "<stage-file>",
	})

	if !strings.Contains(output, `diagnostics:`) {
		t.Fatalf("output must include diagnostics section: %q", output)
	}
	if !strings.Contains(output, `[missing_transition_target]`) {
		t.Fatalf("output must include validation diagnostic code: %q", output)
	}
}

func TestRunStageTextSummarizesEventuallyAndOmitsStageAbortedCards(t *testing.T) {
	t.Parallel()

	helper := testkit.BuildCommandHelper(t)
	path := writeStageYAML(t, `
id: main
scenarios:
  - id: flaky
    acts:
      - id: wait
        eventually:
          timeout: 20ms
          interval: 5ms
        action:
          use: action.command
          repeatable: true
          with:
            executable:
              kind: literal
              value: `+helper+`
            args:
              kind: list
              list:
                - kind: literal
                  value: emit
                - kind: literal
                  value: --exit-code
                - kind: literal
                  value: "1"
        expectations:
          - id: exit-zero
            subject: exit_code
            assert:
              eq: 0
scenario_calls:
  - id: first
    scenario_id: flaky
  - id: second
    scenario_id: flaky
    dependencies:
      - call_id: first
        when: success
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"run", "-file", path}, &stdout, &stderr)
	if got, want := code, 1; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}

	output := normalizeRunOutput(stdout.String(), map[string]string{
		path:   "<stage-file>",
		helper: "<command-helper>",
	})

	if !strings.Contains(output, `scenario first [failed]`) {
		t.Fatalf("output must include failing scenario card: %q", output)
	}
	if !strings.Contains(output, `eventually: attempts=`) {
		t.Fatalf("output must include eventually summary: %q", output)
	}
	if !strings.Contains(output, `termination=deadline_exceeded`) {
		t.Fatalf("output must include deadline termination reason: %q", output)
	}
	if strings.Contains(output, `scenario second [`) {
		t.Fatalf("stage-aborted pending scenario must not produce detail card: %q", output)
	}
}

func TestRunStageTextShowsConvergenceSummaryForPassedEventually(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		current := requests.Add(1)
		if current < 3 {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	t.Setenv("THEATER_STAGE_URL", server.URL)

	path := writeStageYAML(t, `
id: main
scenarios:
  - id: probe
    acts:
      - id: request
        eventually:
          timeout: 100ms
          interval: 5ms
        action:
          use: action.http
          repeatable: true
          with:
            url:
              kind: literal
              value: `+server.URL+`
        expectations:
          - id: status
            subject: status_code
            assert:
              eq: 200
scenario_calls:
  - id: probe-server
    scenario_id: probe
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"run", "-file", path}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}

	output := normalizeRunOutput(stdout.String(), map[string]string{
		path:       "<stage-file>",
		server.URL: "<server-url>",
	})

	if !strings.Contains(output, `<stage-file>: passed`) {
		t.Fatalf("output must include passed stage summary: %q", output)
	}
	if !strings.Contains(output, `eventually: converged_acts=1 extra_attempts=2`) {
		t.Fatalf("output must include convergence summary: %q (requests=%s)", output, strconv.Itoa(int(requests.Load())))
	}
}

func TestRunStageTextRendersCommandObservationsAndHidesSecretInputs(t *testing.T) {
	t.Parallel()

	helper := testkit.BuildCommandHelper(t)
	path := writeStageYAML(t, `
id: main
scenarios:
  - id: command
    acts:
      - id: run
        action:
          use: action.command
          with:
            executable:
              kind: literal
              value: `+helper+`
            working_dir:
              kind: literal
              value: `+filepath.Dir(helper)+`
            timeout:
              kind: literal
              value: 20ms
            stdin:
              kind: literal
              value: hidden-stdin
            env:
              kind: object
              object:
                COMMAND_TEST_TOKEN:
                  kind: literal
                  value: secret-value
            args:
              kind: list
              list:
                - kind: literal
                  value: emit
                - kind: literal
                  value: --stdout
                - kind: literal
                  value: out
                - kind: literal
                  value: --stderr
                - kind: literal
                  value: warn
                - kind: literal
                  value: --sleep-after-ms
                - kind: literal
                  value: "100"
scenario_calls:
  - id: run-command
    scenario_id: command
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"run", "-file", path}, &stdout, &stderr)
	if got, want := code, 1; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}

	output := normalizeRunOutput(stdout.String(), map[string]string{
		path:                 "<stage-file>",
		helper:               "<command-helper>",
		filepath.Dir(helper): "<command-dir>",
	})

	if !strings.Contains(output, `scenario run-command [failed]`) {
		t.Fatalf("output must include command failure card: %q", output)
	}
	if !strings.Contains(output, "inputs:\n") {
		t.Fatalf("output must include input observations: %q", output)
	}
	if !strings.Contains(output, `executable: `) {
		t.Fatalf("output must include executable preview: %q", output)
	}
	if !strings.Contains(output, `timeout: 20ms`) {
		t.Fatalf("output must include timeout preview: %q", output)
	}
	if !strings.Contains(output, "streams:\n") {
		t.Fatalf("output must include stream observations: %q", output)
	}
	if !strings.Contains(output, `stdout: out`) || !strings.Contains(output, `stderr: warn`) {
		t.Fatalf("output must include partial stream previews: %q", output)
	}
	if strings.Contains(output, `secret-value`) {
		t.Fatalf("output must not leak secret env values: %q", output)
	}
	if strings.Contains(output, `hidden-stdin`) {
		t.Fatalf("output must not leak hidden stdin: %q", output)
	}
}

func TestRunStageLiveAutoUsesPlainTransitionsOnDumbTerminal(t *testing.T) {
	t.Setenv(terminalEnvName, terminalTypeDumb)

	helper := testkit.BuildCommandHelper(t)
	path := writeStageYAML(t, `
id: main
scenarios:
  - id: command
    acts:
      - id: run
        action:
          use: action.command
          with:
            executable:
              kind: literal
              value: `+helper+`
            working_dir:
              kind: literal
              value: `+filepath.Dir(helper)+`
            timeout:
              kind: literal
              value: 50ms
            args:
              kind: list
              list:
                - kind: literal
                  value: emit
                - kind: literal
                  value: --stdout
                - kind: literal
                  value: out
                - kind: literal
                  value: --sleep-after-ms
                - kind: literal
                  value: "10"
scenario_calls:
  - id: run-command
    scenario_id: command
`)

	var stdout strings.Builder
	var stderr strings.Builder
	app := newApplication(&stdout, &stderr)
	app.isTerminal = func(io.Writer) bool { return true }

	code := app.Run([]string{commandRun, "--file", path, "--live", string(liveModeAuto)})
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}

	if strings.Contains(stderr.String(), ansiEscapePrefix) {
		t.Fatalf("TERM=dumb live output must not use terminal frame escapes: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "[live] stage.running status=running") {
		t.Fatalf("TERM=dumb live output must fall back to plain transition lines: %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), path+": passed") {
		t.Fatalf("stdout must still include the final text summary: %q", stdout.String())
	}
}

func TestRunStageJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"status":"json-report-log-body"}`))
	}))
	defer server.Close()

	t.Setenv("THEATER_STAGE_URL", server.URL)

	path := writeStageYAML(t, `
id: main
scenarios:
  - id: probe
    acts:
      - id: request
        properties:
          url:
            inventory:
              use: inventory.env
              with:
                name:
                  kind: literal
                  value: THEATER_STAGE_URL
        action:
          use: action.http
          with:
            url:
              kind: ref
              ref: url
        logs:
          - id: response
            value:
              field: body
            capture: summary
            sensitivity: internal
scenario_calls:
  - id: probe-server
    scenario_id: probe
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"run", "-file", path, "-format", "json"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}

	if !strings.Contains(stdout.String(), `"status": "passed"`) {
		t.Fatalf("json output must include passed report status: %q", stdout.String())
	}

	if !strings.Contains(stdout.String(), `"report_schema_version": "v1alpha1"`) {
		t.Fatalf("json output must include canonical schema version: %q", stdout.String())
	}
	envelope := decodeRunDocumentOutput(t, stdout.String())
	assertRunDocumentIdentity(t, envelope.Result)

	if !strings.Contains(stdout.String(), `"kind": "action"`) {
		t.Fatalf("json output must include materialized action node: %q", stdout.String())
	}

	if strings.Contains(stdout.String(), `"events":`) {
		t.Fatalf("json output must not expose raw events: %q", stdout.String())
	}

	if !strings.Contains(stdout.String(), `"logs":`) ||
		!strings.Contains(stdout.String(), `"id": "response"`) ||
		!strings.Contains(stdout.String(), `json-report-log-body`) {
		t.Fatalf("json output must include scenario logs in the report: %q", stdout.String())
	}

	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}
}

func TestRunStageWritesSidecarOutputsFromOneExecution(t *testing.T) {
	helper := testkit.BuildCommandHelper(t)
	outputDir := t.TempDir()
	markerPath := filepath.Join(outputDir, "marker.txt")
	jsonPath := filepath.Join(outputDir, "run.json")
	junitPath := filepath.Join(outputDir, "run.junit.xml")
	markdownPath := filepath.Join(outputDir, "run.md")
	path := writeStageYAML(t, `
id: main
scenarios:
  - id: probe
    acts:
      - id: append-marker
        action:
          use: action.command
          with:
            executable:
              kind: literal
              value: `+helper+`
            args:
              kind: list
              list:
                - kind: literal
                  value: append-marker
                - kind: literal
                  value: --path
                - kind: literal
                  value: `+markerPath+`
        expectations:
          - id: exit-zero
            subject: exit_code
            assert:
              eq: 0
scenario_calls:
  - id: run-probe
    scenario_id: probe
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{
		"run",
		"-file", path,
		"--live", "off",
		"--format", "text",
		"--json-output", jsonPath,
		"--junit-output", junitPath,
		"--markdown-output", markdownPath,
	}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}

	if got, want := strings.Count(readFileString(t, markerPath), "marker\n"), 1; got != want {
		t.Fatalf("stage executed wrong number of times: got %d marker writes want %d", got, want)
	}
	jsonSidecar := readFileString(t, jsonPath)
	if !strings.Contains(jsonSidecar, `"report_schema_version": "v1alpha1"`) {
		t.Fatalf("json sidecar missing run document schema: %q", jsonSidecar)
	}
	assertRunDocumentIdentity(t, decodeRunDocumentOutput(t, jsonSidecar).Result)
	if !strings.Contains(readFileString(t, junitPath), `<testsuites`) {
		t.Fatalf("junit sidecar missing testsuites root: %q", readFileString(t, junitPath))
	}
	if !strings.Contains(readFileString(t, markdownPath), "# Theater Run Report") {
		t.Fatalf("markdown sidecar missing report title: %q", readFileString(t, markdownPath))
	}
	if !strings.Contains(stdout.String(), "passed") {
		t.Fatalf("stdout text output missing passed summary: %q", stdout.String())
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}
}

func TestRunStageWritesSidecarsForFailedRun(t *testing.T) {
	outputDir := t.TempDir()
	jsonPath := filepath.Join(outputDir, "failed.json")
	junitPath := filepath.Join(outputDir, "failed.junit.xml")
	markdownPath := filepath.Join(outputDir, "failed.md")
	path := writeStageYAML(t, `
id: main
scenarios:
  - id: probe
    acts:
      - id: generate
        action:
          use: action.generate
          with:
            outputs:
              ok: true
        expectations:
          - id: wrong-value
            subject:
              field: values
              path: /ok
            assert:
              eq: false
scenario_calls:
  - id: run-probe
    scenario_id: probe
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{
		"run",
		"-file", path,
		"--live", "off",
		"--json-output", jsonPath,
		"--junit-output", junitPath,
		"--markdown-output", markdownPath,
	}, &stdout, &stderr)
	if got, want := code, 1; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}

	if !strings.Contains(readFileString(t, jsonPath), `"status": "failed"`) {
		t.Fatalf("json sidecar missing failed status: %q", readFileString(t, jsonPath))
	}
	if !strings.Contains(readFileString(t, junitPath), `<failure`) {
		t.Fatalf("junit sidecar missing failure: %q", readFileString(t, junitPath))
	}
	if !strings.Contains(readFileString(t, markdownPath), "failed") {
		t.Fatalf("markdown sidecar missing failed status: %q", readFileString(t, markdownPath))
	}
}

func TestRunStageRejectsExistingSidecarWithoutOverwriteBeforeExecution(t *testing.T) {
	helper := testkit.BuildCommandHelper(t)
	outputDir := t.TempDir()
	markerPath := filepath.Join(outputDir, "marker.txt")
	jsonPath := filepath.Join(outputDir, "run.json")
	if err := os.WriteFile(jsonPath, []byte("existing"), 0o600); err != nil {
		t.Fatalf("write existing sidecar failed: %v", err)
	}
	path := writeStageYAML(t, `
id: main
scenarios:
  - id: probe
    acts:
      - id: append-marker
        action:
          use: action.command
          with:
            executable:
              kind: literal
              value: `+helper+`
            args:
              kind: list
              list:
                - kind: literal
                  value: append-marker
                - kind: literal
                  value: --path
                - kind: literal
                  value: `+markerPath+`
scenario_calls:
  - id: run-probe
    scenario_id: probe
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{
		"run",
		"-file", path,
		"--live", "off",
		"--json-output", jsonPath,
	}, &stdout, &stderr)
	if got, want := code, exitCodeCommandError; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}

	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Fatalf("stage must not execute when sidecar preflight fails, marker stat err=%v", err)
	}
	if got, want := readFileString(t, jsonPath), "existing"; got != want {
		t.Fatalf("existing sidecar changed: got %q want %q", got, want)
	}
	if !strings.Contains(stderr.String(), "sidecar output") {
		t.Fatalf("stderr must explain sidecar failure: %q", stderr.String())
	}
}

func TestRunStageOverwritesExistingSidecarWhenRequested(t *testing.T) {
	outputDir := t.TempDir()
	jsonPath := filepath.Join(outputDir, "run.json")
	if err := os.WriteFile(jsonPath, []byte("existing"), 0o644); err != nil {
		t.Fatalf("write existing sidecar failed: %v", err)
	}
	path := writeStageYAML(t, `
id: main
scenarios:
  - id: probe
    acts:
      - id: generate
        action:
          use: action.generate
          with:
            outputs:
              ok: true
scenario_calls:
  - id: run-probe
    scenario_id: probe
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{
		"run",
		"-file", path,
		"--live", "off",
		"--json-output", jsonPath,
		"--overwrite",
	}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}
	if got := readFileString(t, jsonPath); !strings.Contains(got, `"report_schema_version": "v1alpha1"`) || strings.Contains(got, "existing") {
		t.Fatalf("json sidecar was not replaced with run document: %q", got)
	}
	info, err := os.Stat(jsonPath)
	if err != nil {
		t.Fatalf("stat json sidecar failed: %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Fatalf("json sidecar mode mismatch: got %o want %o", got, want)
	}
}

func TestRunStageRejectsDuplicateSidecarPathBeforeExecution(t *testing.T) {
	helper := testkit.BuildCommandHelper(t)
	outputDir := t.TempDir()
	markerPath := filepath.Join(outputDir, "marker.txt")
	sidecarPath := filepath.Join(outputDir, "run.out")
	path := writeStageYAML(t, `
id: main
scenarios:
  - id: probe
    acts:
      - id: append-marker
        action:
          use: action.command
          with:
            executable:
              kind: literal
              value: `+helper+`
            args:
              kind: list
              list:
                - kind: literal
                  value: append-marker
                - kind: literal
                  value: --path
                - kind: literal
                  value: `+markerPath+`
scenario_calls:
  - id: run-probe
    scenario_id: probe
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{
		"run",
		"-file", path,
		"--live", "off",
		"--json-output", sidecarPath,
		"--junit-output", sidecarPath,
	}, &stdout, &stderr)
	if got, want := code, exitCodeCommandError; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}
	if !strings.Contains(stderr.String(), "used for both") {
		t.Fatalf("stderr must explain duplicate sidecar path: %q", stderr.String())
	}
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Fatalf("stage must not execute when sidecar preflight fails, marker stat err=%v", err)
	}
}

func TestRunStageReportsSidecarWriteFailureAfterExecution(t *testing.T) {
	helper := testkit.BuildCommandHelper(t)
	outputDir := t.TempDir()
	sidecarDir := filepath.Join(outputDir, "sidecars")
	if err := os.Mkdir(sidecarDir, 0o755); err != nil {
		t.Fatalf("mkdir sidecar dir failed: %v", err)
	}
	jsonPath := filepath.Join(sidecarDir, "run.json")
	path := writeStageYAML(t, `
id: main
scenarios:
  - id: probe
    acts:
      - id: remove-sidecar-dir
        action:
          use: action.command
          with:
            executable:
              kind: literal
              value: `+helper+`
            args:
              kind: list
              list:
                - kind: literal
                  value: remove-path
                - kind: literal
                  value: --path
                - kind: literal
                  value: `+sidecarDir+`
scenario_calls:
  - id: run-probe
    scenario_id: probe
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{
		"run",
		"-file", path,
		"--live", "off",
		"--json-output", jsonPath,
	}, &stdout, &stderr)
	if got, want := code, exitCodeCommandError; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}
	if _, err := os.Stat(sidecarDir); !os.IsNotExist(err) {
		t.Fatalf("stage must execute before sidecar write failure, sidecar dir stat err=%v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "" {
		t.Fatalf("stdout must stay empty when sidecar writing fails before stdout rendering, got %q", got)
	}
	if !strings.Contains(stderr.String(), "write sidecar output") || !strings.Contains(stderr.String(), "sidecar output parent") {
		t.Fatalf("stderr must explain sidecar write failure: %q", stderr.String())
	}
}

func TestRunStageRejectsSymlinkedSidecarParentAfterExecution(t *testing.T) {
	helper := testkit.BuildCommandHelper(t)
	outputDir := t.TempDir()
	sidecarDir := filepath.Join(outputDir, "sidecars")
	escapeDir := filepath.Join(outputDir, "escape")
	for _, dir := range []string{sidecarDir, escapeDir} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s failed: %v", dir, err)
		}
	}
	jsonPath := filepath.Join(sidecarDir, "run.json")
	path := writeStageYAML(t, `
id: main
scenarios:
  - id: probe
    acts:
      - id: replace-sidecar-dir
        action:
          use: action.command
          with:
            executable:
              kind: literal
              value: `+helper+`
            args:
              kind: list
              list:
                - kind: literal
                  value: replace-with-symlink
                - kind: literal
                  value: --path
                - kind: literal
                  value: `+sidecarDir+`
                - kind: literal
                  value: --target
                - kind: literal
                  value: `+escapeDir+`
scenario_calls:
  - id: run-probe
    scenario_id: probe
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{
		"run",
		"-file", path,
		"--live", "off",
		"--json-output", jsonPath,
		"--overwrite",
	}, &stdout, &stderr)
	if got, want := code, exitCodeCommandError; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "" {
		t.Fatalf("stdout must stay empty when sidecar writing fails before stdout rendering, got %q", got)
	}
	if !strings.Contains(stderr.String(), "must not contain symlink directory") {
		t.Fatalf("stderr must explain symlinked parent rejection: %q", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(escapeDir, "run.json")); !os.IsNotExist(err) {
		t.Fatalf("sidecar escaped into symlink target, stat err=%v", err)
	}
}

func TestRunStageRejectsUnsafeSidecarPathsBeforeExecution(t *testing.T) {
	helper := testkit.BuildCommandHelper(t)
	outputDir := t.TempDir()
	markerPath := filepath.Join(outputDir, "marker.txt")
	directoryOutput := filepath.Join(outputDir, "sidecar-dir")
	if err := os.Mkdir(directoryOutput, 0o755); err != nil {
		t.Fatalf("mkdir sidecar dir failed: %v", err)
	}
	symlinkOutput := filepath.Join(outputDir, "sidecar-link")
	if err := os.Symlink(filepath.Join(outputDir, "target.json"), symlinkOutput); err != nil {
		t.Fatalf("symlink sidecar failed: %v", err)
	}
	traversalOutput := outputDir + string(os.PathSeparator) + ".." + string(os.PathSeparator) + "escape.json"
	missingParentOutput := filepath.Join(outputDir, "missing", "run.json")
	parentFile := filepath.Join(outputDir, "parent-file")
	if err := os.WriteFile(parentFile, []byte("parent"), 0o600); err != nil {
		t.Fatalf("write parent file failed: %v", err)
	}
	fileParentOutput := filepath.Join(parentFile, "run.json")
	path := writeStageYAML(t, `
id: main
scenarios:
  - id: probe
    acts:
      - id: append-marker
        action:
          use: action.command
          with:
            executable:
              kind: literal
              value: `+helper+`
            args:
              kind: list
              list:
                - kind: literal
                  value: append-marker
                - kind: literal
                  value: --path
                - kind: literal
                  value: `+markerPath+`
scenario_calls:
  - id: run-probe
    scenario_id: probe
`)

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "directory", path: directoryOutput, want: "is a directory"},
		{name: "symlink", path: symlinkOutput, want: "is a symlink"},
		{name: "parent traversal", path: traversalOutput, want: "must not contain parent traversal"},
		{name: "missing parent", path: missingParentOutput, want: "no such file or directory"},
		{name: "parent is file", path: fileParentOutput, want: "is not a directory"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout strings.Builder
			var stderr strings.Builder

			code := run([]string{
				"run",
				"-file", path,
				"--live", "off",
				"--json-output", test.path,
				"--overwrite",
			}, &stdout, &stderr)
			if got, want := code, exitCodeCommandError; got != want {
				t.Fatalf("exit code mismatch: got %d want %d, stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
			}
			if !strings.Contains(stderr.String(), test.want) {
				t.Fatalf("stderr mismatch: got %q want substring %q", stderr.String(), test.want)
			}
			if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
				t.Fatalf("stage must not execute when sidecar preflight fails, marker stat err=%v", err)
			}
		})
	}
}

func TestRunStageRejectsDashSidecarOutput(t *testing.T) {
	path := writeStageYAML(t, `
id: main
scenarios:
  - id: probe
    acts:
      - id: generate
        action:
          use: action.generate
          with:
            outputs:
              ok: true
scenario_calls:
  - id: run-probe
    scenario_id: probe
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"run", "-file", path, "--json-output", "-"}, &stdout, &stderr)
	if got, want := code, exitCodeCommandError; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}
	if !strings.Contains(stderr.String(), "does not accept -") {
		t.Fatalf("stderr must explain dash sidecar rejection: %q", stderr.String())
	}
}

func TestRunStageJSONDebugDumpKeepsStdoutClean(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	t.Setenv("THEATER_STAGE_URL", server.URL)

	path := writeStageYAML(t, `
id: main
scenarios:
  - id: probe
    acts:
      - id: request
        properties:
          url:
            inventory:
              use: inventory.env
              with:
                name:
                  kind: literal
                  value: THEATER_STAGE_URL
        action:
          use: action.http
          with:
            url:
              kind: ref
              ref: url
scenario_calls:
  - id: probe-server
    scenario_id: probe
`)
	dumpPath := filepath.Join(t.TempDir(), "run.debug.ndjson")

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{
		"run",
		"-file", path,
		"-format", "json",
		"-debug", "dump",
		"-break", "name=before-request,kind=action,phase=before,path=**",
		"-debug-dump", dumpPath,
	}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"report_schema_version": "v1alpha1"`) {
		t.Fatalf("json output must include canonical schema version: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"status": "passed"`) {
		t.Fatalf("json output must include passed report status: %q", stdout.String())
	}
	for _, unexpected := range []string{`"snapshot"`, `"breakpoint"`, `"reason":"breakpoint"`, "PAUSED start"} {
		if strings.Contains(stdout.String(), unexpected) {
			t.Fatalf("json stdout must stay free of debug noise %q: %q", unexpected, stdout.String())
		}
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}

	data, err := os.ReadFile(dumpPath)
	if err != nil {
		t.Fatalf("read debug dump failed: %v", err)
	}

	dump := string(data)
	if !strings.Contains(dump, `"kind":"pause"`) {
		t.Fatalf("debug dump must contain a pause record: %q", dump)
	}
	if !strings.Contains(dump, `"reason":"breakpoint"`) {
		t.Fatalf("debug dump must contain breakpoint reason: %q", dump)
	}
	if !strings.Contains(dump, `"breakpoint":"before-request"`) {
		t.Fatalf("debug dump must contain breakpoint label: %q", dump)
	}
}

func TestRunStageRejectsInteractiveDebugWithoutTTY(t *testing.T) {
	t.Parallel()

	path := writeStageYAML(t, `
id: main
scenarios:
  - id: probe
    acts:
      - id: request
        action:
          use: action.command
          with:
            executable:
              kind: literal
              value: echo
scenario_calls:
  - id: probe-server
    scenario_id: probe
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"run", "-file", path, "-debug", "interactive"}, &stdout, &stderr)
	if got, want := code, exitCodeCommandError; got != want {
		t.Fatalf("exit code mismatch: got %d want %d", got, want)
	}
	if strings.TrimSpace(stdout.String()) != "" {
		t.Fatalf("stdout mismatch: got %q want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "interactive debug requires a TTY") {
		t.Fatalf("stderr must explain missing TTY: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "--debug dump") {
		t.Fatalf("stderr must suggest dump mode: %q", stderr.String())
	}
}

func TestRunStageRejectsDebugDumpWithoutDumpPath(t *testing.T) {
	t.Parallel()

	path := writeStageYAML(t, `
id: main
scenarios:
  - id: probe
    acts:
      - id: request
        action:
          use: action.command
          with:
            executable:
              kind: literal
              value: echo
scenario_calls:
  - id: probe-server
    scenario_id: probe
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"run", "-file", path, "-debug", "dump"}, &stdout, &stderr)
	if got, want := code, exitCodeCommandError; got != want {
		t.Fatalf("exit code mismatch: got %d want %d", got, want)
	}
	if strings.TrimSpace(stdout.String()) != "" {
		t.Fatalf("stdout mismatch: got %q want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "--debug dump requires --debug-dump <path>") {
		t.Fatalf("stderr must explain missing dump path: %q", stderr.String())
	}
}

func TestRunStageDebugBreakFileLoadsSelectors(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	path := writeStageYAML(t, `
id: main
scenarios:
  - id: probe
    acts:
      - id: request
        action:
          use: action.http
          with:
            url:
              kind: literal
              value: `+server.URL+`
        expectations:
          - id: status
            subject: status_code
            assert:
              eq: 200
scenario_calls:
  - id: probe-server
    scenario_id: probe
`)
	breakFile := filepath.Join(t.TempDir(), "team.debug")
	if err := os.WriteFile(breakFile, []byte(strings.Join([]string{
		"# terminal failure preset",
		"when=terminal-failure",
		"",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("write break file failed: %v", err)
	}
	dumpPath := filepath.Join(t.TempDir(), "run.debug.ndjson")

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{
		"run",
		"-file", path,
		"-format", "json",
		"-debug", "dump",
		"-break-file", breakFile,
		"-debug-dump", dumpPath,
	}, &stdout, &stderr)
	if got, want := code, 1; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}

	data, err := os.ReadFile(dumpPath)
	if err != nil {
		t.Fatalf("read debug dump failed: %v", err)
	}

	dump := string(data)
	if !strings.Contains(dump, `"reason":"terminal-failure"`) {
		t.Fatalf("debug dump must contain terminal failure reason: %q", dump)
	}
	if !strings.Contains(dump, `"breakpoint":"stage.main/call.probe-server/act.request/expectation.status"`) {
		t.Fatalf("debug dump must use matched path label for unnamed break-file selector: %q", dump)
	}
}

func TestRunStageInteractiveDebugKeepsStdoutCanonicalAndWritesPromptToStderr(t *testing.T) {
	t.Parallel()

	path := writeStageYAML(t, `
id: main
scenarios:
  - id: probe
    acts:
      - id: request
        action:
          use: action.command
          with:
            executable:
              kind: literal
              value: echo
scenario_calls:
  - id: probe-server
    scenario_id: probe
`)

	var stdout strings.Builder
	var stderr strings.Builder

	app := newApplication(&stdout, &stderr)
	app.stdin = strings.NewReader("continue\n")
	app.isTerminal = func(io.Writer) bool { return true }
	app.isInputTerminal = func(io.Reader) bool { return true }

	code := app.Run([]string{
		"run",
		"-file", path,
		"-format", "json",
		"-debug", "interactive",
		"-step",
	})
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}

	if !strings.Contains(stdout.String(), `"report_schema_version": "v1alpha1"`) {
		t.Fatalf("stdout must contain canonical run document: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"status": "passed"`) {
		t.Fatalf("stdout must contain passed run status: %q", stdout.String())
	}
	for _, unexpected := range []string{"PAUSED start", "(debug)", `"snapshot"`, `"breakpoint"`, `"reason":"breakpoint"`} {
		if strings.Contains(stdout.String(), unexpected) {
			t.Fatalf("stdout must stay free of interactive debug noise %q: %q", unexpected, stdout.String())
		}
	}

	for _, want := range []string{
		"PAUSED start",
		"kind: action",
		"phase: before",
		"(debug) ",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr missing %q: %q", want, stderr.String())
		}
	}
	if strings.Contains(stderr.String(), `"report_schema_version": "v1alpha1"`) {
		t.Fatalf("stderr must stay free of canonical json output: %q", stderr.String())
	}
}

func TestValidateStageDebugPathsPrintsReusableBreakpointSelectors(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	path := writeStageYAML(t, `
id: main
scenarios:
  - id: probe
    acts:
      - id: request
        eventually:
          timeout: 3s
          interval: 1s
        action:
          repeatable: true
          use: action.http
          with:
            url:
              kind: literal
              value: `+server.URL+`
        expectations:
          - id: status
            subject: status_code
            assert:
              eq: 200
scenario_calls:
  - id: probe-server
    scenario_id: probe
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"validate", "-file", path, "-debug-paths"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}

	output := stdout.String()
	for _, want := range []string{
		"# use with --break or --break-file",
		"# retry-aware selector; attempt filters are valid",
		"kind=scenario_call,phase=before,path=stage.main/call.probe-server",
		"kind=act,phase=after,path=stage.main/call.probe-server/act.request",
		"kind=action,phase=before,path=stage.main/call.probe-server/act.request/action",
		"kind=expectation,phase=after,path=stage.main/call.probe-server/act.request/expectation.status",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("debug path output missing %q: %q", want, output)
		}
	}
}

func TestValidateStageDebugPathsJSONUsesDocumentedFieldNames(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	path := writeStageYAML(t, `
id: main
scenarios:
  - id: probe
    acts:
      - id: request
        eventually:
          timeout: 3s
          interval: 1s
        action:
          repeatable: true
          use: action.http
          with:
            url:
              kind: literal
              value: `+server.URL+`
        expectations:
          - id: status
            subject: status_code
            assert:
              eq: 200
scenario_calls:
  - id: probe-server
    scenario_id: probe
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"validate", "-file", path, "-debug-paths", "-format", "json"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}

	output := stdout.String()
	for _, want := range []string{`"path":`, `"kind":`, `"phase":`, `"retry_aware": true`} {
		if !strings.Contains(output, want) {
			t.Fatalf("debug path json output missing %q: %q", want, output)
		}
	}
	for _, unexpected := range []string{`"Path":`, `"Kind":`, `"Phase":`, `"RetryAware":`} {
		if strings.Contains(output, unexpected) {
			t.Fatalf("debug path json output must not use Go field names %q: %q", unexpected, output)
		}
	}
}

func TestRunStageDebugDumpSupportsScenarioCallAndActBreakpoints(t *testing.T) {
	t.Parallel()

	path := writeStageYAML(t, `
id: main
scenarios:
  - id: probe
    acts:
      - id: request
        action:
          use: action.command
          with:
            executable:
              kind: literal
              value: echo
scenario_calls:
  - id: probe-server
    scenario_id: probe
`)
	dumpPath := filepath.Join(t.TempDir(), "run.debug.ndjson")

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{
		"run",
		"-file", path,
		"-format", "json",
		"-debug", "dump",
		"-break", "name=before-call,kind=scenario_call,phase=before,path=stage.main/call.probe-server",
		"-break", "name=after-act,kind=act,phase=after,path=stage.main/call.probe-server/act.request",
		"-debug-dump", dumpPath,
	}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"status": "passed"`) {
		t.Fatalf("json output must include passed report status: %q", stdout.String())
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}

	data, err := os.ReadFile(dumpPath)
	if err != nil {
		t.Fatalf("read debug dump failed: %v", err)
	}

	dump := string(data)
	for _, want := range []string{
		`"breakpoint":"before-call"`,
		`"breakpoint":"after-act"`,
		`"kind":"scenario_call"`,
		`"path":"stage.main/call.probe-server"`,
		`"phase":"before"`,
		`"kind":"act"`,
		`"path":"stage.main/call.probe-server/act.request"`,
		`"phase":"after"`,
	} {
		if !strings.Contains(dump, want) {
			t.Fatalf("debug dump missing %q: %q", want, dump)
		}
	}
}

func TestRunStageDebugStopOnFailureWritesTerminalFailureArtifact(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	t.Setenv("THEATER_STAGE_URL", server.URL)

	path := writeStageYAML(t, `
id: main
scenarios:
  - id: probe
    acts:
      - id: request
        action:
          use: action.http
          with:
            url:
              kind: literal
              value: `+server.URL+`
        expectations:
          - id: status
            subject: status_code
            assert:
              eq: 201
scenario_calls:
  - id: probe-server
    scenario_id: probe
`)
	dumpPath := filepath.Join(t.TempDir(), "run.debug.ndjson")

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{
		"run",
		"-file", path,
		"-format", "json",
		"-debug", "dump",
		"-stop-on-failure",
		"-debug-dump", dumpPath,
	}, &stdout, &stderr)
	if got, want := code, 1; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"status": "failed"`) {
		t.Fatalf("json output must include failed report status: %q", stdout.String())
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}

	data, err := os.ReadFile(dumpPath)
	if err != nil {
		t.Fatalf("read debug dump failed: %v", err)
	}

	dump := string(data)
	if !strings.Contains(dump, `"reason":"terminal-failure"`) {
		t.Fatalf("debug dump must contain terminal failure reason: %q", dump)
	}
	if !strings.Contains(dump, `"breakpoint":"stop-on-failure"`) {
		t.Fatalf("debug dump must contain stop-on-failure label: %q", dump)
	}
}

func TestRunStageJUnitDebugDumpKeepsStdoutClean(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	t.Setenv("THEATER_STAGE_URL", server.URL)

	path := writeStageYAML(t, `
id: main
scenarios:
  - id: probe
    acts:
      - id: request
        action:
          use: action.http
          with:
            url:
              kind: literal
              value: `+server.URL+`
        expectations:
          - id: status
            subject: status_code
            assert:
              eq: 201
scenario_calls:
  - id: probe-server
    scenario_id: probe
`)
	dumpPath := filepath.Join(t.TempDir(), "run.debug.ndjson")

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{
		"run",
		"-file", path,
		"-format", "junit",
		"-debug", "dump",
		"-stop-on-failure",
		"-debug-dump", dumpPath,
	}, &stdout, &stderr)
	if got, want := code, 1; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}
	if !strings.Contains(stdout.String(), `<?xml version="1.0" encoding="UTF-8"?>`) {
		t.Fatalf("junit output must include xml header: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), `<testsuite`) {
		t.Fatalf("junit output must include a test suite: %q", stdout.String())
	}
	for _, unexpected := range []string{`"snapshot"`, `"breakpoint"`, "terminal-failure", "PAUSED start", "(debug) "} {
		if strings.Contains(stdout.String(), unexpected) {
			t.Fatalf("junit stdout must stay free of debug noise %q: %q", unexpected, stdout.String())
		}
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}

	data, err := os.ReadFile(dumpPath)
	if err != nil {
		t.Fatalf("read debug dump failed: %v", err)
	}

	dump := string(data)
	if !strings.Contains(dump, `"reason":"terminal-failure"`) {
		t.Fatalf("debug dump must contain terminal failure reason: %q", dump)
	}
	if !strings.Contains(dump, `"breakpoint":"stop-on-failure"`) {
		t.Fatalf("debug dump must contain stop-on-failure label: %q", dump)
	}
}

func TestRunStageJUnit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte("ok"))
	}))
	defer server.Close()

	t.Setenv("THEATER_STAGE_URL", server.URL)

	path := writeStageYAML(t, `
id: main
scenarios:
  - id: probe
    acts:
      - id: request
        properties:
          url:
            inventory:
              use: inventory.env
              with:
                name:
                  kind: literal
                  value: THEATER_STAGE_URL
        action:
          use: action.http
          with:
            url:
              kind: ref
              ref: url
scenario_calls:
  - id: probe-server
    scenario_id: probe
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"run", "-file", path, "-format", "junit"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}

	if !strings.Contains(stdout.String(), `<?xml version="1.0" encoding="UTF-8"?>`) {
		t.Fatalf("junit output must include xml header: %q", stdout.String())
	}

	if !strings.Contains(stdout.String(), `<testsuite`) {
		t.Fatalf("junit output must include testsuite: %q", stdout.String())
	}

	if !strings.Contains(stdout.String(), `classname="probe"`) {
		t.Fatalf("junit output must include scenario classname: %q", stdout.String())
	}

	if !strings.Contains(stdout.String(), `name="probe-server"`) {
		t.Fatalf("junit output must include scenario call name: %q", stdout.String())
	}

	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}
}

func TestRunValidateRejectsJUnitFormat(t *testing.T) {
	t.Parallel()

	path := writeStageYAML(t, `
id: main
scenarios:
  - id: probe
    acts:
      - id: request
        action:
          use: action.command
          with:
            executable:
              kind: literal
              value: echo
scenario_calls:
  - id: probe-server
    scenario_id: probe
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"validate", "-file", path, "-format", "junit"}, &stdout, &stderr)
	if got, want := code, exitCodeCommandError; got != want {
		t.Fatalf("exit code mismatch: got %d want %d", got, want)
	}

	if !strings.Contains(stderr.String(), `unsupported format "junit"`) {
		t.Fatalf("stderr mismatch: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), `supported: text, json`) {
		t.Fatalf("stderr must include valid format alternatives: %q", stderr.String())
	}
}

func TestRunStageRejectsUnsupportedRunFormatWithAlternatives(t *testing.T) {
	t.Parallel()

	path := writeStageYAML(t, `
id: main
scenarios:
  - id: probe
    acts:
      - id: request
        action:
          use: action.command
          with:
            executable:
              kind: literal
              value: echo
scenario_calls:
  - id: probe-server
    scenario_id: probe
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"run", path, "--format", "xml"}, &stdout, &stderr)
	if got, want := code, exitCodeCommandError; got != want {
		t.Fatalf("exit code mismatch: got %d want %d", got, want)
	}
	if got := strings.TrimSpace(stdout.String()); got != "" {
		t.Fatalf("stdout mismatch: got %q want empty", got)
	}
	if !strings.Contains(stderr.String(), `unsupported format "xml" (supported: text, json, junit)`) {
		t.Fatalf("stderr must include valid run format alternatives: %q", stderr.String())
	}
}

func TestRunStageRejectsUnsupportedModesWithAlternatives(t *testing.T) {
	t.Parallel()

	path := writeStageYAML(t, `
id: main
scenarios:
  - id: probe
    acts:
      - id: request
        action:
          use: action.command
          with:
            executable:
              kind: literal
              value: echo
scenario_calls:
  - id: probe-server
    scenario_id: probe
`)

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "live",
			args: []string{"run", path, "--live", "quiet"},
			want: `unsupported live mode "quiet" (supported: auto, off)`,
		},
		{
			name: "debug",
			args: []string{"run", path, "--debug", "trace"},
			want: `unsupported debug mode "trace" (supported: off, dump, interactive)`,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var stdout strings.Builder
			var stderr strings.Builder

			code := run(test.args, &stdout, &stderr)
			if got, want := code, exitCodeCommandError; got != want {
				t.Fatalf("exit code mismatch: got %d want %d", got, want)
			}
			if got := strings.TrimSpace(stdout.String()); got != "" {
				t.Fatalf("stdout mismatch: got %q want empty", got)
			}
			if !strings.Contains(stderr.String(), test.want) {
				t.Fatalf("stderr missing %q: %q", test.want, stderr.String())
			}
		})
	}
}

func TestRunStageWithHTTPExpectation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte(`{"token":"issued-token"}`))
	}))
	defer server.Close()

	t.Setenv("THEATER_STAGE_URL", server.URL)

	path := writeStageYAML(t, `
id: main
scenarios:
  - id: probe
    inputs:
      expected_status_code:
        type: number
    acts:
      - id: request
        properties:
          url:
            inventory:
              use: inventory.env
              with:
                name:
                  kind: literal
                  value: THEATER_STAGE_URL
        action:
          use: action.http
          with:
            url:
              kind: ref
              ref: url
        expectations:
          - id: ok
            subject: status_code
            assert:
              between:
                min: 200
                max: 299
          - id: exact
            subject: status_code
            assert:
              eq:
                kind: ref
                ref: expected_status_code
          - id: body
            subject: body
            assert:
              contains: issued-token
scenario_calls:
  - id: probe-server
    scenario_id: probe
    bindings:
      expected_status_code:
        kind: literal
        value: 200
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"run", "-file", path}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}

	if !strings.Contains(stdout.String(), "passed") {
		t.Fatalf("stdout mismatch: %q", stdout.String())
	}
}

func TestRunStageAutoDetectsRepoFlowFile(t *testing.T) {
	restore := lockWorkingDirForTest(t)
	defer restore()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"token":"issued-token"}`))
	}))
	defer server.Close()

	t.Setenv("THEATER_LOGIN_URL", server.URL)

	path := filepath.Join("..", "..", "theater", "flows", "auth", "login-smoke.yaml")

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"run", "-file", path, "-format", "junit"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}

	if !strings.Contains(stdout.String(), `classname="auth/login"`) {
		t.Fatalf("junit output must include canonical scenario id: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), `name="smoke-login"`) {
		t.Fatalf("junit output must include scenario call id: %q", stdout.String())
	}
}

func TestValidateStageAutoDetectsRepoFlowFile(t *testing.T) {
	t.Parallel()

	restore := lockWorkingDirForTest(t)
	defer restore()

	path := filepath.Join("..", "..", "theater", "flows", "auth", "login-smoke.yaml")

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"validate", "-file", path}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}

	if got, want := strings.TrimSpace(stdout.String()), path+": valid"; got != want {
		t.Fatalf("stdout mismatch: got %q want %q", got, want)
	}
}

func TestValidateRepoFlowReportsSelectedLibraryStaticHTTPAuth(t *testing.T) {
	t.Parallel()

	repoRoot := createCLIFlowRepo(t)
	flowPath := writeCLIRepoFile(t, repoRoot, filepath.Join("theater", "flows", "service", "sample.thtr"), `stage sample-flow

call run-sample = service/sample-ready()
`)
	libraryPath := writeCLIRepoFile(t, repoRoot, filepath.Join("theater", "lib", "service", "sample.thtr"), `stage service-lib

http
  auth service_api = http.auth
    attach: list [
      object { bearer: object { token: "selected-library-secret" } }
    ]

scenario service/sample-ready
  act get-sample
    do action.http(auth: "service_api")
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"validate", "--file", flowPath}, &stdout, &stderr)
	if got, want := code, 1; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}

	output := normalizeRunOutput(stdout.String(), map[string]string{
		libraryPath: "LIBRARY",
	})
	if !strings.Contains(output, "invalid_selected_library_http_auth") {
		t.Fatalf("output must include selected library auth diagnostic code: %q", output)
	}
	if !strings.Contains(output, "LIBRARY:6:7") {
		t.Fatalf("output must include library source span: %q", output)
	}
	if strings.Contains(output, "selected-library-secret") {
		t.Fatalf("output leaked static credential: %q", output)
	}
}

func TestValidateRepoFlowReportsDuplicateSelectedLibraryHTTPAuth(t *testing.T) {
	t.Parallel()

	repoRoot := createCLIFlowRepo(t)
	flowPath := writeCLIRepoFile(t, repoRoot, filepath.Join("theater", "flows", "service", "sample.thtr"), `stage sample-flow

http
  auth service_api = http.auth
    attach: list [
      object { bearer: object { token_slot: "session_token" } }
    ]

call run-sample = service/sample-ready()
`)
	libraryPath := writeCLIRepoFile(t, repoRoot, filepath.Join("theater", "lib", "service", "sample.thtr"), `stage service-lib

http
  auth service_api = http.auth
    attach: list [
      object { bearer: object { token_slot: "session_token" } }
    ]

scenario service/sample-ready
  bind auth service_api
    session_token: "token-from-runtime"
  act get-sample
    do action.http(auth: "service_api")
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"validate", "--file", flowPath}, &stdout, &stderr)
	if got, want := code, 1; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}

	output := normalizeRunOutput(stdout.String(), map[string]string{
		libraryPath: "LIBRARY",
	})
	if !strings.Contains(output, "duplicate_selected_library_http_auth") {
		t.Fatalf("output must include duplicate selected library auth diagnostic code: %q", output)
	}
	if !strings.Contains(output, "LIBRARY:4:3") {
		t.Fatalf("output must include library source span: %q", output)
	}
}

func TestRunStageAutoDetectsRepoFlowFileFromNestedWorkingDirectory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"token":"issued-token"}`))
	}))
	defer server.Close()

	t.Setenv("THEATER_LOGIN_URL", server.URL)

	workingDir := filepath.Join("..", "..", "theater", "flows", "auth")
	restore := chdirForTest(t, workingDir)
	defer restore()

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"run", "-file", "./login-smoke.yaml", "-format", "junit"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}

	if !strings.Contains(stdout.String(), `classname="auth/login"`) {
		t.Fatalf("junit output must include canonical scenario id: %q", stdout.String())
	}
}

func TestValidateStageAutoDetectsRepoFlowFileFromNestedWorkingDirectory(t *testing.T) {
	workingDir := filepath.Join("..", "..", "theater", "flows", "auth")
	restore := chdirForTest(t, workingDir)
	defer restore()

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"validate", "-file", "./login-smoke.yaml"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}

	if got, want := strings.TrimSpace(stdout.String()), "./login-smoke.yaml: valid"; got != want {
		t.Fatalf("stdout mismatch: got %q want %q", got, want)
	}
}

func TestApplicationBuildDebugOptionsRejectsInteractiveWithoutTTYInput(t *testing.T) {
	t.Parallel()

	app := newApplication(io.Discard, io.Discard)
	app.stdin = strings.NewReader("")
	app.isTerminal = func(io.Writer) bool { return true }
	app.isInputTerminal = func(io.Reader) bool { return false }

	_, _, err := app.buildDebugOptions(commandOptions{debugMode: theater.DebugModeInteractive})
	if err == nil {
		t.Fatal("buildDebugOptions error = nil, want TTY validation failure")
	}
	if !strings.Contains(err.Error(), "interactive debug requires a TTY on stdin and stderr") {
		t.Fatalf("buildDebugOptions error mismatch: %q", err.Error())
	}
}

func TestApplicationBuildDebugOptionsRejectsBreakFileWhenDebugModeIsOff(t *testing.T) {
	t.Parallel()

	app := newApplication(io.Discard, io.Discard)

	_, _, err := app.buildDebugOptions(commandOptions{
		debugMode:       theater.DebugModeOff,
		debugBreakFiles: []string{"team.debug"},
	})
	if err == nil {
		t.Fatal("buildDebugOptions error = nil, want debug mode validation failure")
	}
	if !strings.Contains(err.Error(), "debug flags require --debug dump or --debug interactive") {
		t.Fatalf("buildDebugOptions error mismatch: %q", err.Error())
	}
}

func writeStageYAML(t *testing.T, body string) string {
	return writeStageFile(t, "stage.yaml", body)
}

func writeStageFile(t *testing.T, name, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write stage failed: %v", err)
	}

	return path
}

func readFileString(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file %s failed: %v", path, err)
	}
	return string(data)
}

func decodeRunDocumentOutput(t *testing.T, raw string) runJSONEnvelope {
	t.Helper()

	var envelope runJSONEnvelope
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		t.Fatalf("decode run document output failed: %v\n%s", err, raw)
	}
	if err := envelope.Result.Validate(); err != nil {
		t.Fatalf("run document output validation failed: %v\n%s", err, raw)
	}
	return envelope
}

func assertRunDocumentIdentity(t *testing.T, document theater.RunDocument) {
	t.Helper()

	if got, want := document.ReportSchemaVersion, theater.RunDocumentSchemaVersion; got != want {
		t.Fatalf("report schema version mismatch: got %q want %q", got, want)
	}
	if got, want := document.TheaterVersion, theater.Version(); got != want {
		t.Fatalf("theater version mismatch: got %q want %q", got, want)
	}
	if document.RunID == "" {
		t.Fatal("run id must be present")
	}
	if len(document.Report.Nodes) == 0 {
		t.Fatal("run document must include report nodes")
	}
	for _, node := range document.Report.Nodes {
		if node.ID == "" {
			t.Fatalf("node %s must include id", node.Path)
		}
	}
}

func chdirForTest(t *testing.T, path string) func() {
	t.Helper()

	chdirTestMu.Lock()

	workingDir, err := os.Getwd()
	if err != nil {
		chdirTestMu.Unlock()
		t.Fatalf("getwd failed: %v", err)
	}

	if err := os.Chdir(path); err != nil {
		chdirTestMu.Unlock()
		t.Fatalf("chdir to %s failed: %v", path, err)
	}

	return func() {
		if err := os.Chdir(workingDir); err != nil {
			chdirTestMu.Unlock()
			t.Fatalf("restore cwd failed: %v", err)
		}
		chdirTestMu.Unlock()
	}
}

func lockWorkingDirForTest(t *testing.T) func() {
	t.Helper()

	chdirTestMu.Lock()
	return chdirTestMu.Unlock
}

func normalizeRunOutput(output string, replacements map[string]string) string {
	normalized := output
	for from, to := range replacements {
		normalized = strings.ReplaceAll(normalized, from, to)
	}

	durationPattern := regexp.MustCompile(` duration=[^)]+\)`)
	normalized = durationPattern.ReplaceAllString(normalized, ")")
	scenarioDurationPattern := regexp.MustCompile(`(?m)^  duration: .+\n`)
	normalized = scenarioDurationPattern.ReplaceAllString(normalized, "")
	return normalized
}
