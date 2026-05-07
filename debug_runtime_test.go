package theater

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestRunCallsDebugBoundaryHookAroundScenarioCallActActionAndExpectation(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{
			{
				ID: "login",
				Acts: []ActSpec{
					{
						ID:     "submit",
						Action: ActionSpec{Use: "action.login"},
						Expectations: []ExpectationSpec{
							{
								ID:      "token",
								Subject: SubjectSpec{Field: "token"},
								Assert:  AssertSpec{Ref: "expectation.token"},
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

	sequence := make([]string, 0, 6)
	var actionResources ResourceScope
	action := debugBoundaryTestAction{
		RunFunc: func(request ActionRequest) (Outputs, error) {
			sequence = append(sequence, "action.run")
			actionResources = request.Resources
			return Outputs{"token": "issued-token"}, nil
		},
	}

	expectation := debugBoundaryTestExpectation{
		CheckFunc: func(actual any) error {
			sequence = append(sequence, "expectation.check")
			if got, want := actual, "issued-token"; got != want {
				t.Fatalf("expectation actual mismatch: got %v want %v", got, want)
			}

			return nil
		},
	}

	catalog := NewCatalog()
	if err := catalog.RegisterAction("action.login", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	matchers, err := NewMatcherCatalog(expectation.Descriptor("expectation.token"))
	if err != nil {
		t.Fatalf("register matcher catalog failed: %v", err)
	}

	hits := make([]debugBoundaryState, 0, 8)
	result, err := NewRunner(catalog, matchers).runWithDebugRuntime(
		context.Background(),
		spec,
		RunOptions{},
		&debugRuntime{
			breakpointSpecs: []string{
				"path=**",
				"path=**,kind=scenario_call",
				"path=**,kind=act",
			},
			boundaryHook: func(_ context.Context, state debugBoundaryState) error {
				sequence = append(sequence, string(state.Ref.Kind)+"."+string(state.Ref.Phase))
				hits = append(hits, state)
				return nil
			},
		},
	)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if got, want := result.Report.Status, StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}

	if actionResources == nil {
		t.Fatal("action resources were not captured")
	}

	wantSequence := []string{
		"scenario_call.before",
		"act.before",
		"action.before",
		"action.run",
		"action.after",
		"expectation.before",
		"expectation.check",
		"expectation.after",
		"act.after",
		"scenario_call.after",
	}
	if !reflect.DeepEqual(sequence, wantSequence) {
		t.Fatalf("boundary sequence mismatch:\n got: %#v\nwant: %#v", sequence, wantSequence)
	}

	if got, want := len(hits), 8; got != want {
		t.Fatalf("boundary hit count mismatch: got %d want %d", got, want)
	}

	wantPaths := []string{
		"stage.main/call.login-user",
		"stage.main/call.login-user/act.submit",
		"stage.main/call.login-user/act.submit/action",
		"stage.main/call.login-user/act.submit/action",
		"stage.main/call.login-user/act.submit/expectation.token",
		"stage.main/call.login-user/act.submit/expectation.token",
		"stage.main/call.login-user/act.submit",
		"stage.main/call.login-user",
	}
	wantKinds := []debugBoundaryKind{
		debugBoundaryKindScenarioCall,
		debugBoundaryKindAct,
		debugBoundaryKindAction,
		debugBoundaryKindAction,
		debugBoundaryKindExpectation,
		debugBoundaryKindExpectation,
		debugBoundaryKindAct,
		debugBoundaryKindScenarioCall,
	}
	wantPhases := []debugBoundaryPhase{
		debugBoundaryPhaseBefore,
		debugBoundaryPhaseBefore,
		debugBoundaryPhaseBefore,
		debugBoundaryPhaseAfter,
		debugBoundaryPhaseBefore,
		debugBoundaryPhaseAfter,
		debugBoundaryPhaseAfter,
		debugBoundaryPhaseAfter,
	}
	wantStatuses := []Status{
		StatusRunning,
		StatusRunning,
		StatusRunning,
		StatusPassed,
		StatusRunning,
		StatusPassed,
		StatusPassed,
		StatusPassed,
	}

	for i := range hits {
		if got, want := hits[i].Ref.Path, wantPaths[i]; got != want {
			t.Fatalf("hit[%d] path mismatch: got %q want %q", i, got, want)
		}
		if got, want := hits[i].Ref.Kind, wantKinds[i]; got != want {
			t.Fatalf("hit[%d] kind mismatch: got %q want %q", i, got, want)
		}
		if got, want := hits[i].Ref.Phase, wantPhases[i]; got != want {
			t.Fatalf("hit[%d] phase mismatch: got %q want %q", i, got, want)
		}
		if got, want := hits[i].Ref.Attempt, 1; got != want {
			t.Fatalf("hit[%d] attempt mismatch: got %d want %d", i, got, want)
		}
		if got, want := hits[i].Ref.ScenarioPath, "stage.main/call.login-user"; got != want {
			t.Fatalf("hit[%d] scenario path mismatch: got %q want %q", i, got, want)
		}
		if got, want := hits[i].Ref.ScenarioCallID, "login-user"; got != want {
			t.Fatalf("hit[%d] scenario call id mismatch: got %q want %q", i, got, want)
		}
		if got, want := hits[i].Status, wantStatuses[i]; got != want {
			t.Fatalf("hit[%d] status mismatch: got %q want %q", i, got, want)
		}
		if hits[i].Failure != nil {
			t.Fatalf("hit[%d] failure = %v, want nil", i, hits[i].Failure)
		}
		if hits[i].Resources != actionResources {
			t.Fatalf("hit[%d] resources mismatch", i)
		}
	}

	if got, want := len(hits[1].Output.Fields), 0; got != want {
		t.Fatalf("act before output field count mismatch: got %d want %d", got, want)
	}
	if got, want := len(hits[3].Output.Fields), 1; got != want {
		t.Fatalf("action after output field count mismatch: got %d want %d", got, want)
	}
	if got, want := hits[3].Output.Fields[0].Key, "token"; got != want {
		t.Fatalf("action after output key mismatch: got %q want %q", got, want)
	}
	if got, want := hits[3].Output.Fields[0].Value.Text, "issued-token"; got != want {
		t.Fatalf("action after output text mismatch: got %q want %q", got, want)
	}
	if got, want := len(hits[4].Inputs.Fields), 1; got != want {
		t.Fatalf("expectation before input field count mismatch: got %d want %d", got, want)
	}
	if got, want := hits[4].Inputs.Fields[0].Key, "actual"; got != want {
		t.Fatalf("expectation before input key mismatch: got %q want %q", got, want)
	}
	if got, want := hits[4].Inputs.Fields[0].Origin, "expectation.actual"; got != want {
		t.Fatalf("expectation before input origin mismatch: got %q want %q", got, want)
	}
	if got, want := hits[4].Inputs.Fields[0].Value.Text, "issued-token"; got != want {
		t.Fatalf("expectation before input text mismatch: got %q want %q", got, want)
	}
	if got, want := len(hits[6].Output.Fields), 0; got != want {
		t.Fatalf("act after output field count mismatch: got %d want %d", got, want)
	}
	if got, want := len(hits[7].Output.Fields), 0; got != want {
		t.Fatalf("scenario after output field count mismatch: got %d want %d", got, want)
	}
}

func TestRunDebugBoundaryHookFiltersHitsThroughCompiledBreakpoints(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{{
			ID: "login",
			Acts: []ActSpec{{
				ID:     "submit",
				Action: ActionSpec{Use: "action.login"},
				Expectations: []ExpectationSpec{{
					ID:      "token",
					Subject: SubjectSpec{Field: "token"},
					Assert:  AssertSpec{Ref: "expectation.token"},
				}},
			}},
		}},
		ScenarioCalls: []ScenarioCallSpec{{ID: "login-user", ScenarioID: "login"}},
	}

	catalog := NewCatalog()
	if err := catalog.RegisterAction("action.login", debugBoundaryTestAction{
		RunFunc: func(request ActionRequest) (Outputs, error) {
			return Outputs{"token": "issued-token"}, nil
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	matchers, err := NewMatcherCatalog(debugBoundaryTestExpectation{
		CheckFunc: func(actual any) error { return nil },
	}.Descriptor("expectation.token"))
	if err != nil {
		t.Fatalf("new matcher catalog failed: %v", err)
	}

	hits := make([]debugBoundaryState, 0, 1)
	result, err := NewRunner(catalog, matchers).runWithDebugRuntime(
		context.Background(),
		spec,
		RunOptions{},
		&debugRuntime{
			breakpointSpecs: []string{
				"path=**/expectation.token,kind=expectation,phase=after",
			},
			boundaryHook: func(_ context.Context, state debugBoundaryState) error {
				hits = append(hits, state)
				return nil
			},
		},
	)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if got, want := result.Report.Status, StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
	if got, want := len(hits), 1; got != want {
		t.Fatalf("boundary hit count mismatch: got %d want %d", got, want)
	}
	if got, want := hits[0].Ref.Path, "stage.main/call.login-user/act.submit/expectation.token"; got != want {
		t.Fatalf("boundary path mismatch: got %q want %q", got, want)
	}
	if got, want := hits[0].Ref.Kind, debugBoundaryKindExpectation; got != want {
		t.Fatalf("boundary kind mismatch: got %q want %q", got, want)
	}
	if got, want := hits[0].Ref.Phase, debugBoundaryPhaseAfter; got != want {
		t.Fatalf("boundary phase mismatch: got %q want %q", got, want)
	}
	if got, want := hits[0].Ref.ActID, "submit"; got != want {
		t.Fatalf("boundary act id mismatch: got %q want %q", got, want)
	}
	if got, want := hits[0].Ref.NodeRef, "token"; got != want {
		t.Fatalf("boundary node ref mismatch: got %q want %q", got, want)
	}
}

func TestRunDebugBoundaryHookClearsCompiledBreakpointsWhenSpecsAreRemoved(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{{
			ID: "login",
			Acts: []ActSpec{{
				ID:     "submit",
				Action: ActionSpec{Use: "action.login"},
				Expectations: []ExpectationSpec{{
					ID:      "token",
					Subject: SubjectSpec{Field: "token"},
					Assert:  AssertSpec{Ref: "expectation.token"},
				}},
			}},
		}},
		ScenarioCalls: []ScenarioCallSpec{{ID: "login-user", ScenarioID: "login"}},
	}

	catalog := NewCatalog()
	if err := catalog.RegisterAction("action.login", debugBoundaryTestAction{
		RunFunc: func(request ActionRequest) (Outputs, error) {
			return Outputs{"token": "issued-token"}, nil
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	matchers, err := NewMatcherCatalog(debugBoundaryTestExpectation{
		CheckFunc: func(actual any) error { return nil },
	}.Descriptor("expectation.token"))
	if err != nil {
		t.Fatalf("new matcher catalog failed: %v", err)
	}

	debug := &debugRuntime{
		breakpointSpecs: []string{
			"path=**/expectation.token,kind=expectation,phase=after",
		},
	}

	firstHits := 0
	debug.boundaryHook = func(_ context.Context, state debugBoundaryState) error {
		firstHits++
		return nil
	}

	first, err := NewRunner(catalog, matchers).runWithDebugRuntime(context.Background(), spec, RunOptions{}, debug)
	if err != nil {
		t.Fatalf("first run failed: %v", err)
	}
	if got, want := first.Report.Status, StatusPassed; got != want {
		t.Fatalf("first run status mismatch: got %q want %q", got, want)
	}
	if got, want := firstHits, 1; got != want {
		t.Fatalf("first run hit count mismatch: got %d want %d", got, want)
	}

	debug.breakpointSpecs = nil

	secondHits := 0
	debug.boundaryHook = func(_ context.Context, state debugBoundaryState) error {
		secondHits++
		return nil
	}

	second, err := NewRunner(catalog, matchers).runWithDebugRuntime(context.Background(), spec, RunOptions{}, debug)
	if err != nil {
		t.Fatalf("second run failed: %v", err)
	}
	if got, want := second.Report.Status, StatusPassed; got != want {
		t.Fatalf("second run status mismatch: got %q want %q", got, want)
	}
	if got, want := secondHits, 4; got != want {
		t.Fatalf("second run hit count mismatch: got %d want %d", got, want)
	}
}

func TestRunDebugBoundaryHookSupportsScenarioCallAndActBreakpoints(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{{
			ID: "login",
			Acts: []ActSpec{{
				ID:     "submit",
				Action: ActionSpec{Use: "action.login"},
			}},
		}},
		ScenarioCalls: []ScenarioCallSpec{{ID: "login-user", ScenarioID: "login"}},
	}

	catalog := NewCatalog()
	if err := catalog.RegisterAction("action.login", debugBoundaryTestAction{
		RunFunc: func(request ActionRequest) (Outputs, error) {
			return Outputs{"token": "issued-token"}, nil
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}
	matchers, err := NewMatcherCatalog()
	if err != nil {
		t.Fatalf("new matcher catalog failed: %v", err)
	}

	hits := make([]debugBoundaryState, 0, 2)
	result, err := NewRunner(catalog, matchers).runWithDebugRuntime(
		context.Background(),
		spec,
		RunOptions{},
		&debugRuntime{
			breakpointSpecs: []string{
				"path=stage.main/call.login-user,kind=scenario_call,phase=before",
				"path=stage.main/call.login-user/act.submit,kind=act,phase=after",
			},
			boundaryHook: func(_ context.Context, state debugBoundaryState) error {
				hits = append(hits, state)
				return nil
			},
		},
	)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if got, want := result.Report.Status, StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
	if got, want := len(hits), 2; got != want {
		t.Fatalf("boundary hit count mismatch: got %d want %d", got, want)
	}

	if got, want := hits[0].Ref.Kind, debugBoundaryKindScenarioCall; got != want {
		t.Fatalf("scenario boundary kind mismatch: got %q want %q", got, want)
	}
	if got, want := hits[0].Ref.Phase, debugBoundaryPhaseBefore; got != want {
		t.Fatalf("scenario boundary phase mismatch: got %q want %q", got, want)
	}
	if got, want := hits[0].Ref.Path, "stage.main/call.login-user"; got != want {
		t.Fatalf("scenario boundary path mismatch: got %q want %q", got, want)
	}
	if got, want := hits[1].Ref.Kind, debugBoundaryKindAct; got != want {
		t.Fatalf("act boundary kind mismatch: got %q want %q", got, want)
	}
	if got, want := hits[1].Ref.Phase, debugBoundaryPhaseAfter; got != want {
		t.Fatalf("act boundary phase mismatch: got %q want %q", got, want)
	}
	if got, want := hits[1].Ref.Path, "stage.main/call.login-user/act.submit"; got != want {
		t.Fatalf("act boundary path mismatch: got %q want %q", got, want)
	}
}

func TestRunDebugBoundaryHookSupportsRetryTargetedActAfterBreakpoint(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{{
			ID: "probe",
			Acts: []ActSpec{{
				ID:         "wait-ready",
				Action:     ActionSpec{Use: "action.ready", Repeatable: true},
				Eventually: &EventuallySpec{Timeout: "100ms", Interval: "1ms"},
				Expectations: []ExpectationSpec{{
					ID:      "ready",
					Subject: SubjectSpec{Field: "ready"},
					Assert:  AssertSpec{Ref: "expectation.ready"},
				}},
			}},
		}},
		ScenarioCalls: []ScenarioCallSpec{{ID: "probe-ready", ScenarioID: "probe"}},
	}

	attempts := 0
	catalog := NewCatalog()
	if err := catalog.RegisterAction("action.ready", debugControllerReadyAction{
		RunFunc: func(request ActionRequest) (Outputs, error) {
			attempts++
			return Outputs{"ready": attempts >= 3}, nil
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	matchers, err := NewMatcherCatalog(debugBoundaryTestExpectation{
		CheckFunc: func(actual any) error {
			ready, _ := actual.(bool)
			if ready {
				return nil
			}

			return errors.New("not ready")
		},
	}.Descriptor("expectation.ready"))
	if err != nil {
		t.Fatalf("new matcher catalog failed: %v", err)
	}

	hits := make([]debugBoundaryState, 0, 1)
	result, err := NewRunner(catalog, matchers).runWithDebugRuntime(
		context.Background(),
		spec,
		RunOptions{},
		&debugRuntime{
			breakpointSpecs: []string{
				"path=stage.main/call.probe-ready/act.wait-ready,kind=act,phase=after,attempt=retry-only",
			},
			boundaryHook: func(_ context.Context, state debugBoundaryState) error {
				hits = append(hits, state)
				return nil
			},
		},
	)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if got, want := result.Report.Status, StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
	if got, want := attempts, 3; got != want {
		t.Fatalf("retry attempt count mismatch: got %d want %d", got, want)
	}
	if got, want := len(hits), 1; got != want {
		t.Fatalf("boundary hit count mismatch: got %d want %d", got, want)
	}
	if got, want := hits[0].Ref.Kind, debugBoundaryKindAct; got != want {
		t.Fatalf("boundary kind mismatch: got %q want %q", got, want)
	}
	if got, want := hits[0].Ref.Phase, debugBoundaryPhaseAfter; got != want {
		t.Fatalf("boundary phase mismatch: got %q want %q", got, want)
	}
	if got, want := hits[0].Ref.Attempt, 3; got != want {
		t.Fatalf("boundary attempt mismatch: got %d want %d", got, want)
	}
	if got, want := hits[0].Status, StatusPassed; got != want {
		t.Fatalf("boundary status mismatch: got %q want %q", got, want)
	}
}

func TestDebugRuntimeTerminalFailureBreakpointMatchesRetryAwareBoundaryOnTerminalDispatch(t *testing.T) {
	t.Parallel()

	hits := 0
	debug := &debugRuntime{
		compiledBreakpoints: []debugCompiledBreakpoint{{
			Boundary: debugCompiledBoundary{
				Path:         "stage.main/call.login-user/act.submit/expectation.token",
				ScenarioPath: "stage.main/call.login-user",
				ActID:        "submit",
				NodeRef:      "token",
				Kind:         debugBoundaryKindExpectation,
				Phase:        debugBoundaryPhaseAfter,
				RetryAware:   true,
			},
			When:     debugBreakpointWhenTerminalFailure,
			Attempt:  debugBreakpointAttempt{Mode: debugBreakpointAttemptModeAny},
			Reaction: debugBreakpointActionPause,
		}},
		boundaryHook: func(_ context.Context, state debugBoundaryState) error {
			hits++
			return nil
		},
	}

	state := debugBoundaryState{
		Ref: debugBoundaryRef{
			ScenarioPath: "stage.main/call.login-user",
			ActID:        "submit",
			NodeRef:      "token",
			Path:         "stage.main/call.login-user/act.submit/expectation.token",
			Kind:         debugBoundaryKindExpectation,
			Phase:        debugBoundaryPhaseAfter,
			Attempt:      3,
		},
		Status: StatusFailed,
		Failure: &Failure{
			Kind:    FailureKindExpectation,
			Phase:   PhaseRun,
			At:      "stage.main/call.login-user/act.submit/expectation.token",
			Summary: "expectation failed",
		},
	}

	if err := debug.atBoundary(context.Background(), state); err != nil {
		t.Fatalf("boundary dispatch failed: %v", err)
	}
	if got, want := hits, 0; got != want {
		t.Fatalf("regular boundary hit count mismatch: got %d want %d", got, want)
	}

	if err := debug.atTerminalFailure(context.Background(), state); err != nil {
		t.Fatalf("terminal failure dispatch failed: %v", err)
	}
	if got, want := hits, 1; got != want {
		t.Fatalf("terminal failure hit count mismatch: got %d want %d", got, want)
	}
}

func TestRunReportsFailedExpectationInAfterBoundary(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{{
			ID: "login",
			Acts: []ActSpec{{
				ID:     "submit",
				Action: ActionSpec{Use: "action.login"},
				Expectations: []ExpectationSpec{{
					ID:      "token",
					Subject: SubjectSpec{Field: "token"},
					Assert:  AssertSpec{Ref: "expectation.token"},
				}},
			}},
		}},
		ScenarioCalls: []ScenarioCallSpec{{ID: "login-user", ScenarioID: "login"}},
	}

	action := debugBoundaryTestAction{
		RunFunc: func(request ActionRequest) (Outputs, error) {
			return Outputs{"token": "issued-token"}, nil
		},
	}
	expectation := debugBoundaryTestExpectation{
		CheckFunc: func(actual any) error {
			return errors.New("token mismatch")
		},
	}

	catalog := NewCatalog()
	if err := catalog.RegisterAction("action.login", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	matchers, err := NewMatcherCatalog(expectation.Descriptor("expectation.token"))
	if err != nil {
		t.Fatalf("register matcher catalog failed: %v", err)
	}

	hits := make([]debugBoundaryState, 0, 8)
	result, err := NewRunner(catalog, matchers).runWithDebugRuntime(
		context.Background(),
		spec,
		RunOptions{},
		&debugRuntime{
			boundaryHook: func(_ context.Context, state debugBoundaryState) error {
				hits = append(hits, state)
				return nil
			},
		},
	)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if got, want := result.Report.Status, StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}

	if got, want := len(hits), 4; got != want {
		t.Fatalf("boundary hit count mismatch: got %d want %d", got, want)
	}

	var after *debugBoundaryState
	for i := range hits {
		if hits[i].Ref.Kind == debugBoundaryKindExpectation && hits[i].Ref.Phase == debugBoundaryPhaseAfter {
			after = &hits[i]
			break
		}
	}
	if after == nil {
		t.Fatal("expectation after-boundary was not captured")
	}
	if got, want := after.Ref.Kind, debugBoundaryKindExpectation; got != want {
		t.Fatalf("after-boundary kind mismatch: got %q want %q", got, want)
	}
	if got, want := after.Ref.Phase, debugBoundaryPhaseAfter; got != want {
		t.Fatalf("after-boundary phase mismatch: got %q want %q", got, want)
	}
	if got, want := after.Status, StatusFailed; got != want {
		t.Fatalf("after-boundary status mismatch: got %q want %q", got, want)
	}
	if after.Failure == nil {
		t.Fatal("after-boundary failure = nil, want expectation failure")
	}
	if got, want := after.Failure.Kind, FailureKindExpectation; got != want {
		t.Fatalf("after-boundary failure kind mismatch: got %q want %q", got, want)
	}
	if got, want := after.Failure.Cause.Error(), "token mismatch"; got != want {
		t.Fatalf("after-boundary failure cause mismatch: got %q want %q", got, want)
	}
}

func TestRunConvertsDebugBoundaryHookErrorIntoFailedReport(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{{
			ID: "login",
			Acts: []ActSpec{{
				ID:     "submit",
				Action: ActionSpec{Use: "action.login"},
			}},
		}},
		ScenarioCalls: []ScenarioCallSpec{{ID: "login-user", ScenarioID: "login"}},
	}

	actionCalled := false
	action := debugBoundaryTestAction{
		RunFunc: func(request ActionRequest) (Outputs, error) {
			actionCalled = true
			return Outputs{"token": "issued-token"}, nil
		},
	}

	catalog := NewCatalog()
	if err := catalog.RegisterAction("action.login", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	matchers, err := NewMatcherCatalog()
	if err != nil {
		t.Fatalf("register matcher catalog failed: %v", err)
	}

	hookErr := errors.New("debug hook failed")
	result, err := NewRunner(catalog, matchers).runWithDebugRuntime(
		context.Background(),
		spec,
		RunOptions{},
		&debugRuntime{
			boundaryHook: func(_ context.Context, state debugBoundaryState) error {
				if state.Ref.Kind == debugBoundaryKindAction && state.Ref.Phase == debugBoundaryPhaseBefore {
					return hookErr
				}

				return nil
			},
		},
	)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if actionCalled {
		t.Fatal("action ran before the failing boundary hook")
	}

	if got, want := result.Report.Status, StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
	if result.Report.Failure == nil {
		t.Fatal("report failure = nil, want contained debug failure")
	}
	if got, want := result.Report.Failure.Kind, FailureKindInternal; got != want {
		t.Fatalf("report failure kind mismatch: got %q want %q", got, want)
	}
	if got, want := result.Report.Failure.Summary, "debug boundary hook failed"; got != want {
		t.Fatalf("report failure summary mismatch: got %q want %q", got, want)
	}
	if got, want := result.Report.Failure.Cause.Error(), "debug hook failed"; got != want {
		t.Fatalf("report failure cause mismatch: got %q want %q", got, want)
	}
}

func TestRunConvertsDebugBoundaryHookPanicIntoFailedReport(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{{
			ID: "login",
			Acts: []ActSpec{{
				ID:     "submit",
				Action: ActionSpec{Use: "action.login"},
			}},
		}},
		ScenarioCalls: []ScenarioCallSpec{{ID: "login-user", ScenarioID: "login"}},
	}

	actionCalled := false
	action := debugBoundaryTestAction{
		RunFunc: func(request ActionRequest) (Outputs, error) {
			actionCalled = true
			return Outputs{"token": "issued-token"}, nil
		},
	}

	catalog := NewCatalog()
	if err := catalog.RegisterAction("action.login", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	matchers, err := NewMatcherCatalog()
	if err != nil {
		t.Fatalf("register matcher catalog failed: %v", err)
	}

	result, err := NewRunner(catalog, matchers).runWithDebugRuntime(
		context.Background(),
		spec,
		RunOptions{},
		&debugRuntime{
			boundaryHook: func(_ context.Context, state debugBoundaryState) error {
				if state.Ref.Kind == debugBoundaryKindAction && state.Ref.Phase == debugBoundaryPhaseBefore {
					panic("debug panic")
				}

				return nil
			},
		},
	)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if actionCalled {
		t.Fatal("action ran before the panicking boundary hook")
	}

	if got, want := result.Report.Status, StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
	if result.Report.Failure == nil {
		t.Fatal("report failure = nil, want contained debug failure")
	}
	if got, want := result.Report.Failure.Kind, FailureKindInternal; got != want {
		t.Fatalf("report failure kind mismatch: got %q want %q", got, want)
	}
	if got, want := result.Report.Failure.Summary, "debug boundary hook panicked"; got != want {
		t.Fatalf("report failure summary mismatch: got %q want %q", got, want)
	}
	if got, want := result.Report.Failure.Cause.Error(), `debug boundary hook "stage.main/call.login-user/act.submit/action" panicked: debug panic`; got != want {
		t.Fatalf("report failure cause mismatch: got %q want %q", got, want)
	}
}

func TestDebugRuntimeSkipsStateEnrichmentForUnmatchedBoundary(t *testing.T) {
	t.Parallel()

	debug := &debugRuntime{
		controller: &debugController{mode: debugModeDump},
		stateRecorder: &debugStateRecorder{
			builder: newDebugSnapshotBuilder(),
			enrichments: []debugStateRecorderEnrichment{{
				backend:     "debug",
				snapshotter: debugRuntimePanickingStateSnapshotter{message: "snapshot boom"},
			}},
		},
		compiledBreakpoints: []debugCompiledBreakpoint{{
			Boundary: debugCompiledBoundary{
				Path:         "stage.main/call.login-user/act.submit/action",
				ScenarioPath: "stage.main/call.login-user",
				ActID:        "submit",
				NodeRef:      "action",
				Kind:         debugBoundaryKindAction,
				Phase:        debugBoundaryPhaseAfter,
			},
			When:     debugBreakpointWhenAlways,
			Attempt:  debugBreakpointAttempt{Mode: debugBreakpointAttemptModeAny},
			Reaction: debugBreakpointActionPause,
		}},
	}

	state := debugBoundaryState{
		Ref: debugBoundaryRef{
			ScenarioPath: "stage.main/call.login-user",
			ActID:        "submit",
			NodeRef:      "action",
			Path:         "stage.main/call.login-user/act.submit/action",
			Kind:         debugBoundaryKindAction,
			Phase:        debugBoundaryPhaseBefore,
		},
		Status: StatusRunning,
	}

	if err := debug.atBoundary(context.Background(), state); err != nil {
		t.Fatalf("boundary dispatch failed: %v", err)
	}
}

type debugBoundaryTestAction struct {
	RunFunc      func(ActionRequest) (Outputs, error)
	ContractFunc func() ActionContract
}

type debugBoundaryTestExpectation struct {
	CheckFunc func(actual any) error
	Args      []MatcherArg
}

func (a debugBoundaryTestAction) Contract() ActionContract {
	if a.ContractFunc != nil {
		return a.ContractFunc()
	}

	return ActionContract{
		Outputs: map[string]ValueContract{
			"token": {Kind: ValueKindString},
		},
	}
}

func (a debugBoundaryTestAction) Run(_ context.Context, request ActionRequest) (Outputs, error) {
	return a.RunFunc(request)
}

func (e debugBoundaryTestExpectation) Descriptor(ref string) MatcherDescriptor {
	return MatcherDescriptor{
		Ref:    ref,
		Actual: ValueContract{Kind: ValueKindAny},
		Args:   e.Args,
		Sugar:  SugarSpec{Form: SugarFormNone},
		Compile: func(_ MatcherCompileContext, _ Values) (Matcher, error) {
			return debugBoundaryTestMatcher{check: e.CheckFunc}, nil
		},
	}
}

type debugBoundaryTestMatcher struct {
	check func(actual any) error
}

func (m debugBoundaryTestMatcher) Check(_ context.Context, actual any) error {
	return m.check(actual)
}

type debugRuntimePanickingStateSnapshotter struct {
	message string
}

func (s debugRuntimePanickingStateSnapshotter) DebugStateSnapshot(context.Context) (Values, error) {
	panic(s.message)
}
