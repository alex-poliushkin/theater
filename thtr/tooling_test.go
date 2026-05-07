package thtr_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/builtin"
	"github.com/alex-poliushkin/theater/thtr"
)

func TestAnalyzeExposesToolingResult(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "stage.thtr")
	source := []byte(`stage smoke
scenario ping
  act get-health
    eventually 1s every 1s
    do repeatable action.http(method: "GET", url: "/health")
    expect status-ok: field(status_code) == 200
`)

	analysis, err := thtr.Analyze(source, thtr.AnalyzeOptions{Path: path})
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}

	if got, want := analysis.Spec.ID, "smoke"; got != want {
		t.Fatalf("stage id mismatch: got %q want %q", got, want)
	}
	if !strings.Contains(string(analysis.CanonicalYAML), "id: smoke") {
		t.Fatalf("canonical YAML missing stage id:\n%s", string(analysis.CanonicalYAML))
	}

	entry, ok := analysis.SourceMap.LookupSpecPath("stage.smoke/scenario.ping/act.get-health/action/binding.method")
	if !ok {
		t.Fatal("source map must contain action method binding")
	}
	if got, want := entry.Source.File, path; got != want {
		t.Fatalf("source map file mismatch: got %q want %q", got, want)
	}
	if got, want := entry.Source.StartLine, 5; got != want {
		t.Fatalf("source map line mismatch: got %d want %d", got, want)
	}
	if entry.YAML == nil {
		t.Fatal("source map entry must include YAML range")
	}

	roundtrip, ok := analysis.SourceMap.LookupYAMLPosition(entry.YAML.StartLine, entry.YAML.StartColumn)
	if !ok {
		t.Fatal("source map must support YAML position lookup")
	}
	if got, want := roundtrip.SpecPath, entry.SpecPath; got != want {
		t.Fatalf("YAML position lookup mismatch: got %q want %q", got, want)
	}

	childPath := "stage.smoke/scenario.ping/act.get-health/action/binding.url.parts[1]"
	if _, ok := analysis.SourceMap.LookupExactSpecPath(childPath); ok {
		t.Fatalf("source map must not contain exact synthetic child path %q", childPath)
	}
	fallback, ok := analysis.SourceMap.LookupSpecPath(childPath)
	if !ok {
		t.Fatalf("source map must find nearest ancestor for %q", childPath)
	}
	if got, want := fallback.SpecPath, "stage.smoke/scenario.ping/act.get-health/action/binding.url"; got != want {
		t.Fatalf("fallback source-map path mismatch: got %q want %q", got, want)
	}
	if got, want := fallback.Source.StartLine, 5; got != want {
		t.Fatalf("fallback source-map start line mismatch: got %d want %d", got, want)
	}
	if got, want := fallback.Source.StartColumn, 46; got != want {
		t.Fatalf("fallback source-map start column mismatch: got %d want %d", got, want)
	}
	if got, want := fallback.Source.EndLine, 5; got != want {
		t.Fatalf("fallback source-map end line mismatch: got %d want %d", got, want)
	}
	if got, want := fallback.Source.EndColumn, 60; got != want {
		t.Fatalf("fallback source-map end column mismatch: got %d want %d", got, want)
	}

	bundle, err := builtin.NewBundle()
	if err != nil {
		t.Fatalf("new bundle failed: %v", err)
	}
	diagnostics := analysis.RewriteDiagnostics(theater.NewValidator(bundle.Catalog, bundle.Matchers).Validate(analysis.Spec))
	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d diagnostics=%#v", got, want, diagnostics)
	}
	if got, want := diagnostics[0].Code, "invalid_eventually_interval"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostics[0].Span.Line, 4; got != want {
		t.Fatalf("diagnostic line mismatch: got %d want %d", got, want)
	}
}

func TestAnalyzeUsesLibraryOverlayForFlowTooling(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	flowPath := writePublicTHTRFile(t, repoRoot, filepath.Join("theater", "flows", "auth", "smoke.thtr"), `stage smoke

call login-user = auth/login()
`)
	libraryPath := writePublicTHTRFile(t, repoRoot, filepath.Join("theater", "lib", "auth", "login.thtr"), `stage auth-lib

scenario auth/login
  act submit
    eventually 2s every 1s
    do repeatable action.http(method: "GET", url: "/health")
    expect status-ok: field(status_code) == 200
`)

	overlay := []byte(`stage auth-lib

# unsaved editor overlay
scenario auth/login
  act submit
    eventually 1s every 1s
    do repeatable action.http(method: "GET", url: "/health")
    expect status-ok: field(status_code) == 200
`)
	analysis, err := thtr.Analyze([]byte(readPublicTHTRFile(t, flowPath)), thtr.AnalyzeOptions{
		Path: flowPath,
		LibraryOverlay: map[string][]byte{
			libraryPath: overlay,
		},
	})
	if err != nil {
		t.Fatalf("analyze flow failed: %v", err)
	}
	if !strings.Contains(string(analysis.CanonicalYAML), "scenario_calls:") {
		t.Fatalf("canonical flow YAML missing scenario calls:\n%s", string(analysis.CanonicalYAML))
	}
	if !strings.Contains(string(analysis.CanonicalYAML), "id: auth/login") {
		t.Fatalf("canonical flow YAML missing overlay-backed library scenario:\n%s", string(analysis.CanonicalYAML))
	}
	if !strings.Contains(string(analysis.CanonicalYAML), "timeout: 1s") {
		t.Fatalf("canonical flow YAML missing overlay-backed timeout:\n%s", string(analysis.CanonicalYAML))
	}

	bundle, err := builtin.NewBundle()
	if err != nil {
		t.Fatalf("new bundle failed: %v", err)
	}
	diagnostics := analysis.RewriteDiagnostics(theater.NewValidator(bundle.Catalog, bundle.Matchers).Validate(analysis.Spec))
	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d diagnostics=%#v", got, want, diagnostics)
	}
	if got, want := diagnostics[0].Span.File, libraryPath; got != want {
		t.Fatalf("diagnostic file mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostics[0].Span.Line, 6; got != want {
		t.Fatalf("diagnostic line mismatch: got %d want %d", got, want)
	}
}

func TestAnalyzeFileUsesReadPathByDefault(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "stage.thtr")
	if err := os.WriteFile(path, []byte(`stage smoke
scenario ping
  act get-health
    do action.http(method: "GET", url: "/health")
`), 0o600); err != nil {
		t.Fatalf("write fixture failed: %v", err)
	}

	analysis, err := thtr.AnalyzeFile(path, thtr.AnalyzeOptions{})
	if err != nil {
		t.Fatalf("analyze file failed: %v", err)
	}
	entry, ok := analysis.SourceMap.LookupExactSpecPath("stage.smoke/scenario.ping")
	if !ok {
		t.Fatal("source map must contain scenario entry")
	}
	if got, want := entry.Source.File, path; got != want {
		t.Fatalf("source map file mismatch: got %q want %q", got, want)
	}
}

func TestFormatFileAndTokenizeArePublicToolingAPIs(t *testing.T) {
	t.Parallel()

	formattedSource, err := thtr.Format([]byte(`stage smoke
scenario ping
  act get-health
    do action.http(method:"GET",url:"/health")
`))
	if err != nil {
		t.Fatalf("format source failed: %v", err)
	}
	if !strings.Contains(string(formattedSource), `method: "GET", url: "/health"`) {
		t.Fatalf("formatted source missing normalized call spacing:\n%s", string(formattedSource))
	}

	path := filepath.Join(t.TempDir(), "stage.thtr")
	if err := os.WriteFile(path, []byte(`stage smoke
scenario ping
  act get-health
    do action.http(method:"GET",url:"/health")
`), 0o600); err != nil {
		t.Fatalf("write fixture failed: %v", err)
	}

	formattedFile, err := thtr.FormatFile(path)
	if err != nil {
		t.Fatalf("format file failed: %v", err)
	}
	if !strings.Contains(string(formattedFile), `method: "GET", url: "/health"`) {
		t.Fatalf("formatted file missing normalized call spacing:\n%s", string(formattedFile))
	}

	tokens, err := thtr.Tokenize([]byte("stage smoke\n"))
	if err != nil {
		t.Fatalf("tokenize failed: %v", err)
	}
	if got, want := tokens[0].Text, "stage"; got != want {
		t.Fatalf("first token text mismatch: got %q want %q", got, want)
	}
	if got, want := tokens[0].Kind, thtr.TokenIdentifier; got != want {
		t.Fatalf("first token kind mismatch: got %q want %q", got, want)
	}
	if got, want := tokens[0].StartOffset, 0; got != want {
		t.Fatalf("first token start offset mismatch: got %d want %d", got, want)
	}
	if got, want := tokens[0].EndOffset, 5; got != want {
		t.Fatalf("first token end offset mismatch: got %d want %d", got, want)
	}
	if got, want := tokens[0].StartLine, 1; got != want {
		t.Fatalf("first token line mismatch: got %d want %d", got, want)
	}
	if got, want := tokens[0].EndColumn, 6; got != want {
		t.Fatalf("first token end column mismatch: got %d want %d", got, want)
	}
}

func TestAnalyzeReturnsPublicDiagnosticError(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "broken.thtr")
	_, err := thtr.Analyze([]byte("stage smoke\nscenario\n"), thtr.AnalyzeOptions{Path: path})
	if err == nil {
		t.Fatal("expected analyze error, got nil")
	}

	var diagnosticError *thtr.DiagnosticError
	if !errors.As(err, &diagnosticError) {
		t.Fatalf("expected public diagnostic error, got %T", err)
	}
	if got, want := diagnosticError.Code(), "thtr_parse_error"; got != want {
		t.Fatalf("diagnostic code accessor mismatch: got %q want %q", got, want)
	}
	diagnostic := diagnosticError.Diagnostic()
	if got, want := diagnostic.Code, "thtr_parse_error"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Span.File, path; got != want {
		t.Fatalf("diagnostic file mismatch: got %q want %q", got, want)
	}
	if got, want := diagnosticError.Span().File, path; got != want {
		t.Fatalf("diagnostic span accessor mismatch: got %q want %q", got, want)
	}
	if !strings.Contains(err.Error(), "thtr_parse_error") {
		t.Fatalf("diagnostic error string must include code: %q", err.Error())
	}
}

func TestToolingEntryPointsReturnPublicDiagnosticErrors(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "broken.thtr")
	if err := os.WriteFile(path, []byte("stage smoke\nscenario\n"), 0o600); err != nil {
		t.Fatalf("write fixture failed: %v", err)
	}
	_, err := thtr.FormatFile(path)
	if err == nil {
		t.Fatal("expected format file error, got nil")
	}
	formatDiagnostic := requirePublicDiagnosticError(t, err)
	if got, want := formatDiagnostic.Code, "thtr_parse_error"; got != want {
		t.Fatalf("format diagnostic code mismatch: got %q want %q", got, want)
	}
	if got, want := formatDiagnostic.Span.File, path; got != want {
		t.Fatalf("format diagnostic file mismatch: got %q want %q", got, want)
	}

	_, err = thtr.Tokenize([]byte("stage \"unterminated\n"))
	if err == nil {
		t.Fatal("expected tokenize error, got nil")
	}
	tokenizeDiagnostic := requirePublicDiagnosticError(t, err)
	if got, want := tokenizeDiagnostic.Code, "thtr_lex_error"; got != want {
		t.Fatalf("tokenize diagnostic code mismatch: got %q want %q", got, want)
	}
	if got, want := tokenizeDiagnostic.Span.Line, 1; got != want {
		t.Fatalf("tokenize diagnostic line mismatch: got %d want %d", got, want)
	}
}

func readPublicTHTRFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file failed: %v", err)
	}
	return string(data)
}

func requirePublicDiagnosticError(t *testing.T, err error) theater.Diagnostic {
	t.Helper()

	var diagnosticError *thtr.DiagnosticError
	if !errors.As(err, &diagnosticError) {
		t.Fatalf("expected public diagnostic error, got %T", err)
	}
	return diagnosticError.Diagnostic()
}
