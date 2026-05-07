package theatercli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/builtin"
)

func TestListScenariosTextListsPublicLibraryScenarios(t *testing.T) {
	t.Parallel()

	repo := writeListScenariosRepo(t)

	var stdout, stderr bytes.Buffer
	code := run([]string{commandList, commandListScenarios, "--root", repo}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("list scenarios exit code mismatch: got %d stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}

	output := stdout.String()
	for _, want := range []string{
		"SCENARIO",
		"SYNTAX",
		"auth/login",
		"yaml",
		"expected_status_code:number",
		"method:string; required",
		"billing/check",
		"thtr",
		"invoice_id:string; required",
		"theater/lib/auth/login.yaml:",
		"theater/lib/billing/check.thtr:",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("list output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "auth/internal/bootstrap") {
		t.Fatalf("list output must not expose internal scenario ids:\n%s", output)
	}
	if strings.Contains(output, "Call skeletons:") {
		t.Fatalf("default list output must not include call skeletons:\n%s", output)
	}
}

func TestListScenariosJSONListsInputsAndSourceLocations(t *testing.T) {
	t.Parallel()

	repo := writeListScenariosRepo(t)

	var stdout, stderr bytes.Buffer
	code := run([]string{commandList, commandListScenarios, "--root", repo, "--format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("list scenarios exit code mismatch: got %d stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}

	var response scenarioListResult
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatalf("decode list response: %v\n%s", err, stdout.String())
	}
	var raw map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil {
		t.Fatalf("decode raw list response: %v\n%s", err, stdout.String())
	}
	assertJSONKeys(t, raw, []string{"library_root", "repo_root", "scenarios"})
	if got, want := len(response.Scenarios), 2; got != want {
		t.Fatalf("scenario count mismatch: got %d want %d: %#v", got, want, response.Scenarios)
	}
	rawScenarios := raw["scenarios"].([]any)
	rawLogin := rawScenarios[0].(map[string]any)
	assertJSONKeys(t, rawLogin, []string{"call", "id", "inputs", "source", "syntax"})
	rawInputs := rawLogin["inputs"].([]any)
	assertJSONKeys(t, rawInputs[0].(map[string]any), []string{"contract", "name"})
	assertJSONKeys(t, rawInputs[1].(map[string]any), []string{"contract", "name", "required"})
	rawCall := rawLogin["call"].(map[string]any)
	assertJSONKeys(t, rawCall, []string{"id", "required_inputs", "snippet", "syntax"})
	rawSource := rawLogin["source"].(map[string]any)
	assertJSONKeys(t, rawSource, []string{"column", "file", "line"})
	if got, want := response.Scenarios[0].ID, "auth/login"; got != want {
		t.Fatalf("first scenario id mismatch: got %q want %q", got, want)
	}
	if got, want := response.Scenarios[0].Syntax, "yaml"; got != want {
		t.Fatalf("first scenario syntax mismatch: got %q want %q", got, want)
	}
	if got, want := response.Scenarios[0].Inputs[0].Name, "expected_status_code"; got != want {
		t.Fatalf("first input name mismatch: got %q want %q", got, want)
	}
	if got, want := response.Scenarios[0].Source.File, "theater/lib/auth/login.yaml"; got != want {
		t.Fatalf("source file mismatch: got %q want %q", got, want)
	}
	if got, want := response.Scenarios[0].Call.ID, "run-auth-login-x617574682f6c6f67696e"; got != want {
		t.Fatalf("call id mismatch: got %q want %q", got, want)
	}
	if got, want := response.Scenarios[0].Call.Syntax, "yaml"; got != want {
		t.Fatalf("call syntax mismatch: got %q want %q", got, want)
	}
	if got, want := response.Scenarios[0].Call.RequiredInputs, []string{"method"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("call required inputs mismatch: got %v want %v", got, want)
	}
	for _, want := range []string{
		"scenario_calls:",
		"- id: run-auth-login-x617574682f6c6f67696e",
		`scenario_id: "auth/login"`,
		"bindings:",
		`"method": TODO-string`,
	} {
		if !strings.Contains(response.Scenarios[0].Call.Snippet, want) {
			t.Fatalf("call skeleton missing %q:\n%s", want, response.Scenarios[0].Call.Snippet)
		}
	}
	if response.Scenarios[0].Source.Line == 0 {
		t.Fatalf("source line must be populated: %#v", response.Scenarios[0].Source)
	}
}

func TestListScenariosTextCanPrintCallSkeletons(t *testing.T) {
	t.Parallel()

	repo := writeListScenariosRepo(t)

	var stdout, stderr bytes.Buffer
	code := run([]string{commandList, commandListScenarios, "--root", repo, "--call-skeleton"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("list scenarios exit code mismatch: got %d stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}

	output := stdout.String()
	for _, want := range []string{
		"Call skeletons:",
		"auth/login (yaml):",
		"scenario_calls:\n  - id: run-auth-login-x617574682f6c6f67696e",
		`scenario_id: "auth/login"`,
		`"method": TODO-string`,
		"billing/check (thtr):",
		`call run-billing-check-x62696c6c696e672f636865636b = billing/check(invoice_id: "TODO-string")`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("call skeleton output missing %q:\n%s", want, output)
		}
	}
}

func TestListScenariosCallSkeletonIDsAreUnique(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	writeListTestFile(t, filepath.Join(repo, "theater", "flows", ".keep"), "")
	writeListTestFile(t, filepath.Join(repo, "theater", "lib", "collisions.yaml"), `id: collisions
scenarios:
  - id: foo-a
    acts: []
  - id: foo/a
    acts: []
scenario_calls: []
`)

	var stdout, stderr bytes.Buffer
	code := run([]string{commandList, commandListScenarios, "--root", repo, "--format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("list scenarios exit code mismatch: got %d stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}

	var response scenarioListResult
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatalf("decode list response: %v\n%s", err, stdout.String())
	}
	if got, want := len(response.Scenarios), 2; got != want {
		t.Fatalf("scenario count mismatch: got %d want %d: %#v", got, want, response.Scenarios)
	}
	if response.Scenarios[0].Call.RequiredInputs == nil {
		t.Fatalf("call required_inputs must be present as an empty list for no-input scenarios: %#v", response.Scenarios[0].Call)
	}
	if got, want := response.Scenarios[0].Call.ID, "run-foo-a-x666f6f2d61"; got != want {
		t.Fatalf("first call id mismatch: got %q want %q", got, want)
	}
	if got, want := response.Scenarios[1].Call.ID, "run-foo-a-x666f6f2f61"; got != want {
		t.Fatalf("second call id mismatch: got %q want %q", got, want)
	}
	if !strings.Contains(response.Scenarios[0].Call.Snippet, "id: run-foo-a-x666f6f2d61") {
		t.Fatalf("first call snippet was not updated:\n%s", response.Scenarios[0].Call.Snippet)
	}
	if !strings.Contains(response.Scenarios[1].Call.Snippet, "id: run-foo-a-x666f6f2f61") {
		t.Fatalf("second call snippet was not updated:\n%s", response.Scenarios[1].Call.Snippet)
	}
}

func TestListScenariosCallSkeletonPlaceholdersUseRequiredInputTypes(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	writeListTestFile(t, filepath.Join(repo, "theater", "flows", ".keep"), "")
	writeListTestFile(t, filepath.Join(repo, "theater", "lib", "shapes.yaml"), `id: shapes-yaml
scenarios:
  - id: shapes/yaml
    inputs:
      count:
        type: number
        required: true
      enabled:
        type: bool
        required: true
      meta:
        type: object
        required: true
      none:
        type: "null"
        required: true
      tags:
        type: list
        required: true
    acts: []
scenario_calls: []
`)
	writeListTestFile(t, filepath.Join(repo, "theater", "lib", "shapes.thtr"), `stage shapes-thtr

scenario shapes/thtr(count: number!, enabled: bool!, meta: object!, none: null!, tags: list!)
  act noop
    do action.http(method: "GET", url: "https://example.com", session: "none")
`)

	var stdout, stderr bytes.Buffer
	code := run([]string{commandList, commandListScenarios, "--root", repo, "--format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("list scenarios exit code mismatch: got %d stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}

	var response scenarioListResult
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatalf("decode list response: %v\n%s", err, stdout.String())
	}
	if got, want := len(response.Scenarios), 2; got != want {
		t.Fatalf("scenario count mismatch: got %d want %d: %#v", got, want, response.Scenarios)
	}

	yamlSnippet := listScenarioByID(t, response, "shapes/yaml").Call.Snippet
	for _, want := range []string{
		`"count": 0`,
		`"enabled": false`,
		`"meta": {}`,
		`"none": null`,
		`"tags": []`,
	} {
		if !strings.Contains(yamlSnippet, want) {
			t.Fatalf("YAML call skeleton missing %q:\n%s", want, yamlSnippet)
		}
	}

	thtrSnippet := listScenarioByID(t, response, "shapes/thtr").Call.Snippet
	for _, want := range []string{
		"count: 0",
		"enabled: false",
		"meta: object {}",
		"none: null",
		"tags: list []",
	} {
		if !strings.Contains(thtrSnippet, want) {
			t.Fatalf(".thtr call skeleton missing %q:\n%s", want, thtrSnippet)
		}
	}
}

func TestListScenariosFiltersBySyntax(t *testing.T) {
	t.Parallel()

	repo := writeListScenariosRepo(t)

	tests := []struct {
		name     string
		syntax   string
		want     []string
		wantNone []string
	}{
		{
			name:     "yaml",
			syntax:   "yaml",
			want:     []string{"auth/login", "yaml", "theater/lib/auth/login.yaml:"},
			wantNone: []string{"billing/check", "thtr", "theater/lib/billing/check.thtr:"},
		},
		{
			name:     "thtr",
			syntax:   "thtr",
			want:     []string{"billing/check", "thtr", "theater/lib/billing/check.thtr:"},
			wantNone: []string{"auth/login", "yaml", "theater/lib/auth/login.yaml:"},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer
			code := run([]string{commandList, commandListScenarios, "--root", repo, "--syntax", test.syntax}, &stdout, &stderr)
			if code != 0 {
				t.Fatalf("list scenarios exit code mismatch: got %d stderr=%q stdout=%q", code, stderr.String(), stdout.String())
			}
			if got := stderr.String(); got != "" {
				t.Fatalf("stderr mismatch: got %q want empty", got)
			}
			for _, want := range test.want {
				if !strings.Contains(stdout.String(), want) {
					t.Fatalf("list output missing %q:\n%s", want, stdout.String())
				}
			}
			for _, absent := range test.wantNone {
				if strings.Contains(stdout.String(), absent) {
					t.Fatalf("list output contains %q:\n%s", absent, stdout.String())
				}
			}
			assertListScenariosJSONSyntaxFilter(t, repo, test.syntax, test.want[0])
		})
	}
}

func TestListScenariosAdvertisedSyntaxLoadsWithMatchingFlowSyntax(t *testing.T) {
	t.Parallel()

	repo := writeListScenariosRepo(t)
	assertListScenariosFlowLoadsScenario(t, filepath.Join(repo, "theater", "flows", "auth", "login.yaml"), "auth/login")
	assertListScenariosFlowLoadsScenario(t, filepath.Join(repo, "theater", "flows", "billing", "check.thtr"), "billing/check")
}

func TestListScenariosClassifiesYMLExtensionAsYAML(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	writeListTestFile(t, filepath.Join(repo, "theater", "flows", ".keep"), "")
	writeListTestFile(t, filepath.Join(repo, "theater", "lib", "legacy", "ping.yml"), `id: legacy
scenarios:
  - id: legacy/ping
    acts: []
scenario_calls: []
`)

	var stdout, stderr bytes.Buffer
	code := run([]string{commandList, commandListScenarios, "--root", repo, "--syntax", "yaml", "--format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("list scenarios exit code mismatch: got %d stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}

	var response scenarioListResult
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatalf("decode list response: %v\n%s", err, stdout.String())
	}
	if got, want := len(response.Scenarios), 1; got != want {
		t.Fatalf("scenario count mismatch: got %d want %d: %#v", got, want, response.Scenarios)
	}
	if got, want := response.Scenarios[0].Syntax, "yaml"; got != want {
		t.Fatalf("scenario syntax mismatch: got %q want %q", got, want)
	}
	if got, want := response.Scenarios[0].Source.File, "theater/lib/legacy/ping.yml"; got != want {
		t.Fatalf("source file mismatch: got %q want %q", got, want)
	}
}

func TestListScenariosRejectsUnsupportedLibraryFileSyntax(t *testing.T) {
	t.Parallel()

	repo := writeListScenariosRepo(t)
	writeListTestFile(t, filepath.Join(repo, "theater", "lib", "bad.json"), "{}")

	assertListCommandError(t, []string{commandList, commandListScenarios, "--root", repo}, "unsupported library file syntax")
}

func TestListScenariosErrorTextSanitizesRepoControlledPaths(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	writeListTestFile(t, filepath.Join(repo, "theater", "flows", ".keep"), "")
	writeListTestFile(t, filepath.Join(repo, "theater", "lib", "bad\x1b[31m.json"), "{}")

	var stdout, stderr bytes.Buffer
	code := run([]string{commandList, commandListScenarios, "--root", repo}, &stdout, &stderr)
	if code != exitCodeCommandError {
		t.Fatalf("exit code mismatch: got %d want %d stdout=%q stderr=%q", code, exitCodeCommandError, stdout.String(), stderr.String())
	}
	if strings.Contains(stderr.String(), "\x1b") {
		t.Fatalf("stderr contains raw control data: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), `bad\x1b[31m.json`) {
		t.Fatalf("stderr missing sanitized path: %q", stderr.String())
	}
}

func TestListScenariosIgnoresSupportDirectories(t *testing.T) {
	t.Parallel()

	repo := writeListScenariosRepo(t)
	for _, name := range []string{"internal", "examples", "fixtures", "testdata"} {
		writeListTestFile(t, filepath.Join(repo, "theater", "lib", name, "invalid.yaml"), "{")
		writeListTestFile(t, filepath.Join(repo, "theater", "lib", name, "visible.yaml"), `id: ignored
scenarios:
  - id: ignored/`+name+`
    acts: []
scenario_calls: []
`)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{commandList, commandListScenarios, "--root", repo}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("list scenarios exit code mismatch: got %d stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if strings.Contains(stdout.String(), "ignored/") {
		t.Fatalf("ignored library directory leaked into output:\n%s", stdout.String())
	}
}

func TestListScenariosRejectsSymlinkLibraryFiles(t *testing.T) {
	t.Parallel()

	repo := writeListScenariosRepo(t)
	target := filepath.Join(t.TempDir(), "outside.yaml")
	writeListTestFile(t, target, `id: outside
scenarios: []
scenario_calls: []
`)
	link := filepath.Join(repo, "theater", "lib", "leak.yaml")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	assertListCommandError(t, []string{commandList, commandListScenarios, "--root", repo}, "symlinks are not allowed")
}

func TestListScenariosRejectsOversizedLibraryFiles(t *testing.T) {
	t.Parallel()

	repo := writeListScenariosRepo(t)
	oversized := strings.Repeat(" ", maxScenarioLibraryFileSize+1)
	writeListTestFile(t, filepath.Join(repo, "theater", "lib", "huge.yaml"), oversized)

	assertListCommandError(t, []string{commandList, commandListScenarios, "--root", repo}, "exceeds discovery size limit")
}

func TestListScenariosRejectsTooManyLibraryFiles(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	writeListTestFile(t, filepath.Join(repo, "theater", "flows", ".keep"), "")
	for i := 0; i <= maxScenarioLibraryFiles; i++ {
		writeListTestFile(t, filepath.Join(repo, "theater", "lib", fmt.Sprintf("empty-%04d.yaml", i)), `id: empty
scenarios: []
scenario_calls: []
`)
	}

	assertListCommandError(t, []string{commandList, commandListScenarios, "--root", repo}, "supports at most")
}

func TestListScenariosTextSanitizesRepoControlledValues(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	writeListTestFile(t, filepath.Join(repo, "theater", "flows", ".keep"), "")
	writeListTestFile(t, filepath.Join(repo, "theater", "lib", "bad.yaml"), "id: bad\nscenarios:\n  - id: \"evil\\x1b[31m\"\n    inputs:\n      \"line\\nkey\":\n        type: string\n        required: true\n    acts: []\nscenario_calls: []\n")

	var stdout, stderr bytes.Buffer
	code := run([]string{commandList, commandListScenarios, "--root", repo}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("list scenarios exit code mismatch: got %d stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if strings.Contains(stdout.String(), "\x1b") || strings.Contains(stdout.String(), "\nkey") {
		t.Fatalf("text output contains unsanitized control data: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), `evil\x1b[31m`) || !strings.Contains(stdout.String(), "line key:string") {
		t.Fatalf("text output missing sanitized data: %q", stdout.String())
	}

	var skeletonStdout, skeletonStderr bytes.Buffer
	skeletonCode := run([]string{commandList, commandListScenarios, "--root", repo, "--call-skeleton"}, &skeletonStdout, &skeletonStderr)
	if skeletonCode != 0 {
		t.Fatalf("list scenarios skeleton exit code mismatch: got %d stderr=%q stdout=%q", skeletonCode, skeletonStderr.String(), skeletonStdout.String())
	}
	if strings.Contains(skeletonStdout.String(), "\x1b") || strings.Contains(skeletonStdout.String(), "\nkey") {
		t.Fatalf("skeleton text output contains unsanitized control data: %q", skeletonStdout.String())
	}
	for _, want := range []string{
		`evil\x1b[31m (yaml):`,
		`scenario_id: "evil\\x1b[31m"`,
		`"line key": TODO-string`,
	} {
		if !strings.Contains(skeletonStdout.String(), want) {
			t.Fatalf("skeleton text output missing sanitized value %q: %q", want, skeletonStdout.String())
		}
	}
}

func TestListScenariosRejectsLibraryScenarioCalls(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	writeListTestFile(t, filepath.Join(repo, "theater", "flows", ".keep"), "")
	writeListTestFile(t, filepath.Join(repo, "theater", "lib", "bad.yaml"), `id: bad-library
scenarios: []
scenario_calls:
  - id: bad
    scenario_id: auth/login
`)

	var stdout, stderr bytes.Buffer
	code := run([]string{commandList, commandListScenarios, "--root", repo}, &stdout, &stderr)
	if code != exitCodeCommandError {
		t.Fatalf("list scenarios exit code mismatch: got %d want %d stdout=%q stderr=%q", code, exitCodeCommandError, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "must not declare scenario_calls") {
		t.Fatalf("stderr mismatch: got %q", stderr.String())
	}
}

func TestListCommandRejectsInvalidArguments(t *testing.T) {
	t.Parallel()

	repo := writeListScenariosRepo(t)
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing resource",
			args:    []string{commandList},
			wantErr: "list requires a resource",
		},
		{
			name:    "unknown resource",
			args:    []string{commandList, "widgets"},
			wantErr: `unknown list resource "widgets"`,
		},
		{
			name:    "extra positional",
			args:    []string{commandList, commandListScenarios, "--root", repo, "extra"},
			wantErr: "list scenarios does not accept positional arguments",
		},
		{
			name:    "unsupported format",
			args:    []string{commandList, commandListScenarios, "--root", repo, "--format", "junit"},
			wantErr: `unsupported format "junit"`,
		},
		{
			name:    "unsupported syntax",
			args:    []string{commandList, commandListScenarios, "--root", repo, "--syntax", "json"},
			wantErr: `unsupported scenario syntax "json"`,
		},
		{
			name:    "call skeleton with json",
			args:    []string{commandList, commandListScenarios, "--root", repo, "--format", "json", "--call-skeleton"},
			wantErr: "--call-skeleton is only supported with --format text",
		},
		{
			name:    "missing repo roots",
			args:    []string{commandList, commandListScenarios, "--root", t.TempDir()},
			wantErr: "repo-local theater roots not found",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertListCommandError(t, test.args, test.wantErr)
		})
	}
}

func assertListCommandError(t *testing.T, args []string, wantErr string) {
	t.Helper()

	var stdout, stderr bytes.Buffer
	code := run(args, &stdout, &stderr)
	if code != exitCodeCommandError {
		t.Fatalf("exit code mismatch: got %d want %d stdout=%q stderr=%q", code, exitCodeCommandError, stdout.String(), stderr.String())
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("stdout mismatch: got %q want empty", got)
	}
	if !strings.Contains(stderr.String(), wantErr) {
		t.Fatalf("stderr mismatch: got %q want substring %q", stderr.String(), wantErr)
	}
}

func assertJSONKeys(t *testing.T, object map[string]any, want []string) {
	t.Helper()

	got := make([]string, 0, len(object))
	for key := range object {
		got = append(got, key)
	}
	if strings.Join(got, "\x00") == "" {
		t.Fatalf("object has no keys, want %v", want)
	}
	for _, key := range want {
		if _, ok := object[key]; !ok {
			t.Fatalf("object keys mismatch: got %v want key %q", got, key)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("object key count mismatch: got %v want %v", got, want)
	}
}

func listScenarioByID(t *testing.T, result scenarioListResult, id string) listedScenario {
	t.Helper()

	for i := range result.Scenarios {
		if result.Scenarios[i].ID == id {
			return result.Scenarios[i]
		}
	}
	t.Fatalf("listed scenario %q not found: %#v", id, result.Scenarios)
	return listedScenario{}
}

func assertListScenariosJSONSyntaxFilter(t *testing.T, repo, syntax, wantID string) {
	t.Helper()

	var stdout, stderr bytes.Buffer
	code := run([]string{commandList, commandListScenarios, "--root", repo, "--syntax", syntax, "--format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("list scenarios json exit code mismatch: got %d stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}

	var response scenarioListResult
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatalf("decode list response: %v\n%s", err, stdout.String())
	}
	if got, want := len(response.Scenarios), 1; got != want {
		t.Fatalf("scenario count mismatch for syntax %s: got %d want %d: %#v", syntax, got, want, response.Scenarios)
	}
	if got := response.Scenarios[0].ID; got != wantID {
		t.Fatalf("scenario id mismatch for syntax %s: got %q want %q", syntax, got, wantID)
	}
	if got := response.Scenarios[0].Syntax; got != syntax {
		t.Fatalf("scenario syntax mismatch: got %q want %q", got, syntax)
	}
}

func assertListScenariosFlowLoadsScenario(t *testing.T, path string, wantID string) {
	t.Helper()

	matchers, err := builtin.Matchers()
	if err != nil {
		t.Fatalf("build matchers failed: %v", err)
	}
	loaded, err := newStageFileLoader(matchers).Load(path)
	if err != nil {
		t.Fatalf("load advertised scenario flow %s: %v", path, err)
	}
	if !listScenariosStageHasScenario(loaded.Spec, wantID) {
		t.Fatalf("loaded flow %s does not include scenario %q: %v", path, wantID, listScenariosScenarioIDs(loaded.Spec))
	}
}

func listScenariosStageHasScenario(spec theater.StageSpec, wantID string) bool {
	for _, scenario := range spec.Scenarios {
		if scenario.ID == wantID {
			return true
		}
	}
	return false
}

func listScenariosScenarioIDs(spec theater.StageSpec) []string {
	ids := make([]string, 0, len(spec.Scenarios))
	for _, scenario := range spec.Scenarios {
		ids = append(ids, scenario.ID)
	}
	return ids
}

func writeListScenariosRepo(t *testing.T) string {
	t.Helper()

	repo := t.TempDir()
	writeListTestFile(t, filepath.Join(repo, "theater", "flows", "auth", "login.yaml"), `id: auth-flow
scenarios: []
scenario_calls:
  - id: run-login
    scenario_id: auth/login
    bindings:
      method: POST
      expected_status_code: 200
`)
	writeListTestFile(t, filepath.Join(repo, "theater", "flows", "billing", "check.thtr"), `stage billing-flow

call run-check = billing/check(invoice_id: "inv-1")
`)
	writeListTestFile(t, filepath.Join(repo, "theater", "lib", "auth", "login.yaml"), `id: auth-library
scenarios:
  - id: auth/login
    inputs:
      method:
        type: string
        required: true
      expected_status_code:
        type: number
    acts:
      - id: submit
        action:
          use: action.http
          with:
            method:
              kind: ref
              ref: method
            url:
              kind: literal
              value: https://example.com
  - id: auth/internal/bootstrap
    acts:
      - id: submit
        action:
          use: action.http
scenario_calls: []
`)
	writeListTestFile(t, filepath.Join(repo, "theater", "lib", "billing", "check.thtr"), `stage billing-library

scenario billing/check(invoice_id: string!)
  act lookup
    do action.http(method: "GET", url: "https://example.com", session: "none")
    expect ok: field(status_code) == 200
`)

	return repo
}

func writeListTestFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("prepare %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
