package yaml

import (
	"errors"
	"strings"
	"testing"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/internal/testkit/errtest"
	goyaml "gopkg.in/yaml.v3"
)

const (
	yamlLowerInvalidPathLocation = "line 3, col 7"
	yamlLowerRFC6901Fragment     = "RFC 6901"
)

func TestLowerStageBindsSourceFileAndMatcherSugar(t *testing.T) {
	t.Parallel()

	raw := rawStageSpec{
		ID:   "main",
		Name: "login-smoke",
		Scenarios: []rawScenarioSpec{
			{
				ID: "login",
				Acts: []rawActSpec{
					{
						ID: "submit",
						Action: rawActionSpec{
							Use:        "action.login",
							With:       map[string]rawBindingNode{"username": {Node: mustParseYAMLNode(t, `"alex"`)}},
							Repeatable: true,
							Span:       theater.SourceRef{Line: 6, Column: 9},
						},
						Expectations: []rawExpectationSpec{
							{
								ID:      "token",
								Subject: rawNode{Node: mustParseYAMLNode(t, "field: token\n")},
								Assert:  rawNode{Node: mustParseYAMLNode(t, "eq: issued-token\n")},
								Span:    theater.SourceRef{Line: 8, Column: 11},
							},
						},
						Span: theater.SourceRef{Line: 5, Column: 7},
					},
				},
				Span: theater.SourceRef{Line: 3, Column: 5},
			},
		},
		ScenarioCalls: []rawScenarioCallSpec{
			{
				ID:         "login-user",
				ScenarioID: "login",
				Span:       theater.SourceRef{Line: 12, Column: 5},
			},
		},
		Span: theater.SourceRef{Line: 1, Column: 1},
	}

	spec, err := lowerStage(raw, lowerTestMatcherResolver{
		descriptor: theater.MatcherDescriptor{
			Ref: "expectation.equal",
			Sugar: theater.SugarSpec{
				Keys:           []string{"eq"},
				Form:           theater.SugarFormUnary,
				PositionalArgs: []string{"expected"},
			},
		},
	}, "/tmp/login.yaml")
	if err != nil {
		t.Fatalf("lower stage failed: %v", err)
	}

	if got, want := spec.SourceSpan.File, "/tmp/login.yaml"; got != want {
		t.Fatalf("stage source file mismatch: got %q want %q", got, want)
	}

	if got, want := spec.Scenarios[0].Acts[0].Action.SourceSpan.File, "/tmp/login.yaml"; got != want {
		t.Fatalf("action source file mismatch: got %q want %q", got, want)
	}

	actionBinding := spec.Scenarios[0].Acts[0].Action.With["username"]
	if actionBinding.SourceSpan == nil {
		t.Fatal("action binding source span must be present")
	}
	if got, want := actionBinding.SourceSpan.File, "/tmp/login.yaml"; got != want {
		t.Fatalf("action binding source file mismatch: got %q want %q", got, want)
	}

	expectation := spec.Scenarios[0].Acts[0].Expectations[0]
	if got, want := expectation.Assert.Ref, "expectation.equal"; got != want {
		t.Fatalf("assert ref mismatch: got %q want %q", got, want)
	}

	expectedBinding := expectation.Assert.Args["expected"]
	if expectedBinding.SourceSpan == nil {
		t.Fatal("expectation binding source span must be present")
	}
	if got, want := expectedBinding.SourceSpan.File, "/tmp/login.yaml"; got != want {
		t.Fatalf("expectation binding source file mismatch: got %q want %q", got, want)
	}

	if got, want := expectation.Assert.Args["expected"].Kind, theater.BindingKindLiteral; got != want {
		t.Fatalf("assert binding kind mismatch: got %q want %q", got, want)
	}

	if got, want := expectation.Assert.Args["expected"].Value, "issued-token"; got != want {
		t.Fatalf("assert binding value mismatch: got %#v want %#v", got, want)
	}
}

func TestLowerStageLowersPreflightGuard(t *testing.T) {
	t.Parallel()

	raw := rawStageSpec{
		ID: "main",
		Scenarios: []rawScenarioSpec{{
			ID: "send-email",
			Inputs: map[string]theater.ValueContract{
				"recipient_email": {Kind: theater.ValueKindString},
			},
			Preflight: []rawPreflightSpec{{
				ID:    "recipient-test-domain",
				Input: rawPreflightRefSpec{Ref: "recipient_email"},
				Assert: rawNode{
					Node: mustParseYAMLNode(t, "matches: '^[^@]+@example\\.test$'\n"),
				},
				Override: &rawPreflightRefSpec{Ref: "allow_non_test_recipient"},
				Span:     theater.SourceRef{Line: 6, Column: 9},
			}},
			Acts: []rawActSpec{{
				ID: "send",
				Action: rawActionSpec{
					Use:  "action.send",
					Span: theater.SourceRef{Line: 10, Column: 9},
				},
			}},
		}},
	}

	spec, err := lowerStage(raw, lowerTestMatcherResolver{
		descriptor: theater.MatcherDescriptor{
			Ref: "expectation.matches",
			Sugar: theater.SugarSpec{
				Keys:           []string{"matches"},
				Form:           theater.SugarFormUnary,
				PositionalArgs: []string{"pattern"},
			},
		},
	}, "/tmp/send.yaml")
	if err != nil {
		t.Fatalf("lower stage failed: %v", err)
	}

	preflight := spec.Scenarios[0].Preflight[0]
	if got, want := preflight.ID, "recipient-test-domain"; got != want {
		t.Fatalf("preflight id mismatch: got %q want %q", got, want)
	}
	if got, want := preflight.Input.Name, "recipient_email"; got != want {
		t.Fatalf("preflight input mismatch: got %q want %q", got, want)
	}
	if preflight.Override == nil || preflight.Override.Name != "allow_non_test_recipient" {
		t.Fatalf("preflight override mismatch: %#v", preflight.Override)
	}
	if got, want := preflight.Assert.Ref, "expectation.matches"; got != want {
		t.Fatalf("preflight assert ref mismatch: got %q want %q", got, want)
	}
	if got, want := preflight.Assert.Args["pattern"].Value, `^[^@]+@example\.test$`; got != want {
		t.Fatalf("preflight pattern mismatch: got %#v want %#v", got, want)
	}
	if preflight.SourceSpan == nil || preflight.SourceSpan.File != "/tmp/send.yaml" {
		t.Fatalf("preflight source span mismatch: %#v", preflight.SourceSpan)
	}
}

func TestLowerStageLowersPropertyValueBinding(t *testing.T) {
	t.Parallel()

	raw := rawStageSpec{
		ID: "main",
		Scenarios: []rawScenarioSpec{{
			ID: "login",
			Acts: []rawActSpec{{
				ID: "submit",
				Properties: map[string]rawPropertySpec{
					"email": {
						Value: rawBindingNode{Node: mustParseYAMLNode(t, `
kind: coalesce
candidates:
  - kind: env
    name: THEATER_EMAIL
  - generated@example.test
`)},
					},
				},
				Action: rawActionSpec{Use: "action.generate"},
			}},
		}},
	}

	spec, err := lowerStage(raw, nil, "")
	if err != nil {
		t.Fatalf("lower stage failed: %v", err)
	}

	property := spec.Scenarios[0].Acts[0].Properties["email"]
	if property.Inventory != nil {
		t.Fatal("property inventory must be empty")
	}
	if property.Value == nil {
		t.Fatal("property value must be present")
	}
	if got, want := property.Value.Kind, theater.BindingKindCoalesce; got != want {
		t.Fatalf("property value kind mismatch: got %q want %q", got, want)
	}
	if got, want := property.Value.Candidates[0].Kind, theater.BindingKindEnv; got != want {
		t.Fatalf("first candidate kind mismatch: got %q want %q", got, want)
	}
}

func TestLowerBindingNodeBindsNestedYAMLSourceSpans(t *testing.T) {
	t.Parallel()

	source := `kind: object
object:
  payload:
    kind: list
    list:
      - "first-yaml"
      - kind: string
        parts:
          - "hello-yaml"
          - kind: generate
            generator: email
            domain: example.test
`
	binding, err := lowerBindingNodeWithSource(rawBindingNode{Node: mustParseYAMLNode(t, source)}, "/tmp/nested.yaml")
	if err != nil {
		t.Fatalf("lower binding failed: %v", err)
	}

	payload := binding.Object["payload"]
	requireYAMLSourceSpanAt(t, payload.SourceSpan, "/tmp/nested.yaml", source, "kind: list")
	requireYAMLSourceSpanAt(t, payload.List[0].SourceSpan, "/tmp/nested.yaml", source, `"first-yaml"`)
	requireYAMLSourceSpanAt(t, payload.List[1].SourceSpan, "/tmp/nested.yaml", source, "kind: string")
	requireYAMLSourceSpanAt(t, payload.List[1].Parts[0].SourceSpan, "/tmp/nested.yaml", source, `"hello-yaml"`)
	requireYAMLSourceSpanAt(t, payload.List[1].Parts[1].SourceSpan, "/tmp/nested.yaml", source, "kind: generate")
	requireYAMLSourceSpanAt(t, payload.List[1].Parts[1].Args["domain"].SourceSpan, "/tmp/nested.yaml", source, "example.test")
}

func TestLowerBindingNodeBindsYAMLSelectorArgumentSourceSpans(t *testing.T) {
	t.Parallel()

	source := `kind: ref
ref:
  name: notifications
  through:
    - pick:
        at: /receiver
        equals: expected@example.test
    - pick:
        where:
          - subject:
              path: /receiver
            assert:
              ref: expectation.equal
              args:
                expected: primary@example.test
`
	binding, err := lowerBindingNodeWithSource(rawBindingNode{Node: mustParseYAMLNode(t, source)}, "/tmp/selector.yaml")
	if err != nil {
		t.Fatalf("lower binding failed: %v", err)
	}

	requireYAMLSourceSpanAt(
		t,
		binding.Ref.Through[0].Pick.Equals.SourceSpan,
		"/tmp/selector.yaml",
		source,
		"expected@example.test",
	)
	requireYAMLSourceSpanAt(
		t,
		binding.Ref.Through[1].Pick.Where[0].Assert.Args["expected"].SourceSpan,
		"/tmp/selector.yaml",
		source,
		"primary@example.test",
	)
}

func TestLowerBindingNodeLowersTransformThroughStep(t *testing.T) {
	t.Parallel()

	source := `kind: ref
ref:
  name: token
  through:
    - transform:
        use: transform.jwt.claims
        with:
          audience: mobile
    - path: /uid
`
	binding, err := lowerBindingNodeWithSource(rawBindingNode{Node: mustParseYAMLNode(t, source)}, "/tmp/selector.yaml")
	if err != nil {
		t.Fatalf("lower binding failed: %v", err)
	}

	transform := binding.Ref.Through[0].Transform
	if transform == nil {
		t.Fatal("first through step must be transform")
	}
	if got, want := transform.Use, "transform.jwt.claims"; got != want {
		t.Fatalf("transform use mismatch: got %q want %q", got, want)
	}
	if got, want := transform.With["audience"], "mobile"; got != want {
		t.Fatalf("transform arg mismatch: got %#v want %#v", got, want)
	}
	if got, want := binding.Ref.Through[1].Path, theater.JSONPointer("/uid"); got != want {
		t.Fatalf("path step mismatch: got %q want %q", got, want)
	}
}

func TestDecodeBindingSpecOrLiteralRecognizesBindingShape(t *testing.T) {
	t.Parallel()

	binding, err := decodeBindingSpecOrLiteralWithSource(mustParseYAMLNode(t, `
kind: ref
ref:
  name: issued_token
`), "")
	if err != nil {
		t.Fatalf("decode binding failed: %v", err)
	}

	if got, want := binding.Kind, theater.BindingKindRef; got != want {
		t.Fatalf("binding kind mismatch: got %q want %q", got, want)
	}

	if binding.Ref == nil {
		t.Fatal("binding ref must be present")
	}

	if got, want := binding.Ref.Name, "issued_token"; got != want {
		t.Fatalf("binding ref mismatch: got %q want %q", got, want)
	}
}

func TestDecodeBindingSpecOrLiteralFallsBackToLiteral(t *testing.T) {
	t.Parallel()

	binding, err := decodeBindingSpecOrLiteralWithSource(mustParseYAMLNode(t, `"issued-token"`), "")
	if err != nil {
		t.Fatalf("decode literal binding failed: %v", err)
	}

	if got, want := binding.Kind, theater.BindingKindLiteral; got != want {
		t.Fatalf("binding kind mismatch: got %q want %q", got, want)
	}

	if got, want := binding.Value, "issued-token"; got != want {
		t.Fatalf("binding value mismatch: got %#v want %#v", got, want)
	}
}

func TestDecodeBindingSpecOrLiteralTreatsMatcherObjectAsLiteral(t *testing.T) {
	t.Parallel()

	binding, err := decodeBindingSpecOrLiteralWithSource(mustParseYAMLNode(t, `
ref: expectation.not
args:
  assert:
    ref: expectation.gte
    args:
      expected: 500
`), "")
	if err != nil {
		t.Fatalf("decode matcher object failed: %v", err)
	}

	if got, want := binding.Kind, theater.BindingKindLiteral; got != want {
		t.Fatalf("binding kind mismatch: got %q want %q", got, want)
	}

	value, ok := binding.Value.(map[string]any)
	if !ok {
		t.Fatalf("binding value type mismatch: got %T want map[string]any", binding.Value)
	}

	if got, want := value["ref"], "expectation.not"; got != want {
		t.Fatalf("binding ref mismatch: got %#v want %#v", got, want)
	}
}

func TestDecodeBindingSpecOrLiteralLowersGenerateBinding(t *testing.T) {
	t.Parallel()

	binding, err := decodeBindingSpecOrLiteralWithSource(mustParseYAMLNode(t, `
kind: generate
generator: email
domain: example.test
stem:
  kind: ref
  ref:
    name: stem
`), "")
	if err != nil {
		t.Fatalf("decode generator binding failed: %v", err)
	}

	if got, want := binding.Kind, theater.BindingKindGenerate; got != want {
		t.Fatalf("binding kind mismatch: got %q want %q", got, want)
	}

	if got, want := binding.Generator, "email"; got != want {
		t.Fatalf("binding generator mismatch: got %q want %q", got, want)
	}

	if got, want := binding.Args["domain"].Kind, theater.BindingKindLiteral; got != want {
		t.Fatalf("domain arg kind mismatch: got %q want %q", got, want)
	}

	if got, want := binding.Args["stem"].Kind, theater.BindingKindRef; got != want {
		t.Fatalf("stem arg kind mismatch: got %q want %q", got, want)
	}
}

func TestDecodeBindingSpecOrLiteralLowersDateGenerateBinding(t *testing.T) {
	t.Parallel()

	binding, err := decodeBindingSpecOrLiteralWithSource(mustParseYAMLNode(t, `
kind: generate
generator: date
format: basic
offset: 240h
`), "")
	if err != nil {
		t.Fatalf("decode date generator binding failed: %v", err)
	}

	if got, want := binding.Kind, theater.BindingKindGenerate; got != want {
		t.Fatalf("binding kind mismatch: got %q want %q", got, want)
	}
	if got, want := binding.Generator, "date"; got != want {
		t.Fatalf("binding generator mismatch: got %q want %q", got, want)
	}
	if got, want := binding.Args["format"].Value, "basic"; got != want {
		t.Fatalf("format arg mismatch: got %#v want %#v", got, want)
	}
	if got, want := binding.Args["offset"].Value, "240h"; got != want {
		t.Fatalf("offset arg mismatch: got %#v want %#v", got, want)
	}
}

func TestDecodeBindingSpecOrLiteralLowersCoalesceBinding(t *testing.T) {
	t.Parallel()

	binding, err := decodeBindingSpecOrLiteralWithSource(mustParseYAMLNode(t, `
kind: coalesce
candidates:
  - kind: ref
    ref:
      name: email
  - generated@example.test
`), "")
	if err != nil {
		t.Fatalf("decode coalesce binding failed: %v", err)
	}

	if got, want := binding.Kind, theater.BindingKindCoalesce; got != want {
		t.Fatalf("binding kind mismatch: got %q want %q", got, want)
	}

	if got, want := len(binding.Candidates), 2; got != want {
		t.Fatalf("candidate count mismatch: got %d want %d", got, want)
	}

	if got, want := binding.Candidates[0].Kind, theater.BindingKindRef; got != want {
		t.Fatalf("first candidate kind mismatch: got %q want %q", got, want)
	}

	if got, want := binding.Candidates[1].Kind, theater.BindingKindLiteral; got != want {
		t.Fatalf("second candidate kind mismatch: got %q want %q", got, want)
	}
}

func TestDecodeBindingSpecOrLiteralLowersEnvBinding(t *testing.T) {
	t.Parallel()

	binding, err := decodeBindingSpecOrLiteralWithSource(mustParseYAMLNode(t, `
kind: env
name: THEATER_EMAIL
`), "")
	if err != nil {
		t.Fatalf("decode env binding failed: %v", err)
	}

	if got, want := binding.Kind, theater.BindingKindEnv; got != want {
		t.Fatalf("binding kind mismatch: got %q want %q", got, want)
	}
	if got, want := binding.Env, "THEATER_EMAIL"; got != want {
		t.Fatalf("env name mismatch: got %q want %q", got, want)
	}
}

func TestDecodeBindingSpecOrLiteralLowersObjectRefSelector(t *testing.T) {
	t.Parallel()

	binding, err := decodeBindingSpecOrLiteralWithSource(mustParseYAMLNode(t, `
kind: ref
ref:
  name: payload
  decode: json
  path: /token/id
`), "")
	if err != nil {
		t.Fatalf("decode binding failed: %v", err)
	}

	if got, want := binding.Kind, theater.BindingKindRef; got != want {
		t.Fatalf("binding kind mismatch: got %q want %q", got, want)
	}

	if binding.Ref == nil {
		t.Fatal("binding ref must be present")
	}

	if got, want := binding.Ref.Name, "payload"; got != want {
		t.Fatalf("binding ref name mismatch: got %q want %q", got, want)
	}

	if got, want := binding.Ref.Decode, theater.DecodeJSON; got != want {
		t.Fatalf("binding ref decode mismatch: got %q want %q", got, want)
	}

	if got, want := binding.Ref.Path, theater.JSONPointer("/token/id"); got != want {
		t.Fatalf("binding ref path mismatch: got %q want %q", got, want)
	}
}

func TestLowerSubjectRejectsInvalidJSONPointerPath(t *testing.T) {
	t.Parallel()

	_, err := lowerSubject(mustParseYAMLNode(t, `
field: body
path: "#/token/id"
`), "")
	if err == nil {
		t.Fatal("expected invalid subject path error")
	}

	errtest.RequireContains(t, err, yamlLowerInvalidPathLocation)
	errtest.RequireContains(t, err, yamlLowerRFC6901Fragment)
}

func TestLowerSubjectAcceptsPropertySourceMapping(t *testing.T) {
	t.Parallel()

	subject, err := lowerSubject(mustParseYAMLNode(t, `
from: property
ref: notifications
path: /data
`), "")
	if err != nil {
		t.Fatalf("lower subject failed: %v", err)
	}

	if got, want := subject.From, theater.SubjectFromProperty; got != want {
		t.Fatalf("subject from mismatch: got %q want %q", got, want)
	}

	if got, want := subject.Ref, "notifications"; got != want {
		t.Fatalf("subject ref mismatch: got %q want %q", got, want)
	}

	if got, want := subject.Path, theater.JSONPointer("/data"); got != want {
		t.Fatalf("subject path mismatch: got %q want %q", got, want)
	}
}

func TestLoweringRejectsDanglingMappingKey(t *testing.T) {
	t.Parallel()

	fixedTupleDescriptor := theater.MatcherDescriptor{
		Ref: "expectation.between",
		Sugar: theater.SugarSpec{
			Keys:           []string{"between"},
			Form:           theater.SugarFormFixedTuple,
			PositionalArgs: []string{"min", "max"},
		},
	}

	tests := []struct {
		name string
		run  func(*goyaml.Node) error
		node *goyaml.Node
		want string
	}{
		{
			name: "subject mapping",
			run: func(node *goyaml.Node) error {
				_, err := lowerSubject(node, "")
				return err
			},
			node: malformedMappingNode(scalarYAMLNode(4, 5, "field")),
			want: "line 4, col 5: mapping key is missing a value",
		},
		{
			name: "assert sugar mapping",
			run: func(node *goyaml.Node) error {
				_, err := lowerAssert(node, lowerTestMatcherResolver{
					descriptor: theater.MatcherDescriptor{
						Ref: "expectation.equal",
						Sugar: theater.SugarSpec{
							Keys:           []string{"eq"},
							Form:           theater.SugarFormUnary,
							PositionalArgs: []string{"expected"},
						},
					},
				}, "")
				return err
			},
			node: malformedMappingNode(scalarYAMLNode(8, 7, "eq")),
			want: "line 8, col 7: mapping key is missing a value",
		},
		{
			name: "canonical assert mapping",
			run: func(node *goyaml.Node) error {
				_, err := lowerCanonicalAssert(node, "")
				return err
			},
			node: malformedMappingNode(scalarYAMLNode(12, 9, "ref")),
			want: "line 12, col 9: mapping key is missing a value",
		},
		{
			name: "binding map",
			run: func(node *goyaml.Node) error {
				_, err := decodeBindingMapWithSource(node, "")
				return err
			},
			node: malformedMappingNode(scalarYAMLNode(16, 11, "expected")),
			want: "line 16, col 11: mapping key is missing a value",
		},
		{
			name: "fixed tuple mapping",
			run: func(node *goyaml.Node) error {
				_, err := lowerFixedTupleMapping(node, fixedTupleDescriptor, "")
				return err
			},
			node: malformedMappingNode(scalarYAMLNode(20, 13, "min")),
			want: "line 20, col 13: mapping key is missing a value",
		},
		{
			name: "binding spec detection",
			run: func(node *goyaml.Node) error {
				_, err := decodeBindingSpecOrLiteralWithSource(node, "")
				return err
			},
			node: malformedMappingNode(scalarYAMLNode(24, 15, "kind")),
			want: "line 24, col 15: mapping key is missing a value",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.run(tt.node)
			if err == nil {
				t.Fatal("expected malformed mapping error")
			}

			if got := err.Error(); got != tt.want {
				t.Fatalf("error mismatch: got %q want %q", got, tt.want)
			}
		})
	}
}

type lowerTestMatcherResolver struct {
	descriptor theater.MatcherDescriptor
}

func (r lowerTestMatcherResolver) ResolveSugarKey(key string) (theater.MatcherDescriptor, error) {
	for _, sugarKey := range r.descriptor.Sugar.Keys {
		if key == sugarKey {
			return r.descriptor, nil
		}
	}
	return theater.MatcherDescriptor{}, errors.New("unexpected matcher sugar")
}

func mustParseYAMLNode(t *testing.T, source string) *goyaml.Node {
	t.Helper()

	decoder := goyaml.NewDecoder(strings.NewReader(source))

	var document goyaml.Node
	if err := decoder.Decode(&document); err != nil {
		t.Fatalf("decode yaml node failed: %v", err)
	}

	if len(document.Content) != 1 {
		t.Fatalf("yaml document content mismatch: got %d want 1", len(document.Content))
	}

	return document.Content[0]
}

func malformedMappingNode(content ...*goyaml.Node) *goyaml.Node {
	return &goyaml.Node{
		Kind:    goyaml.MappingNode,
		Content: content,
	}
}

func scalarYAMLNode(line, column int, value string) *goyaml.Node {
	return &goyaml.Node{
		Kind:   goyaml.ScalarNode,
		Tag:    "!!str",
		Value:  value,
		Line:   line,
		Column: column,
	}
}

func requireYAMLSourceSpanAt(
	t *testing.T,
	sourceRef *theater.SourceRef,
	file string,
	source string,
	needle string,
) {
	t.Helper()

	wantLine, wantColumn := yamlSourcePosition(t, source, needle)
	if sourceRef == nil {
		t.Fatalf("source span is nil, want %s:%d:%d", file, wantLine, wantColumn)
	}
	if got := sourceRef.File; got != file {
		t.Fatalf("source file mismatch for %q: got %q want %q", needle, got, file)
	}
	if got := sourceRef.Line; got != wantLine {
		t.Fatalf("source line mismatch for %q: got %d want %d", needle, got, wantLine)
	}
	if got := sourceRef.Column; got != wantColumn {
		t.Fatalf("source column mismatch for %q: got %d want %d", needle, got, wantColumn)
	}
}

func yamlSourcePosition(t *testing.T, source string, needle string) (line int, column int) {
	t.Helper()

	offset := strings.Index(source, needle)
	if offset < 0 {
		t.Fatalf("source fixture does not contain %q", needle)
	}

	line, column = 1, 1
	for _, char := range source[:offset] {
		if char == '\n' {
			line++
			column = 1
			continue
		}
		column++
	}

	return line, column
}
