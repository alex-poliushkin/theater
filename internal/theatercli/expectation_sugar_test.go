package theatercli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRunLowerTHTRExpectationSugarFixtureMatchesGolden(t *testing.T) {
	t.Parallel()

	source := readExpectationSugarFixture(t, "success-input.thtr")
	want := readExpectationSugarFixture(t, "success-lowered.yaml")
	path := writeStageFile(t, "stage.thtr", string(source))

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"lower", "-file", path}, &stdout, &stderr)
	if got, wantCode := code, 0; got != wantCode {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, wantCode, stderr.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}
	if got, wantOutput := stdout.String(), string(want); got != wantOutput {
		t.Fatalf("lower output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, wantOutput)
	}
}

func TestRunFmtTHTRExpectationSugarFixtureMatchesGolden(t *testing.T) {
	t.Parallel()

	source := readExpectationSugarFixture(t, "success-input.thtr")
	want := readExpectationSugarFixture(t, "success-formatted.thtr")
	path := writeStageFile(t, "stage.thtr", string(source))

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"fmt", "-file", path}, &stdout, &stderr)
	if got, wantCode := code, 0; got != wantCode {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, wantCode, stderr.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}
	if got, wantOutput := stdout.String(), string(want); got != wantOutput {
		t.Fatalf("fmt output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, wantOutput)
	}
}

func TestRunLowerTHTRExpectationSugarParseFixtureShowsDiagnostics(t *testing.T) {
	t.Parallel()

	source := readExpectationSugarFixture(t, "parse-error-missing-comma.thtr")
	path := writeStageFile(t, "invalid.thtr", string(source))

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"lower", "-file", path}, &stdout, &stderr)
	if got, want := code, 1; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}

	output := normalizeRunOutput(stdout.String(), map[string]string{
		path: "<stage-file>",
	})
	if !strings.Contains(output, "[thtr_parse_error]") {
		t.Fatalf("output must include parse diagnostic code: %q", output)
	}
	if !strings.Contains(output, "source: <stage-file>:7:7") {
		t.Fatalf("output must include parse diagnostic source span: %q", output)
	}
}

func TestRunValidateTHTRExpectationSugarLowerFixtureShowsClauseLocalBreadcrumb(t *testing.T) {
	t.Parallel()

	source := readExpectationSugarFixture(t, "invalid-relative-subject.thtr")
	path := writeStageFile(t, "invalid.thtr", string(source))

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"validate", "-file", path}, &stdout, &stderr)
	if got, want := code, 1; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}

	output := normalizeRunOutput(stdout.String(), map[string]string{
		path: "<stage-file>",
	})
	if !strings.Contains(output, "[thtr_lower_error]") {
		t.Fatalf("output must include lower diagnostic code: %q", output)
	}
	if !strings.Contains(output, "source: <stage-file>:5:") {
		t.Fatalf("output must include lower diagnostic source span: %q", output)
	}
	if !strings.Contains(output, "breadcrumb: scenario login -> act submit -> expect bad -> assert -> clause[0] -> subject") {
		t.Fatalf("output must include clause-local breadcrumb: %q", output)
	}
}

func readExpectationSugarFixture(t *testing.T, name string) []byte {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve fixture path failed")
	}

	path := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "thtr-expectation-sugar", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s failed: %v", name, err)
	}

	return data
}
