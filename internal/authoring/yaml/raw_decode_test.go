package yaml

import (
	"strings"
	"testing"

	goyaml "gopkg.in/yaml.v3"
)

func TestDecodeRawStageRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	_, err := decodeRawStage(strings.NewReader(`
id: main
unknown_field: true
scenarios: []
scenario_calls: []
`))
	if err == nil {
		t.Fatal("expected strict yaml error")
	}

	if got, want := err.Error(), "line 3, col 1: field unknown_field not found in type"; got != want {
		t.Fatalf("error mismatch: got %q want %q", got, want)
	}
}

func TestDecodeRawStageRejectsMultipleDocuments(t *testing.T) {
	t.Parallel()

	_, err := decodeRawStage(strings.NewReader(`
id: main
scenarios: []
scenario_calls: []
---
id: secondary
scenarios: []
scenario_calls: []
`))
	if err == nil {
		t.Fatal("expected multiple document error")
	}

	if got, want := err.Error(), "multiple YAML documents are not supported"; got != want {
		t.Fatalf("error mismatch: got %q want %q", got, want)
	}
}

func TestDecodeRawStageCapturesSpansAndExpectationNodes(t *testing.T) {
	t.Parallel()

	raw, err := decodeRawStage(strings.NewReader(`id: main
scenarios:
  - id: login
    acts:
      - id: submit
        action:
          use: action.login
        expectations:
          - id: token
            subject:
              field: token
            assert:
              eq: issued-token
scenario_calls:
  - id: login-user
    scenario_id: login
`))
	if err != nil {
		t.Fatalf("decode raw stage failed: %v", err)
	}

	if raw.Span.Line == 0 || raw.Span.Column == 0 {
		t.Fatalf("stage span must be populated: %#v", raw.Span)
	}

	expectation := raw.Scenarios[0].Acts[0].Expectations[0]
	if expectation.Span.Line == 0 || expectation.Span.Column == 0 {
		t.Fatalf("expectation span must be populated: %#v", expectation.Span)
	}

	if expectation.Subject.Node == nil || expectation.Assert.Node == nil {
		t.Fatal("raw expectation nodes must be preserved")
	}

	if got, want := expectation.Subject.Node.Kind, goyaml.MappingNode; got != want {
		t.Fatalf("subject node kind mismatch: got %v want %v", got, want)
	}

	if got, want := expectation.Assert.Node.Kind, goyaml.MappingNode; got != want {
		t.Fatalf("assert node kind mismatch: got %v want %v", got, want)
	}
}

func TestDecodeRawStageCapturesPreflightNodes(t *testing.T) {
	t.Parallel()

	raw, err := decodeRawStage(strings.NewReader(`id: main
scenarios:
  - id: send-email
    inputs:
      recipient_email:
        type: string
    preflight:
      - id: recipient-test-domain
        input:
          ref: recipient_email
        assert:
          matches: '^[^@]+@example\.test$'
    acts:
      - id: send
        action:
          use: action.send
scenario_calls:
  - id: send-test
    scenario_id: send-email
`))
	if err != nil {
		t.Fatalf("decode raw stage failed: %v", err)
	}

	preflight := raw.Scenarios[0].Preflight[0]
	if preflight.Span.Line == 0 || preflight.Span.Column == 0 {
		t.Fatalf("preflight span must be populated: %#v", preflight.Span)
	}
	if got, want := preflight.Input.Ref, "recipient_email"; got != want {
		t.Fatalf("preflight input ref mismatch: got %q want %q", got, want)
	}
	if preflight.Assert.Node == nil {
		t.Fatal("raw preflight assert node must be preserved")
	}
}

func TestRawStageSpecUnmarshalRejectsDanglingMappingKey(t *testing.T) {
	t.Parallel()

	var raw rawStageSpec
	err := raw.UnmarshalYAML(malformedMappingNode(scalarYAMLNode(7, 3, "scenarios")))
	if err == nil {
		t.Fatal("expected malformed mapping error")
	}

	if got, want := err.Error(), "line 7, col 3: mapping key is missing a value"; got != want {
		t.Fatalf("error mismatch: got %q want %q", got, want)
	}
}
