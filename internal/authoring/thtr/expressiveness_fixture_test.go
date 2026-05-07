package thtr

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/builtin"
	builtinexpectation "github.com/alex-poliushkin/theater/builtin/expectation"
	"github.com/alex-poliushkin/theater/internal/testkit"
)

func TestExpressivenessStressFixtureRoundTripFormatLowerValidateAndMap(t *testing.T) {
	t.Parallel()

	sourcePath := expressivenessFixturePath(t, "success-input.thtr")
	source := readExpressivenessFixture(t, "success-input.thtr")
	wantFormatted := readExpressivenessFixture(t, "success-formatted.thtr")
	wantLowered := readExpressivenessFixture(t, "success-lowered.yaml")

	formatted, err := Format(source)
	if err != nil {
		t.Fatalf("format fixture failed: %v", err)
	}
	if got, want := string(formatted), string(wantFormatted); got != want {
		t.Fatalf("formatted fixture mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}

	reformatted, err := Format(formatted)
	if err != nil {
		t.Fatalf("format idempotence failed: %v", err)
	}
	if got, want := string(reformatted), string(wantFormatted); got != want {
		t.Fatalf("formatter must be idempotent:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}

	result, err := ParseDetailed(source, sourcePath, nil)
	if err != nil {
		t.Fatalf("parse fixture failed: %v", err)
	}
	if got, want := string(result.CanonicalYAML()), string(wantLowered); got != want {
		t.Fatalf("lowered fixture mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}

	catalog, matchers := expressivenessFixtureCatalog(t)
	diagnostics := result.RewriteDiagnostics(theater.NewValidator(catalog, matchers).Validate(result.Spec))
	if len(diagnostics) != 0 {
		t.Fatalf("expressiveness fixture must validate, got diagnostics: %#v", diagnostics)
	}

	requireExpressivenessSourceMapRange(t, result, "stage.expressiveness-stress/scenario.auth~1register-confirm-login/act.register/action/binding.json.Content-Type", sourceRange{
		startLine: 41, startColumn: 9, endLine: 41, endColumn: 43,
	})
	requireExpressivenessSourceMapRange(t, result, "stage.expressiveness-stress/scenario.auth~1register-confirm-login/act.login/capture_auth/slot.csrf", sourceRange{
		startLine: 56, startColumn: 7, endLine: 56, endColumn: 44,
	})
	requireExpressivenessSourceMapRange(t, result, "stage.expressiveness-stress/scenario.mail~1wait-for-otp/act.poll-notifications/eventually/timeout", sourceRange{
		startLine: 63, startColumn: 5, endLine: 63, endColumn: 28,
	})
	requireExpressivenessSourceMapRange(t, result, "stage.expressiveness-stress/scenario.mail~1wait-for-otp/act.poll-notifications/export.otp/through[0]/pick/where[0]/subject/path", sourceRange{
		startLine: 74, startColumn: 7, endLine: 74, endColumn: 31,
	})
	requireExpressivenessSourceMapRange(t, result, "stage.expressiveness-stress/scenario.plugins~1echo-check/act.echo/expectation.not-other/assert/binding.assert", sourceRange{
		startLine: 111, startColumn: 46, endLine: 111, endColumn: 84,
	})
}

func TestExpressivenessStressRuntimeFailureFixtures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		wantCause   string
		wantFailure theater.FailureKind
		wantActPath string
	}{
		{
			name:        "runtime-pick-zero.thtr",
			wantCause:   "pick matched no items",
			wantFailure: theater.FailureKindInternal,
			wantActPath: "stage.pick-zero/call.wait/act.poll",
		},
		{
			name:        "runtime-pick-multiple.thtr",
			wantCause:   "pick matched multiple items",
			wantFailure: theater.FailureKindInternal,
			wantActPath: "stage.pick-multiple/call.wait/act.poll",
		},
		{
			name:        "runtime-missing-null-matrix.thtr",
			wantCause:   "missing",
			wantFailure: theater.FailureKindObservation,
			wantActPath: "stage.missing-null-matrix/call.check/act.fetch",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			result, err := LoadFileDetailed(expressivenessFixturePath(t, test.name), nil)
			if err != nil {
				t.Fatalf("load fixture failed: %v", err)
			}

			catalog, matchers := expressivenessFixtureCatalog(t)
			diagnostics := result.RewriteDiagnostics(theater.NewValidator(catalog, matchers).Validate(result.Spec))
			if len(diagnostics) != 0 {
				t.Fatalf("runtime fixture must validate before execution, got diagnostics: %#v", diagnostics)
			}

			runResult, err := theater.NewRunner(catalog, matchers).Run(context.Background(), result.Spec, theater.RunOptions{})
			if err != nil {
				t.Fatalf("run fixture failed unexpectedly: %v", err)
			}
			if got, want := runResult.Report.Status, theater.StatusFailed; got != want {
				t.Fatalf("run status mismatch: got %q want %q", got, want)
			}
			if runResult.Report.Failure == nil || runResult.Report.Failure.Cause == nil {
				t.Fatalf("run failure must include cause: %#v", runResult.Report.Failure)
			}
			if got, want := runResult.Report.Failure.Kind, test.wantFailure; got != want {
				t.Fatalf("failure kind mismatch: got %q want %q", got, want)
			}
			if got := runResult.Report.Failure.Message(); !strings.Contains(got, test.wantCause) {
				t.Fatalf("failure cause mismatch: got %q want contains %q", got, test.wantCause)
			}
			actNode := findExpressivenessNodeReport(t, runResult.Report, theater.NodeKindAct, test.wantActPath)
			if got, want := actNode.Status, theater.StatusFailed; got != want {
				t.Fatalf("act node status mismatch: got %q want %q", got, want)
			}
			if got, want := actNode.Failure.Kind, test.wantFailure; got != want {
				t.Fatalf("act node failure kind mismatch: got %q want %q", got, want)
			}
			requireExpressivenessActionPassed(t, runResult.Report, test.wantActPath+"/action")
			if test.name == "runtime-missing-null-matrix.thtr" {
				requireExpressivenessExpectationStatus(t, runResult.Report, "stage.missing-null-matrix/call.check/act.fetch/expectation.deleted-null", theater.StatusPassed)
				requireExpressivenessExpectationStatus(t, runResult.Report, "stage.missing-null-matrix/call.check/act.fetch/expectation.error-absent", theater.StatusPassed)
				requireExpressivenessExpectationStatus(t, runResult.Report, "stage.missing-null-matrix/call.check/act.fetch/expectation.missing-terminal", theater.StatusFailed)
			}
		})
	}
}

func TestExpressivenessStressInvalidFixtures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		wantCode    string
		wantLine    int
		wantSummary string
	}{
		{
			name:        "invalid-quoted-core-id.thtr",
			wantCode:    "thtr_parse_error",
			wantLine:    1,
			wantSummary: "quoted core identifiers are not supported; use an unquoted identifier",
		},
		{
			name:        "invalid-state-claim-fields.thtr",
			wantCode:    "thtr_lower_error",
			wantLine:    16,
			wantSummary: "state.claim fields only supports exact top-level field matching",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			path := expressivenessFixturePath(t, test.name)
			_, err := LoadFileDetailed(path, nil)
			if err == nil {
				t.Fatal("expected invalid fixture to fail, got nil")
			}

			var diagnosticError *DiagnosticError
			if !errors.As(err, &diagnosticError) {
				t.Fatalf("expected diagnostic error, got %T", err)
			}

			diagnostic := diagnosticError.Diagnostic()
			if got, want := diagnostic.Code, test.wantCode; got != want {
				t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
			}
			if got, want := diagnostic.Span.File, path; got != want {
				t.Fatalf("diagnostic source file mismatch: got %q want %q", got, want)
			}
			if got, want := diagnostic.Span.Line, test.wantLine; got != want {
				t.Fatalf("diagnostic source line mismatch: got %d want %d", got, want)
			}
			if got, want := diagnostic.Summary, test.wantSummary; got != want {
				t.Fatalf("diagnostic summary mismatch: got %q want %q", got, want)
			}
		})
	}
}

func requireExpressivenessSourceMapRange(t *testing.T, result LoadResult, specPath string, want sourceRange) {
	t.Helper()

	if result.sourceMap == nil {
		t.Fatal("source map must be present")
	}
	entry, ok := result.sourceMap.LookupSpecPath(specPath)
	if !ok {
		t.Fatalf("source map must contain %s", specPath)
	}
	if got := entry.Source.StartLine; got != want.startLine {
		t.Fatalf("source map start line mismatch for %s: got %d want %d", specPath, got, want.startLine)
	}
	if got := entry.Source.StartColumn; got != want.startColumn {
		t.Fatalf("source map start column mismatch for %s: got %d want %d", specPath, got, want.startColumn)
	}
	if got := entry.Source.EndLine; got != want.endLine {
		t.Fatalf("source map end line mismatch for %s: got %d want %d", specPath, got, want.endLine)
	}
	if got := entry.Source.EndColumn; got != want.endColumn {
		t.Fatalf("source map end column mismatch for %s: got %d want %d", specPath, got, want.endColumn)
	}
}

func requireExpressivenessActionPassed(t *testing.T, report theater.Report, path string) {
	t.Helper()

	node := findExpressivenessNodeReport(t, report, theater.NodeKindAction, path)
	if got, want := node.Status, theater.StatusPassed; got != want {
		t.Fatalf("action node status mismatch for %s: got %q want %q", path, got, want)
	}
}

func requireExpressivenessExpectationStatus(t *testing.T, report theater.Report, path string, want theater.Status) {
	t.Helper()

	node := findExpressivenessNodeReport(t, report, theater.NodeKindExpectation, path)
	if got := node.Status; got != want {
		t.Fatalf("expectation node status mismatch for %s: got %q want %q", path, got, want)
	}
}

func findExpressivenessNodeReport(t *testing.T, report theater.Report, kind theater.NodeKind, path string) theater.NodeReport {
	t.Helper()

	for i := range report.Nodes {
		node := report.Nodes[i]
		if node.Kind == kind && node.Path == path {
			return node
		}
	}

	t.Fatalf("node %q at path %q not found", kind, path)
	return theater.NodeReport{}
}

func expressivenessFixtureCatalog(t *testing.T) (*theater.Catalog, *theater.MatcherCatalog) {
	t.Helper()

	bundle, err := builtin.NewBundle()
	if err != nil {
		t.Fatalf("new builtin bundle failed: %v", err)
	}

	registerExpressivenessActions(t, bundle.Catalog)
	descriptors := append([]theater.MatcherDescriptor{}, builtinexpectation.Descriptors()...)
	descriptors = append(descriptors, smokeEqualMatcherDescriptor())
	matchers, err := theater.NewMatcherCatalog(descriptors...)
	if err != nil {
		t.Fatalf("new matcher catalog failed: %v", err)
	}

	return bundle.Catalog, matchers
}

func registerExpressivenessActions(t *testing.T, catalog *theater.Catalog) {
	t.Helper()

	if err := catalog.RegisterAction("action.smoke.echo", &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Inputs: map[string]theater.ValueContract{
				"value": {Kind: theater.ValueKindString, Required: true},
			},
			Outputs: map[string]theater.ValueContract{
				"echo": {Kind: theater.ValueKindString},
			},
		},
		RunFunc: func(args theater.Args) (theater.Outputs, error) {
			return theater.Outputs{"echo": args["value"]}, nil
		},
	}); err != nil {
		t.Fatalf("register smoke action failed: %v", err)
	}

	if err := catalog.RegisterAction("action.fixture.notifications", &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Inputs: map[string]theater.ValueContract{
				"case": {Kind: theater.ValueKindString, Required: true},
			},
			Outputs: map[string]theater.ValueContract{
				"body": {Kind: theater.ValueKindString},
			},
		},
		RunFunc: func(args theater.Args) (theater.Outputs, error) {
			switch args["case"] {
			case "zero":
				return theater.Outputs{"body": `{"items":[{"receiverAddress":"other@example.test","subject":"Verification Code","body":"Code 111111"}]}`}, nil
			case "multiple":
				return theater.Outputs{"body": `{"items":[{"receiverAddress":"demo@example.test","subject":"Verification Code","body":"Code 111111"},{"receiverAddress":"demo@example.test","subject":"Verification Code","body":"Code 222222"}]}`}, nil
			default:
				return nil, fmt.Errorf("unknown notifications fixture case %q", args["case"])
			}
		},
	}); err != nil {
		t.Fatalf("register notifications action failed: %v", err)
	}

	if err := catalog.RegisterAction("action.fixture.profile", &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"body": {Kind: theater.ValueKindString},
			},
		},
		Output: theater.Outputs{"body": `{"deleted_at":null,"name":"Demo"}`},
	}); err != nil {
		t.Fatalf("register profile action failed: %v", err)
	}
}

func smokeEqualMatcherDescriptor() theater.MatcherDescriptor {
	return theater.MatcherDescriptor{
		Ref: "matcher.smoke.equal",
		Args: []theater.MatcherArg{{
			Name:     "expected",
			Required: true,
			Accepts:  theater.ValueContract{Kind: theater.ValueKindAny},
		}},
		Actual: theater.ValueContract{Kind: theater.ValueKindAny},
		Sugar:  theater.SugarSpec{Form: theater.SugarFormNone},
		Compile: func(_ theater.MatcherCompileContext, args theater.Values) (theater.Matcher, error) {
			expected, ok := args["expected"]
			if !ok {
				return nil, errors.New(`matcher requires arg "expected"`)
			}

			return smokeEqualMatcher{expected: expected}, nil
		},
	}
}

type smokeEqualMatcher struct {
	expected any
}

func (m smokeEqualMatcher) Check(_ context.Context, actual any) error {
	if reflect.DeepEqual(actual, m.expected) {
		return nil
	}

	return theater.MismatchError(fmt.Errorf("actual %v does not equal expected %v", actual, m.expected))
}

func expressivenessFixturePath(t *testing.T, name string) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve fixture path failed")
	}

	return filepath.Join(filepath.Dir(file), "..", "..", "..", "testdata", "thtr-expressiveness", name)
}

func readExpressivenessFixture(t *testing.T, name string) []byte {
	t.Helper()

	data, err := os.ReadFile(expressivenessFixturePath(t, name))
	if err != nil {
		t.Fatalf("read fixture %s failed: %v", name, err)
	}

	return data
}
