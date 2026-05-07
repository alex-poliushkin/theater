package theatercli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alex-poliushkin/theater"
)

func TestValidateTHTRPluginActionAndMatcherDescriptorsPass(t *testing.T) {
	t.Parallel()

	configPath, lockPath := writeLockedSmokePluginFiles(t)
	stagePath := writePluginTHTRStage(t, `stage plugin_ux
scenario smoke
  act echo
    do action.smoke.echo(value: "hello")
    expect echoed: field(echo) assert matcher.smoke.equal(expected: "hello")
    expect not-other: field(echo) not assert matcher.smoke.equal(expected: "other")
`)

	var stdout, stderr bytes.Buffer
	code := run([]string{
		commandValidate,
		"--file", stagePath,
		"--plugins-config", configPath,
		"--plugins-lock", lockPath,
		"--format", "json",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("validate exit code mismatch: got %d stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}

	response := decodeValidationResponse(t, stdout.Bytes())
	if !response.Valid {
		t.Fatalf("plugin .thtr stage must validate: %#v", response.Diagnostics)
	}
}

func TestValidateTHTRPluginDescriptorDiagnosticsAreSourceMapped(t *testing.T) {
	t.Parallel()

	configPath, lockPath := writeLockedSmokePluginFiles(t)
	tests := []struct {
		name        string
		stage       string
		wantCode    string
		wantLine    int
		wantPrefix  string
		wantSummary string
		wantCount   int
	}{
		{
			name: "unknown plugin action",
			stage: `stage plugin_ux
scenario smoke
  act echo
    do action.smoke.missing(value: "hello")
`,
			wantCode:    "unknown_action_use",
			wantLine:    4,
			wantPrefix:  `do action.smoke.missing`,
			wantSummary: `action "action.smoke.missing" is not registered`,
		},
		{
			name: "missing plugin action arg",
			stage: `stage plugin_ux
scenario smoke
  act echo
    do action.smoke.echo()
`,
			wantCode:    "missing_action_arg",
			wantLine:    4,
			wantPrefix:  `do action.smoke.echo`,
			wantSummary: `action input "value" is required`,
		},
		{
			name: "wrong plugin action arg shape",
			stage: `stage plugin_ux
scenario smoke
  act echo
    do action.smoke.echo(value: 42)
`,
			wantCode:    "incompatible_action_arg",
			wantLine:    4,
			wantPrefix:  `value: 42`,
			wantSummary: `action input "value"`,
		},
		{
			name: "unknown plugin matcher",
			stage: `stage plugin_ux
scenario smoke
  act echo
    do action.smoke.echo(value: "hello")
    expect echoed: field(echo) assert matcher.smoke.missing(expected: "hello")
`,
			wantCode:    "unknown_expectation_assert_ref",
			wantLine:    5,
			wantPrefix:  `matcher.smoke.missing`,
			wantSummary: `matcher "matcher.smoke.missing" is not registered`,
		},
		{
			name: "missing plugin matcher arg",
			stage: `stage plugin_ux
scenario smoke
  act echo
    do action.smoke.echo(value: "hello")
    expect echoed: field(echo) assert matcher.smoke.equal()
`,
			wantCode:    "missing_expectation_assert_arg",
			wantLine:    5,
			wantPrefix:  `matcher.smoke.equal`,
			wantSummary: `assert "matcher.smoke.equal" requires arg "expected"`,
		},
		{
			name: "wrong plugin matcher arg shape",
			stage: `stage plugin_ux
scenario smoke
  act echo
    do action.smoke.echo(value: "hello")
    expect echoed: field(echo) assert matcher.smoke.equal(expected: 42)
`,
			wantCode:    "incompatible_expectation_assert_arg",
			wantLine:    5,
			wantPrefix:  `expected: 42`,
			wantSummary: `assert "matcher.smoke.equal" arg "expected"`,
		},
		{
			name: "negated plugin matcher missing arg",
			stage: `stage plugin_ux
scenario smoke
  act echo
    do action.smoke.echo(value: "hello")
    expect echoed: field(echo) not assert matcher.smoke.equal()
`,
			wantCode:    "missing_expectation_assert_arg",
			wantLine:    5,
			wantPrefix:  `matcher.smoke.equal`,
			wantSummary: `assert "matcher.smoke.equal" requires arg "expected"`,
		},
		{
			name: "collection plugin matcher missing arg",
			stage: `stage plugin_ux
scenario smoke
  act echo
    do action.smoke.echo(value: "hello")
    expect echoed: field(echo) has item where path("/subject") assert matcher.smoke.equal()
`,
			wantCode:    "missing_expectation_assert_arg",
			wantLine:    5,
			wantPrefix:  `matcher.smoke.equal`,
			wantSummary: `assert "matcher.smoke.equal" requires arg "expected"`,
			wantCount:   2,
		},
		{
			name: "has entry plugin matcher missing arg",
			stage: `stage plugin_ux
scenario smoke
  act echo
    do action.smoke.echo(value: "hello")
    expect echoed: field(echo) has entry("status") assert matcher.smoke.equal()
`,
			wantCode:    "missing_expectation_assert_arg",
			wantLine:    5,
			wantPrefix:  `matcher.smoke.equal`,
			wantSummary: `assert "matcher.smoke.equal" requires arg "expected"`,
			wantCount:   2,
		},
		{
			name: "has entry plugin matcher wrong arg shape",
			stage: `stage plugin_ux
scenario smoke
  act echo
    do action.smoke.echo(value: "hello")
    expect echoed: field(echo) has entry("status") assert matcher.smoke.equal(expected: 42)
`,
			wantCode:    "incompatible_expectation_assert_arg",
			wantLine:    5,
			wantPrefix:  `expected: 42`,
			wantSummary: `assert "matcher.smoke.equal" arg "expected"`,
			wantCount:   2,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			stagePath := writePluginTHTRStage(t, test.stage)
			var stdout, stderr bytes.Buffer
			code := run([]string{
				commandValidate,
				"--file", stagePath,
				"--plugins-config", configPath,
				"--plugins-lock", lockPath,
				"--format", "json",
			}, &stdout, &stderr)
			if code != 1 {
				t.Fatalf("validate exit code mismatch: got %d want 1 stderr=%q stdout=%q", code, stderr.String(), stdout.String())
			}

			response := decodeValidationResponse(t, stdout.Bytes())
			wantCount := test.wantCount
			if wantCount == 0 {
				wantCount = 1
			}
			if got, want := len(response.Diagnostics), wantCount; got != want {
				t.Fatalf("diagnostic count mismatch: got %d want %d diagnostics=%#v", got, want, response.Diagnostics)
			}
			for _, diagnostic := range response.Diagnostics {
				if diagnostic.Code == "invalid_expectation_assert_args" {
					t.Fatalf("nested matcher structural diagnostics must suppress compile diagnostics: %#v", response.Diagnostics)
				}
			}
			diagnostic := findValidationDiagnostic(response.Diagnostics, test.wantCode)
			if diagnostic == nil {
				t.Fatalf("missing diagnostic %q in %#v", test.wantCode, response.Diagnostics)
			}
			if diagnostic.Span.File != stagePath {
				t.Fatalf("diagnostic source file mismatch: got %q want %q", diagnostic.Span.File, stagePath)
			}
			if diagnostic.Span.Line != test.wantLine {
				t.Fatalf("diagnostic source line mismatch: got %d want %d", diagnostic.Span.Line, test.wantLine)
			}
			if diagnostic.Span.Column == 0 {
				t.Fatalf("diagnostic source column must be populated: %#v", diagnostic.Span)
			}
			assertDiagnosticSourcePrefix(t, test.stage, *diagnostic, test.wantPrefix)
			if !strings.Contains(diagnostic.Summary, test.wantSummary) {
				t.Fatalf("diagnostic summary mismatch: got %q want substring %q", diagnostic.Summary, test.wantSummary)
			}
		})
	}
}

func writeLockedSmokePluginFiles(t *testing.T) (string, string) {
	t.Helper()

	configPath, lockPath := writeSmokePluginFiles(t)
	var stdout, stderr bytes.Buffer
	code := run([]string{
		commandPlugins,
		commandPluginsLock,
		"--plugins-config", configPath,
		"--plugins-lock", lockPath,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("plugins lock exit code mismatch: got %d stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}

	return configPath, lockPath
}

func writePluginTHTRStage(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "plugin.thtr")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write .thtr stage: %v", err)
	}

	return path
}

func decodeValidationResponse(t *testing.T, raw []byte) struct {
	File        string               `json:"file"`
	Valid       bool                 `json:"valid"`
	Diagnostics []theater.Diagnostic `json:"diagnostics"`
} {
	t.Helper()

	var response struct {
		File        string               `json:"file"`
		Valid       bool                 `json:"valid"`
		Diagnostics []theater.Diagnostic `json:"diagnostics"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode validation response: %v\n%s", err, raw)
	}

	return response
}

func findValidationDiagnostic(diagnostics []theater.Diagnostic, code string) *theater.Diagnostic {
	for i := range diagnostics {
		if diagnostics[i].Code == code {
			return &diagnostics[i]
		}
	}

	return nil
}

func assertDiagnosticSourcePrefix(t *testing.T, source string, diagnostic theater.Diagnostic, prefix string) {
	t.Helper()

	lines := strings.Split(source, "\n")
	lineIndex := diagnostic.Span.Line - 1
	if lineIndex < 0 || lineIndex >= len(lines) {
		t.Fatalf("diagnostic line outside source: line=%d source=%q", diagnostic.Span.Line, source)
	}
	columnIndex := diagnostic.Span.Column - 1
	if columnIndex < 0 || columnIndex > len(lines[lineIndex]) {
		t.Fatalf("diagnostic column outside source: span=%#v line=%q", diagnostic.Span, lines[lineIndex])
	}
	if got := lines[lineIndex][columnIndex:]; !strings.HasPrefix(got, prefix) {
		t.Fatalf("diagnostic source prefix mismatch: got %q want prefix %q at span %#v", got, prefix, diagnostic.Span)
	}
}
