package theatercli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReportRenderJunitReadsSavedRunJSON(t *testing.T) {
	t.Parallel()

	input, _ := writeRunJSONFixture(t, "docs/examples/first-stage/stage.thtr", 0)

	var stdout strings.Builder
	var stderr strings.Builder
	code := run([]string{commandReport, commandReportRender, "--input", input, "--format", "junit"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q", got, want, stderr.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}
	for _, want := range []string{
		`<?xml version="1.0" encoding="UTF-8"?>`,
		`<testsuite name="docs-first"`,
		`<testcase classname="hello" name="run"`,
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("junit output missing %q: %q", want, stdout.String())
		}
	}
}

func TestReportRenderMarkdownReadsSavedRunJSON(t *testing.T) {
	t.Parallel()

	input, stagePath := writeRunJSONFixture(t, "docs/examples/reference/logs.thtr", 0)

	var stdout strings.Builder
	var stderr strings.Builder
	code := run([]string{commandReport, commandReportRender, "--input", input, "--format", "markdown"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q", got, want, stderr.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}

	output := stdout.String()
	for _, want := range []string{
		"# Theater Run Report",
		"- File: `" + filepath.ToSlash(stagePath) + "`",
		"- Status: `passed`",
		"## Scenarios",
		"### Scenario `run`",
		"- Scenario: `inspect`",
		"- Act `read` passed",
		"- Log `response` emitted:",
		"- Log `audit` emitted:",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("markdown output missing %q: %q", want, output)
		}
	}
}

func TestReportRenderSummaryMarkdownReadsSavedRunJSON(t *testing.T) {
	t.Parallel()

	input, stagePath := writeRunJSONFixture(t, "docs/examples/reference/logs.thtr", 0)

	var stdout strings.Builder
	var stderr strings.Builder
	code := run([]string{commandReport, commandReportRender, "--input", input, "--format", "summary-md"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q", got, want, stderr.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}

	output := stdout.String()
	for _, want := range []string{
		"# Theater Run Summary",
		"- File: `" + filepath.ToSlash(stagePath) + "`",
		"- Status: `passed`",
		"- Scenarios: total=1 passed=1 failed=0 canceled=0 skipped=0",
		"- Run: `",
		"- Theater: `",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("summary output missing %q: %q", want, output)
		}
	}
	for _, forbidden := range []string{
		"## Scenarios",
		"- Log `response` emitted:",
		"log response",
		"log audit",
		"HTTP body:",
	} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("summary output must stay compact and report-safe, found %q: %q", forbidden, output)
		}
	}
}

func TestReportRenderMarkdownReturnsZeroForFailedRunDocument(t *testing.T) {
	t.Parallel()

	input, _ := writeRunJSONFixture(t, "docs/examples/first-stage/stage-wrong.thtr", 1)

	var stdout strings.Builder
	var stderr strings.Builder
	code := run([]string{commandReport, commandReportRender, "--input", input, "--format", "markdown"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}

	output := stdout.String()
	for _, want := range []string{
		"- Status: `failed`",
		"Failure: expectation failed",
		"Kind: `expectation`",
		"At: `stage.docs-first/call.run/act.say-hello/expectation.message`",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("markdown output missing %q: %q", want, output)
		}
	}
	if strings.Contains(output, "actual hello does not equal expected goodbye") {
		t.Fatalf("markdown output must not restore raw failure cause from saved JSON: %q", output)
	}
}

func TestReportRenderSummaryMarkdownReturnsZeroForFailedRunDocument(t *testing.T) {
	t.Parallel()

	input, _ := writeRunJSONFixture(t, "docs/examples/first-stage/stage-wrong.thtr", 1)

	var stdout strings.Builder
	var stderr strings.Builder
	code := run([]string{commandReport, commandReportRender, "--input", input, "--format", "summary-md"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}

	output := stdout.String()
	for _, want := range []string{
		"- Status: `failed`",
		"## Failed Scenarios",
		"- Scenario `run` failed",
		"- Failed node: `stage.docs-first/call.run/act.say-hello/expectation.message`",
		"expectation failed",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("summary output missing %q: %q", want, output)
		}
	}
	if strings.Contains(output, "actual hello does not equal expected goodbye") {
		t.Fatalf("summary output must not restore raw failure cause from saved JSON: %q", output)
	}
}

func TestReportRenderSummaryMarkdownOmitsHTTPDiagnosticDetails(t *testing.T) {
	t.Parallel()

	input := repoFilePath(t, "docs/examples/reference/failed-http-run.json")

	var stdout strings.Builder
	var stderr strings.Builder
	code := run([]string{commandReport, commandReportRender, "--input", input, "--format", "summary-md"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}

	output := stdout.String()
	for _, want := range []string{
		"# Theater Run Summary",
		"- Status: `failed`",
		"## Failed Scenarios",
		"- Scenario `probe` failed",
		"- Failed node: `stage.docs-http-diagnostics/call.probe/act.fetch/expectation.status`",
		"status mismatch",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("summary output missing %q: %q", want, output)
		}
	}
	for _, forbidden := range []string{
		"HTTP request",
		"HTTP body:",
		"api.example.test",
		"retry later",
		"credential-secret",
		"token",
		"Bad Gateway",
		"req-123",
	} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("summary output must omit HTTP diagnostic detail %q: %q", forbidden, output)
		}
	}
}

func TestReportRenderJunitReturnsZeroForFailedRunDocument(t *testing.T) {
	t.Parallel()

	input, _ := writeRunJSONFixture(t, "docs/examples/first-stage/stage-wrong.thtr", 1)

	var stdout strings.Builder
	var stderr strings.Builder
	code := run([]string{commandReport, commandReportRender, "--input", input, "--format", "junit"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}
	for _, want := range []string{
		`<failure message=`,
		`failure_at: stage.docs-first/call.run/act.say-hello/expectation.message`,
		`summary: expectation failed`,
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("junit output missing %q: %q", want, stdout.String())
		}
	}
	if strings.Contains(stdout.String(), "actual hello does not equal expected goodbye") {
		t.Fatalf("junit output must not restore raw failure cause from saved JSON: %q", stdout.String())
	}
}

func TestReportRenderRejectsOversizedInput(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "large.json")
	content := strings.Repeat(" ", int(reportRenderMaxInputBytes)+1)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write input failed: %v", err)
	}

	var stdout strings.Builder
	var stderr strings.Builder
	code := run([]string{commandReport, commandReportRender, "--input", path, "--format", "markdown"}, &stdout, &stderr)
	if got, want := code, exitCodeCommandError; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stdout=%q stderr=%q", got, want, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "input exceeds") {
		t.Fatalf("stderr must explain input size limit: %q", stderr.String())
	}
}

func TestReportRenderRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte(`{"result":{"report_schema_version":""}}`), 0o600); err != nil {
		t.Fatalf("write input failed: %v", err)
	}

	var stdout strings.Builder
	var stderr strings.Builder
	code := run([]string{commandReport, commandReportRender, "--input", path, "--format", "markdown"}, &stdout, &stderr)
	if got, want := code, exitCodeCommandError; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stdout=%q stderr=%q", got, want, stdout.String(), stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "" {
		t.Fatalf("stdout mismatch: got %q want empty", got)
	}
	if !strings.Contains(stderr.String(), "render report:") {
		t.Fatalf("stderr must explain render failure: %q", stderr.String())
	}
}

func writeRunJSONFixture(t *testing.T, repoStagePath string, wantCode int) (string, string) {
	t.Helper()

	stagePath := repoFilePath(t, repoStagePath)
	var stdout strings.Builder
	var stderr strings.Builder
	code := run([]string{commandRun, stagePath, "--live", "off", "--format", "json"}, &stdout, &stderr)
	if got := code; got != wantCode {
		t.Fatalf("run fixture exit code mismatch: got %d want %d stdout=%q stderr=%q", got, wantCode, stdout.String(), stderr.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("run fixture stderr mismatch: got %q want empty", got)
	}

	path := filepath.Join(t.TempDir(), "run.json")
	if err := os.WriteFile(path, []byte(stdout.String()), 0o600); err != nil {
		t.Fatalf("write run json fixture failed: %v", err)
	}
	return path, stagePath
}

func repoFilePath(t *testing.T, repoPath string) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test file failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
	return filepath.Join(repoRoot, filepath.FromSlash(repoPath))
}
