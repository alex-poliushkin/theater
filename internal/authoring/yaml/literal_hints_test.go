package yaml

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alex-poliushkin/theater"
)

func TestLiteralWrapperHintsReportsRedundantActionBinding(t *testing.T) {
	t.Parallel()

	source := `id: main
scenarios:
  - id: login
    acts:
      - id: submit
        action:
          use: action.http
          with:
            method:
              kind: literal
              value: GET
`

	hints, err := LiteralWrapperHints(strings.NewReader(source), "/tmp/stage.yaml")
	if err != nil {
		t.Fatalf("literal wrapper hints failed: %v", err)
	}

	if got, want := len(hints), 1; got != want {
		t.Fatalf("hint count mismatch: got %d want %d: %#v", got, want, hints)
	}

	hint := hints[0]
	if got, want := hint.Code, redundantLiteralWrapperCode; got != want {
		t.Fatalf("hint code mismatch: got %q want %q", got, want)
	}
	if got, want := hint.Severity, theater.SeverityHint; got != want {
		t.Fatalf("hint severity mismatch: got %q want %q", got, want)
	}
	if got, want := hint.Path, "stage.main/scenario.login/act.submit/action/binding.method"; got != want {
		t.Fatalf("hint path mismatch: got %q want %q", got, want)
	}
	if got, want := hint.Span.File, "/tmp/stage.yaml"; got != want {
		t.Fatalf("hint source file mismatch: got %q want %q", got, want)
	}
	if got, want := hint.Span.Line, 10; got != want {
		t.Fatalf("hint source line mismatch: got %d want %d", got, want)
	}
	if hint.Span.Column == 0 {
		t.Fatalf("hint source column must be set: %#v", hint.Span)
	}
}

func TestLiteralWrapperHintsIgnoreCanonicalBindingForms(t *testing.T) {
	t.Parallel()

	source := `id: main
scenarios:
  - id: login
    acts:
      - id: submit
        action:
          use: action.http
          with:
            method: GET
            url:
              kind: ref
              ref:
                name: base_url
`

	hints, err := LiteralWrapperHints(strings.NewReader(source), "/tmp/stage.yaml")
	if err != nil {
		t.Fatalf("literal wrapper hints failed: %v", err)
	}

	if len(hints) != 0 {
		t.Fatalf("canonical forms must not produce hints: %#v", hints)
	}
}

func TestLiteralWrapperHintsKeepWrappersForReservedLiteralObjectKeys(t *testing.T) {
	t.Parallel()

	for _, key := range []string{"kind", "value", "ref", "object", "list", "parts", "generator"} {
		key := key
		t.Run(key, func(t *testing.T) {
			t.Parallel()

			source := fmt.Sprintf(`id: main
scenarios:
  - id: login
    acts:
      - id: submit
        action:
          use: action.http
          with:
            payload:
              kind: literal
              value:
                %s: reserved
`, key)

			hints, err := LiteralWrapperHints(strings.NewReader(source), "/tmp/stage.yaml")
			if err != nil {
				t.Fatalf("literal wrapper hints failed: %v", err)
			}

			if len(hints) != 0 {
				t.Fatalf("required literal wrapper must not produce hints: %#v", hints)
			}
		})
	}
}

func TestLiteralWrapperHintsReportNestedWrappers(t *testing.T) {
	t.Parallel()

	source := `id: main
scenarios:
  - id: login
    acts:
      - id: submit
        action:
          use: action.http
          with:
            payload:
              kind: object
              object:
                email:
                  kind: literal
                  value: user@example.test
                tokens:
                  kind: list
                  list:
                    - kind: literal
                      value: first
                    - kind: string
                      parts:
                        - kind: literal
                          value: hello
                generated:
                  kind: generate
                  generator: email
                  domain:
                    kind: literal
                    value: example.test
        exports:
          - as: selected
            ref:
              name: response
              through:
                - pick:
                    at: /items/0
                    equals:
                      kind: literal
                      value: ok
`

	hints, err := LiteralWrapperHints(strings.NewReader(source), "/tmp/stage.yaml")
	if err != nil {
		t.Fatalf("literal wrapper hints failed: %v", err)
	}

	wantPaths := map[string]struct{}{
		"stage.main/scenario.login/act.submit/action/binding.payload/binding.email":                                {},
		"stage.main/scenario.login/act.submit/action/binding.payload/binding.generated/binding.domain":             {},
		"stage.main/scenario.login/act.submit/action/binding.payload/binding.tokens/binding.item-0":                {},
		"stage.main/scenario.login/act.submit/action/binding.payload/binding.tokens/binding.item-1/binding.part-0": {},
		"stage.main/scenario.login/act.submit/export.selected/through.0/binding.equals":                            {},
	}
	if got, want := len(hints), len(wantPaths); got != want {
		t.Fatalf("hint count mismatch: got %d want %d: %#v", got, want, hints)
	}

	for _, hint := range hints {
		if _, ok := wantPaths[hint.Path]; !ok {
			t.Fatalf("unexpected hint path %q in %#v", hint.Path, hints)
		}
	}
}

func TestLiteralWrapperHintsReportMatcherSugarWrapper(t *testing.T) {
	t.Parallel()

	source := `id: main
scenarios:
  - id: login
    acts:
      - id: submit
        action:
          use: action.http
        expectations:
          - id: status
            subject: status_code
            assert:
              eq:
                kind: literal
                value: 200
`

	hints, err := literalWrapperHints(strings.NewReader(source), "/tmp/stage.yaml", literalWrapperHintOptions{
		matchers: literalHintSugarResolver{
			"eq": {
				Ref: "expectation.equal",
				Sugar: theater.SugarSpec{
					Form:           theater.SugarFormUnary,
					PositionalArgs: []string{"expected"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("literal wrapper hints failed: %v", err)
	}

	if got, want := len(hints), 1; got != want {
		t.Fatalf("hint count mismatch: got %d want %d: %#v", got, want, hints)
	}
	if got, want := hints[0].Path, "stage.main/scenario.login/act.submit/expectation.status/assert/binding.expected"; got != want {
		t.Fatalf("hint path mismatch: got %q want %q", got, want)
	}
}

func TestLiteralWrapperHintsReportFixedTupleMatcherSugarWrappers(t *testing.T) {
	t.Parallel()

	source := `id: main
scenarios:
  - id: login
    acts:
      - id: submit
        action:
          use: action.http
        expectations:
          - id: status
            subject: status_code
            assert:
              between:
                - kind: literal
                  value: 200
                - kind: literal
                  value: 299
`

	hints, err := literalWrapperHints(strings.NewReader(source), "/tmp/stage.yaml", literalWrapperHintOptions{
		matchers: literalHintSugarResolver{
			"between": {
				Ref: "expectation.between",
				Sugar: theater.SugarSpec{
					Form:           theater.SugarFormFixedTuple,
					PositionalArgs: []string{"min", "max"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("literal wrapper hints failed: %v", err)
	}

	wantPaths := map[string]struct{}{
		"stage.main/scenario.login/act.submit/expectation.status/assert/binding.min": {},
		"stage.main/scenario.login/act.submit/expectation.status/assert/binding.max": {},
	}
	if got, want := len(hints), len(wantPaths); got != want {
		t.Fatalf("hint count mismatch: got %d want %d: %#v", got, want, hints)
	}
	for _, hint := range hints {
		if _, ok := wantPaths[hint.Path]; !ok {
			t.Fatalf("unexpected hint path %q in %#v", hint.Path, hints)
		}
	}
}

func TestLiteralWrapperHintsReportScenarioCallBindings(t *testing.T) {
	t.Parallel()

	source := `id: main
scenario_calls:
  - id: smoke-login
    scenario_id: auth/login
    bindings:
      method:
        kind: literal
        value: POST
scenarios: []
`

	hints, err := LiteralWrapperHints(strings.NewReader(source), "/tmp/stage.yaml")
	if err != nil {
		t.Fatalf("literal wrapper hints failed: %v", err)
	}

	if got, want := len(hints), 1; got != want {
		t.Fatalf("hint count mismatch: got %d want %d: %#v", got, want, hints)
	}
	if got, want := hints[0].Path, "stage.main/call.smoke-login/binding.method"; got != want {
		t.Fatalf("hint path mismatch: got %q want %q", got, want)
	}
}

func TestLiteralWrapperHintsForLocationIncludesSelectedFlowLibraryScenarios(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	flowPath := filepath.Join(root, "theater", "flows", "auth", "login-smoke.yaml")
	libraryPath := filepath.Join(root, "theater", "lib", "auth", "login.yaml")
	writeLiteralHintTestFile(t, flowPath, `id: login-smoke
scenario_calls:
  - id: smoke-login
    scenario_id: auth/login
    bindings:
      method:
        kind: literal
        value: POST
scenarios: []
`)
	writeLiteralHintTestFile(t, libraryPath, `id: auth-library
scenarios:
  - id: auth/login
    acts:
      - id: request
        action:
          use: action.command
          with:
            executable:
              kind: literal
              value: echo
  - id: auth/unused
    acts:
      - id: request
        action:
          use: action.command
          with:
            executable:
              kind: literal
              value: ignored
scenario_calls: []
`)

	location, err := ResolveFlowFileLocation(flowPath)
	if err != nil {
		t.Fatalf("resolve flow location failed: %v", err)
	}

	hints, err := LiteralWrapperHintsForLocation(location, nil)
	if err != nil {
		t.Fatalf("flow literal wrapper hints failed: %v", err)
	}

	wantPaths := map[string]struct{}{
		"stage.login-smoke/call.smoke-login/binding.method":                            {},
		"stage.login-smoke/scenario.auth~1login/act.request/action/binding.executable": {},
	}
	if got, want := len(hints), len(wantPaths); got != want {
		t.Fatalf("hint count mismatch: got %d want %d: %#v", got, want, hints)
	}
	for _, hint := range hints {
		if _, ok := wantPaths[hint.Path]; !ok {
			t.Fatalf("unexpected hint path %q in %#v", hint.Path, hints)
		}
		if hint.Path == "stage.login-smoke/scenario.auth~1login/act.request/action/binding.executable" {
			if got, want := hint.Span.File, libraryPath; got != want {
				t.Fatalf("library hint source file mismatch: got %q want %q", got, want)
			}
		}
	}
}

type literalHintSugarResolver map[string]theater.MatcherDescriptor

func (r literalHintSugarResolver) ResolveSugarKey(key string) (theater.MatcherDescriptor, error) {
	descriptor, ok := r[key]
	if !ok {
		return theater.MatcherDescriptor{}, errors.New("matcher sugar key not found")
	}

	return descriptor, nil
}

func writeLiteralHintTestFile(t *testing.T, path, body string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("make test dir failed: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write test file failed: %v", err)
	}
}
