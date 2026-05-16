package thtr

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/builtin"
)

func TestToolingContractFixtureRoundTripFormatLowerValidateAndMap(t *testing.T) {
	t.Parallel()

	sourcePath := toolingContractFixturePath(t, "success-input.thtr")
	source := readToolingContractFixture(t, "success-input.thtr")
	wantFormatted := readToolingContractFixture(t, "success-formatted.thtr")
	wantLowered := readToolingContractFixture(t, "success-lowered.yaml")

	formatted, err := Format(source)
	if err != nil {
		t.Fatalf("format fixture failed: %v", err)
	}
	if got, want := string(formatted), string(wantFormatted); got != want {
		t.Fatalf("formatted fixture mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}

	reformatted, err := Format(formatted)
	if err != nil {
		t.Fatalf("format idempotence failed: %v", err)
	}
	if got, want := string(reformatted), string(wantFormatted); got != want {
		t.Fatalf("formatter must be idempotent:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}

	result, err := ParseDetailed(source, sourcePath, nil)
	if err != nil {
		t.Fatalf("parse fixture failed: %v", err)
	}
	if got, want := string(result.CanonicalYAML()), string(wantLowered); got != want {
		t.Fatalf("lowered fixture mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}

	bundle, err := builtin.NewBundle()
	if err != nil {
		t.Fatalf("new builtin bundle failed: %v", err)
	}
	diagnostics := result.RewriteDiagnostics(theater.NewValidator(bundle.Catalog, bundle.Matchers).Validate(result.Spec))
	if len(diagnostics) != 0 {
		t.Fatalf("tooling contract fixture must validate, got diagnostics: %#v", diagnostics)
	}

	requireToolingSourceMapRange(t, result, "stage.tooling-smoke/scenario.verify-items/act.fetch/action/binding.session", sourceRange{
		startLine: 10, startColumn: 7, endLine: 10, endColumn: 22,
	})
	requireToolingSourceMapRange(t, result, "stage.tooling-smoke/scenario.verify-items/act.fetch/action/binding.headers.x-trace.id", sourceRange{
		startLine: 13, startColumn: 9, endLine: 13, endColumn: 32,
	})
	requireToolingSourceMapRange(t, result, "stage.tooling-smoke/scenario.verify-items/act.fetch/expectation.has-item/assert/binding.where", sourceRange{
		startLine: 15, startColumn: 75, endLine: 18, endColumn: 6,
	})
	requireToolingSourceMapRange(t, result, "stage.tooling-smoke/scenario.verify-items/act.fetch/export.item_label/through[0]/pick/where[0]/subject/path", sourceRange{
		startLine: 20, startColumn: 7, endLine: 20, endColumn: 18,
	})
	requireToolingSourceMapRange(t, result, "stage.tooling-smoke/scenario.verify-items/act.fetch/export.item_label/through[2]", sourceRange{
		startLine: 22, startColumn: 26, endLine: 22, endColumn: 69,
	})
}

func TestToolingContractParseErrorFixtures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		wantLine    int
		wantSummary string
	}{
		{
			name:        "parse-error-bad-indentation.thtr",
			wantLine:    2,
			wantSummary: "expected scenario, call, or end of file",
		},
		{
			name:        "parse-error-incomplete-paren.thtr",
			wantLine:    5,
			wantSummary: "expected )",
		},
		{
			name:        "parse-error-malformed-clause.thtr",
			wantLine:    7,
			wantSummary: `expected "," or ")" after relative clause`,
		},
		{
			name:        "parse-error-quoted-core-id.thtr",
			wantLine:    1,
			wantSummary: "quoted core identifiers are not supported; use an unquoted identifier",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			path := toolingContractFixturePath(t, test.name)
			_, err := LoadFileDetailed(path, nil)
			if err == nil {
				t.Fatal("expected parse fixture to fail, got nil")
			}

			var diagnosticError *DiagnosticError
			if !errors.As(err, &diagnosticError) {
				t.Fatalf("expected diagnostic error, got %T", err)
			}

			diagnostic := diagnosticError.Diagnostic()
			if got, want := diagnostic.Code, "thtr_parse_error"; got != want {
				t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
			}
			if got, want := diagnostic.Span.File, path; got != want {
				t.Fatalf("diagnostic source file mismatch: got %q want %q", got, want)
			}
			if got, want := diagnostic.Span.Line, test.wantLine; got != want {
				t.Fatalf("diagnostic source line mismatch: got %d want %d", got, want)
			}
			if got, want := diagnostic.Summary, test.wantSummary; got != want {
				t.Fatalf("diagnostic summary mismatch: got %q want %q", got, want)
			}
		})
	}
}

type sourceRange struct {
	startLine   int
	startColumn int
	endLine     int
	endColumn   int
}

func requireToolingSourceMapRange(t *testing.T, result LoadResult, specPath string, want sourceRange) {
	t.Helper()

	if result.sourceMap == nil {
		t.Fatal("source map must be present")
	}
	entry, ok := result.sourceMap.LookupSpecPath(specPath)
	if !ok {
		t.Fatalf("source map must contain %s", specPath)
	}
	if got := entry.Source.StartLine; got != want.startLine {
		t.Fatalf("source map start line mismatch for %s: got %d want %d", specPath, got, want.startLine)
	}
	if got := entry.Source.StartColumn; got != want.startColumn {
		t.Fatalf("source map start column mismatch for %s: got %d want %d", specPath, got, want.startColumn)
	}
	if got := entry.Source.EndLine; got != want.endLine {
		t.Fatalf("source map end line mismatch for %s: got %d want %d", specPath, got, want.endLine)
	}
	if got := entry.Source.EndColumn; got != want.endColumn {
		t.Fatalf("source map end column mismatch for %s: got %d want %d", specPath, got, want.endColumn)
	}
}

func toolingContractFixturePath(t *testing.T, name string) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve fixture path failed")
	}

	return filepath.Join(filepath.Dir(file), "..", "..", "..", "testdata", "thtr-tooling-contract", name)
}

func readToolingContractFixture(t *testing.T, name string) []byte {
	t.Helper()

	data, err := os.ReadFile(toolingContractFixturePath(t, name))
	if err != nil {
		t.Fatalf("read fixture %s failed: %v", name, err)
	}

	return data
}
