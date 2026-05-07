package thtr

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/alex-poliushkin/theater"
	builtinexpectation "github.com/alex-poliushkin/theater/builtin/expectation"
	"github.com/alex-poliushkin/theater/internal/testkit"
)

func TestExpectationSugarFixturesRoundTripFormatAndLower(t *testing.T) {
	t.Parallel()

	sourcePath := expectationSugarFixturePath(t, "success-input.thtr")
	source := readExpectationSugarFixture(t, "success-input.thtr")
	wantFormatted := readExpectationSugarFixture(t, "success-formatted.thtr")
	wantLowered := readExpectationSugarFixture(t, "success-lowered.yaml")

	formatted, err := Format(source)
	if err != nil {
		t.Fatalf("format fixture failed: %v", err)
	}
	if got, want := string(formatted), string(wantFormatted); got != want {
		t.Fatalf("formatted fixture mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}

	result, err := ParseDetailed(source, sourcePath, nil)
	if err != nil {
		t.Fatalf("parse fixture failed: %v", err)
	}
	if got, want := string(result.CanonicalYAML()), string(wantLowered); got != want {
		t.Fatalf("lowered fixture mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}

	betweenMinEntry, ok := result.sourceMap.LookupSpecPath(
		"stage.smoke/scenario.ping/act.get-health/expectation.retries-in-range/assert/binding.min",
	)
	if !ok {
		t.Fatal("source map must contain between min entry")
	}
	if got, want := betweenMinEntry.Source.StartLine, 8; got != want {
		t.Fatalf("between min source line mismatch: got %d want %d", got, want)
	}

	betweenMaxEntry, ok := result.sourceMap.LookupSpecPath(
		"stage.smoke/scenario.ping/act.get-health/expectation.retries-in-range/assert/binding.max",
	)
	if !ok {
		t.Fatal("source map must contain between max entry")
	}
	if got, want := betweenMaxEntry.Source.StartLine, 8; got != want {
		t.Fatalf("between max source line mismatch: got %d want %d", got, want)
	}

	whereEntry, ok := result.sourceMap.LookupSpecPath(
		"stage.smoke/scenario.ping/act.get-health/expectation.all-recipients-present/assert/binding.where",
	)
	if !ok {
		t.Fatal("source map must contain collection where entry")
	}
	if got, want := whereEntry.Source.StartLine, 12; got != want {
		t.Fatalf("collection where source line mismatch: got %d want %d", got, want)
	}

	assertEntry, ok := result.sourceMap.LookupSpecPath(
		"stage.smoke/scenario.ping/act.get-health/expectation.all-recipients-present/assert/binding.where/binding.item-1/binding.assert/binding.args/binding.assert/binding.args/binding.expected",
	)
	if !ok {
		t.Fatal("source map must contain nested negated clause assert entry")
	}
	if got, want := assertEntry.Source.StartLine, 14; got != want {
		t.Fatalf("nested negated clause source line mismatch: got %d want %d", got, want)
	}
}

func TestExpectationSugarFixtureParseErrorIsStructural(t *testing.T) {
	t.Parallel()

	_, err := LoadFileDetailed(expectationSugarFixturePath(t, "parse-error-missing-comma.thtr"), nil)
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
	if got, want := diagnostic.Span.Line, 7; got != want {
		t.Fatalf("diagnostic line mismatch: got %d want %d", got, want)
	}
	if got, want := diagnostic.Summary, `expected "," or ")" after relative clause`; got != want {
		t.Fatalf("diagnostic summary mismatch: got %q want %q", got, want)
	}
}

func TestExpectationSugarFixtureInvalidRelativeSubjectHasClauseLocalDiagnostic(t *testing.T) {
	t.Parallel()

	path := expectationSugarFixturePath(t, "invalid-relative-subject.thtr")
	_, err := LoadFileDetailed(path, nil)
	if err == nil {
		t.Fatal("expected invalid relative subject fixture to fail, got nil")
	}

	var diagnosticError *DiagnosticError
	if !errors.As(err, &diagnosticError) {
		t.Fatalf("expected diagnostic error, got %T", err)
	}

	diagnostic := diagnosticError.Diagnostic()
	if got, want := diagnostic.Code, "thtr_lower_error"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Path, "stage.main/scenario.login/act.submit/expectation.bad/assert/clause[0]/subject"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Span.Line, 5; got != want {
		t.Fatalf("diagnostic line mismatch: got %d want %d", got, want)
	}
	if got, want := diagnostic.Summary, `relative clause subject may start only with decode(...) or path(...)`; got != want {
		t.Fatalf("diagnostic summary mismatch: got %q want %q", got, want)
	}
}

func TestExpectationSugarFixtureValidationRewriteUsesClauseSource(t *testing.T) {
	t.Parallel()

	path := expectationSugarFixturePath(t, "validation-unresolved-clause-ref.thtr")
	result, err := LoadFileDetailed(path, nil)
	if err != nil {
		t.Fatalf("load fixture failed: %v", err)
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.http", &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"body": {Kind: theater.ValueKindString},
			},
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	matchers, err := theater.NewMatcherCatalog(builtinexpectation.Descriptors()...)
	if err != nil {
		t.Fatalf("new matcher catalog failed: %v", err)
	}

	validator := theater.NewValidator(catalog, matchers)
	diagnostics := result.RewriteDiagnostics(validator.Validate(result.Spec))
	diagnostic := findDiagnosticByCodeValue(diagnostics, "unresolved_binding_ref")
	if diagnostic == nil {
		t.Fatalf("expected unresolved_binding_ref diagnostic, got %#v", diagnostics)
	}
	if got, want := diagnostic.Span.File, path; got != want {
		t.Fatalf("diagnostic source file mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Span.Line, 5; got != want {
		t.Fatalf("diagnostic source line mismatch: got %d want %d", got, want)
	}
}

func expectationSugarFixturePath(t *testing.T, name string) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve fixture path failed")
	}

	return filepath.Join(filepath.Dir(file), "..", "..", "..", "testdata", "thtr-expectation-sugar", name)
}

func readExpectationSugarFixture(t *testing.T, name string) []byte {
	t.Helper()

	data, err := os.ReadFile(expectationSugarFixturePath(t, name))
	if err != nil {
		t.Fatalf("read fixture %s failed: %v", name, err)
	}

	return data
}
