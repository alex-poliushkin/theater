package theatercli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunFmtTHTRExpressivenessFixtureMatchesGolden(t *testing.T) {
	t.Parallel()

	sourcePath := expressivenessFixturePath(t, "success-input.thtr")
	want := readExpressivenessFixture(t, "success-formatted.thtr")

	var stdout, stderr bytes.Buffer
	code := run([]string{commandFmt, "--file", sourcePath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("fmt exit code mismatch: got %d stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}
	if got, wantOutput := stdout.String(), string(want); got != wantOutput {
		t.Fatalf("fmt output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, wantOutput)
	}
}

func TestRunLowerTHTRExpressivenessFixtureMatchesGolden(t *testing.T) {
	t.Parallel()

	sourcePath := expressivenessFixturePath(t, "success-input.thtr")
	want := readExpressivenessFixture(t, "success-lowered.yaml")

	var stdout, stderr bytes.Buffer
	code := run([]string{commandLower, "--file", sourcePath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("lower exit code mismatch: got %d stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}
	if got, wantOutput := stdout.String(), string(want); got != wantOutput {
		t.Fatalf("lower output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, wantOutput)
	}
}

func TestRunValidateTHTRExpressivenessFixturePassesWithPluginDescriptors(t *testing.T) {
	t.Parallel()

	configPath, lockPath := writeLockedSmokePluginFiles(t)
	sourcePath := expressivenessFixturePath(t, "success-input.thtr")

	var stdout, stderr bytes.Buffer
	code := run([]string{
		commandValidate,
		"--file", sourcePath,
		"--plugins-config", configPath,
		"--plugins-lock", lockPath,
		"--format", "json",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("validate exit code mismatch: got %d stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}

	response := decodeValidationResponse(t, stdout.Bytes())
	if !response.Valid {
		t.Fatalf("expressiveness fixture must validate: %#v", response.Diagnostics)
	}
}

func TestRunValidateTHTRExpressivenessPluginDescriptorFixtureShowsSourceDiagnostic(t *testing.T) {
	t.Parallel()

	configPath, lockPath := writeLockedSmokePluginFiles(t)
	sourcePath := expressivenessFixturePath(t, "invalid-plugin-descriptor.thtr")
	source := string(readExpressivenessFixture(t, "invalid-plugin-descriptor.thtr"))

	var stdout, stderr bytes.Buffer
	code := run([]string{
		commandValidate,
		"--file", sourcePath,
		"--plugins-config", configPath,
		"--plugins-lock", lockPath,
		"--format", "json",
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("validate exit code mismatch: got %d want 1 stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}

	response := decodeValidationResponse(t, stdout.Bytes())
	diagnostic := findValidationDiagnostic(response.Diagnostics, "incompatible_action_arg")
	if diagnostic == nil {
		t.Fatalf("missing incompatible_action_arg diagnostic: %#v", response.Diagnostics)
	}
	if diagnostic.Span.File != sourcePath {
		t.Fatalf("diagnostic source file mismatch: got %q want %q", diagnostic.Span.File, sourcePath)
	}
	if got, want := diagnostic.Span.Line, 5; got != want {
		t.Fatalf("diagnostic source line mismatch: got %d want %d", got, want)
	}
	assertDiagnosticSourcePrefix(t, source, *diagnostic, "value: 42")
	if !strings.Contains(diagnostic.Summary, `action input "value"`) {
		t.Fatalf("diagnostic summary mismatch: %q", diagnostic.Summary)
	}
}

func TestRunValidateTHTRExpressivenessInvalidFixturesShowDiagnostics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		wantCode   string
		wantSource string
		wantText   string
	}{
		{
			name:       "invalid-quoted-core-id.thtr",
			wantCode:   "[thtr_parse_error]",
			wantSource: "source: <stage-file>:1:7",
			wantText:   "quoted core identifiers are not supported; use an unquoted identifier",
		},
		{
			name:       "invalid-state-claim-fields.thtr",
			wantCode:   "[thtr_lower_error]",
			wantSource: "source: <stage-file>:16:9",
			wantText:   "state.claim fields only supports exact top-level field matching",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			sourcePath := expressivenessFixturePath(t, test.name)
			var stdout, stderr bytes.Buffer
			code := run([]string{commandValidate, "--file", sourcePath}, &stdout, &stderr)
			if code != 1 {
				t.Fatalf("validate exit code mismatch: got %d want 1 stderr=%q stdout=%q", code, stderr.String(), stdout.String())
			}
			if got := strings.TrimSpace(stderr.String()); got != "" {
				t.Fatalf("stderr mismatch: got %q want empty", got)
			}

			output := normalizeRunOutput(stdout.String(), map[string]string{
				sourcePath: "<stage-file>",
			})
			for _, want := range []string{test.wantCode, test.wantSource, test.wantText} {
				if !strings.Contains(output, want) {
					t.Fatalf("diagnostic output missing %q:\n%s", want, output)
				}
			}
		})
	}
}

func TestRunValidateTHTRExpressivenessRepoFixtureAndListScenarios(t *testing.T) {
	t.Parallel()

	repo := filepath.Join(repoRoot(t), "testdata", "thtr-expressiveness", "repo")
	sourcePath := filepath.Join(repo, "theater", "flows", "http", "page-text-stress.thtr")

	var validateStdout, validateStderr bytes.Buffer
	validateCode := run([]string{commandValidate, "--file", sourcePath}, &validateStdout, &validateStderr)
	if validateCode != 0 {
		t.Fatalf("validate exit code mismatch: got %d stderr=%q stdout=%q", validateCode, validateStderr.String(), validateStdout.String())
	}
	if got := strings.TrimSpace(validateStderr.String()); got != "" {
		t.Fatalf("validate stderr mismatch: got %q want empty", got)
	}
	if !strings.Contains(validateStdout.String(), sourcePath+": valid") {
		t.Fatalf("validate output mismatch: %q", validateStdout.String())
	}

	var listStdout, listStderr bytes.Buffer
	listCode := run([]string{commandList, commandListScenarios, "--root", repo}, &listStdout, &listStderr)
	if listCode != 0 {
		t.Fatalf("list scenarios exit code mismatch: got %d stderr=%q stdout=%q", listCode, listStderr.String(), listStdout.String())
	}
	if got := strings.TrimSpace(listStderr.String()); got != "" {
		t.Fatalf("list stderr mismatch: got %q want empty", got)
	}
	for _, want := range []string{
		"web/check-page-text",
		"url:string; required",
		"expected_text:string; required",
		"theater/lib/web/check-page-text.thtr:",
	} {
		if !strings.Contains(listStdout.String(), want) {
			t.Fatalf("list output missing %q:\n%s", want, listStdout.String())
		}
	}

	var jsonStdout, jsonStderr bytes.Buffer
	jsonCode := run([]string{commandList, commandListScenarios, "--root", repo, "--format", "json"}, &jsonStdout, &jsonStderr)
	if jsonCode != 0 {
		t.Fatalf("list scenarios json exit code mismatch: got %d stderr=%q stdout=%q", jsonCode, jsonStderr.String(), jsonStdout.String())
	}
	if got := strings.TrimSpace(jsonStderr.String()); got != "" {
		t.Fatalf("list json stderr mismatch: got %q want empty", got)
	}

	var response scenarioListResult
	if err := json.Unmarshal(jsonStdout.Bytes(), &response); err != nil {
		t.Fatalf("decode list scenarios json: %v\n%s", err, jsonStdout.String())
	}
	if got, want := len(response.Scenarios), 1; got != want {
		t.Fatalf("scenario count mismatch: got %d want %d: %#v", got, want, response.Scenarios)
	}
	scenario := response.Scenarios[0]
	if got, want := scenario.ID, "web/check-page-text"; got != want {
		t.Fatalf("scenario id mismatch: got %q want %q", got, want)
	}
	if got, want := scenario.Source.File, "theater/lib/web/check-page-text.thtr"; got != want {
		t.Fatalf("scenario source file mismatch: got %q want %q", got, want)
	}
	if got, want := len(scenario.Inputs), 2; got != want {
		t.Fatalf("scenario input count mismatch: got %d want %d: %#v", got, want, scenario.Inputs)
	}
	requireExpressivenessListedInput(t, scenario.Inputs, "url", "string; required", true)
	requireExpressivenessListedInput(t, scenario.Inputs, "expected_text", "string; required", true)
}

func requireExpressivenessListedInput(t *testing.T, inputs []listedScenarioInput, name string, contract string, required bool) {
	t.Helper()

	for _, input := range inputs {
		if input.Name == name {
			if got := input.Contract; got != contract {
				t.Fatalf("input %q contract mismatch: got %q want %q", name, got, contract)
			}
			if got := input.Required; got != required {
				t.Fatalf("input %q required mismatch: got %v want %v", name, got, required)
			}

			return
		}
	}

	t.Fatalf("input %q not found in %#v", name, inputs)
}

func expressivenessFixturePath(t *testing.T, name string) string {
	t.Helper()

	return filepath.Join(repoRoot(t), "testdata", "thtr-expressiveness", name)
}

func readExpressivenessFixture(t *testing.T, name string) []byte {
	t.Helper()

	data, err := os.ReadFile(expressivenessFixturePath(t, name))
	if err != nil {
		t.Fatalf("read fixture %s failed: %v", name, err)
	}

	return data
}
