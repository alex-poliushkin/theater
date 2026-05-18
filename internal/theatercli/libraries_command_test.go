package theatercli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLibrariesInspectTextListsSelectedLibraryContract(t *testing.T) {
	t.Parallel()

	repoRoot := createLibraryInspectRepo(t)
	flowPath := writeLibraryInspectRepoFile(t, repoRoot, filepath.Join("theater", "flows", "auth", "login-smoke.yaml"), `
id: login-smoke
scenarios: []
scenario_calls:
  - id: smoke-login
    scenario_id: auth/login
    bindings:
      token:
        kind: literal
        value: runtime-token
    exports:
      - as: session_id
        ref:
          name: session_id
`)
	writeLibraryInspectRepoFile(t, repoRoot, filepath.Join("theater", "lib", "auth", "login.yaml"), `
id: auth-lib
http:
  auth:
    service_api:
      attach:
        - bearer:
            token_slot: token
scenarios:
  - id: auth/login
    inputs:
      token:
        type: string
        required: true
    auth_bindings:
      service_api:
        slots:
          token:
            kind: ref
            ref:
              name: token
    acts:
      - id: submit
        action:
          use: action.http
          with:
            auth:
              kind: literal
              value: service_api
        exports:
          - as: session_id
            field: body
  - id: auth/unused
    acts:
      - id: noop
        action:
          use: action.generate
scenario_calls: []
`)
	writeLibraryInspectRepoFile(t, repoRoot, filepath.Join("theater", "lib", "auth", "password.yaml"), `
id: password-lib
http:
  auth:
    unused_static:
      attach:
        - bearer:
            token: unselected-secret
scenarios:
  - id: auth/password
    acts:
      - id: noop
        action:
          use: action.generate
scenario_calls: []
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{commandLibraries, commandLibrariesInspect, flowPath}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}

	output := stdout.String()
	for _, want := range []string{
		"Selected library files",
		"theater/lib/auth/login.yaml",
		"Selected scenarios: auth/login",
		"Ignored scenarios: auth/unused",
		"Auth contributions: service_api",
		"Unselected library files",
		"theater/lib/auth/password.yaml",
		"Scenario call graph",
		"smoke-login -> auth/login",
		"Input requirements",
		"auth/login.token",
		"string; required",
		"Exports",
		"smoke-login.session_id",
		"auth/login.submit.session_id",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("library inspection missing %q:\n%s", want, output)
		}
	}
	for _, forbidden := range []string{"runtime-token", "unselected-secret"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("library inspection leaked value %q:\n%s", forbidden, output)
		}
	}
}

func TestLibrariesInspectJSONSupportsTHTRFlows(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{
		commandLibraries,
		commandLibrariesInspect,
		repoFilePath(t, "docs/examples/reusable-auth/theater/flows/sample-ready.thtr"),
		"--format",
		"json",
	}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}

	var result libraryInspectionResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode library inspection json failed: %v\n%s", err, stdout.String())
	}
	if got, want := result.Syntax, string(scenarioLibrarySyntaxTHTR); got != want {
		t.Fatalf("syntax mismatch: got %q want %q", got, want)
	}
	if got, want := len(result.SelectedLibraryFiles), 1; got != want {
		t.Fatalf("selected library count mismatch: got %d want %d: %#v", got, want, result.SelectedLibraryFiles)
	}
	if got, want := result.SelectedLibraryFiles[0].Path, "theater/lib/service/sample-ready.thtr"; got != want {
		t.Fatalf("selected library path mismatch: got %q want %q", got, want)
	}
	if got, want := result.SelectedLibraryFiles[0].SelectedScenarios[0].ID, "service/sample-ready"; got != want {
		t.Fatalf("selected scenario mismatch: got %q want %q", got, want)
	}
	if got, want := len(result.UnselectedLibraryFiles), 0; got != want {
		t.Fatalf("unselected library count mismatch: got %d want %d", got, want)
	}
	if got, want := result.ScenarioCallEdges[0].CallID, "run-sample"; got != want {
		t.Fatalf("call edge id mismatch: got %q want %q", got, want)
	}
	if got, want := result.ScenarioCallEdges[0].ScenarioID, "service/sample-ready"; got != want {
		t.Fatalf("call edge scenario mismatch: got %q want %q", got, want)
	}
	if got, want := result.ScenarioCallEdges[0].Kind, "library"; got != want {
		t.Fatalf("call edge kind mismatch: got %q want %q", got, want)
	}
	if got, want := result.AuthContributions[0].Name, "service_api"; got != want {
		t.Fatalf("auth contribution mismatch: got %q want %q", got, want)
	}
	if got, want := result.InputRequirements[0].Name, "session_token"; got != want {
		t.Fatalf("input name mismatch: got %q want %q", got, want)
	}
	if got, want := result.InputRequirements[0].Contract, "string; required"; got != want {
		t.Fatalf("input contract mismatch: got %q want %q", got, want)
	}
}

func TestLibrariesInspectReportsSelectedLibraryAuthDiagnosticsWithoutValues(t *testing.T) {
	t.Parallel()

	repoRoot := createLibraryInspectRepo(t)
	flowPath := writeLibraryInspectRepoFile(t, repoRoot, filepath.Join("theater", "flows", "auth", "static-smoke.yaml"), `
id: static-smoke
scenarios: []
scenario_calls:
  - id: call-static
    scenario_id: auth/static
`)
	writeLibraryInspectRepoFile(t, repoRoot, filepath.Join("theater", "lib", "auth", "static.yaml"), `
id: static-lib
http:
  auth:
    service_api:
      attach:
        - bearer:
            token: selected-secret
scenarios:
  - id: auth/static
    acts:
      - id: request
        action:
          use: action.http
          with:
            auth:
              kind: literal
              value: service_api
scenario_calls: []
`)
	writeLibraryInspectRepoFile(t, repoRoot, filepath.Join("theater", "lib", "auth", "unused.yaml"), `
id: unused-lib
http:
  auth:
    unused_api:
      attach:
        - bearer:
            token: unselected-secret
scenarios:
  - id: auth/unused
    acts:
      - id: noop
        action:
          use: action.generate
scenario_calls: []
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{commandLibraries, commandLibrariesInspect, flowPath, "--format", "json"}, &stdout, &stderr)
	if got, want := code, 1; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}

	var result libraryInspectionResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode library inspection json failed: %v\n%s", err, stdout.String())
	}
	if got, want := len(result.Diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d: %#v", got, want, result.Diagnostics)
	}
	if got, want := result.Diagnostics[0].Code, "invalid_selected_library_http_auth"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
	if got, want := result.RejectedAuth[0].Name, "service_api"; got != want {
		t.Fatalf("rejected auth mismatch: got %q want %q", got, want)
	}
	if result.Diagnostics[0].Span.File == "" {
		t.Fatalf("diagnostic must include source file: %#v", result.Diagnostics[0])
	}
	if strings.Contains(stdout.String(), repoRoot) {
		t.Fatalf("library inspection must keep paths repo-relative:\n%s", stdout.String())
	}
	for _, forbidden := range []string{"selected-secret", "unselected-secret"} {
		if strings.Contains(stdout.String(), forbidden) {
			t.Fatalf("library inspection leaked credential value %q:\n%s", forbidden, stdout.String())
		}
	}
}

func TestLibrariesInspectReportsDuplicateSelectedLibraryAuth(t *testing.T) {
	t.Parallel()

	repoRoot := createLibraryInspectRepo(t)
	flowPath := writeLibraryInspectRepoFile(t, repoRoot, filepath.Join("theater", "flows", "auth", "duplicate-smoke.yaml"), `
id: duplicate-smoke
http:
  auth:
    service_api:
      attach:
        - bearer:
            token_slot: token
scenarios: []
scenario_calls:
  - id: call-login
    scenario_id: auth/login
`)
	writeLibraryInspectRepoFile(t, repoRoot, filepath.Join("theater", "lib", "auth", "login.yaml"), `
id: login-lib
http:
  auth:
    service_api:
      attach:
        - bearer:
            token_slot: token
scenarios:
  - id: auth/login
    acts:
      - id: request
        action:
          use: action.http
          with:
            auth:
              kind: literal
              value: service_api
scenario_calls: []
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{commandLibraries, commandLibrariesInspect, flowPath, "--format", "json"}, &stdout, &stderr)
	if got, want := code, 1; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}

	var result libraryInspectionResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode library inspection json failed: %v\n%s", err, stdout.String())
	}
	if got, want := result.Diagnostics[0].Code, "duplicate_selected_library_http_auth"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
	if got, want := result.RejectedAuth[0].Name, "service_api"; got != want {
		t.Fatalf("rejected auth mismatch: got %q want %q", got, want)
	}
	if strings.Contains(stdout.String(), "token_slot") {
		t.Fatalf("library inspection must not print auth attachment detail:\n%s", stdout.String())
	}
}

func TestLibrariesInspectIgnoresBrokenUnselectedLibraryFiles(t *testing.T) {
	t.Parallel()

	repoRoot := createLibraryInspectRepo(t)
	flowPath := writeLibraryInspectRepoFile(t, repoRoot, filepath.Join("theater", "flows", "auth", "login-smoke.yaml"), `
id: login-smoke
scenarios: []
scenario_calls:
  - id: smoke-login
    scenario_id: auth/login
`)
	writeLibraryInspectRepoFile(t, repoRoot, filepath.Join("theater", "lib", "auth", "login.yaml"), `
id: auth-lib
scenarios:
  - id: auth/login
    acts:
      - id: submit
        action:
          use: action.http
scenario_calls: []
`)
	writeLibraryInspectRepoFile(t, repoRoot, filepath.Join("theater", "lib", "auth", "broken.yaml"), `
id: broken-lib
scenarios: [}
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{commandLibraries, commandLibrariesInspect, flowPath, "--format", "json"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}

	var result libraryInspectionResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode library inspection json failed: %v\n%s", err, stdout.String())
	}
	if got, want := result.SelectedLibraryFiles[0].Path, "theater/lib/auth/login.yaml"; got != want {
		t.Fatalf("selected library path mismatch: got %q want %q", got, want)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("broken unselected file must not contribute diagnostics: %#v", result.Diagnostics)
	}
	if strings.Contains(stdout.String(), "yaml:") || strings.Contains(stderr.String(), "broken") {
		t.Fatalf("broken unselected file must not fail inspection stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestLibrariesInspectReportsBrokenSelectedLibraryIndex(t *testing.T) {
	t.Parallel()

	repoRoot := createLibraryInspectRepo(t)
	flowPath := writeLibraryInspectRepoFile(t, repoRoot, filepath.Join("theater", "flows", "auth", "broken-smoke.yaml"), `
id: broken-smoke
scenarios: []
scenario_calls:
  - id: smoke-broken
    scenario_id: auth/broken
`)
	writeLibraryInspectRepoFile(t, repoRoot, filepath.Join("theater", "lib", "auth", "broken.yaml"), `
id: broken-lib
scenarios: [}
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{commandLibraries, commandLibrariesInspect, flowPath}, &stdout, &stderr)
	if got, want := code, exitCodeCommandError; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "" {
		t.Fatalf("stdout mismatch: got %q want empty", got)
	}
	output := stderr.String()
	for _, want := range []string{
		`referenced library scenario "auth/broken" is not found`,
		"inspect library scenario index theater/lib/auth/broken.yaml",
		"yaml:",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("stderr missing %q: %q", want, output)
		}
	}
	if strings.Contains(output, repoRoot) {
		t.Fatalf("stderr leaked absolute repo path: %q", output)
	}
}

func TestLibrariesInspectIgnoresDuplicateUnselectedScenarioIDs(t *testing.T) {
	t.Parallel()

	repoRoot := createLibraryInspectRepo(t)
	flowPath := writeLibraryInspectRepoFile(t, repoRoot, filepath.Join("theater", "flows", "auth", "login-smoke.yaml"), `
id: login-smoke
scenarios: []
scenario_calls:
  - id: smoke-login
    scenario_id: auth/login
`)
	writeLibraryInspectRepoFile(t, repoRoot, filepath.Join("theater", "lib", "auth", "login.yaml"), `
id: auth-lib
scenarios:
  - id: auth/login
    acts:
      - id: submit
        action:
          use: action.http
scenario_calls: []
`)
	writeLibraryInspectRepoFile(t, repoRoot, filepath.Join("theater", "lib", "auth", "first-unused.yaml"), `
id: first-unused-lib
scenarios:
  - id: auth/unused
    acts:
      - id: noop
        action:
          use: action.generate
scenario_calls: []
`)
	writeLibraryInspectRepoFile(t, repoRoot, filepath.Join("theater", "lib", "auth", "second-unused.yaml"), `
id: second-unused-lib
scenarios:
  - id: auth/unused
    acts:
      - id: noop
        action:
          use: action.generate
scenario_calls: []
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{commandLibraries, commandLibrariesInspect, flowPath, "--format", "json"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}
	if strings.Contains(stdout.String(), "declared in multiple files") {
		t.Fatalf("duplicate unselected scenario must not fail inspection:\n%s", stdout.String())
	}
}

func TestLibrariesInspectFailureUsesRepoRelativePaths(t *testing.T) {
	t.Parallel()

	repoRoot := createLibraryInspectRepo(t)
	flowPath := writeLibraryInspectRepoFile(t, repoRoot, filepath.Join("theater", "flows", "auth", "login-smoke.yaml"), `
id: login-smoke
scenarios: []
scenario_calls:
  - id: smoke-login
    scenario_id: auth/login
`)
	writeLibraryInspectRepoFile(t, repoRoot, filepath.Join("theater", "lib", "auth", "login.yaml"), `
id: auth-lib
scenarios:
  - id: auth/login
    acts:
      - id: submit
        action:
          use: action.http
scenario_calls:
  - id: forbidden
    scenario_id: auth/login
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{commandLibraries, commandLibrariesInspect, flowPath}, &stdout, &stderr)
	if got, want := code, exitCodeCommandError; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "" {
		t.Fatalf("stdout mismatch: got %q want empty", got)
	}
	output := stderr.String()
	if !strings.Contains(output, "theater/lib/auth/login.yaml") {
		t.Fatalf("stderr missing repo-relative path: %q", output)
	}
	if strings.Contains(output, repoRoot) {
		t.Fatalf("stderr leaked absolute repo path: %q", output)
	}
}

func createLibraryInspectRepo(t *testing.T) string {
	t.Helper()

	repoRoot := t.TempDir()
	for _, dir := range []string{
		filepath.Join(repoRoot, "theater", "flows"),
		filepath.Join(repoRoot, "theater", "lib"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("create repo dir failed: %v", err)
		}
	}
	return repoRoot
}

func writeLibraryInspectRepoFile(t *testing.T, repoRoot, relativePath, body string) string {
	t.Helper()

	path := filepath.Join(repoRoot, relativePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create repo file parent failed: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write repo file failed: %v", err)
	}
	return path
}
