package theater_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/internal/testkit"
)

type secretAwareAction struct {
	formattedArg string
}

type secretStructuredAction struct{}

func (a *secretAwareAction) Contract() theater.ActionContract {
	return theater.ActionContract{
		Inputs: map[string]theater.ValueContract{
			"token": {
				Kind:        theater.ValueKindString,
				Required:    true,
				Sensitivity: theater.SensitivitySecret,
				Capture:     theater.CaptureSummary,
			},
		},
		Outputs: map[string]theater.ValueContract{
			"token": {
				Kind:        theater.ValueKindString,
				Required:    true,
				Sensitivity: theater.SensitivitySecret,
				Capture:     theater.CaptureSummary,
			},
		},
	}
}

func (a *secretAwareAction) Run(_ context.Context, request theater.ActionRequest) (theater.Outputs, error) {
	value, ok := request.Args["token"].(theater.Secret)
	if !ok {
		return nil, fmt.Errorf("token arg type mismatch: got %T", request.Args["token"])
	}

	a.formattedArg = fmt.Sprintf("token=%v quoted=%q map=%v", value, value, theater.Args{"token": value})
	raw, ok := value.Reveal().(string)
	if !ok {
		return nil, fmt.Errorf("revealed token type mismatch: got %T", value.Reveal())
	}

	return theater.Outputs{"token": raw}, nil
}

func (secretStructuredAction) Contract() theater.ActionContract {
	return theater.ActionContract{
		Outputs: map[string]theater.ValueContract{
			"payload": {
				Kind:        theater.ValueKindObject,
				Required:    true,
				Sensitivity: theater.SensitivitySecret,
				Capture:     theater.CaptureSummary,
				Fields: map[string]theater.ValueContract{
					"token": {
						Kind: theater.ValueKindObject,
						Fields: map[string]theater.ValueContract{
							"id": {Kind: theater.ValueKindString},
						},
					},
				},
			},
		},
	}
}

func (secretStructuredAction) Run(_ context.Context, _ theater.ActionRequest) (theater.Outputs, error) {
	return theater.Outputs{
		"payload": map[string]any{
			"token": map[string]any{"id": "issued-token"},
		},
	}, nil
}

func TestRunnerWrapsSecretValuesAcrossRuntimeBoundaries(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID: "submit",
						Action: theater.ActionSpec{
							Use: "action.login",
							With: map[string]theater.BindingSpec{
								"token": {
									Kind:  theater.BindingKindLiteral,
									Value: "issued-token",
								},
							},
						},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "token",
								Subject: theater.SubjectSpec{Field: "token"},
								Assert:  theater.AssertSpec{Ref: "expectation.secret"},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	action := &secretAwareAction{}
	expectation := &testkit.ScriptedExpectation{
		CheckFunc: func(actual any) error {
			value, ok := actual.(theater.Secret)
			if !ok {
				return fmt.Errorf("actual type mismatch: got %T", actual)
			}

			if got, want := value.Reveal(), any("issued-token"); got != want {
				return fmt.Errorf("revealed actual mismatch: got %#v want %#v", got, want)
			}

			formatted := fmt.Sprintf("actual=%v quoted=%q", value, value)
			if strings.Contains(formatted, "issued-token") {
				return errors.New("formatted expectation actual leaked secret")
			}

			return nil
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(
		context.Background(),
		spec,
		catalog,
		matcherCatalog(t, expectation.Descriptor("expectation.secret")),
	)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf(
			"report status mismatch: got %s want %s diagnostics=%#v report=%#v",
			got,
			want,
			result.Diagnostics,
			result.Report,
		)
	}

	if strings.Contains(action.formattedArg, "issued-token") {
		t.Fatalf("action formatting leaked secret: %q", action.formattedArg)
	}

	actionNode := findActionNode(t, result.Report.Nodes, "stage.main/call.login-user/act.submit/action")
	if actionNode.Observations == nil {
		t.Fatal("action observations are required")
	}

	inputToken := actionNode.Observations.Inputs["token"]
	if inputToken.Preview == nil || !inputToken.Preview.Redacted {
		t.Fatalf("input token preview must be redacted: %#v", inputToken.Preview)
	}

	outputToken := actionNode.Observations.Outputs["token"]
	if outputToken.Preview == nil || !outputToken.Preview.Redacted {
		t.Fatalf("output token preview must be redacted: %#v", outputToken.Preview)
	}

	if got, want := inputToken.Preview.Text, "[redacted]"; got != want {
		t.Fatalf("input preview text mismatch: got %q want %q", got, want)
	}

	if got, want := outputToken.Preview.Text, "[redacted]"; got != want {
		t.Fatalf("output preview text mismatch: got %q want %q", got, want)
	}
}

func TestRunnerPreservesSecretSelectionFromStructuredOutputs(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID: "submit",
						Action: theater.ActionSpec{
							Use: "action.login",
						},
						Expectations: []theater.ExpectationSpec{
							{
								ID: "token",
								Subject: theater.SubjectSpec{
									Field: "payload",
									Path:  theater.JSONPointer("/token/id"),
								},
								Assert: theater.AssertSpec{Ref: "expectation.secret"},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	expectation := &testkit.ScriptedExpectation{
		CheckFunc: func(actual any) error {
			value, ok := actual.(theater.Secret)
			if !ok {
				return fmt.Errorf("actual type mismatch: got %T", actual)
			}

			if got, want := value.Reveal(), any("issued-token"); got != want {
				return fmt.Errorf("revealed actual mismatch: got %#v want %#v", got, want)
			}

			formatted := fmt.Sprintf("actual=%v quoted=%q", value, value)
			if strings.Contains(formatted, "issued-token") {
				return errors.New("formatted expectation actual leaked secret")
			}

			return nil
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", secretStructuredAction{}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(
		context.Background(),
		spec,
		catalog,
		matcherCatalog(t, expectation.Descriptor("expectation.secret")),
	)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %s want %s diagnostics=%#v report=%#v", got, want, result.Diagnostics, result.Report)
	}

	actionNode := findActionNode(t, result.Report.Nodes, "stage.main/call.login-user/act.submit/action")
	if actionNode.Observations == nil {
		t.Fatal("action observations are required")
	}

	outputPayload := actionNode.Observations.Outputs["payload"]
	if outputPayload.Preview == nil || !outputPayload.Preview.Redacted {
		t.Fatalf("output payload preview must be redacted: %#v", outputPayload.Preview)
	}
}

func findActionNode(t *testing.T, nodes []theater.NodeReport, path string) theater.NodeReport {
	t.Helper()

	for _, node := range nodes {
		if node.Kind == theater.NodeKindAction && node.Path == path {
			return node
		}
	}

	t.Fatalf("missing action node path=%q", path)
	return theater.NodeReport{}
}
