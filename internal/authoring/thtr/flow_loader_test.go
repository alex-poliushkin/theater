package thtr

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
