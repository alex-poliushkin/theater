package yaml

import (
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
	flowPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "flows", "auth", "smoke.yaml"), `
id: smoke
scenarios:
  - id: setup
    acts:
      - id: prepare
        action:
          use: action.local
scenario_calls:
  - id: login-user
    scenario_id: auth/login
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "auth", "login.yaml"), `
id: auth-lib
scenarios:
  - id: auth/login
    acts:
      - id: submit
        action:
          use: action.http
  - id: auth/register
    acts:
      - id: register
        action:
          use: action.http
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "ops", "report.yaml"), `
id: ops-lib
scenarios:
  - id: ops/report
    acts:
      - id: collect
        action:
          use: action.http
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
	flowPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "flows", "auth", "smoke.yaml"), `
id: smoke
scenarios: []
scenario_calls:
  - id: call-b
    scenario_id: pkg/b
  - id: call-a
    scenario_id: pkg/a
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "pkg", "a.yaml"), `
id: pkg-a
scenarios:
  - id: pkg/a
    acts:
      - id: run
        action:
          use: action.http
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "pkg", "b.yaml"), `
id: pkg-b
scenarios:
  - id: pkg/b
    acts:
      - id: run
        action:
          use: action.http
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
	flowPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "flows", "auth", "smoke.yaml"), `
id: smoke
scenarios: []
scenario_calls:
  - id: login-user
    scenario_id: auth/login
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "auth", "register.yaml"), `
id: auth-lib
scenarios:
  - id: auth/register
    acts:
      - id: register
        action:
          use: action.http
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
	flowPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "flows", "auth", "smoke.yaml"), `
id: smoke
scenarios: []
scenario_calls:
  - id: login-user
    scenario_id: auth/login
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "auth", "login.yaml"), `
id: auth-lib
scenarios:
  - id: auth/login
    acts:
      - id: submit
        action:
          use: action.http
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "ops", "broken.yaml"), `
id: ops-lib
scenarios:
  - id: ops/report
    acts:
      - id: collect
        action:
          use: action.http
scenario_calls:
  - id: invalid
    scenario_id: ops/report
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
	flowPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "flows", "auth", "smoke.yaml"), `
id: smoke
scenarios: []
scenario_calls:
  - id: login-user
    scenario_id: auth/login
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "auth", "login.yaml"), `
id: auth-lib
scenarios:
  - id: auth/login
    acts:
      - id: submit
        action:
          use: action.http
scenario_calls:
  - id: invalid
    scenario_id: auth/login
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
	flowPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "flows", "auth", "smoke.yaml"), `
id: smoke
http:
  sessions:
    browser:
      use: http.session.browser
state:
  backends:
    local:
      use: state.backend.file
      with:
        root: /tmp/theater-state
scenarios: []
scenario_calls:
  - id: login-user
    scenario_id: auth/login
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "auth", "login.yaml"), `
id: auth-lib
scenarios:
  - id: auth/login
    acts:
      - id: submit
        action:
          use: action.http
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

func TestLoadFlowFilePreservesDynamicHTTPAuthBindings(t *testing.T) {
	t.Parallel()

	repoRoot := createFlowLoaderRepo(t)
	flowPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "flows", "service", "sample.yaml"), `
id: service-sample
http:
  auth:
    service_api:
      attach:
        - bearer:
            token_slot: session_token
scenarios:
  - id: service/sample-ready
    inputs:
      session_token:
        type: string
        required: true
        sensitivity: secret
        capture: omit
    auth_bindings:
      service_api:
        slots:
          session_token:
            kind: ref
            ref: session_token
    acts:
      - id: get-sample
        action:
          use: action.http
          with:
            method: GET
            url: https://api.example.test/sample
            session: none
            auth: service_api
scenario_calls:
  - id: run-sample
    scenario_id: service/sample-ready
    bindings:
      session_token: token-from-runtime
`)

	spec, err := LoadFlowFile(flowPath, flowLoaderTestMatcherResolver{})
	if err != nil {
		t.Fatalf("load flow file failed: %v", err)
	}

	bearer := spec.HTTP.Auth["service_api"].Attach[0].Bearer
	if bearer == nil {
		t.Fatal("service_api bearer attachment must be preserved")
	}
	if got, want := bearer.TokenSlot, "session_token"; got != want {
		t.Fatalf("bearer token slot mismatch: got %q want %q", got, want)
	}

	binding := spec.Scenarios[0].AuthBindings["service_api"].Slots["session_token"]
	if binding.Ref == nil {
		t.Fatal("auth binding slot ref must be preserved")
	}
	if got, want := binding.Ref.Name, "session_token"; got != want {
		t.Fatalf("auth binding ref mismatch: got %q want %q", got, want)
	}
}

func TestLoadFlowFileComposesSelectedLibrarySlotBackedHTTPAuth(t *testing.T) {
	t.Parallel()

	repoRoot := createFlowLoaderRepo(t)
	flowPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "flows", "service", "sample.yaml"), `
id: sample-flow
scenarios: []
scenario_calls:
  - id: run-sample
    scenario_id: service/sample-ready
    bindings:
      session_token: token-from-runtime
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "service", "sample.yaml"), `
id: service-lib
http:
  auth:
    service_api:
      attach:
        - bearer:
            token_slot: session_token
scenarios:
  - id: service/sample-ready
    inputs:
      session_token:
        type: string
        required: true
        sensitivity: secret
        capture: omit
    auth_bindings:
      service_api:
        slots:
          session_token:
            kind: ref
            ref: session_token
    acts:
      - id: get-sample
        action:
          use: action.http
          with:
            method: GET
            url: https://api.example.test/sample
            auth: service_api
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

func TestLoadFlowFileComposesSelectedLibraryAuthUsedByHTTPInventory(t *testing.T) {
	t.Parallel()

	repoRoot := createFlowLoaderRepo(t)
	flowPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "flows", "service", "sample.yaml"), `
id: sample-flow
scenarios: []
scenario_calls:
  - id: run-sample
    scenario_id: service/sample-ready
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "service", "sample.yaml"), `
id: service-lib
http:
  auth:
    service_api:
      attach:
        - header_slot:
            name: X-Session-Token
            slot: session_token
scenarios:
  - id: service/sample-ready
    acts:
      - id: load-sample
        properties:
          sample:
            inventory:
              use: inventory.http.get
              with:
                url: https://api.example.test/sample
                auth: service_api
        action:
          use: action.generate
          with:
            outputs:
              ok: true
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
	flowPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "flows", "service", "sample.yaml"), `
id: sample-flow
scenarios: []
scenario_calls:
  - id: run-sample
    scenario_id: service/sample-ready
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "service", "sample.yaml"), `
id: service-lib
scenarios:
  - id: service/sample-ready
    acts:
      - id: get-sample
        action:
          use: action.http
          with:
            method: GET
            url: https://api.example.test/sample
            auth: service_api
`)

	spec, err := LoadFlowFile(flowPath, flowLoaderTestMatcherResolver{})
	if err != nil {
		t.Fatalf("load flow file failed: %v", err)
	}

	diagnostics := theater.NewValidator(nil, nil).Validate(spec)
	if !hasDiagnosticCode(diagnostics, "unknown_http_auth_ref") {
		t.Fatalf("expected unknown_http_auth_ref diagnostic, got %#v", diagnostics)
	}
}

func TestLoadFlowFileIgnoresUnselectedLibraryHTTPAuth(t *testing.T) {
	t.Parallel()

	repoRoot := createFlowLoaderRepo(t)
	flowPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "flows", "service", "sample.yaml"), `
id: sample-flow
scenarios: []
scenario_calls:
  - id: run-sample
    scenario_id: service/sample-ready
    bindings:
      session_token: token-from-runtime
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "service", "sample.yaml"), `
id: service-lib
http:
  auth:
    service_api:
      attach:
        - bearer:
            token_slot: session_token
scenarios:
  - id: service/sample-ready
    inputs:
      session_token:
        type: string
        required: true
    auth_bindings:
      service_api:
        slots:
          session_token:
            kind: ref
            ref: session_token
    acts:
      - id: get-sample
        action:
          use: action.http
          with:
            auth: service_api
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "ops", "report.yaml"), `
id: ops-lib
http:
  auth:
    service_api:
      attach:
        - bearer:
            token: unselected-secret
scenarios:
  - id: ops/report
    acts:
      - id: collect
        action:
          use: action.http
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
	flowPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "flows", "service", "sample.yaml"), `
id: sample-flow
scenarios: []
scenario_calls:
  - id: run-sample
    scenario_id: service/sample-ready
  - id: run-check
    scenario_id: check/sample-ready
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "service", "sample.yaml"), `
id: service-lib
http:
  auth:
    service_api:
      attach:
        - bearer:
            token_slot: session_token
scenarios:
  - id: service/sample-ready
    auth_bindings:
      service_api:
        slots:
          session_token:
            kind: ref
            ref: session_token
    acts:
      - id: get-sample
        action:
          use: action.http
          with:
            auth: service_api
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "check", "sample.yaml"), `
id: check-lib
http:
  auth:
    service_api:
      attach:
        - bearer:
            token_slot: session_token
scenarios:
  - id: check/sample-ready
    auth_bindings:
      service_api:
        slots:
          session_token:
            kind: ref
            ref: session_token
    acts:
      - id: check-sample
        action:
          use: action.http
          with:
            auth: service_api
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
	flowPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "flows", "service", "sample.yaml"), `
id: sample-flow
http:
  auth:
    service_api:
      attach:
        - bearer:
            token_slot: session_token
scenarios: []
scenario_calls:
  - id: run-sample
    scenario_id: service/sample-ready
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "service", "sample.yaml"), `
id: service-lib
http:
  auth:
    service_api:
      attach:
        - bearer:
            token_slot: session_token
scenarios:
  - id: service/sample-ready
    auth_bindings:
      service_api:
        slots:
          session_token:
            kind: ref
            ref: session_token
    acts:
      - id: get-sample
        action:
          use: action.http
          with:
            auth: service_api
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
	flowPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "flows", "service", "sample.yaml"), `
id: sample-flow
scenarios: []
scenario_calls:
  - id: run-sample
    scenario_id: service/sample-ready
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "service", "sample.yaml"), `
id: service-lib
http:
  auth:
    service_api:
      attach:
        - bearer:
            token: selected-library-secret
scenarios:
  - id: service/sample-ready
    acts:
      - id: get-sample
        action:
          use: action.http
          with:
            auth: service_api
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

func TestLoadFlowFileRejectsSelectedLibraryStaticHTTPAuthDeterministically(t *testing.T) {
	t.Parallel()

	repoRoot := createFlowLoaderRepo(t)
	flowPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "flows", "service", "sample.yaml"), `
id: sample-flow
scenarios: []
scenario_calls:
  - id: run-sample
    scenario_id: service/sample-ready
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "service", "sample.yaml"), `
id: service-lib
http:
  auth:
    zeta_api:
      attach:
        - bearer:
            token: zeta-secret
    alpha_api:
      attach:
        - bearer:
            token: alpha-secret
scenarios:
  - id: service/sample-ready
    acts:
      - id: get-sample
        action:
          use: action.http
`)

	_, err := LoadFlowFile(flowPath, flowLoaderTestMatcherResolver{})
	if err == nil {
		t.Fatal("expected selected library static auth error, got nil")
	}

	errtest.RequireContains(t, err, `selected library http auth "alpha_api" must use slot-backed attachments`)
	if strings.Contains(err.Error(), "alpha-secret") || strings.Contains(err.Error(), "zeta-secret") {
		t.Fatalf("static credential value leaked in error: %v", err)
	}
}

func TestLoadFlowFileRejectsUnusedSelectedLibraryStaticHTTPAuth(t *testing.T) {
	t.Parallel()

	repoRoot := createFlowLoaderRepo(t)
	flowPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "flows", "service", "sample.yaml"), `
id: sample-flow
scenarios: []
scenario_calls:
  - id: run-sample
    scenario_id: service/sample-ready
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "service", "sample.yaml"), `
id: service-lib
http:
  auth:
    service_api:
      attach:
        - bearer:
            token: selected-library-secret
scenarios:
  - id: service/sample-ready
    acts:
      - id: get-sample
        action:
          use: action.http
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
	flowPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "flows", "auth", "smoke.yaml"), `
id: smoke
scenarios: []
scenario_calls:
  - id: login-user
    scenario_id: auth/login
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "auth", "login-a.yaml"), `
id: auth-lib-a
scenarios:
  - id: auth/login
    acts:
      - id: submit
        action:
          use: action.http
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "auth", "login-b.yaml"), `
id: auth-lib-b
scenarios:
  - id: auth/login
    acts:
      - id: submit
        action:
          use: action.http
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

func hasDiagnosticCode(diagnostics []theater.Diagnostic, code string) bool {
	for i := range diagnostics {
		if diagnostics[i].Code == code {
			return true
		}
	}
	return false
}
