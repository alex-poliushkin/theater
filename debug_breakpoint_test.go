package theater

import (
	"context"
	"strings"
	"testing"
)

func TestParseDebugBreakpointSpecParsesSelectorFields(t *testing.T) {
	t.Parallel()

	spec, err := parseDebugBreakpointSpec(
		"name=reserve-before,path=**/action,kind=action,phase=before,when=attempt-failure,attempt=retry-only,action=snapshot-continue",
	)
	if err != nil {
		t.Fatalf("parse breakpoint spec failed: %v", err)
	}

	if got, want := spec.Name, "reserve-before"; got != want {
		t.Fatalf("name mismatch: got %q want %q", got, want)
	}
	if got, want := spec.Path, "**/action"; got != want {
		t.Fatalf("path mismatch: got %q want %q", got, want)
	}
	if got, want := spec.Kind, debugBreakpointKindAction; got != want {
		t.Fatalf("kind mismatch: got %q want %q", got, want)
	}
	if got, want := spec.Phase, debugBoundaryPhaseBefore; got != want {
		t.Fatalf("phase mismatch: got %q want %q", got, want)
	}
	if got, want := spec.When, debugBreakpointWhenAttemptFailure; got != want {
		t.Fatalf("when mismatch: got %q want %q", got, want)
	}
	if got, want := spec.Attempt.Mode, debugBreakpointAttemptModeRetryOnly; got != want {
		t.Fatalf("attempt mode mismatch: got %q want %q", got, want)
	}
	if got, want := spec.Action, debugBreakpointActionSnapshotContinue; got != want {
		t.Fatalf("action mismatch: got %q want %q", got, want)
	}
}

func TestCompileDebugBreakpointsBuildsStableMatchers(t *testing.T) {
	t.Parallel()

	stage, err := planPreparer{}.Prepare(compileStageSpec(StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{{
			ID: "login",
			Acts: []ActSpec{{
				ID:         "submit",
				Eventually: &EventuallySpec{Timeout: "10ms", Interval: "1ms"},
				Action:     ActionSpec{Use: "action.login"},
				Expectations: []ExpectationSpec{{
					ID:      "token",
					Subject: SubjectSpec{Field: "token"},
					Assert:  AssertSpec{Ref: "expectation.token"},
				}},
			}},
		}},
		ScenarioCalls: []ScenarioCallSpec{{
			ID:         "login-user",
			ScenarioID: "login",
		}},
	}))
	if err != nil {
		t.Fatalf("prepare stage failed: %v", err)
	}

	compiled, err := compileDebugBreakpoints(stage, []string{
		"path=stage.main/call.login-user/act.submit/action,kind=action,phase=before",
		"name=token-failure,path=**/expectation.token,kind=expectation,phase=after,when=attempt-failure,attempt=retry-only,action=snapshot-continue",
	})
	if err != nil {
		t.Fatalf("compile debug breakpoints failed: %v", err)
	}

	if got, want := len(compiled), 2; got != want {
		t.Fatalf("compiled breakpoint count mismatch: got %d want %d", got, want)
	}

	if got, want := compiled[0].Boundary.Path, "stage.main/call.login-user/act.submit/action"; got != want {
		t.Fatalf("action boundary path mismatch: got %q want %q", got, want)
	}
	if got, want := compiled[0].Boundary.Kind, debugBoundaryKindAction; got != want {
		t.Fatalf("action boundary kind mismatch: got %q want %q", got, want)
	}
	if got, want := compiled[0].Boundary.ScenarioPath, "stage.main/call.login-user"; got != want {
		t.Fatalf("action scenario path mismatch: got %q want %q", got, want)
	}
	if got, want := compiled[0].Boundary.ActID, "submit"; got != want {
		t.Fatalf("action act id mismatch: got %q want %q", got, want)
	}
	if got, want := compiled[0].Boundary.NodeRef, "action"; got != want {
		t.Fatalf("action node ref mismatch: got %q want %q", got, want)
	}
	if got, want := compiled[0].Reaction, debugBreakpointActionPause; got != want {
		t.Fatalf("action reaction mismatch: got %q want %q", got, want)
	}

	if got, want := compiled[1].Name, "token-failure"; got != want {
		t.Fatalf("expectation breakpoint name mismatch: got %q want %q", got, want)
	}
	if got, want := compiled[1].Boundary.Path, "stage.main/call.login-user/act.submit/expectation.token"; got != want {
		t.Fatalf("expectation boundary path mismatch: got %q want %q", got, want)
	}
	if got, want := compiled[1].Boundary.Kind, debugBoundaryKindExpectation; got != want {
		t.Fatalf("expectation boundary kind mismatch: got %q want %q", got, want)
	}
	if got, want := compiled[1].Boundary.ActID, "submit"; got != want {
		t.Fatalf("expectation act id mismatch: got %q want %q", got, want)
	}
	if got, want := compiled[1].Boundary.NodeRef, "token"; got != want {
		t.Fatalf("expectation node ref mismatch: got %q want %q", got, want)
	}
	if got, want := compiled[1].When, debugBreakpointWhenAttemptFailure; got != want {
		t.Fatalf("expectation when mismatch: got %q want %q", got, want)
	}
	if got, want := compiled[1].Attempt.Mode, debugBreakpointAttemptModeRetryOnly; got != want {
		t.Fatalf("expectation attempt mode mismatch: got %q want %q", got, want)
	}
	if got, want := compiled[1].Reaction, debugBreakpointActionSnapshotContinue; got != want {
		t.Fatalf("expectation reaction mismatch: got %q want %q", got, want)
	}
}

func TestCompileDebugBreakpointsUsesDefaultWildcardPath(t *testing.T) {
	t.Parallel()

	stage, err := planPreparer{}.Prepare(compileStageSpec(StageSpec{
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
		ScenarioCalls: []ScenarioCallSpec{{
			ID:         "login-user",
			ScenarioID: "login",
		}},
	}))
	if err != nil {
		t.Fatalf("prepare stage failed: %v", err)
	}

	compiled, err := compileDebugBreakpoints(stage, []string{"phase=after"})
	if err != nil {
		t.Fatalf("compile debug breakpoints failed: %v", err)
	}

	if got, want := len(compiled), 2; got != want {
		t.Fatalf("compiled breakpoint count mismatch: got %d want %d", got, want)
	}
	if got, want := compiled[0].Boundary.Path, "stage.main/call.login-user/act.submit/action"; got != want {
		t.Fatalf("first boundary path mismatch: got %q want %q", got, want)
	}
	if got, want := compiled[1].Boundary.Path, "stage.main/call.login-user/act.submit/expectation.token"; got != want {
		t.Fatalf("second boundary path mismatch: got %q want %q", got, want)
	}
}

func TestCompileDebugBreakpointsRejectsInvalidSelectors(t *testing.T) {
	t.Parallel()

	stage := compileStageSpec(StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{{
			ID: "login",
			Acts: []ActSpec{{
				ID:     "submit",
				Action: ActionSpec{Use: "action.login"},
			}},
		}},
		ScenarioCalls: []ScenarioCallSpec{{
			ID:         "login-user",
			ScenarioID: "login",
		}},
	})

	testCases := []struct {
		name    string
		spec    string
		wantErr string
	}{
		{
			name:    "unknown field",
			spec:    "path=**/action,nope=value",
			wantErr: `unknown debug breakpoint field "nope"`,
		},
		{
			name:    "unknown kind",
			spec:    "kind=node,path=**",
			wantErr: `debug breakpoint kind "node" is invalid`,
		},
		{
			name:    "retry contradiction",
			spec:    "when=retry-only,attempt=first,path=**/action",
			wantErr: `debug breakpoint attempt "first" contradicts when "retry-only"`,
		},
		{
			name:    "no matching boundary",
			spec:    "path=**/expectation.missing,kind=expectation",
			wantErr: `matched no debuggable boundaries`,
		},
		{
			name:    "attempt failure requires retry-aware boundary",
			spec:    "path=stage.main/call.login-user/act.submit,kind=act,phase=after,when=attempt-failure",
			wantErr: `matched no debuggable boundaries`,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := compileDebugBreakpoints(stage, []string{testCase.spec})
			if err == nil {
				t.Fatal("expected compile error, got nil")
			}
			if !strings.Contains(err.Error(), testCase.wantErr) {
				t.Fatalf("error mismatch: got %q want substring %q", err.Error(), testCase.wantErr)
			}
		})
	}
}

func TestCompileDebugBreakpointsSupportsScenarioCallAndActKinds(t *testing.T) {
	t.Parallel()

	stage, err := planPreparer{}.Prepare(compileStageSpec(StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{{
			ID: "login",
			Inputs: map[string]ValueContract{
				"token": {Kind: ValueKindString},
			},
			Acts: []ActSpec{{
				ID:     "submit",
				Action: ActionSpec{Use: "action.login"},
			}},
		}},
		ScenarioCalls: []ScenarioCallSpec{{
			ID:         "login-user",
			ScenarioID: "login",
		}},
	}))
	if err != nil {
		t.Fatalf("prepare stage failed: %v", err)
	}

	compiled, err := compileDebugBreakpoints(stage, []string{
		"path=stage.main/call.login-user,kind=scenario_call,phase=before",
		"path=stage.main/call.login-user/act.submit,kind=act,phase=after",
	})
	if err != nil {
		t.Fatalf("compile debug breakpoints failed: %v", err)
	}

	if got, want := len(compiled), 2; got != want {
		t.Fatalf("compiled breakpoint count mismatch: got %d want %d", got, want)
	}
	if got, want := compiled[0].Boundary.Kind, debugBoundaryKindScenarioCall; got != want {
		t.Fatalf("scenario boundary kind mismatch: got %q want %q", got, want)
	}
	if got, want := compiled[0].Boundary.Path, "stage.main/call.login-user"; got != want {
		t.Fatalf("scenario boundary path mismatch: got %q want %q", got, want)
	}
	if got, want := compiled[1].Boundary.Kind, debugBoundaryKindAct; got != want {
		t.Fatalf("act boundary kind mismatch: got %q want %q", got, want)
	}
	if got, want := compiled[1].Boundary.Path, "stage.main/call.login-user/act.submit"; got != want {
		t.Fatalf("act boundary path mismatch: got %q want %q", got, want)
	}
}

func TestCompileDebugBreakpointsSupportsRetryTargetedActAfterBoundary(t *testing.T) {
	t.Parallel()

	stage, err := planPreparer{}.Prepare(compileStageSpec(StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{{
			ID: "login",
			Acts: []ActSpec{{
				ID:         "submit",
				Action:     ActionSpec{Use: "action.login", Repeatable: true},
				Eventually: &EventuallySpec{Timeout: "100ms", Interval: "10ms"},
			}},
		}},
		ScenarioCalls: []ScenarioCallSpec{{
			ID:         "login-user",
			ScenarioID: "login",
		}},
	}))
	if err != nil {
		t.Fatalf("prepare stage failed: %v", err)
	}

	compiled, err := compileDebugBreakpoints(stage, []string{
		"path=stage.main/call.login-user/act.submit,kind=act,phase=after,attempt=retry-only",
	})
	if err != nil {
		t.Fatalf("compile debug breakpoints failed: %v", err)
	}

	if got, want := len(compiled), 1; got != want {
		t.Fatalf("compiled breakpoint count mismatch: got %d want %d", got, want)
	}
	if got, want := compiled[0].Boundary.Kind, debugBoundaryKindAct; got != want {
		t.Fatalf("act boundary kind mismatch: got %q want %q", got, want)
	}
	if got, want := compiled[0].Boundary.Phase, debugBoundaryPhaseAfter; got != want {
		t.Fatalf("act boundary phase mismatch: got %q want %q", got, want)
	}
	if got, want := compiled[0].Boundary.RetryAware, true; got != want {
		t.Fatalf("act boundary retry-aware mismatch: got %t want %t", got, want)
	}
}

func TestRunRejectsInvalidDebugBreakpointSpecsBeforeExecution(t *testing.T) {
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
		ScenarioCalls: []ScenarioCallSpec{{
			ID:         "login-user",
			ScenarioID: "login",
		}},
	}

	calls := 0
	catalog := NewCatalog()
	if err := catalog.RegisterAction("action.login", debugBoundaryTestAction{
		RunFunc: func(request ActionRequest) (Outputs, error) {
			calls++
			return Outputs{"token": "issued-token"}, nil
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	matchers, err := NewMatcherCatalog()
	if err != nil {
		t.Fatalf("new matcher catalog failed: %v", err)
	}

	result, err := NewRunner(catalog, matchers).runWithDebugRuntime(
		context.Background(),
		spec,
		RunOptions{},
		&debugRuntime{
			breakpointSpecs: []string{"path=**/expectation.missing,kind=expectation"},
		},
	)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if got, want := calls, 0; got != want {
		t.Fatalf("action call count mismatch: got %d want %d", got, want)
	}
	if got, want := result.Report.Status, StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
	if result.Report.Failure == nil {
		t.Fatal("report failure must be present")
	}
	if got, want := result.Report.Failure.At, "debug/breakpoint[0]"; got != want {
		t.Fatalf("failure path mismatch: got %q want %q", got, want)
	}
	if got, want := result.Report.Failure.Summary, "stage preparation failed"; got != want {
		t.Fatalf("failure summary mismatch: got %q want %q", got, want)
	}
	if got := result.Report.Failure.Cause.Error(); !strings.Contains(got, "matched no debuggable boundaries") {
		t.Fatalf("failure cause mismatch: got %q", got)
	}
}
