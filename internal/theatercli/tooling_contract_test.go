package theatercli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRunLowerTHTRToolingContractFixtureMatchesGolden(t *testing.T) {
	t.Parallel()

	source := readToolingContractFixture(t, "success-input.thtr")
	want := readToolingContractFixture(t, "success-lowered.yaml")
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

func TestRunLowerTHTRToolingContractFixtureWritesSourceMap(t *testing.T) {
	t.Parallel()

	source := readToolingContractFixture(t, "success-input.thtr")
	want := readToolingContractFixture(t, "success-lowered.yaml")
	path := writeStageFile(t, "stage.thtr", string(source))
	mapPath := filepath.Join(t.TempDir(), "stage.map.json")

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"lower", "-file", path, "-map", mapPath}, &stdout, &stderr)
	if got, wantCode := code, 0; got != wantCode {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, wantCode, stderr.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}
	if got, wantOutput := stdout.String(), string(want); got != wantOutput {
		t.Fatalf("lower output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, wantOutput)
	}

	mapData, err := os.ReadFile(mapPath)
	if err != nil {
		t.Fatalf("read source map failed: %v", err)
	}
	if !strings.Contains(string(mapData), `"spec_path": "stage.tooling-smoke/scenario.verify-items/act.fetch/action/binding.headers.x-trace.id"`) {
		t.Fatalf("source map must include quoted data-key binding path: %q", string(mapData))
	}
}

func TestRunFmtTHTRToolingContractFixtureMatchesGolden(t *testing.T) {
	t.Parallel()

	source := readToolingContractFixture(t, "success-input.thtr")
	want := readToolingContractFixture(t, "success-formatted.thtr")
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

func TestRunValidateTHTRToolingContractFixturePasses(t *testing.T) {
	t.Parallel()

	source := readToolingContractFixture(t, "success-input.thtr")
	path := writeStageFile(t, "stage.thtr", string(source))

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"validate", "-file", path}, &stdout, &stderr)
	if got, wantCode := code, 0; got != wantCode {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, wantCode, stderr.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}
	output := normalizeRunOutput(stdout.String(), map[string]string{
		path: "<stage-file>",
	})
	if got, want := strings.TrimSpace(output), "<stage-file>: valid"; got != want {
		t.Fatalf("validate output mismatch: got %q want %q", got, want)
	}
}

func TestRunValidateTHTRToolingContractParseFixturesShowDiagnostics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		wantSource string
		wantText   string
	}{
		{
			name:       "parse-error-bad-indentation.thtr",
			wantSource: "source: <stage-file>:2:1",
			wantText:   "expected scenario, call, or end of file",
		},
		{
			name:       "parse-error-incomplete-paren.thtr",
			wantSource: "source: <stage-file>:5:1",
			wantText:   "expected )",
		},
		{
			name:       "parse-error-malformed-clause.thtr",
			wantSource: "source: <stage-file>:7:7",
			wantText:   `expected "," or ")" after relative clause`,
		},
		{
			name:       "parse-error-quoted-core-id.thtr",
			wantSource: "source: <stage-file>:1:7",
			wantText:   "quoted core identifiers are not supported; use an unquoted identifier",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			source := readToolingContractFixture(t, test.name)
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
			if !strings.Contains(output, "[thtr_parse_error]") {
				t.Fatalf("output must include parse diagnostic code: %q", output)
			}
			if !strings.Contains(output, test.wantSource) {
				t.Fatalf("output must include parse diagnostic source span %q: %q", test.wantSource, output)
			}
			if !strings.Contains(output, test.wantText) {
				t.Fatalf("output must include parse diagnostic summary %q: %q", test.wantText, output)
			}
		})
	}
}

func readToolingContractFixture(t *testing.T, name string) []byte {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve fixture path failed")
	}

	path := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "thtr-tooling-contract", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s failed: %v", name, err)
	}

	return data
}
