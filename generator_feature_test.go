package theater

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

const testEmailGeneratorRef = "generator.test_email"

func registerTestEmailGenerator(t *testing.T, catalog *Catalog) {
	t.Helper()

	err := catalog.RegisterGenerator(testEmailGeneratorRef, GeneratorDef{
		Contract: GeneratorContract{
			Args: []ArgSpec{
				{
					Name:     "domain",
					Accepts:  ValueContract{Kind: ValueKindString},
					Required: true,
				},
			},
			Produces: ValueContract{Kind: ValueKindString, Required: true},
		},
		Validate: func(args Values) error {
			domain, err := runtimeStringArg(args, "domain")
			if err != nil {
				return err
			}
			if domain == "" {
				return fmt.Errorf("domain is required")
			}

			return nil
		},
		Generate: func(request GeneratorRequest) (any, error) {
			domain, err := runtimeStringArg(Values(request.Args), "domain")
			if err != nil {
				return nil, err
			}

			return fmt.Sprintf("user-%s-%d@%s", request.RunToken(6), request.SequenceIndex()+1, domain), nil
		},
	})
	if err != nil {
		t.Fatalf("register test generator failed: %v", err)
	}
}

func runtimeStringArg(values Values, key string) (string, error) {
	value, ok := values[key]
	if !ok {
		return "", fmt.Errorf("%s is required", key)
	}

	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s must be string", key)
	}

	return text, nil
}

type generatorEchoAction struct{}

type generatorExpectation struct {
	t *testing.T
}

func (generatorEchoAction) Contract() ActionContract {
	return ActionContract{
		Inputs: map[string]ValueContract{
			"email": {Kind: ValueKindString, Required: true},
		},
		Outputs: map[string]ValueContract{
			"email": {Kind: ValueKindString, Required: true},
		},
	}
}

func (generatorEchoAction) Run(_ context.Context, request ActionRequest) (Outputs, error) {
	return Outputs{"email": request.Args["email"]}, nil
}

func (g generatorExpectation) Resolve(ref string) (MatcherDescriptor, error) {
	if ref != "expectation.email" {
		return MatcherDescriptor{}, fmt.Errorf("unexpected matcher ref %q", ref)
	}

	return MatcherDescriptor{
		Ref:    ref,
		Actual: ValueContract{Kind: ValueKindString},
		Compile: func(MatcherCompileContext, Values) (Matcher, error) {
			return generatorExpectationMatcher{t: g.t}, nil
		},
	}, nil
}

type generatorExpectationMatcher struct {
	t *testing.T
}

func (m generatorExpectationMatcher) Check(_ context.Context, actual any) error {
	text, ok := actual.(string)
	if !ok {
		m.t.Fatalf("generated email type mismatch: got %T", actual)
	}
	if !strings.HasSuffix(text, "@example.test") {
		m.t.Fatalf("generated email mismatch: got %q", text)
	}

	return nil
}

func TestResolveGenerateBindingMemoizesPerScenarioCall(t *testing.T) {
	t.Parallel()

	catalog := NewCatalog()
	registerTestEmailGenerator(t, catalog)

	runtime := newGenerationRuntime(time.Date(2026, time.March, 28, 9, 0, 0, 0, time.UTC))
	binding := bindingPlan{
		Path:      "stage.main/scenario.register/call.bindings/email",
		Kind:      BindingKindGenerate,
		Generator: testEmailGeneratorRef,
		Args: map[string]bindingPlan{
			"domain": {
				Path:  "stage.main/scenario.register/call.bindings/email.domain",
				Kind:  BindingKindLiteral,
				Value: "example.test",
			},
		},
	}

	firstResolver := newReferenceResolver(nil).withGeneration(catalog, runtime, executionIdentity{
		scenarioCallID: "register-1",
		scenarioSeq:    1,
	})
	first, err := firstResolver.ResolveBinding(binding)
	if err != nil {
		t.Fatalf("resolve first generated binding failed: %v", err)
	}

	second, err := firstResolver.ResolveBinding(binding)
	if err != nil {
		t.Fatalf("resolve second generated binding failed: %v", err)
	}

	if got, want := first, second; got != want {
		t.Fatalf("memoized value mismatch: got %v want %v", got, want)
	}

	otherResolver := newReferenceResolver(nil).withGeneration(catalog, runtime, executionIdentity{
		scenarioCallID: "register-2",
		scenarioSeq:    2,
	})
	third, err := otherResolver.ResolveBinding(binding)
	if err != nil {
		t.Fatalf("resolve third generated binding failed: %v", err)
	}

	if first == third {
		t.Fatalf("distinct scenario calls must produce distinct values, got %q", first)
	}
}

func TestValidateGenerateBindingRejectsInvalidLiteralArgs(t *testing.T) {
	t.Parallel()

	catalog := NewCatalog()
	registerTestEmailGenerator(t, catalog)

	err := validateBindingContractWithResolver(catalog, nil, catalog, bindingPlan{
		Path:      "stage.main/scenario.register/call.bindings/email",
		Kind:      BindingKindGenerate,
		Generator: testEmailGeneratorRef,
		Args: map[string]bindingPlan{
			"domain": {
				Path:  "stage.main/scenario.register/call.bindings/email.domain",
				Kind:  BindingKindLiteral,
				Value: "",
			},
		},
	}, ValueContract{Kind: ValueKindString})
	if err == nil {
		t.Fatal("expected invalid generator config error")
	}

	if got := err.Error(); !strings.Contains(got, "domain is required") {
		t.Fatalf("generator validation mismatch: got %q", got)
	}
}

func TestRunnerReportsGenerationMetadata(t *testing.T) {
	t.Parallel()

	catalog := NewCatalog()
	registerTestEmailGenerator(t, catalog)
	if err := catalog.RegisterAction("action.echo", generatorEchoAction{}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	recorder := &recordingEventRecorder{}
	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{
			{
				ID: "generate",
				Acts: []ActSpec{
					{
						ID: "fixtures",
						Action: ActionSpec{
							Use: "action.echo",
							With: map[string]BindingSpec{
								"email": {
									Kind:      BindingKindGenerate,
									Generator: testEmailGeneratorRef,
									Args: map[string]BindingSpec{
										"domain": {Kind: BindingKindLiteral, Value: "example.test"},
									},
								},
							},
						},
						Expectations: []ExpectationSpec{
							{
								ID: "generated-email",
								Subject: SubjectSpec{
									Field: "email",
								},
								Assert: AssertSpec{
									Ref: "expectation.email",
								},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []ScenarioCallSpec{
			{ID: "generate-1", ScenarioID: "generate"},
		},
	}

	result, err := NewRunner(catalog, generatorExpectation{t: t}).Run(context.Background(), spec, RunOptions{Events: recorder})
	if err != nil {
		t.Fatalf("runner run failed: %v", err)
	}

	if got, want := result.Report.Status, StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %s want %s", got, want)
	}

	if result.Report.Generation == nil {
		t.Fatal("report generation metadata must be present")
	}
	if result.Report.Generation.Seed == "" {
		t.Fatal("report generation seed must be present")
	}
	if result.Report.Generation.BaseTime.IsZero() {
		t.Fatal("report generation base time must be present")
	}

	events := recorder.Snapshot()
	if len(events) == 0 {
		t.Fatal("expected recorded events")
	}
	if events[0].Generation == nil {
		t.Fatal("stage running event must carry generation metadata")
	}
	if events[len(events)-1].Generation == nil {
		t.Fatal("stage finished event must carry generation metadata")
	}

	projected, err := NewProjector().Project(events)
	if err != nil {
		t.Fatalf("project events failed: %v", err)
	}

	if !reflect.DeepEqual(projected.Generation, result.Report.Generation) {
		t.Fatalf("projected generation mismatch: got %#v want %#v", projected.Generation, result.Report.Generation)
	}
}
