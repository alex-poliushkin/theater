package theater

import (
	"strings"
	"testing"
)

func TestCompileStageSpecAssignsStablePathsAndOrdinals(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{
			{
				ID: "confirm",
				Acts: []ActSpec{
					{ID: "prepare", Action: ActionSpec{Use: "action.prepare"}},
				},
			},
			{
				ID: "login",
				Acts: []ActSpec{
					{ID: "submit", Action: ActionSpec{Use: "action.submit"}},
					{ID: "verify", Action: ActionSpec{Use: "action.verify"}},
				},
			},
		},
		ScenarioCalls: []ScenarioCallSpec{
			{ID: "prepare-user", ScenarioID: "confirm"},
			{
				ID:         "login-user",
				ScenarioID: "login",
				Dependencies: []ScenarioDependencySpec{
					{CallID: "prepare-user"},
				},
			},
		},
	}

	stage := compileStageSpec(spec)

	if got, want := stage.Path, "stage.main"; got != want {
		t.Fatalf("stage path mismatch: got %q want %q", got, want)
	}

	if got, want := stage.PlanOrdinal, 1; got != want {
		t.Fatalf("stage ordinal mismatch: got %d want %d", got, want)
	}

	if got, want := stage.Scenarios[0].Path, "stage.main/scenario.confirm"; got != want {
		t.Fatalf("scenario path mismatch: got %q want %q", got, want)
	}

	if got, want := stage.Scenarios[0].PlanOrdinal, 2; got != want {
		t.Fatalf("scenario ordinal mismatch: got %d want %d", got, want)
	}

	if got, want := stage.Scenarios[0].Acts[0].Path, "stage.main/scenario.confirm/act.prepare"; got != want {
		t.Fatalf("act path mismatch: got %q want %q", got, want)
	}

	if got, want := stage.Scenarios[0].Acts[0].PlanOrdinal, 3; got != want {
		t.Fatalf("act ordinal mismatch: got %d want %d", got, want)
	}

	if got, want := stage.ScenarioCalls[0].Path, "stage.main/call.prepare-user"; got != want {
		t.Fatalf("scenario call path mismatch: got %q want %q", got, want)
	}

	if got, want := stage.ScenarioCalls[0].PlanOrdinal, 7; got != want {
		t.Fatalf("scenario call ordinal mismatch: got %d want %d", got, want)
	}

	if got, want := stage.ScenarioCalls[1].Dependencies[0].When, TriggerPredicateSuccess; got != want {
		t.Fatalf("dependency predicate mismatch: got %q want %q", got, want)
	}
}

func TestValidateStageSpecReportsMissingTransitionTargetsInStableOrder(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{
			{
				ID: "login",
				Acts: []ActSpec{
					{
						ID:     "entry",
						Action: ActionSpec{Use: "action.entry"},
						Transitions: []TransitionSpec{
							{On: TransitionOnPass, To: "missing-first"},
						},
					},
					{
						ID:     "finish",
						Action: ActionSpec{Use: "action.finish"},
						Transitions: []TransitionSpec{
							{On: TransitionOnFail, To: "missing-second"},
						},
					},
				},
			},
		},
		ScenarioCalls: []ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	stage := compileStageSpec(spec)
	diagnostics := validateStage(stage)

	if got, want := len(diagnostics), 2; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := diagnostics[0].Code, "missing_transition_target"; got != want {
		t.Fatalf("first diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[0].Path, "stage.main/scenario.login/act.entry"; got != want {
		t.Fatalf("first diagnostic path mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[1].Code, "missing_transition_target"; got != want {
		t.Fatalf("second diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[1].Path, "stage.main/scenario.login/act.finish"; got != want {
		t.Fatalf("second diagnostic path mismatch: got %q want %q", got, want)
	}
}

func TestCompileStageSpecEscapesSlashSeparatedScenarioIDsInInternalPaths(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{
			{
				ID: "auth/login",
				Acts: []ActSpec{
					{ID: "submit", Action: ActionSpec{Use: "action.submit"}},
				},
			},
		},
		ScenarioCalls: []ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "auth/login"},
		},
	}

	stage := compileStageSpec(spec)

	if got, want := stage.Scenarios[0].Path, "stage.main/scenario.auth~1login"; got != want {
		t.Fatalf("scenario path mismatch: got %q want %q", got, want)
	}

	if got, want := stage.Scenarios[0].Acts[0].Path, "stage.main/scenario.auth~1login/act.submit"; got != want {
		t.Fatalf("act path mismatch: got %q want %q", got, want)
	}
}

func TestCompileStageSpecEscapesDottedIDsInInternalPaths(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main.v1",
		Scenarios: []ScenarioSpec{
			{
				ID: "auth/login.v1",
				Acts: []ActSpec{
					{ID: "wait.ready", Action: ActionSpec{Use: "action.submit"}},
				},
			},
		},
		ScenarioCalls: []ScenarioCallSpec{
			{ID: "login.user", ScenarioID: "auth/login.v1"},
		},
	}

	stage := compileStageSpec(spec)

	if got, want := stage.Path, "stage.main~2v1"; got != want {
		t.Fatalf("stage path mismatch: got %q want %q", got, want)
	}

	if got, want := stage.Scenarios[0].Path, "stage.main~2v1/scenario.auth~1login~2v1"; got != want {
		t.Fatalf("scenario path mismatch: got %q want %q", got, want)
	}

	if got, want := stage.Scenarios[0].Acts[0].Path, "stage.main~2v1/scenario.auth~1login~2v1/act.wait~2ready"; got != want {
		t.Fatalf("act path mismatch: got %q want %q", got, want)
	}

	if got, want := stage.ScenarioCalls[0].Path, "stage.main~2v1/call.login~2user"; got != want {
		t.Fatalf("scenario call path mismatch: got %q want %q", got, want)
	}
}

func TestValidateStageSpecReportsScenarioDependencyCycle(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{
			{
				ID: "prepare",
				Acts: []ActSpec{
					{ID: "bootstrap", Action: ActionSpec{Use: "action.prepare"}},
				},
			},
			{
				ID: "login",
				Acts: []ActSpec{
					{ID: "submit", Action: ActionSpec{Use: "action.login"}},
				},
			},
		},
		ScenarioCalls: []ScenarioCallSpec{
			{
				ID:         "prepare-user",
				ScenarioID: "prepare",
				Dependencies: []ScenarioDependencySpec{
					{CallID: "login-user"},
				},
			},
			{
				ID:         "login-user",
				ScenarioID: "login",
				Dependencies: []ScenarioDependencySpec{
					{CallID: "prepare-user"},
				},
			},
		},
	}

	stage := compileStageSpec(spec)
	diagnostics := validateStage(stage)

	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := diagnostics[0].Code, "scenario_dependency_cycle"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[0].Path, "stage.main"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}
}

func TestValidateStageSpecUsesExactCanonicalScenarioAddressLookup(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{
			{
				ID: "auth/login",
				Acts: []ActSpec{
					{ID: "submit", Action: ActionSpec{Use: "action.submit"}},
				},
			},
		},
		ScenarioCalls: []ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "theater/lib/auth/login.yaml"},
		},
	}

	stage := compileStageSpec(spec)
	diagnostics := validateStage(stage)

	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := diagnostics[0].Code, "missing_scenario_ref"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[0].Path, "stage.main/call.login-user"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}
}

func TestValidateStageSpecRejectsInvalidCanonicalScenarioAddresses(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{
			{
				ID: "auth//login",
				Acts: []ActSpec{
					{ID: "submit", Action: ActionSpec{Use: "action.submit"}},
				},
			},
		},
		ScenarioCalls: []ScenarioCallSpec{
			{ID: "invalid-leading", ScenarioID: "/auth/login"},
			{ID: "invalid-current", ScenarioID: "./auth/login"},
		},
	}

	stage := compileStageSpec(spec)
	diagnostics := validateStage(stage)

	if got, want := len(diagnostics), 3; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := diagnostics[0].Code, "invalid_scenario_call_scenario_id"; got != want {
		t.Fatalf("first diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[0].Path, "stage.main/call.invalid-current"; got != want {
		t.Fatalf("first diagnostic path mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[1].Code, "invalid_scenario_call_scenario_id"; got != want {
		t.Fatalf("second diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[1].Path, "stage.main/call.invalid-leading"; got != want {
		t.Fatalf("second diagnostic path mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[2].Code, "invalid_scenario_id"; got != want {
		t.Fatalf("third diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[2].Path, "stage.main/scenario.auth~1~1login"; got != want {
		t.Fatalf("third diagnostic path mismatch: got %q want %q", got, want)
	}
}

func TestValidateStageSpecRejectsDirectCallsIntoInternalScenarioNamespace(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{
			{
				ID: "identity/internal/bootstrap",
				Acts: []ActSpec{
					{ID: "submit", Action: ActionSpec{Use: "action.submit"}},
				},
			},
		},
		ScenarioCalls: []ScenarioCallSpec{
			{ID: "bootstrap-user", ScenarioID: "identity/internal/bootstrap"},
		},
	}

	stage := compileStageSpec(spec)
	diagnostics := validateStage(stage)

	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := diagnostics[0].Code, "internal_scenario_not_accessible"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[0].Path, "stage.main/call.bootstrap-user"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}
}

func TestValidateStageSpecEncodesDottedBindingAndExportNamesInDiagnosticPaths(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{
			{
				ID: "login",
				Acts: []ActSpec{
					{
						ID:     "submit",
						Action: ActionSpec{Use: "action.submit"},
						Exports: []ExportSpec{
							{As: "issued.token"},
						},
					},
				},
			},
		},
		ScenarioCalls: []ScenarioCallSpec{
			{
				ID:         "login-user",
				ScenarioID: "login",
				Bindings: map[string]BindingSpec{
					"profile.name": {Kind: BindingKindLiteral, Value: "alice"},
				},
			},
		},
	}

	stage := compileStageSpec(spec)
	diagnostics := validateStage(stage)

	if got, want := len(diagnostics), 2; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := diagnostics[0].Code, "unknown_scenario_input"; got != want {
		t.Fatalf("first diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[0].Path, "stage.main/call.login-user/binding.profile~2name"; got != want {
		t.Fatalf("first diagnostic path mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[1].Code, "missing_act_export_field"; got != want {
		t.Fatalf("second diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[1].Path, "stage.main/scenario.login/act.submit/export.issued~2token"; got != want {
		t.Fatalf("second diagnostic path mismatch: got %q want %q", got, want)
	}
}

func TestValidateStageSpecRejectsRootInternalScenarioNamespace(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{
			{
				ID: "internal/bootstrap",
				Acts: []ActSpec{
					{ID: "submit", Action: ActionSpec{Use: "action.submit"}},
				},
			},
		},
		ScenarioCalls: []ScenarioCallSpec{
			{ID: "bootstrap-user", ScenarioID: "internal/bootstrap"},
		},
	}

	stage := compileStageSpec(spec)
	diagnostics := validateStage(stage)

	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := diagnostics[0].Code, "internal_scenario_not_accessible"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
}

func TestValidateStageSpecReportsActTransitionCycle(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{
			{
				ID: "login",
				Acts: []ActSpec{
					{
						ID:     "submit",
						Action: ActionSpec{Use: "action.submit"},
						Transitions: []TransitionSpec{
							{On: TransitionOnPass, To: "verify"},
						},
					},
					{
						ID:     "verify",
						Action: ActionSpec{Use: "action.verify"},
						Transitions: []TransitionSpec{
							{On: TransitionOnPass, To: "submit"},
						},
					},
				},
			},
		},
		ScenarioCalls: []ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	stage := compileStageSpec(spec)
	diagnostics := validateStage(stage)

	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := diagnostics[0].Code, "act_transition_cycle"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[0].Path, "stage.main/scenario.login"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}
}

func TestValidateStageSpecReportsDuplicateScenarioAndCallIDsInStableOrder(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{
			{
				ID: "login",
				Acts: []ActSpec{
					{ID: "submit", Action: ActionSpec{Use: "action.login"}},
				},
			},
			{
				ID: "login",
				Acts: []ActSpec{
					{ID: "submit-again", Action: ActionSpec{Use: "action.login"}},
				},
			},
		},
		ScenarioCalls: []ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	stage := compileStageSpec(spec)
	diagnostics := validateStage(stage)

	if got, want := len(diagnostics), 2; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := diagnostics[0].Code, "duplicate_scenario_call_id"; got != want {
		t.Fatalf("first diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[0].Path, "stage.main/call.login-user"; got != want {
		t.Fatalf("first diagnostic path mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[1].Code, "duplicate_scenario_id"; got != want {
		t.Fatalf("second diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[1].Path, "stage.main/scenario.login"; got != want {
		t.Fatalf("second diagnostic path mismatch: got %q want %q", got, want)
	}
}

func TestValidateStageSpecReportsDuplicateActIDs(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{
			{
				ID: "login",
				Acts: []ActSpec{
					{ID: "submit", Action: ActionSpec{Use: "action.submit"}},
					{ID: "submit", Action: ActionSpec{Use: "action.verify"}},
				},
			},
		},
		ScenarioCalls: []ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	stage := compileStageSpec(spec)
	diagnostics := validateStage(stage)

	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := diagnostics[0].Code, "duplicate_act_id"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[0].Path, "stage.main/scenario.login/act.submit"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}
}

func TestValidateStageSpecReportsDuplicateExpectationIDs(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{
			{
				ID: "login",
				Acts: []ActSpec{
					{
						ID:     "submit",
						Action: ActionSpec{Use: "action.submit"},
						Expectations: []ExpectationSpec{
							{ID: "token", Subject: SubjectSpec{Field: "token"}, Assert: AssertSpec{Ref: "expectation.first"}},
							{ID: "token", Subject: SubjectSpec{Field: "token"}, Assert: AssertSpec{Ref: "expectation.second"}},
						},
					},
				},
			},
		},
		ScenarioCalls: []ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	stage := compileStageSpec(spec)
	diagnostics := validateStage(stage)

	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := diagnostics[0].Code, "duplicate_expectation_id"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[0].Path, "stage.main/scenario.login/act.submit/expectation.token"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}
}

func TestValidateStageSpecReportsInvalidActLogs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		logs            []LogSpec
		code            string
		path            string
		summaryContains string
		allowAdditional bool
	}{
		{
			name: "missing id",
			logs: []LogSpec{{
				Value: LogValueSpec{Field: "body"},
			}},
			code: "missing_log_id",
			path: "stage.main/scenario.login/act.submit/log",
		},
		{
			name: "duplicate id",
			logs: []LogSpec{
				{ID: "response", Value: LogValueSpec{Field: "status_code"}},
				{ID: "response", Value: LogValueSpec{Field: "body"}},
			},
			code: "duplicate_log_id",
			path: "stage.main/scenario.login/act.submit/log.response",
		},
		{
			name: "invalid format",
			logs: []LogSpec{{
				ID:     "response",
				Value:  LogValueSpec{Field: "body"},
				Format: LogFormat("raw"),
			}},
			code: "invalid_log_format",
			path: "stage.main/scenario.login/act.submit/log.response",
		},
		{
			name: "invalid capture",
			logs: []LogSpec{{
				ID:      "response",
				Value:   LogValueSpec{Field: "body"},
				Capture: CaptureArtifactRef,
			}},
			code: "invalid_log_capture",
			path: "stage.main/scenario.login/act.submit/log.response",
		},
		{
			name: "invalid sensitivity",
			logs: []LogSpec{{
				ID:          "response",
				Value:       LogValueSpec{Field: "body"},
				Sensitivity: SensitivityNone,
			}},
			code: "invalid_log_sensitivity",
			path: "stage.main/scenario.login/act.submit/log.response",
		},
		{
			name: "value with message",
			logs: []LogSpec{{
				ID:      "response",
				Value:   LogValueSpec{Field: "body"},
				Message: "response received",
			}},
			code:            "invalid_log_form",
			path:            "stage.main/scenario.login/act.submit/log.response",
			summaryContains: "must use either value or message with fields",
		},
		{
			name: "fields without message",
			logs: []LogSpec{{
				ID: "response",
				Fields: map[string]LogValueSpec{
					"status": {Field: "status_code"},
				},
			}},
			code:            "invalid_log_form",
			path:            "stage.main/scenario.login/act.submit/log.response",
			summaryContains: "fields require message",
		},
		{
			name: "missing value or message",
			logs: []LogSpec{{
				ID: "response",
			}},
			code:            "invalid_log_form",
			path:            "stage.main/scenario.login/act.submit/log.response",
			summaryContains: "must define value or message",
		},
		{
			name: "invalid field selector",
			logs: []LogSpec{{
				ID:    "response",
				Value: LogValueSpec{Field: "body.id"},
			}},
			code: "invalid_log_value_field",
			path: "stage.main/scenario.login/act.submit/log.response/value",
		},
		{
			name: "invalid ref selector",
			logs: []LogSpec{{
				ID:    "response",
				Value: LogValueSpec{Ref: "session/token"},
			}},
			code:            "invalid_log_value_ref",
			path:            "stage.main/scenario.login/act.submit/log.response/value",
			allowAdditional: true,
		},
		{
			name: "missing value source",
			logs: []LogSpec{{
				ID: "response",
				Value: LogValueSpec{
					Decode: DecodeJSON,
				},
			}},
			code: "invalid_log_value_source",
			path: "stage.main/scenario.login/act.submit/log.response/value",
		},
		{
			name: "multiple value sources",
			logs: []LogSpec{{
				ID: "response",
				Value: LogValueSpec{
					Field: "body",
					Ref:   "request_id",
				},
			}},
			code: "invalid_log_value_source",
			path: "stage.main/scenario.login/act.submit/log.response/value",
		},
		{
			name: "selector on object value",
			logs: []LogSpec{{
				ID: "response",
				Value: LogValueSpec{
					Object: map[string]LogValueSpec{
						"body": {Field: "body"},
					},
					Path: "/body",
				},
			}},
			code: "invalid_log_value_selector",
			path: "stage.main/scenario.login/act.submit/log.response/value",
		},
		{
			name: "invalid value decode",
			logs: []LogSpec{{
				ID: "response",
				Value: LogValueSpec{
					Field:  "body",
					Decode: DecodeKind("xml"),
				},
			}},
			code: "invalid_log_value_decode",
			path: "stage.main/scenario.login/act.submit/log.response/value",
		},
		{
			name: "invalid value path",
			logs: []LogSpec{{
				ID: "response",
				Value: LogValueSpec{
					Field: "body",
					Path:  "body",
				},
			}},
			code: "invalid_log_value_path",
			path: "stage.main/scenario.login/act.submit/log.response/value",
		},
		{
			name: "invalid value through",
			logs: []LogSpec{{
				ID: "response",
				Value: LogValueSpec{
					Field:   "body",
					Through: []ThroughStepSpec{{}},
				},
			}},
			code: "invalid_log_value_through_step",
			path: "stage.main/scenario.login/act.submit/log.response/value/through[0]",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			spec := StageSpec{
				ID: "main",
				Scenarios: []ScenarioSpec{{
					ID:     "login",
					Inputs: map[string]ValueContract{"request_id": {Kind: ValueKindString}},
					Acts: []ActSpec{{
						ID:     "submit",
						Action: ActionSpec{Use: "action.submit"},
						Logs:   tc.logs,
					}},
				}},
				ScenarioCalls: []ScenarioCallSpec{{ID: "login-user", ScenarioID: "login"}},
			}

			stage := compileStageSpec(spec)
			diagnostics := validateStage(stage)
			if !tc.allowAdditional && len(diagnostics) != 1 {
				t.Fatalf("diagnostic count mismatch: got %d want %d (%v)", len(diagnostics), 1, diagnostics)
			}
			diagnostic, ok := findDiagnostic(diagnostics, tc.code, tc.path)
			if !ok {
				t.Fatalf("diagnostic not found: code %q path %q in %v", tc.code, tc.path, diagnostics)
			}
			if tc.summaryContains != "" && !strings.Contains(diagnostic.Summary, tc.summaryContains) {
				t.Fatalf("diagnostic summary mismatch: got %q want fragment %q", diagnostic.Summary, tc.summaryContains)
			}
		})
	}
}

func TestCompileStageSpecDefaultsActLogVisibility(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{{
			ID: "login",
			Acts: []ActSpec{{
				ID:     "submit",
				Action: ActionSpec{Use: "action.submit"},
				Logs: []LogSpec{{
					ID:    "response",
					Value: LogValueSpec{Field: "body"},
				}},
			}},
		}},
		ScenarioCalls: []ScenarioCallSpec{{ID: "login-user", ScenarioID: "login"}},
	}

	stage := compileStageSpec(spec)
	log := stage.Scenarios[0].Acts[0].Logs[0]
	if got, want := log.Capture, CaptureOmit; got != want {
		t.Fatalf("log capture mismatch: got %q want %q", got, want)
	}
	if got, want := log.Sensitivity, SensitivityInternal; got != want {
		t.Fatalf("log sensitivity mismatch: got %q want %q", got, want)
	}
}

func TestValidateStageSpecReportsMissingActionAndExpectationRefs(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{
			{
				ID: "login",
				Acts: []ActSpec{
					{
						ID:     "submit",
						Action: ActionSpec{},
						Expectations: []ExpectationSpec{
							{ID: "token"},
						},
					},
				},
			},
		},
		ScenarioCalls: []ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	stage := compileStageSpec(spec)
	diagnostics := validateStage(stage)

	if got, want := len(diagnostics), 3; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := diagnostics[0].Code, "missing_action_use"; got != want {
		t.Fatalf("first diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[0].Path, "stage.main/scenario.login/act.submit/action"; got != want {
		t.Fatalf("first diagnostic path mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[1].Code, "missing_expectation_assert_ref"; got != want {
		t.Fatalf("second diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[1].Path, "stage.main/scenario.login/act.submit/expectation.token"; got != want {
		t.Fatalf("second diagnostic path mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[2].Code, "missing_expectation_subject_field"; got != want {
		t.Fatalf("third diagnostic code mismatch: got %q want %q", got, want)
	}
}

func TestValidateStageSpecReportsMissingRequiredIDs(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		Scenarios: []ScenarioSpec{
			{
				Acts: []ActSpec{
					{
						Action: ActionSpec{Use: "action.submit"},
						Expectations: []ExpectationSpec{
							{Assert: AssertSpec{Ref: "expectation.token"}},
						},
					},
				},
			},
		},
		ScenarioCalls: []ScenarioCallSpec{
			{},
		},
	}

	stage := compileStageSpec(spec)
	diagnostics := validateStage(stage)

	if got, want := len(diagnostics), 7; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := diagnostics[0].Code, "missing_stage_id"; got != want {
		t.Fatalf("first diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[1].Code, "missing_scenario_call_id"; got != want {
		t.Fatalf("second diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[2].Code, "missing_scenario_call_scenario_id"; got != want {
		t.Fatalf("third diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[3].Code, "missing_scenario_id"; got != want {
		t.Fatalf("fourth diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[4].Code, "missing_act_id"; got != want {
		t.Fatalf("fifth diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[5].Code, "missing_expectation_id"; got != want {
		t.Fatalf("sixth diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[6].Code, "missing_expectation_subject_field"; got != want {
		t.Fatalf("seventh diagnostic code mismatch: got %q want %q", got, want)
	}
}

func TestValidateStageSpecReportsInvalidExpectationSubjectAndAssert(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{
			{
				ID: "probe",
				Acts: []ActSpec{
					{
						ID:     "request",
						Action: ActionSpec{Use: "action.http"},
						Expectations: []ExpectationSpec{
							{
								ID:      "check",
								Subject: SubjectSpec{Field: "body", Decode: "xml", Path: JSONPointer("body")},
								Assert:  AssertSpec{Ref: "expectation.between", Args: map[string]BindingSpec{"min": {Kind: BindingKindLiteral, Value: 1}}},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []ScenarioCallSpec{
			{ID: "probe-server", ScenarioID: "probe"},
		},
	}

	diagnostics := validateStage(compileStageSpec(spec))
	if got, want := len(diagnostics), 2; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := diagnostics[0].Code, "invalid_expectation_subject_decode"; got != want {
		t.Fatalf("first diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[1].Code, "invalid_expectation_subject_path"; got != want {
		t.Fatalf("second diagnostic code mismatch: got %q want %q", got, want)
	}

}

func TestValidateStageSpecReportsInvalidBindings(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{
			{
				ID: "login",
				Inputs: map[string]ValueContract{
					"profile": {Kind: ValueKindObject},
					"session": {Kind: ValueKindString},
					"token":   {Kind: ValueKindString},
				},
				Acts: []ActSpec{
					{
						ID:     "submit",
						Action: ActionSpec{Use: "action.submit"},
					},
				},
			},
		},
		ScenarioCalls: []ScenarioCallSpec{
			{
				ID:         "login-user",
				ScenarioID: "login",
				Bindings: map[string]BindingSpec{
					"profile": {
						Kind: BindingKindObject,
						Object: map[string]BindingSpec{
							"name": {
								Kind: BindingKind("bogus"),
							},
						},
					},
					"session": {
						Kind: BindingKindRef,
					},
					"token": {
						Kind: BindingKind("bogus"),
					},
				},
			},
		},
	}

	stage := compileStageSpec(spec)
	diagnostics := validateStage(stage)

	if got, want := len(diagnostics), 3; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := diagnostics[0].Code, "invalid_binding_kind"; got != want {
		t.Fatalf("first diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[0].Path, "stage.main/call.login-user/binding.profile.name"; got != want {
		t.Fatalf("first diagnostic path mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[1].Code, "missing_binding_ref"; got != want {
		t.Fatalf("second diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[1].Path, "stage.main/call.login-user/binding.session"; got != want {
		t.Fatalf("second diagnostic path mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[2].Code, "invalid_binding_kind"; got != want {
		t.Fatalf("third diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[2].Path, "stage.main/call.login-user/binding.token"; got != want {
		t.Fatalf("third diagnostic path mismatch: got %q want %q", got, want)
	}
}

func TestValidateStageSpecReportsInvalidExports(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{
			{
				ID: "login",
				Acts: []ActSpec{
					{
						ID:     "submit",
						Action: ActionSpec{Use: "action.submit"},
						Exports: []ExportSpec{
							{Field: "token", As: "issued"},
							{Field: "other", As: "issued"},
							{As: "missing-source"},
						},
					},
				},
			},
		},
		ScenarioCalls: []ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	stage := compileStageSpec(spec)
	diagnostics := validateStage(stage)

	if got, want := len(diagnostics), 2; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := diagnostics[0].Code, "duplicate_export_name"; got != want {
		t.Fatalf("first diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[0].Path, "stage.main/scenario.login/act.submit/export.issued"; got != want {
		t.Fatalf("first diagnostic path mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[1].Code, "missing_act_export_field"; got != want {
		t.Fatalf("second diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[1].Path, "stage.main/scenario.login/act.submit/export.missing-source"; got != want {
		t.Fatalf("second diagnostic path mismatch: got %q want %q", got, want)
	}
}

func TestValidateStageSpecReportsDuplicateStageExportNames(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{
			{
				ID: "register",
				Acts: []ActSpec{
					{
						ID:     "submit",
						Action: ActionSpec{Use: "action.register"},
					},
				},
			},
			{
				ID: "login",
				Inputs: map[string]ValueContract{
					"token": {Kind: ValueKindString},
				},
				Acts: []ActSpec{
					{
						ID:     "verify",
						Action: ActionSpec{Use: "action.login"},
					},
				},
			},
		},
		ScenarioCalls: []ScenarioCallSpec{
			{
				ID:         "register-user",
				ScenarioID: "register",
				Exports: []ExportSpec{
					{Ref: &RefSpec{Name: "token"}, As: "issued_token"},
				},
			},
			{
				ID:         "login-user",
				ScenarioID: "login",
				Exports: []ExportSpec{
					{Ref: &RefSpec{Name: "token"}, As: "issued_token"},
				},
			},
		},
	}

	stage := compileStageSpec(spec)
	diagnostics := validateStage(stage)

	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := diagnostics[0].Code, "duplicate_stage_export_name"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[0].Path, "stage.main/call.login-user/export.issued_token"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}
}

func TestValidateStageSpecReportsUnresolvedScenarioCallBindingRef(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{
			{
				ID: "register",
				Acts: []ActSpec{
					{
						ID:     "submit",
						Action: ActionSpec{Use: "action.register"},
					},
				},
			},
			{
				ID: "login",
				Inputs: map[string]ValueContract{
					"token": {Kind: ValueKindString, Required: true},
				},
				Acts: []ActSpec{
					{
						ID:     "verify",
						Action: ActionSpec{Use: "action.login"},
					},
				},
			},
		},
		ScenarioCalls: []ScenarioCallSpec{
			{
				ID:         "register-user",
				ScenarioID: "register",
				Exports: []ExportSpec{
					{Ref: &RefSpec{Name: "token"}, As: "issued_token"},
				},
			},
			{
				ID:         "login-user",
				ScenarioID: "login",
				Dependencies: []ScenarioDependencySpec{
					{CallID: "register-user"},
				},
				Bindings: map[string]BindingSpec{
					"token": {
						Kind: BindingKindRef,
						Ref:  &RefSpec{Name: "missing_token"},
					},
				},
			},
		},
	}

	stage := compileStageSpec(spec)
	diagnostics := validateStage(stage)

	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := diagnostics[0].Code, "unresolved_binding_ref"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[0].Path, "stage.main/call.login-user/binding.token"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}
}

func TestValidateStageSpecReportsInvalidProperties(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{
			{
				ID: "login",
				Acts: []ActSpec{
					{
						ID:     "submit",
						Action: ActionSpec{Use: "action.submit"},
						Properties: map[string]PropertySpec{
							"": {
								Inventory: &InventoryCall{
									With: map[string]BindingSpec{
										"value": {Kind: BindingKindRef},
									},
								},
							},
							"seed": {
								Inventory: &InventoryCall{Use: "inventory.seed"},
								Decorators: []DecoratorSpec{
									{With: map[string]any{"comma": ";"}},
								},
							},
							"missing": {},
						},
					},
				},
			},
		},
		ScenarioCalls: []ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	stage := compileStageSpec(spec)
	diagnostics := validateStage(stage)

	if got, want := len(diagnostics), 5; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := diagnostics[0].Code, "missing_property_key"; got != want {
		t.Fatalf("first diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[1].Code, "missing_property_inventory_use"; got != want {
		t.Fatalf("second diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[2].Code, "missing_binding_ref"; got != want {
		t.Fatalf("third diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[3].Code, "missing_property_inventory"; got != want {
		t.Fatalf("fourth diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[4].Code, "missing_property_decorator_use"; got != want {
		t.Fatalf("fifth diagnostic code mismatch: got %q want %q", got, want)
	}
}

func TestValidateStageSpecReportsInvalidDependencyPredicate(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{
			{
				ID: "prepare",
				Acts: []ActSpec{
					{ID: "bootstrap", Action: ActionSpec{Use: "action.prepare"}},
				},
			},
			{
				ID: "login",
				Acts: []ActSpec{
					{ID: "submit", Action: ActionSpec{Use: "action.login"}},
				},
			},
		},
		ScenarioCalls: []ScenarioCallSpec{
			{ID: "prepare-user", ScenarioID: "prepare"},
			{
				ID:         "login-user",
				ScenarioID: "login",
				Dependencies: []ScenarioDependencySpec{
					{CallID: "prepare-user", When: TriggerPredicate("bogus")},
				},
			},
		},
	}

	stage := compileStageSpec(spec)
	diagnostics := validateStage(stage)

	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := diagnostics[0].Code, "invalid_dependency_predicate"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[0].Path, "stage.main/call.login-user/dependency[0]"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}
}

func TestValidateStageSpecReportsInvalidTransitionOutcomeAndMissingPropertyID(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{
			{
				ID: "login",
				Acts: []ActSpec{
					{
						ID:     "submit",
						Action: ActionSpec{Use: "action.submit"},
						Properties: map[string]PropertySpec{
							"": {Inventory: &InventoryCall{Use: "inventory.seed"}},
						},
						Transitions: []TransitionSpec{
							{On: TransitionOutcome("bogus"), To: "submit"},
						},
					},
				},
			},
		},
		ScenarioCalls: []ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	stage := compileStageSpec(spec)
	diagnostics := validateStage(stage)

	if got, want := len(diagnostics), 3; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := diagnostics[0].Code, "act_transition_cycle"; got != want {
		t.Fatalf("first diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[1].Code, "invalid_transition_outcome"; got != want {
		t.Fatalf("second diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[2].Code, "missing_property_key"; got != want {
		t.Fatalf("third diagnostic code mismatch: got %q want %q", got, want)
	}
}

func TestValidateStageSpecReportsPropertyDependencyCycle(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{
			{
				ID: "login",
				Acts: []ActSpec{
					{
						ID:     "submit",
						Action: ActionSpec{Use: "action.submit"},
						Properties: map[string]PropertySpec{
							"first": {
								Inventory: &InventoryCall{
									Use: "inventory.seed",
									With: map[string]BindingSpec{
										"value": {Kind: BindingKindRef, Ref: &RefSpec{Name: "second"}},
									},
								},
							},
							"second": {
								Inventory: &InventoryCall{
									Use: "inventory.seed",
									With: map[string]BindingSpec{
										"value": {Kind: BindingKindRef, Ref: &RefSpec{Name: "first"}},
									},
								},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	stage := compileStageSpec(spec)
	diagnostics := validateStage(stage)

	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := diagnostics[0].Code, "property_dependency_cycle"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
}

func TestValidateStageSpecReportsInvalidEventuallyTimeout(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{
			{
				ID: "login",
				Acts: []ActSpec{
					{
						ID:         "submit",
						Eventually: &EventuallySpec{Timeout: "0s", Interval: "1s"},
						Action:     ActionSpec{Use: "action.submit", Repeatable: true},
						Expectations: []ExpectationSpec{
							{
								ID:      "done",
								Subject: SubjectSpec{Field: "token"},
								Assert:  AssertSpec{Ref: "expectation.equal"},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	stage := compileStageSpec(spec)
	diagnostics := validateStage(stage)

	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := diagnostics[0].Code, "invalid_eventually_timeout"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[0].Path, "stage.main/scenario.login/act.submit/eventually/timeout"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}
}

func TestValidateStageSpecReportsEventuallyIntervalNotShorterThanTimeout(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{
			{
				ID: "login",
				Acts: []ActSpec{
					{
						ID:         "submit",
						Eventually: &EventuallySpec{Timeout: "1s", Interval: "2s"},
						Action:     ActionSpec{Use: "action.submit", Repeatable: true},
						Expectations: []ExpectationSpec{
							{
								ID:      "done",
								Subject: SubjectSpec{Field: "token"},
								Assert:  AssertSpec{Ref: "expectation.equal"},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	stage := compileStageSpec(spec)
	diagnostics := validateStage(stage)

	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := diagnostics[0].Code, "invalid_eventually_interval"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[0].Path, "stage.main/scenario.login/act.submit/eventually/interval"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}
}

func TestValidateStageSpecReportsEventuallyRequiresRepeatableAction(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{
			{
				ID: "login",
				Acts: []ActSpec{
					{
						ID:         "submit",
						Eventually: &EventuallySpec{Timeout: "30s", Interval: "1s"},
						Action:     ActionSpec{Use: "action.submit"},
						Expectations: []ExpectationSpec{
							{
								ID:      "done",
								Subject: SubjectSpec{Field: "token"},
								Assert:  AssertSpec{Ref: "expectation.equal"},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	stage := compileStageSpec(spec)
	diagnostics := validateStage(stage)

	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := diagnostics[0].Code, "eventually_requires_repeatable_action"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
}

func TestValidateStageSpecReportsRepeatableWithoutEventually(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{
			{
				ID: "login",
				Acts: []ActSpec{
					{
						ID:     "submit",
						Action: ActionSpec{Use: "action.submit", Repeatable: true},
					},
				},
			},
		},
		ScenarioCalls: []ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	stage := compileStageSpec(spec)
	diagnostics := validateStage(stage)

	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := diagnostics[0].Code, "repeatable_without_eventually"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
}

func findDiagnostic(diagnostics []Diagnostic, code, path string) (Diagnostic, bool) {
	for i := range diagnostics {
		if diagnostics[i].Code == code && diagnostics[i].Path == path {
			return diagnostics[i], true
		}
	}

	return Diagnostic{}, false
}
