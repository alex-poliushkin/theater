package thtr

import (
	"path/filepath"
	"testing"

	"github.com/alex-poliushkin/theater"
)

func TestLoadFlowSourceDetailedUsesOverlayFlowBuffer(t *testing.T) {
	t.Parallel()

	repoRoot := createFlowLoaderRepo(t)
	flowPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "flows", "auth", "smoke.thtr"), `stage smoke
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "auth", "login.thtr"), `stage auth-lib

scenario auth/login
  act submit
    do action.http()
`)

	result, err := LoadFlowSourceDetailed(flowPath, []byte(`stage smoke

call login-user = auth/login()
`), nil)
	if err != nil {
		t.Fatalf("load flow source detailed failed: %v", err)
	}

	if got, want := len(result.Spec.ScenarioCalls), 1; got != want {
		t.Fatalf("scenario call count mismatch: got %d want %d", got, want)
	}
	if got, want := scenarioIDs(result.Spec.Scenarios), []string{"auth/login"}; !equalStrings(got, want) {
		t.Fatalf("assembled scenarios mismatch: got %v want %v", got, want)
	}
}

func TestAnalyzePathDetailedRewritesLibraryDiagnosticsForOverlayFlow(t *testing.T) {
	t.Parallel()

	repoRoot := createFlowLoaderRepo(t)
	flowPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "flows", "auth", "smoke.thtr"), `stage smoke
`)
	libraryPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "auth", "login.thtr"), `stage auth-lib

scenario auth/login
  act submit
    eventually 1s every 1s
    do repeatable action.http(method: "GET", url: "/health")
`)

	result, err := AnalyzePathDetailed(flowPath, []byte(`stage smoke

call login-user = auth/login()
`), nil)
	if err != nil {
		t.Fatalf("analyze path detailed failed: %v", err)
	}

	validator := theater.NewValidator(nil, nil)
	diagnostics := result.RewriteDiagnostics(validator.Validate(result.Spec))
	diagnostic := findDiagnosticByCodeValue(diagnostics, "invalid_eventually_interval")
	if diagnostic == nil {
		t.Fatalf("expected invalid_eventually_interval diagnostic, got %#v", diagnostics)
	}
	if got, want := diagnostic.Span.File, libraryPath; got != want {
		t.Fatalf("diagnostic source file mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Span.Line, 5; got != want {
		t.Fatalf("diagnostic source line mismatch: got %d want %d", got, want)
	}
}

func TestLoadFlowSourceDetailedUsesOverlayFlowBufferWithStateErgonomicsLibrary(t *testing.T) {
	t.Parallel()

	repoRoot := createFlowLoaderRepo(t)
	flowPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "flows", "state", "smoke.thtr"), `stage smoke
`)
	writeFlowLoaderFile(
		t,
		repoRoot,
		filepath.Join("theater", "lib", "state", "verify.thtr"),
		string(readStateErgonomicsFixture(t, "success-input.thtr")),
	)

	result, err := LoadFlowSourceDetailed(flowPath, []byte(`stage smoke

call verify = verify-state()
`), nil)
	if err != nil {
		t.Fatalf("load flow source detailed failed: %v", err)
	}

	if got, want := scenarioIDs(result.Spec.Scenarios), []string{"verify-state"}; !equalStrings(got, want) {
		t.Fatalf("assembled scenarios mismatch: got %v want %v", got, want)
	}
	if got, want := result.Spec.Scenarios[0].Acts[0].Action.Use, "action.state.read"; got != want {
		t.Fatalf("read action use mismatch: got %q want %q", got, want)
	}
	if got, want := result.Spec.Scenarios[0].Acts[2].Action.Use, "action.state.claim"; got != want {
		t.Fatalf("claim action use mismatch: got %q want %q", got, want)
	}
}

func TestTokenizeExposesLexerTokenSpans(t *testing.T) {
	t.Parallel()

	tokens, err := Tokenize([]byte("stage smoke\n"))
	if err != nil {
		t.Fatalf("tokenize failed: %v", err)
	}
	if len(tokens) < 2 {
		t.Fatalf("expected at least two tokens, got %d", len(tokens))
	}

	if got, want := tokens[0].Kind, "identifier"; got != want {
		t.Fatalf("first token kind mismatch: got %q want %q", got, want)
	}
	if got, want := tokens[0].Text, "stage"; got != want {
		t.Fatalf("first token text mismatch: got %q want %q", got, want)
	}
	if got, want := tokens[0].StartOffset, 0; got != want {
		t.Fatalf("first token start offset mismatch: got %d want %d", got, want)
	}
	if got, want := tokens[0].StartLine, 1; got != want {
		t.Fatalf("first token start line mismatch: got %d want %d", got, want)
	}
	if got, want := tokens[1].Text, "smoke"; got != want {
		t.Fatalf("second token text mismatch: got %q want %q", got, want)
	}
}
