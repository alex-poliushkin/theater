package theater

import (
	"context"
	"errors"
	"testing"
)

func TestRunDebugControllerSupportsStartPausedStepAndBreakpointMetadata(t *testing.T) {
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

	pauses := make([]debugPause, 0, 3)
	hits := make([]debugBoundaryState, 0, 3)
	debug := &debugRuntime{
		controller: newTestDebugController(debugModeInteractive, true, func(_ context.Context, pause debugPause) (debugResumeCommand, error) {
			pauses = append(pauses, pause)
			switch len(pauses) {
			case 1:
				return debugResumeStep, nil
			default:
				return debugResumeContinue, nil
			}
		}),
		breakpointSpecs: []string{
			"name=after-token,path=**/expectation.token,kind=expectation,phase=after",
		},
		boundaryHook: func(_ context.Context, state debugBoundaryState) error {
			hits = append(hits, state)
			return nil
		},
	}

	result, err := NewRunner(catalog, matchers).runWithDebugRuntime(context.Background(), spec, RunOptions{}, debug)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := result.Report.Status, StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}

	if got, want := len(pauses), 3; got != want {
		t.Fatalf("pause count mismatch: got %d want %d", got, want)
	}
	if got, want := len(hits), 3; got != want {
		t.Fatalf("boundary hit count mismatch: got %d want %d", got, want)
	}
	if got, want := pauses[0].Reason, debugPauseReasonStart; got != want {
		t.Fatalf("pause[0] reason mismatch: got %q want %q", got, want)
	}
	if got, want := pauses[0].State.Ref.Path, "stage.main/call.login-user/act.submit/action"; got != want {
		t.Fatalf("pause[0] path mismatch: got %q want %q", got, want)
	}
	if got, want := pauses[1].Reason, debugPauseReasonStep; got != want {
		t.Fatalf("pause[1] reason mismatch: got %q want %q", got, want)
	}
	if got, want := pauses[1].State.Ref.Path, "stage.main/call.login-user/act.submit/action"; got != want {
		t.Fatalf("pause[1] path mismatch: got %q want %q", got, want)
	}
	if got, want := pauses[2].Reason, debugPauseReasonBreakpoint; got != want {
		t.Fatalf("pause[2] reason mismatch: got %q want %q", got, want)
	}
	if got, want := pauses[2].Breakpoint, "after-token"; got != want {
		t.Fatalf("pause[2] breakpoint mismatch: got %q want %q", got, want)
	}
	if got, want := pauses[2].State.Ref.Path, "stage.main/call.login-user/act.submit/expectation.token"; got != want {
		t.Fatalf("pause[2] path mismatch: got %q want %q", got, want)
	}

	for i := range pauses {
		if got, want := pauses[i].Seq, uint64(i+1); got != want {
			t.Fatalf("pause[%d] seq mismatch: got %d want %d", i, got, want)
		}
		if pauses[i].DurableEventSeq == 0 {
			t.Fatalf("pause[%d] durable event seq is zero", i)
		}
	}
	if pauses[1].DurableEventSeq < pauses[0].DurableEventSeq {
		t.Fatalf("pause durable event seq regressed: first=%d second=%d", pauses[0].DurableEventSeq, pauses[1].DurableEventSeq)
	}
	if pauses[2].DurableEventSeq < pauses[1].DurableEventSeq {
		t.Fatalf("pause durable event seq regressed: second=%d third=%d", pauses[1].DurableEventSeq, pauses[2].DurableEventSeq)
	}
}

func TestRunDebugControllerResetsStartPausedStateBetweenRuns(t *testing.T) {
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

	pauses := 0
	debug := &debugRuntime{
		controller: newTestDebugController(debugModeInteractive, true, func(_ context.Context, pause debugPause) (debugResumeCommand, error) {
			pauses++
			return debugResumeContinue, nil
		}),
	}

	first, err := NewRunner(catalog, matchers).runWithDebugRuntime(context.Background(), spec, RunOptions{}, debug)
	if err != nil {
		t.Fatalf("first run failed: %v", err)
	}
	if got, want := first.Report.Status, StatusPassed; got != want {
		t.Fatalf("first run status mismatch: got %q want %q", got, want)
	}

	second, err := NewRunner(catalog, matchers).runWithDebugRuntime(context.Background(), spec, RunOptions{}, debug)
	if err != nil {
		t.Fatalf("second run failed: %v", err)
	}
	if got, want := second.Report.Status, StatusPassed; got != want {
		t.Fatalf("second run status mismatch: got %q want %q", got, want)
	}

	if got, want := pauses, 2; got != want {
		t.Fatalf("pause count mismatch: got %d want %d", got, want)
	}
}

func TestRunDebugControllerUsesAttemptFailureReasonOnRetryFailure(t *testing.T) {
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

	pauses := make([]debugPause, 0, 1)
	debug := &debugRuntime{
		controller: newTestDebugController(debugModeInteractive, false, func(_ context.Context, pause debugPause) (debugResumeCommand, error) {
			pauses = append(pauses, pause)
			return debugResumeContinue, nil
		}),
		breakpointSpecs: []string{
			"name=retry-failure,path=**/expectation.ready,kind=expectation,phase=after,when=attempt-failure,attempt=retry-only",
		},
	}

	result, err := NewRunner(catalog, matchers).runWithDebugRuntime(context.Background(), spec, RunOptions{}, debug)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := result.Report.Status, StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
	if got, want := len(pauses), 1; got != want {
		t.Fatalf("pause count mismatch: got %d want %d", got, want)
	}

	pause := pauses[0]
	if got, want := pause.Reason, debugPauseReasonAttemptFailure; got != want {
		t.Fatalf("pause reason mismatch: got %q want %q", got, want)
	}
	if got, want := pause.Breakpoint, "retry-failure"; got != want {
		t.Fatalf("pause breakpoint mismatch: got %q want %q", got, want)
	}
	if got, want := pause.State.Ref.Attempt, 2; got != want {
		t.Fatalf("pause attempt mismatch: got %d want %d", got, want)
	}
	if pause.State.Failure == nil {
		t.Fatal("pause failure is nil")
	}
	if got, want := pause.State.Failure.Kind, FailureKindExpectation; got != want {
		t.Fatalf("pause failure kind mismatch: got %q want %q", got, want)
	}
}

func TestRunDebugControllerStepsAcrossRetryAttemptBoundaries(t *testing.T) {
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

	pauses := make([]debugPause, 0, 5)
	debug := &debugRuntime{
		controller: newTestDebugController(debugModeInteractive, true, func(_ context.Context, pause debugPause) (debugResumeCommand, error) {
			pauses = append(pauses, pause)
			if len(pauses) < 5 {
				return debugResumeStep, nil
			}

			return debugResumeContinue, nil
		}),
	}

	result, err := NewRunner(catalog, matchers).runWithDebugRuntime(context.Background(), spec, RunOptions{}, debug)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := result.Report.Status, StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
	if got, want := len(pauses), 5; got != want {
		t.Fatalf("pause count mismatch: got %d want %d", got, want)
	}

	want := []struct {
		reason  debugPauseReason
		path    string
		phase   debugBoundaryPhase
		attempt int
	}{
		{debugPauseReasonStart, "stage.main/call.probe-ready/act.wait-ready/action", debugBoundaryPhaseBefore, 1},
		{debugPauseReasonStep, "stage.main/call.probe-ready/act.wait-ready/action", debugBoundaryPhaseAfter, 1},
		{debugPauseReasonStep, "stage.main/call.probe-ready/act.wait-ready/expectation.ready", debugBoundaryPhaseBefore, 1},
		{debugPauseReasonStep, "stage.main/call.probe-ready/act.wait-ready/expectation.ready", debugBoundaryPhaseAfter, 1},
		{debugPauseReasonStep, "stage.main/call.probe-ready/act.wait-ready/action", debugBoundaryPhaseBefore, 2},
	}

	for i := range want {
		if got := pauses[i].Reason; got != want[i].reason {
			t.Fatalf("pause[%d] reason mismatch: got %q want %q", i, got, want[i].reason)
		}
		if got := pauses[i].State.Ref.Path; got != want[i].path {
			t.Fatalf("pause[%d] path mismatch: got %q want %q", i, got, want[i].path)
		}
		if got := pauses[i].State.Ref.Phase; got != want[i].phase {
			t.Fatalf("pause[%d] phase mismatch: got %q want %q", i, got, want[i].phase)
		}
		if got := pauses[i].State.Ref.Attempt; got != want[i].attempt {
			t.Fatalf("pause[%d] attempt mismatch: got %d want %d", i, got, want[i].attempt)
		}
	}
}

func TestRunDebugControllerUsesTerminalFailureReasonWhenEventuallyTimesOut(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{{
			ID: "probe",
			Acts: []ActSpec{{
				ID:         "wait-ready",
				Action:     ActionSpec{Use: "action.ready", Repeatable: true},
				Eventually: &EventuallySpec{Timeout: "20ms", Interval: "1ms"},
				Expectations: []ExpectationSpec{{
					ID:      "ready",
					Subject: SubjectSpec{Field: "ready"},
					Assert:  AssertSpec{Ref: "expectation.ready"},
				}},
			}},
		}},
		ScenarioCalls: []ScenarioCallSpec{{ID: "probe-ready", ScenarioID: "probe"}},
	}

	catalog := NewCatalog()
	if err := catalog.RegisterAction("action.ready", debugControllerReadyAction{
		RunFunc: func(request ActionRequest) (Outputs, error) {
			return Outputs{"ready": false}, nil
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	matchers, err := NewMatcherCatalog(debugBoundaryTestExpectation{
		CheckFunc: func(actual any) error { return errors.New("not ready") },
	}.Descriptor("expectation.ready"))
	if err != nil {
		t.Fatalf("new matcher catalog failed: %v", err)
	}

	pauses := make([]debugPause, 0, 1)
	debug := &debugRuntime{
		controller: newTestDebugController(debugModeInteractive, false, func(_ context.Context, pause debugPause) (debugResumeCommand, error) {
			pauses = append(pauses, pause)
			return debugResumeContinue, nil
		}),
		breakpointSpecs: []string{
			"name=terminal-ready,path=**/expectation.ready,kind=expectation,phase=after,when=terminal-failure",
		},
	}

	result, err := NewRunner(catalog, matchers).runWithDebugRuntime(context.Background(), spec, RunOptions{}, debug)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := result.Report.Status, StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
	if result.Report.Failure == nil || result.Report.Failure.Kind != FailureKindTimeout {
		t.Fatalf("report failure mismatch: %#v", result.Report.Failure)
	}
	if got, want := len(pauses), 1; got != want {
		t.Fatalf("pause count mismatch: got %d want %d", got, want)
	}

	pause := pauses[0]
	if got, want := pause.Reason, debugPauseReasonTerminalFailure; got != want {
		t.Fatalf("pause reason mismatch: got %q want %q", got, want)
	}
	if got, want := pause.Breakpoint, "terminal-ready"; got != want {
		t.Fatalf("pause breakpoint mismatch: got %q want %q", got, want)
	}
	if got, want := pause.State.Ref.Path, "stage.main/call.probe-ready/act.wait-ready/expectation.ready"; got != want {
		t.Fatalf("pause path mismatch: got %q want %q", got, want)
	}
	if pause.State.Ref.Attempt < 1 {
		t.Fatalf("pause attempt mismatch: got %d want >= 1", pause.State.Ref.Attempt)
	}
	if pause.State.Failure == nil {
		t.Fatal("pause failure is nil")
	}
	if got, want := pause.State.Failure.Kind, FailureKindExpectation; got != want {
		t.Fatalf("pause failure kind mismatch: got %q want %q", got, want)
	}
}

func newTestDebugController(mode debugMode, startPaused bool, pause debugPauseHandler) *debugController {
	controller := &debugController{
		mode:        mode,
		startPaused: startPaused,
		pause:       pause,
	}
	controller.Reset()
	return controller
}

type debugControllerReadyAction struct {
	RunFunc func(ActionRequest) (Outputs, error)
}

func (a debugControllerReadyAction) Contract() ActionContract {
	return ActionContract{
		Outputs: map[string]ValueContract{
			"ready": {Kind: ValueKindBool},
		},
	}
}

func (a debugControllerReadyAction) Run(_ context.Context, request ActionRequest) (Outputs, error) {
	return a.RunFunc(request)
}
