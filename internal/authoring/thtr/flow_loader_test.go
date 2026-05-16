package thtr

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/internal/testkit/errtest"
)

func TestLoadFlowFileAssemblesOnlyReferencedLibraryScenarios(t *testing.T) {
	t.Parallel()

	repoRoot := createFlowLoaderRepo(t)
	flowPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "flows", "auth", "smoke.thtr"), `stage smoke

scenario setup
  act prepare
    do action.local()

call login-user = auth/login()
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "auth", "login.thtr"), `stage auth-lib

scenario auth/login
  act submit
    do action.http()

scenario auth/register
  act register
    do action.http()
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "ops", "report.thtr"), `stage ops-lib

scenario ops/report
  act collect
    do action.http()
`)

	spec, err := LoadFlowFile(flowPath, flowLoaderTestMatcherResolver{})
	if err != nil {
		t.Fatalf("load flow file failed: %v", err)
	}

	if got, want := scenarioIDs(spec.Scenarios), []string{"setup", "auth/login"}; !equalStrings(got, want) {
		t.Fatalf("assembled scenarios mismatch: got %v want %v", got, want)
	}
}

func TestLoadFlowFilePreservesDeterministicSelectedLibraryOrder(t *testing.T) {
	t.Parallel()

	repoRoot := createFlowLoaderRepo(t)
	flowPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "flows", "auth", "smoke.thtr"), `stage smoke

call call-b = pkg/b()
call call-a = pkg/a()
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "pkg", "a.thtr"), `stage pkg-a

scenario pkg/a
  act run
    do action.http()
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "pkg", "b.thtr"), `stage pkg-b

scenario pkg/b
  act run
    do action.http()
`)

	spec, err := LoadFlowFile(flowPath, flowLoaderTestMatcherResolver{})
	if err != nil {
		t.Fatalf("load flow file failed: %v", err)
	}

	if got, want := scenarioIDs(spec.Scenarios), []string{"pkg/a", "pkg/b"}; !equalStrings(got, want) {
		t.Fatalf("selected library order mismatch: got %v want %v", got, want)
	}
}

func TestLoadFlowFileFailsWhenReferencedLibraryScenarioIsMissing(t *testing.T) {
	t.Parallel()

	repoRoot := createFlowLoaderRepo(t)
	flowPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "flows", "auth", "smoke.thtr"), `stage smoke

call login-user = auth/login()
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "auth", "register.thtr"), `stage auth-lib

scenario auth/register
  act register
    do action.http()
`)

	_, err := LoadFlowFile(flowPath, flowLoaderTestMatcherResolver{})
	if err == nil {
		t.Fatal("expected missing library scenario error, got nil")
	}

	errtest.RequireContains(t, err, `referenced library scenario "auth/login" is not found`)
}

func TestLoadFlowFileIgnoresUnrelatedInvalidLibraryFiles(t *testing.T) {
	t.Parallel()

	repoRoot := createFlowLoaderRepo(t)
	flowPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "flows", "auth", "smoke.thtr"), `stage smoke

call login-user = auth/login()
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "auth", "login.thtr"), `stage auth-lib

scenario auth/login
  act submit
    do action.http()
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "ops", "broken.thtr"), `stage ops-lib

scenario ops/report
  act collect
    do action.http()

call invalid = ops/report()
`)

	spec, err := LoadFlowFile(flowPath, flowLoaderTestMatcherResolver{})
	if err != nil {
		t.Fatalf("load flow file failed: %v", err)
	}

	if got, want := scenarioIDs(spec.Scenarios), []string{"auth/login"}; !equalStrings(got, want) {
		t.Fatalf("assembled scenarios mismatch: got %v want %v", got, want)
	}
}

func TestLoadFlowFileFailsWhenReferencedLibraryFileDeclaresScenarioCalls(t *testing.T) {
	t.Parallel()

	repoRoot := createFlowLoaderRepo(t)
	flowPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "flows", "auth", "smoke.thtr"), `stage smoke

call login-user = auth/login()
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "auth", "login.thtr"), `stage auth-lib

scenario auth/login
  act submit
    do action.http()

call invalid = auth/login()
`)

	_, err := LoadFlowFile(flowPath, flowLoaderTestMatcherResolver{})
	if err == nil {
		t.Fatal("expected invalid referenced library file error, got nil")
	}

	errtest.RequireContains(t, err, "must not declare scenario_calls")
}

func TestLoadFlowFilePreservesFlowHTTPAndState(t *testing.T) {
	t.Parallel()

	repoRoot := createFlowLoaderRepo(t)
	flowPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "flows", "auth", "smoke.thtr"), `stage smoke

http
  session browser = http.session.browser()

state
  backend local = state.backend.file(root: "/tmp/theater-state")

call login-user = auth/login()
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "auth", "login.thtr"), `stage auth-lib

scenario auth/login
  act submit
    do action.http()
`)

	spec, err := LoadFlowFile(flowPath, flowLoaderTestMatcherResolver{})
	if err != nil {
		t.Fatalf("load flow file failed: %v", err)
	}

	if spec.HTTP == nil {
		t.Fatal("flow http must be preserved")
	}
	if _, ok := spec.HTTP.Sessions["browser"]; !ok {
		t.Fatal("flow http session browser must be preserved")
	}
	if spec.State == nil {
		t.Fatal("flow state must be preserved")
	}
	if got, want := spec.State.Backends["local"].Use, "state.backend.file"; got != want {
		t.Fatalf("state backend use mismatch: got %q want %q", got, want)
	}
}

func TestLoadFlowFileComposesSelectedLibrarySlotBackedHTTPAuth(t *testing.T) {
	t.Parallel()

	repoRoot := createFlowLoaderRepo(t)
	flowPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "flows", "service", "sample.thtr"), `stage sample-flow

call run-sample = service/sample-ready(session_token: "token-from-runtime")
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "service", "sample.thtr"), `stage service-lib

http
  auth service_api = http.auth
    attach: list [
      object { bearer: object { token_slot: "session_token" } }
    ]

scenario service/sample-ready(session_token: string!)
  bind auth service_api
    session_token: $session_token
  act get-sample
    do action.http(auth: "service_api")
`)

	spec, err := LoadFlowFile(flowPath, flowLoaderTestMatcherResolver{})
	if err != nil {
		t.Fatalf("load flow file failed: %v", err)
	}

	if spec.HTTP == nil {
		t.Fatal("selected library auth must create assembled http registry")
	}
	bearer := spec.HTTP.Auth["service_api"].Attach[0].Bearer
	if bearer == nil {
		t.Fatal("service_api bearer attachment must be composed")
	}
	if got, want := bearer.TokenSlot, "session_token"; got != want {
		t.Fatalf("bearer token slot mismatch: got %q want %q", got, want)
	}
}

func TestLoadFlowFileDetailedRewritesSelectedLibraryHTTPAuthSourceMapYAMLRange(t *testing.T) {
	t.Parallel()

	repoRoot := createFlowLoaderRepo(t)
	flowPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "flows", "service", "sample.thtr"), `stage sample-flow

call run-sample = service/sample-ready(session_token: "token-from-runtime")
`)
	libraryPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "service", "sample.thtr"), `stage service-lib

http
  auth service_api = http.auth
    attach: list [
      object { bearer: object { token_slot: "session_token" } }
    ]

scenario service/sample-ready(session_token: string!)
  bind auth service_api
    session_token: $session_token
  act get-sample
    do action.http(method: "GET", url: "https://api.example.test/sample", auth: "service_api")
`)

	result, err := LoadFlowFileDetailed(flowPath, flowLoaderTestMatcherResolver{})
	if err != nil {
		t.Fatalf("load flow file detailed failed: %v", err)
	}

	entry, ok := result.sourceMap.LookupExactSpecPath("stage.sample-flow/http/auth.service_api/attach[0]")
	if !ok {
		t.Fatalf("selected library auth attachment source map entry is missing: %#v", result.sourceMap)
	}
	if got, want := entry.Source.File, libraryPath; got != want {
		t.Fatalf("source map source file mismatch: got %q want %q", got, want)
	}
	if entry.YAML == nil {
		t.Fatal("selected library auth attachment must have assembled YAML range")
	}
	if entry.YAML.StartLine == 1 && entry.YAML.EndLine > 10 {
		t.Fatalf("selected library auth attachment YAML range must not cover the full document: %#v", entry.YAML)
	}
}

func TestLoadFlowFileComposesSelectedLibraryAuthUsedByHTTPInventory(t *testing.T) {
	t.Parallel()

	repoRoot := createFlowLoaderRepo(t)
	flowPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "flows", "service", "sample.thtr"), `stage sample-flow

call run-sample = service/sample-ready()
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "service", "sample.thtr"), `stage service-lib

http
  auth service_api = http.auth
    attach: list [
      object { header_slot: object { name: "X-Session-Token", slot: "session_token" } }
    ]

scenario service/sample-ready
  act load-sample
    prop sample = inventory.http.get(url: "https://api.example.test/sample", auth: "service_api")
    do action.generate(outputs: object { ok: true })
`)

	spec, err := LoadFlowFile(flowPath, flowLoaderTestMatcherResolver{})
	if err != nil {
		t.Fatalf("load flow file failed: %v", err)
	}

	if spec.HTTP == nil {
		t.Fatal("selected inventory auth must create assembled http registry")
	}
	headerSlot := spec.HTTP.Auth["service_api"].Attach[0].HeaderSlot
	if headerSlot == nil {
		t.Fatal("service_api header slot attachment must be composed")
	}
	if got, want := headerSlot.Slot, "session_token"; got != want {
		t.Fatalf("header slot mismatch: got %q want %q", got, want)
	}
}

func TestLoadFlowFileLeavesUndeclaredSelectedLibraryAuthForValidation(t *testing.T) {
	t.Parallel()

	repoRoot := createFlowLoaderRepo(t)
	flowPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "flows", "service", "sample.thtr"), `stage sample-flow

call run-sample = service/sample-ready()
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "service", "sample.thtr"), `stage service-lib

scenario service/sample-ready
  act get-sample
    do action.http(method: "GET", url: "https://api.example.test/sample", auth: "service_api")
`)

	spec, err := LoadFlowFile(flowPath, flowLoaderTestMatcherResolver{})
	if err != nil {
		t.Fatalf("load flow file failed: %v", err)
	}

	diagnostics := theater.NewValidator(nil, nil).Validate(spec)
	if diagnostic := findDiagnosticByCodeValue(diagnostics, "unknown_http_auth_ref"); diagnostic == nil {
		t.Fatalf("expected unknown_http_auth_ref diagnostic, got %#v", diagnostics)
	}
}

func TestLoadFlowFileDetailedRewritesSelectedLibraryHTTPAuthDiagnostics(t *testing.T) {
	t.Parallel()

	repoRoot := createFlowLoaderRepo(t)
	flowPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "flows", "service", "sample.thtr"), `stage sample-flow

call run-sample = service/sample-ready()
`)
	libraryPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "service", "sample.thtr"), `stage service-lib

http
  auth service_api = http.auth
    attach: list [
      object { header_slot: object { name: "X-Session-Token", slot: "session_token" } },
      object { header_slot: object { name: "X-Session-Token", slot: "backup_token" } }
    ]

scenario service/sample-ready
  act get-sample
    do action.http(auth: "service_api")
`)

	result, err := LoadFlowFileDetailed(flowPath, flowLoaderTestMatcherResolver{})
	if err != nil {
		t.Fatalf("load flow file detailed failed: %v", err)
	}

	validator := theater.NewValidator(nil, nil)
	diagnostics := result.RewriteDiagnostics(validator.Validate(result.Spec))
	diagnostic := findDiagnosticByCodeValue(diagnostics, "duplicate_http_auth_header")
	if diagnostic == nil {
		t.Fatalf("expected duplicate_http_auth_header diagnostic, got %#v", diagnostics)
	}
	if got, want := diagnostic.Path, "stage.sample-flow/http/auth.service_api/attach[1]"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Span.File, libraryPath; got != want {
		t.Fatalf("diagnostic source file mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Span.Line, 7; got != want {
		t.Fatalf("diagnostic source line mismatch: got %d want %d", got, want)
	}
}

func TestLoadFlowFileIgnoresUnselectedLibraryHTTPAuth(t *testing.T) {
	t.Parallel()

	repoRoot := createFlowLoaderRepo(t)
	flowPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "flows", "service", "sample.thtr"), `stage sample-flow

call run-sample = service/sample-ready(session_token: "token-from-runtime")
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "service", "sample.thtr"), `stage service-lib

http
  auth service_api = http.auth
    attach: list [
      object { bearer: object { token_slot: "session_token" } }
    ]

scenario service/sample-ready(session_token: string!)
  bind auth service_api
    session_token: $session_token
  act get-sample
    do action.http(auth: "service_api")
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "ops", "report.thtr"), `stage ops-lib

http
  auth service_api = http.auth
    attach: list [
      object { bearer: object { token: "unselected-secret" } }
    ]

scenario ops/report
  act collect
    do action.http()
`)

	spec, err := LoadFlowFile(flowPath, flowLoaderTestMatcherResolver{})
	if err != nil {
		t.Fatalf("load flow file failed: %v", err)
	}

	bearer := spec.HTTP.Auth["service_api"].Attach[0].Bearer
	if got, want := bearer.TokenSlot, "session_token"; got != want {
		t.Fatalf("selected auth mismatch: got token_slot %q want %q", got, want)
	}
}

func TestLoadFlowFileRejectsDuplicateSelectedLibraryHTTPAuth(t *testing.T) {
	t.Parallel()

	repoRoot := createFlowLoaderRepo(t)
	flowPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "flows", "service", "sample.thtr"), `stage sample-flow

call run-sample = service/sample-ready()
call run-check = check/sample-ready()
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "service", "sample.thtr"), `stage service-lib

http
  auth service_api = http.auth
    attach: list [
      object { bearer: object { token_slot: "session_token" } }
    ]

scenario service/sample-ready
  bind auth service_api
    session_token: "token-from-runtime"
  act get-sample
    do action.http(auth: "service_api")
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "check", "sample.thtr"), `stage check-lib

http
  auth service_api = http.auth
    attach: list [
      object { bearer: object { token_slot: "session_token" } }
    ]

scenario check/sample-ready
  bind auth service_api
    session_token: "token-from-runtime"
  act check-sample
    do action.http(auth: "service_api")
`)

	_, err := LoadFlowFile(flowPath, flowLoaderTestMatcherResolver{})
	if err == nil {
		t.Fatal("expected duplicate selected library auth error, got nil")
	}

	errtest.RequireContains(t, err, `http auth "service_api" is declared by multiple selected library files`)
}

func TestLoadFlowFileRejectsFlowAndLibraryHTTPAuthDuplicate(t *testing.T) {
	t.Parallel()

	repoRoot := createFlowLoaderRepo(t)
	flowPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "flows", "service", "sample.thtr"), `stage sample-flow

http
  auth service_api = http.auth
    attach: list [
      object { bearer: object { token_slot: "session_token" } }
    ]

call run-sample = service/sample-ready()
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "service", "sample.thtr"), `stage service-lib

http
  auth service_api = http.auth
    attach: list [
      object { bearer: object { token_slot: "session_token" } }
    ]

scenario service/sample-ready
  bind auth service_api
    session_token: "token-from-runtime"
  act get-sample
    do action.http(auth: "service_api")
`)

	_, err := LoadFlowFile(flowPath, flowLoaderTestMatcherResolver{})
	if err == nil {
		t.Fatal("expected flow/library auth duplicate error, got nil")
	}

	errtest.RequireContains(t, err, `http auth "service_api" is declared by both flow and selected library file`)
}

func TestLoadFlowFileRejectsSelectedLibraryStaticHTTPAuth(t *testing.T) {
	t.Parallel()

	repoRoot := createFlowLoaderRepo(t)
	flowPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "flows", "service", "sample.thtr"), `stage sample-flow

call run-sample = service/sample-ready()
`)
	libraryPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "service", "sample.thtr"), `stage service-lib

http
  auth service_api = http.auth
    attach: list [
      object { bearer: object { token: "selected-library-secret" } }
    ]

scenario service/sample-ready
  act get-sample
    do action.http(auth: "service_api")
`)

	_, err := LoadFlowFile(flowPath, flowLoaderTestMatcherResolver{})
	if err == nil {
		t.Fatal("expected selected library static auth error, got nil")
	}

	errtest.RequireContains(t, err, `selected library http auth "service_api" must use slot-backed attachments`)
	if strings.Contains(err.Error(), "selected-library-secret") {
		t.Fatalf("static credential value leaked in error: %v", err)
	}

	var diagnosticErr *DiagnosticError
	if !errors.As(err, &diagnosticErr) {
		t.Fatalf("expected diagnostic error, got %T", err)
	}
	diagnostic := diagnosticErr.Diagnostic()
	if got, want := diagnostic.Code, "invalid_selected_library_http_auth"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Path, "stage.service-lib/http/auth.service_api/attach[0]"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Span.File, libraryPath; got != want {
		t.Fatalf("diagnostic source file mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Span.Line, 6; got != want {
		t.Fatalf("diagnostic source line mismatch: got %d want %d", got, want)
	}
}

func TestLoadFlowFileRejectsUnusedSelectedLibraryStaticHTTPAuth(t *testing.T) {
	t.Parallel()

	repoRoot := createFlowLoaderRepo(t)
	flowPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "flows", "service", "sample.thtr"), `stage sample-flow

call run-sample = service/sample-ready()
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "service", "sample.thtr"), `stage service-lib

http
  auth service_api = http.auth
    attach: list [
      object { bearer: object { token: "selected-library-secret" } }
    ]

scenario service/sample-ready
  act get-sample
    do action.http()
`)

	_, err := LoadFlowFile(flowPath, flowLoaderTestMatcherResolver{})
	if err == nil {
		t.Fatal("expected selected library static auth error, got nil")
	}

	errtest.RequireContains(t, err, `selected library http auth "service_api" must use slot-backed attachments`)
	if strings.Contains(err.Error(), "selected-library-secret") {
		t.Fatalf("static credential value leaked in error: %v", err)
	}
}

func TestLoadFlowFileRejectsDuplicateLibraryScenarioIDs(t *testing.T) {
	t.Parallel()

	repoRoot := createFlowLoaderRepo(t)
	flowPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "flows", "auth", "smoke.thtr"), `stage smoke

call login-user = auth/login()
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "auth", "login-a.thtr"), `stage auth-lib-a

scenario auth/login
  act submit
    do action.http()
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "auth", "login-b.thtr"), `stage auth-lib-b

scenario auth/login
  act submit
    do action.http()
`)

	_, err := LoadFlowFile(flowPath, flowLoaderTestMatcherResolver{})
	if err == nil {
		t.Fatal("expected duplicate library scenario id error, got nil")
	}

	errtest.RequireContains(t, err, `library scenario "auth/login" is declared in multiple files`)
}

type flowLoaderTestMatcherResolver struct{}

func (flowLoaderTestMatcherResolver) ResolveSugarKey(key string) (theater.MatcherDescriptor, error) {
	return theater.MatcherDescriptor{}, fmt.Errorf("unexpected matcher sugar lookup %q", key)
}

func createFlowLoaderRepo(t *testing.T) string {
	t.Helper()

	repoRoot := t.TempDir()
	for _, dir := range []string{
		filepath.Join(repoRoot, "theater", "flows"),
		filepath.Join(repoRoot, "theater", "lib"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s failed: %v", dir, err)
		}
	}

	return repoRoot
}

func writeFlowLoaderFile(t *testing.T, repoRoot, relativePath, contents string) string {
	t.Helper()

	path := filepath.Join(repoRoot, relativePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir parent failed: %v", err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimSpace(contents)+"\n"), 0o600); err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	return path
}

func scenarioIDs(scenarios []theater.ScenarioSpec) []string {
	ids := make([]string, 0, len(scenarios))
	for i := range scenarios {
		ids = append(ids, scenarios[i].ID)
	}
	return ids
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}

	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}

	return true
}
