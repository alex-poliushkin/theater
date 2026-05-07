package theatercli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/builtin"
	"github.com/alex-poliushkin/theater/internal/testkit/errtest"
)

const stageLoaderScenarioCallsFragment = "must not declare scenario_calls"

func TestStageFileLoaderUsesRepoFlowResolutionForResolvedRelativePath(t *testing.T) {
	t.Parallel()

	matchers, err := builtin.Matchers()
	if err != nil {
		t.Fatalf("new matcher catalog failed: %v", err)
	}

	repoRoot := createCLIFlowRepo(t)
	flowDir := filepath.Join(repoRoot, "theater", "flows", "auth")
	flowPath := writeCLIRepoFile(t, repoRoot, filepath.Join("theater", "flows", "auth", "login-smoke.yaml"), `
id: login-smoke
scenarios: []
scenario_calls:
  - id: smoke-login
    scenario_id: auth/login
`)
	writeCLIRepoFile(t, repoRoot, filepath.Join("theater", "lib", "auth", "login.yaml"), `
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

	restore := chdirForTest(t, flowDir)
	defer restore()

	_, err = newStageFileLoader(matchers).Load("./login-smoke.yaml")
	if err == nil {
		t.Fatal("expected repo-aware loader error, got nil")
	}

	errtest.RequireContains(t, err, stageLoaderScenarioCallsFragment)

	if _, statErr := os.Stat(flowPath); statErr != nil {
		t.Fatalf("expected flow file to exist: %v", statErr)
	}
}

func TestStageFileLoaderLoadsStandaloneTHTRWithDiagnosticRewrite(t *testing.T) {
	t.Parallel()

	matchers, err := builtin.Matchers()
	if err != nil {
		t.Fatalf("new matcher catalog failed: %v", err)
	}

	path := filepath.Join(t.TempDir(), "stage.thtr")
	if err := os.WriteFile(path, []byte(`stage main
scenario login
  act submit
    eventually 1s every 1s
    do repeatable action.http(method: "GET", url: "/health")
    expect status: field(status_code) == 200
`), 0o600); err != nil {
		t.Fatalf("write fixture failed: %v", err)
	}

	loaded, err := newStageFileLoader(matchers).Load(path)
	if err != nil {
		t.Fatalf("load stage failed: %v", err)
	}

	validator := theater.NewValidator(nil, nil)
	diagnostics := loaded.RewriteDiagnostics(validator.Validate(loaded.Spec))
	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}
	if got, want := diagnostics[0].Span.File, path; got != want {
		t.Fatalf("diagnostic source file mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostics[0].Span.Line, 4; got != want {
		t.Fatalf("diagnostic source line mismatch: got %d want %d", got, want)
	}
}

func TestStageFileLoaderUsesRepoFlowResolutionForResolvedRelativeTHTRPath(t *testing.T) {
	t.Parallel()

	matchers, err := builtin.Matchers()
	if err != nil {
		t.Fatalf("new matcher catalog failed: %v", err)
	}

	repoRoot := createCLIFlowRepo(t)
	flowDir := filepath.Join(repoRoot, "theater", "flows", "auth")
	flowPath := writeCLIRepoFile(t, repoRoot, filepath.Join("theater", "flows", "auth", "login-smoke.thtr"), `stage login-smoke

call smoke-login = auth/login()
`)
	writeCLIRepoFile(t, repoRoot, filepath.Join("theater", "lib", "auth", "login.thtr"), `stage auth-lib

scenario auth/login
  act submit
    do action.http()

call invalid = auth/login()
`)

	restore := chdirForTest(t, flowDir)
	defer restore()

	_, err = newStageFileLoader(matchers).Load("./login-smoke.thtr")
	if err == nil {
		t.Fatal("expected repo-aware .thtr loader error, got nil")
	}

	errtest.RequireContains(t, err, stageLoaderScenarioCallsFragment)

	if _, statErr := os.Stat(flowPath); statErr != nil {
		t.Fatalf("expected flow file to exist: %v", statErr)
	}
}

func TestStageFileLoaderRewritesRepoFlowTHTRDiagnostics(t *testing.T) {
	t.Parallel()

	matchers, err := builtin.Matchers()
	if err != nil {
		t.Fatalf("new matcher catalog failed: %v", err)
	}

	repoRoot := createCLIFlowRepo(t)
	flowDir := filepath.Join(repoRoot, "theater", "flows", "auth")
	flowPath := writeCLIRepoFile(t, repoRoot, filepath.Join("theater", "flows", "auth", "login-smoke.thtr"), `stage login-smoke

call smoke-login = auth/login()
`)
	libraryPath := writeCLIRepoFile(t, repoRoot, filepath.Join("theater", "lib", "auth", "login.thtr"), `stage auth-lib

scenario auth/login
  act submit
    eventually 1s every 1s
    do repeatable action.http(method: "GET", url: "/health")
`)

	restore := chdirForTest(t, flowDir)
	defer restore()

	loaded, err := newStageFileLoader(matchers).Load("./login-smoke.thtr")
	if err != nil {
		t.Fatalf("load stage failed: %v", err)
	}

	validator := theater.NewValidator(nil, nil)
	diagnostics := loaded.RewriteDiagnostics(validator.Validate(loaded.Spec))
	diagnostic := findDiagnosticByCode(diagnostics, "invalid_eventually_interval")
	if diagnostic == nil {
		t.Fatalf("expected invalid_eventually_interval diagnostic, got %#v", diagnostics)
	}
	if got, want := diagnostic.Span.File, libraryPath; got != want {
		t.Fatalf("diagnostic source file mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Span.Line, 5; got != want {
		t.Fatalf("diagnostic source line mismatch: got %d want %d", got, want)
	}
	if _, statErr := os.Stat(flowPath); statErr != nil {
		t.Fatalf("expected flow file to exist: %v", statErr)
	}
}

func TestHTTPStatelessExampleFilesValidate(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	for _, relativePath := range []string{
		"theater/flows/http/example-domain.yaml",
		"theater/flows/http/example-domain.thtr",
		"theater/flows/http/example-domain-with-purpose.yaml",
		"theater/flows/http/page-text-smoke.thtr",
		"theater/lib/web/check-page-text.thtr",
	} {
		t.Run(relativePath, func(t *testing.T) {
			t.Parallel()

			spec := requireValidStageFile(t, filepath.Join(root, relativePath))
			requireHTTPActionsUseSessionNone(t, spec)
		})
	}
}

func requireValidStageFile(t *testing.T, path string) theater.StageSpec {
	t.Helper()

	bundle, err := builtin.NewBundle()
	if err != nil {
		t.Fatalf("new builtin bundle failed: %v", err)
	}

	loaded, err := newStageFileLoader(bundle.Matchers).Load(path)
	if err != nil {
		t.Fatalf("load stage %s failed: %v", path, err)
	}

	diagnostics := loaded.RewriteDiagnostics(theater.NewValidator(bundle.Catalog, bundle.Matchers).Validate(loaded.Spec))
	if len(diagnostics) != 0 {
		t.Fatalf("stage %s must validate, got diagnostics: %#v", path, diagnostics)
	}

	return loaded.Spec
}

func requireHTTPActionsUseSessionNone(t *testing.T, spec theater.StageSpec) {
	t.Helper()

	actionCount := 0
	for _, scenario := range spec.Scenarios {
		for _, act := range scenario.Acts {
			if act.Action.Use != "action.http" {
				continue
			}

			actionCount++
			session, ok := act.Action.With["session"]
			if !ok {
				t.Fatalf("action %s in scenario %s must declare session", act.ID, scenario.ID)
			}
			if got, want := session.Kind, theater.BindingKindLiteral; got != want {
				t.Fatalf("action %s session kind mismatch: got %q want %q", act.ID, got, want)
			}
			if got, want := session.Value, "none"; got != want {
				t.Fatalf("action %s session value mismatch: got %#v want %q", act.ID, got, want)
			}
		}
	}
	if actionCount == 0 {
		t.Fatalf("stage %s must contain at least one action.http action", spec.ID)
	}
}

func findDiagnosticByCode(diagnostics []theater.Diagnostic, code string) *theater.Diagnostic {
	for i := range diagnostics {
		if diagnostics[i].Code == code {
			return &diagnostics[i]
		}
	}

	return nil
}

func createCLIFlowRepo(t *testing.T) string {
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

func writeCLIRepoFile(t *testing.T, repoRoot, relativePath, contents string) string {
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
