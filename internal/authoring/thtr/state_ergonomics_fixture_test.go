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

func TestStateErgonomicsFixturesRoundTripFormatAndLower(t *testing.T) {
	t.Parallel()

	sourcePath := stateErgonomicsFixturePath(t, "success-input.thtr")
	source := readStateErgonomicsFixture(t, "success-input.thtr")
	wantFormatted := readStateErgonomicsFixture(t, "success-formatted.thtr")
	wantLowered := readStateErgonomicsFixture(t, "success-lowered.yaml")

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

	if result.sourceMap == nil {
		t.Fatal("source map must be present")
	}

	recordEntry, ok := result.sourceMap.LookupSpecPath(
		"stage.smoke/scenario.verify-state/act.read-meta/property.thtr:hidden:state:record:shared_meta",
	)
	if !ok {
		t.Fatal("source map must contain hidden record property entry")
	}
	if got, want := recordEntry.Source.StartLine, 5; got != want {
		t.Fatalf("hidden record source line mismatch: got %d want %d", got, want)
	}

	versionEntry, ok := result.sourceMap.LookupSpecPath(
		"stage.smoke/scenario.verify-state/act.update-meta/action/binding.expected_version",
	)
	if !ok {
		t.Fatal("source map must contain state.update if_version entry")
	}
	if got, want := versionEntry.Source.StartLine, 24; got != want {
		t.Fatalf("state.update version source line mismatch: got %d want %d", got, want)
	}

	idEntry, ok := result.sourceMap.LookupSpecPath(
		"stage.smoke/scenario.verify-state/act.claim-primary/action/binding.selector.id",
	)
	if !ok {
		t.Fatal("source map must contain selector id entry")
	}
	if got, want := idEntry.Source.StartLine, 31; got != want {
		t.Fatalf("selector id source line mismatch: got %d want %d", got, want)
	}

	fieldsEntry, ok := result.sourceMap.LookupSpecPath(
		"stage.smoke/scenario.verify-state/act.claim-primary/action/binding.selector.fields",
	)
	if !ok {
		t.Fatal("source map must contain selector fields entry")
	}
	if got, want := fieldsEntry.Source.StartLine, 32; got != want {
		t.Fatalf("selector fields source line mismatch: got %d want %d", got, want)
	}

	bundle, err := builtin.NewBundle()
	if err != nil {
		t.Fatalf("new builtin bundle failed: %v", err)
	}

	validator := theater.NewValidator(bundle.Catalog, bundle.Matchers)
	if diagnostics := validator.Validate(result.Spec); len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
}

func TestStateErgonomicsFixtureAliasDeclarationErrorIsLocal(t *testing.T) {
	t.Parallel()

	_, err := LoadFileDetailed(stateErgonomicsFixturePath(t, "invalid-alias-declaration.thtr"), nil)
	if err == nil {
		t.Fatal("expected alias declaration fixture to fail, got nil")
	}

	var diagnosticError *DiagnosticError
	if !errors.As(err, &diagnosticError) {
		t.Fatalf("expected diagnostic error, got %T", err)
	}

	diagnostic := diagnosticError.Diagnostic()
	if got, want := diagnostic.Code, "thtr_lower_error"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Path, "stage.main/state/record.shared_meta"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Span.Line, 5; got != want {
		t.Fatalf("diagnostic line mismatch: got %d want %d", got, want)
	}
	if got, want := diagnostic.Summary, `state alias "shared_meta" references unknown backend "missing"`; got != want {
		t.Fatalf("diagnostic summary mismatch: got %q want %q", got, want)
	}
}

func TestStateErgonomicsFixtureAliasKindMismatchUsesActionSpan(t *testing.T) {
	t.Parallel()

	_, err := LoadFileDetailed(stateErgonomicsFixturePath(t, "invalid-alias-use.thtr"), nil)
	if err == nil {
		t.Fatal("expected alias use fixture to fail, got nil")
	}

	var diagnosticError *DiagnosticError
	if !errors.As(err, &diagnosticError) {
		t.Fatalf("expected diagnostic error, got %T", err)
	}

	diagnostic := diagnosticError.Diagnostic()
	if got, want := diagnostic.Code, "thtr_lower_error"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Path, "stage.main/scenario.login/act.claim/action"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Span.Line, 12; got != want {
		t.Fatalf("diagnostic line mismatch: got %d want %d", got, want)
	}
	if got, want := diagnostic.Summary, `state action arg "pool" requires pool alias, got record alias "shared_meta"`; got != want {
		t.Fatalf("diagnostic summary mismatch: got %q want %q", got, want)
	}
}

func TestStateErgonomicsFixtureEventuallyValidationRewriteUsesActSource(t *testing.T) {
	t.Parallel()

	path := stateErgonomicsFixturePath(t, "invalid-eventually-claim.thtr")
	result, err := LoadFileDetailed(path, nil)
	if err != nil {
		t.Fatalf("load fixture failed: %v", err)
	}

	bundle, err := builtin.NewBundle()
	if err != nil {
		t.Fatalf("new builtin bundle failed: %v", err)
	}

	validator := theater.NewValidator(bundle.Catalog, bundle.Matchers)
	diagnostics := result.RewriteDiagnostics(validator.Validate(result.Spec))
	diagnostic := findDiagnosticByCodeValue(diagnostics, "state_mutation_inside_eventually")
	if diagnostic == nil {
		t.Fatalf("expected state_mutation_inside_eventually diagnostic, got %#v", diagnostics)
	}
	if got, want := diagnostic.Span.File, path; got != want {
		t.Fatalf("diagnostic source file mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Span.Line, 10; got != want {
		t.Fatalf("diagnostic source line mismatch: got %d want %d", got, want)
	}
	if got, want := diagnostic.Summary, `act "lifecycle" eventually must not use mutating state action "action.state.claim"`; got != want {
		t.Fatalf("diagnostic summary mismatch: got %q want %q", got, want)
	}
}

func stateErgonomicsFixturePath(t *testing.T, name string) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve fixture path failed")
	}

	return filepath.Join(filepath.Dir(file), "..", "..", "..", "testdata", "thtr-state-ergonomics", name)
}

func readStateErgonomicsFixture(t *testing.T, name string) []byte {
	t.Helper()

	data, err := os.ReadFile(stateErgonomicsFixturePath(t, name))
	if err != nil {
		t.Fatalf("read fixture %s failed: %v", name, err)
	}

	return data
}
